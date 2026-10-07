package bdd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func registryWith(pairs ...any) *Registry {
	r := NewRegistry()
	for i := 0; i < len(pairs); i += 2 {
		fn := pairs[i+1].(func(*StepContext, ...any) error)
		if err := r.Register(pairs[i].(string), StepFunc(fn)); err != nil {
			panic(err)
		}
	}
	return r
}

func runOne(t *testing.T, r *Runner, src string) *RunResult {
	t.Helper()
	f, err := Parse("t.feature", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return r.Run(context.Background(), []*Feature{f})
}

func TestRunnerStatuses(t *testing.T) {
	r := &Runner{Registry: registryWith(
		"ok", func(ctx *StepContext, a ...any) error { return nil },
		"boom", func(ctx *StepContext, a ...any) error { return errors.New("kaboom") },
		"todo", func(ctx *StepContext, a ...any) error { return bdd_pending() },
	)}
	res := runOne(t, r, `Feature: f
  Scenario: pass
    Given ok
  Scenario: fail
    Given boom
    Given ok
  Scenario: pend
    Given todo
  Scenario: undef
    Given nothing registered
`)
	want := []Status{StatusPassed, StatusFailed, StatusPending, StatusUndefined}
	for i, s := range res.Scenarios {
		if s.Status != want[i] {
			t.Errorf("scenario %d status = %s, want %s", i, s.Status, want[i])
		}
	}
	// 失败步骤之后的步骤必须标记为 skipped
	fail := res.Scenarios[1]
	if fail.Steps[1].Status != StatusSkipped {
		t.Errorf("step after failure = %s, want skipped", fail.Steps[1].Status)
	}
	if fail.Steps[0].Error != "kaboom" {
		t.Errorf("error = %q", fail.Steps[0].Error)
	}
	if !res.Failed() {
		t.Error("Failed() should be true")
	}
}

func bdd_pending() error { return ErrPending }

func TestHookOrderAndErrorChain(t *testing.T) {
	var mu sync.Mutex
	var order []string
	record := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }

	reg := registryWith(
		"work", func(ctx *StepContext, a ...any) error { record("step"); return errors.New("original failure") },
	)
	r := &Runner{Registry: reg}
	r.Hooks.BeforeScenario = []ScenarioHook{
		func(c *StepContext, e error) error { record("before-scenario-1"); return nil },
		func(c *StepContext, e error) error { record("before-scenario-2"); return nil },
	}
	r.Hooks.AfterScenario = []ScenarioHook{
		func(c *StepContext, e error) error { record("after-scenario-1"); return nil },
		func(c *StepContext, e error) error {
			record("after-scenario-2")
			if e == nil || e.Error() != "original failure" {
				t.Errorf("after hook should see original error, got %v", e)
			}
			return errors.New("after hook exploded")
		},
	}
	r.Hooks.BeforeStep = []StepHook{
		func(c *StepContext, s *Step, e error) error { record("before-step"); return nil },
	}
	r.Hooks.AfterStep = []StepHook{
		func(c *StepContext, s *Step, e error) error { record("after-step-1"); return nil },
		func(c *StepContext, s *Step, e error) error {
			record("after-step-2")
			return errors.New("after-step hook exploded")
		},
	}

	res := runOne(t, r, "Feature: f\n  Scenario: s\n    Given work\n")
	got := strings.Join(order, ",")
	want := "before-scenario-1,before-scenario-2,before-step,step,after-step-2,after-step-1,after-scenario-2,after-scenario-1"
	if got != want {
		t.Errorf("hook order =\n  %s\nwant\n  %s", got, want)
	}

	s := res.Scenarios[0]
	if s.Status != StatusFailed {
		t.Fatalf("status = %s", s.Status)
	}
	// 原始失败原因必须保留
	if s.Steps[0].Error != "original failure" {
		t.Errorf("original error lost: %q", s.Steps[0].Error)
	}
	// 钩子错误追加记录，不覆盖原始错误
	if len(s.Steps[0].HookErrors) != 1 || !strings.Contains(s.Steps[0].HookErrors[0], "after-step hook exploded") {
		t.Errorf("step hook errors = %v", s.Steps[0].HookErrors)
	}
	if len(s.HookErrors) != 1 || !strings.Contains(s.HookErrors[0], "after hook exploded") {
		t.Errorf("scenario hook errors = %v", s.HookErrors)
	}
}

func TestBeforeScenarioHookFailure(t *testing.T) {
	reg := registryWith("work", func(ctx *StepContext, a ...any) error {
		t.Error("step must not run when before-scenario hook fails")
		return nil
	})
	r := &Runner{Registry: reg}
	r.Hooks.BeforeScenario = []ScenarioHook{
		func(c *StepContext, e error) error { return errors.New("setup failed") },
	}
	afterRan := false
	r.Hooks.AfterScenario = []ScenarioHook{
		func(c *StepContext, e error) error { afterRan = true; return nil },
	}
	res := runOne(t, r, "Feature: f\n  Scenario: s\n    Given work\n")
	s := res.Scenarios[0]
	if s.Status != StatusFailed {
		t.Errorf("status = %s", s.Status)
	}
	if s.Steps[0].Status != StatusSkipped {
		t.Errorf("step status = %s, want skipped", s.Steps[0].Status)
	}
	if !afterRan {
		t.Error("after-scenario hook must run even when before hook fails")
	}
	if len(s.HookErrors) == 0 || !strings.Contains(s.HookErrors[0], "setup failed") {
		t.Errorf("hook errors = %v", s.HookErrors)
	}
}

func TestHookPanicIsCaptured(t *testing.T) {
	reg := registryWith("work", func(ctx *StepContext, a ...any) error { return nil })
	r := &Runner{Registry: reg}
	r.Hooks.BeforeStep = []StepHook{
		func(c *StepContext, s *Step, e error) error { panic("hook panicked") },
	}
	res := runOne(t, r, "Feature: f\n  Scenario: s\n    Given work\n")
	s := res.Scenarios[0]
	if s.Status != StatusFailed {
		t.Fatalf("status = %s", s.Status)
	}
	if !strings.Contains(s.Steps[0].Error, "hook panicked") {
		t.Errorf("panic not captured: %q", s.Steps[0].Error)
	}
}

func TestStepPanicIsCaptured(t *testing.T) {
	reg := registryWith("work", func(ctx *StepContext, a ...any) error { panic("step panicked") })
	r := &Runner{Registry: reg}
	res := runOne(t, r, "Feature: f\n  Scenario: s\n    Given work\n")
	if res.Scenarios[0].Status != StatusFailed ||
		!strings.Contains(res.Scenarios[0].Steps[0].Error, "step panicked") {
		t.Errorf("panic not captured: %+v", res.Scenarios[0].Steps[0])
	}
}

// 并发执行时场景间状态必须隔离。
func TestConcurrentIsolation(t *testing.T) {
	reg := registryWith(
		"I store {string}", func(ctx *StepContext, a ...any) error {
			ctx.Set("v", a[0])
			time.Sleep(5 * time.Millisecond) // 放大交错窗口
			return nil
		},
		"value should still be {string}", func(ctx *StepContext, a ...any) error {
			if v, _ := ctx.Get("v"); v != a[0] {
				return fmt.Errorf("state leaked: want %v got %v", a[0], v)
			}
			return nil
		},
	)
	var src strings.Builder
	src.WriteString("Feature: f\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&src, "  Scenario: s%d\n    Given I store \"v%d\"\n    Then value should still be \"v%d\"\n", i, i, i)
	}
	r := &Runner{Registry: reg, Concurrency: 8}
	res := runOne(t, r, src.String())
	for i, s := range res.Scenarios {
		if s.Status != StatusPassed {
			t.Errorf("scenario %d status = %s (%v)", i, s.Status, s.Steps)
		}
	}
}

// 相同输入、不同并发度下，结果顺序与逐项结论必须一致。
func TestDeterministicOrderAcrossConcurrency(t *testing.T) {
	reg := registryWith("wait {int} ms", func(ctx *StepContext, a ...any) error {
		time.Sleep(time.Duration(a[0].(int64)) * time.Millisecond)
		return nil
	})
	var src strings.Builder
	src.WriteString("Feature: f\n")
	// 前面的场景更慢，若按完成顺序排列会乱序
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&src, "  Scenario: s%d\n    Given wait %d ms\n", i, (6-i)*10)
	}
	f, err := Parse("t.feature", []byte(src.String()))
	if err != nil {
		t.Fatal(err)
	}
	signature := func(res *RunResult) string {
		var b strings.Builder
		for _, s := range res.Scenarios {
			fmt.Fprintf(&b, "%s:%s;", s.Name, s.Status)
		}
		return b.String()
	}
	var base string
	for _, conc := range []int{1, 3, 6} {
		r := &Runner{Registry: reg, Concurrency: conc}
		res := r.Run(context.Background(), []*Feature{f})
		sig := signature(res)
		if base == "" {
			base = sig
		} else if sig != base {
			t.Errorf("concurrency %d changed result order: %s != %s", conc, sig, base)
		}
	}
}

func TestOutlineExamplesRunIndependently(t *testing.T) {
	reg := registryWith("check {int}", func(ctx *StepContext, a ...any) error {
		if a[0].(int64) == 2 {
			return errors.New("two is bad")
		}
		return nil
	})
	r := &Runner{Registry: reg}
	res := runOne(t, r, `Feature: f
  Scenario Outline: s
    Given check <n>
    Examples:
      | n |
      | 1 |
      | 2 |
      | 3 |
`)
	if len(res.Scenarios) != 3 {
		t.Fatalf("scenarios = %d", len(res.Scenarios))
	}
	want := []Status{StatusPassed, StatusFailed, StatusPassed}
	for i, s := range res.Scenarios {
		if s.Status != want[i] {
			t.Errorf("example %d status = %s, want %s", i, s.Status, want[i])
		}
		if s.ExampleIndex != i {
			t.Errorf("example index = %d", s.ExampleIndex)
		}
	}
}

func TestCancellationKeepsPartialResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reg := registryWith("wait", func(c *StepContext, a ...any) error {
		time.Sleep(20 * time.Millisecond)
		return nil
	})
	var src strings.Builder
	src.WriteString("Feature: f\n")
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&src, "  Scenario: s%d\n    Given wait\n", i)
	}
	f, err := Parse("t.feature", []byte(src.String()))
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{Registry: reg, Concurrency: 2}
	go func() {
		time.Sleep(30 * time.Millisecond) // 让第一批完成，后续被取消
		cancel()
	}()
	res := r.Run(ctx, []*Feature{f})
	if !res.Cancelled {
		t.Error("Cancelled flag not set")
	}
	var passed, cancelled int
	for _, s := range res.Scenarios {
		switch s.Status {
		case StatusPassed:
			passed++
		case StatusCancelled:
			cancelled++
		}
	}
	if passed == 0 {
		t.Error("completed scenarios must be preserved")
	}
	if cancelled == 0 {
		t.Error("unstarted scenarios must be marked cancelled")
	}
}
