package bdd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Status 是步骤或场景的最终状态。
type Status string

const (
	StatusPassed    Status = "passed"
	StatusFailed    Status = "failed"
	StatusPending   Status = "pending"   // 步骤被标记为待实现
	StatusUndefined Status = "undefined" // 步骤没有匹配的定义
	StatusAmbiguous Status = "ambiguous" // 步骤匹配到多个定义
	StatusSkipped   Status = "skipped"   // 场景被跳过或前序步骤失败
	StatusCancelled Status = "cancelled" // 运行被中途取消，未执行
)

// StepResult 记录单个步骤的执行结果。
type StepResult struct {
	Text       string
	Keyword    string
	Line       int
	Status     Status
	Duration   time.Duration
	Error      string   // 主错误信息
	HookErrors []string // 钩子产生的附加错误（保留完整失败链路）
}

// ScenarioResult 记录单个场景的执行结果。
type ScenarioResult struct {
	Feature      string
	FeaturePath  string
	Name         string
	Line         int
	ExampleIndex int
	Status       Status
	SkipReason   SkipReason // Status 为 Skipped 时有效
	Duration     time.Duration
	Steps        []StepResult
	HookErrors   []string // 场景级钩子错误
}

// RunResult 是整次运行的汇总，场景顺序与输入顺序一致，与并发度无关。
type RunResult struct {
	Scenarios []ScenarioResult
	Cancelled bool
}

// Failed 报告是否存在失败、未定义、歧义或待实现的场景。
func (r *RunResult) Failed() bool {
	for _, s := range r.Scenarios {
		switch s.Status {
		case StatusFailed, StatusUndefined, StatusAmbiguous, StatusPending:
			return true
		}
	}
	return false
}

// Runner 编排并执行场景。
type Runner struct {
	Registry    *Registry
	Hooks       Hooks
	Filter      Filter
	Concurrency int // <=0 表示串行
}

type workItem struct {
	feature *Feature
	sc      *Scenario
}

// Run 并发执行所有选中场景并汇总结果。
// 结果顺序固定为输入顺序；ctx 取消时已完成的部分全部保留，
// 未开始的场景标记为 Cancelled。
func (r *Runner) Run(ctx context.Context, features []*Feature) *RunResult {
	var items []workItem
	for _, f := range features {
		for _, sc := range ExpandOutlines(f) {
			items = append(items, workItem{feature: f, sc: sc})
		}
	}
	res := &RunResult{Scenarios: make([]ScenarioResult, len(items))}

	conc := r.Concurrency
	if conc <= 0 {
		conc = 1
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i, it := range items {
		base := ScenarioResult{
			Feature:      it.feature.Name,
			FeaturePath:  it.feature.Path,
			Name:         it.sc.Name,
			Line:         it.sc.Line,
			ExampleIndex: it.sc.ExampleIndex,
		}
		if ok, reason := r.Filter.Select(it.sc); !ok {
			base.Status = StatusSkipped
			base.SkipReason = reason
			res.Scenarios[i] = base
			continue
		}
		if HasTag(it.sc, "skip") {
			base.Status = StatusSkipped
			base.SkipReason = ReasonSkipped
			res.Scenarios[i] = base
			continue
		}
		if ctx.Err() != nil {
			base.Status = StatusCancelled
			res.Scenarios[i] = base
			res.Cancelled = true
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, it workItem, base ScenarioResult) {
			defer wg.Done()
			defer func() { <-sem }()
			res.Scenarios[i] = r.runScenario(ctx, it.feature, it.sc, base)
		}(i, it, base)
	}
	wg.Wait()
	if ctx.Err() != nil {
		res.Cancelled = true
	}
	return res
}

// safeCall 调用 fn 并把 panic 转换为错误，保证失败链路完整。
func safeCall(fn func() error) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("panic: %v", v)
		}
	}()
	return fn()
}

func (r *Runner) runScenario(ctx context.Context, f *Feature, sc *Scenario, res ScenarioResult) ScenarioResult {
	start := time.Now()
	defer func() { res.Duration = time.Since(start) }()

	sctx := &StepContext{Context: ctx, Scenario: sc, values: map[string]any{}}

	// BeforeScenario：按注册顺序执行；出错则跳过主体，仍执行 AfterScenario。
	var beforeErr error
	for _, h := range r.Hooks.BeforeScenario {
		h := h
		if err := safeCall(func() error { return h(sctx, nil) }); err != nil {
			res.HookErrors = append(res.HookErrors, "before_scenario: "+err.Error())
			beforeErr = err
			break
		}
	}

	if beforeErr == nil {
		r.runSteps(sctx, sc, &res)
	} else {
		for _, st := range sc.Steps {
			res.Steps = append(res.Steps, StepResult{
				Text: st.Text, Keyword: st.Keyword, Line: st.Line, Status: StatusSkipped,
			})
		}
	}

	// AfterScenario：按注册逆序执行，成功/失败/中断路径都会执行；
	// 钩子错误追加记录，不覆盖场景主体的失败原因。
	scenarioErr := scenarioError(&res)
	for i := len(r.Hooks.AfterScenario) - 1; i >= 0; i-- {
		h := r.Hooks.AfterScenario[i]
		if err := safeCall(func() error { return h(sctx, scenarioErr) }); err != nil {
			res.HookErrors = append(res.HookErrors, "after_scenario: "+err.Error())
		}
	}

	res.Status = scenarioStatus(&res)
	return res
}

// runSteps 依次执行场景步骤；任一步骤未通过时，后续步骤标记为 Skipped。
func (r *Runner) runSteps(sctx *StepContext, sc *Scenario, res *ScenarioResult) {
	halted := false
	for _, st := range sc.Steps {
		if halted {
			stepStatus := StatusSkipped
			if sctx.Context.Err() != nil {
				stepStatus = StatusCancelled
			}
			res.Steps = append(res.Steps, StepResult{
				Text: st.Text, Keyword: st.Keyword, Line: st.Line, Status: stepStatus,
			})
			continue
		}
		sr := r.runStep(sctx, st)
		res.Steps = append(res.Steps, sr)
		if sr.Status != StatusPassed {
			halted = true
		}
	}
}

func (r *Runner) runStep(sctx *StepContext, st *Step) StepResult {
	sr := StepResult{Text: st.Text, Keyword: st.Keyword, Line: st.Line}
	start := time.Now()
	defer func() { sr.Duration = time.Since(start) }()

	var stepErr error
	for _, h := range r.Hooks.BeforeStep {
		h := h
		if err := safeCall(func() error { return h(sctx, st, nil) }); err != nil {
			sr.HookErrors = append(sr.HookErrors, "before_step: "+err.Error())
			stepErr = err
			break
		}
	}

	if stepErr == nil {
		b, err := r.Registry.bind(st.Text)
		switch {
		case err == nil:
			args := b.args
			if st.DocString != nil {
				args = append(args, st.DocString)
			}
			if st.Table != nil {
				args = append(args, st.Table)
			}
			err = safeCall(func() error { return b.def.fn(sctx, args...) })
			switch {
			case err == nil:
				sr.Status = StatusPassed
			case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
				sr.Status = StatusCancelled
				sr.Error = err.Error()
			case err == ErrPending:
				sr.Status = StatusPending
				sr.Error = err.Error()
			default:
				sr.Status = StatusFailed
				sr.Error = err.Error()
			}
			stepErr = err
		case errors.Is(err, errAmbiguous):
			sr.Status = StatusAmbiguous
			sr.Error = err.Error()
			stepErr = err
		default:
			sr.Status = StatusUndefined
			sr.Error = err.Error()
			stepErr = err
		}
	} else {
		sr.Status = StatusFailed
		sr.Error = stepErr.Error()
	}

	// AfterStep：按注册逆序执行，无论步骤成败都执行；
	// 钩子错误追加到 HookErrors，不吞掉步骤原始失败原因。
	for i := len(r.Hooks.AfterStep) - 1; i >= 0; i-- {
		h := r.Hooks.AfterStep[i]
		if err := safeCall(func() error { return h(sctx, st, stepErr) }); err != nil {
			sr.HookErrors = append(sr.HookErrors, "after_step: "+err.Error())
		}
	}
	if sr.Status == StatusPassed && len(sr.HookErrors) > 0 {
		sr.Status = StatusFailed
		sr.Error = sr.HookErrors[0]
	}
	return sr
}

// scenarioError 提取场景主体的首要错误，供 AfterScenario 钩子参考。
func scenarioError(res *ScenarioResult) error {
	for _, s := range res.Steps {
		if s.Error != "" {
			return fmt.Errorf("%s", s.Error)
		}
	}
	return nil
}

// scenarioStatus 汇总场景状态：任一步骤失败/歧义/未定义/待实现即决定场景状态；
// 钩子错误视为失败。
func scenarioStatus(res *ScenarioResult) Status {
	if len(res.HookErrors) > 0 {
		return StatusFailed
	}
	status := StatusPassed
	for _, s := range res.Steps {
		switch s.Status {
		case StatusFailed, StatusAmbiguous:
			return s.Status
		case StatusUndefined, StatusPending:
			status = s.Status
		case StatusCancelled:
			if status == StatusPassed {
				status = StatusCancelled
			}
		}
	}
	return status
}
