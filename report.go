package bdd

import (
	"encoding/json"
	"io"
)

// report 是 JSON 报告的顶层结构。
type report struct {
	Cancelled bool             `json:"cancelled"`
	Summary   reportSummary    `json:"summary"`
	Scenarios []reportScenario `json:"scenarios"`
}

type reportSummary struct {
	Total     int `json:"total"`
	Passed    int `json:"passed"`
	Failed    int `json:"failed"`
	Pending   int `json:"pending"`
	Undefined int `json:"undefined"`
	Ambiguous int `json:"ambiguous"`
	Skipped   int `json:"skipped"`
	Cancelled int `json:"cancelled"`
}

type reportScenario struct {
	Feature      string       `json:"feature"`
	FeaturePath  string       `json:"feature_path"`
	Name         string       `json:"name"`
	Line         int          `json:"line"`
	ExampleIndex int          `json:"example_index"`
	Status       Status       `json:"status"`
	SkipReason   SkipReason   `json:"skip_reason,omitempty"`
	DurationMs   float64      `json:"duration_ms"`
	HookErrors   []string     `json:"hook_errors,omitempty"`
	Steps        []reportStep `json:"steps,omitempty"`
}

type reportStep struct {
	Keyword    string   `json:"keyword"`
	Text       string   `json:"text"`
	Line       int      `json:"line"`
	Status     Status   `json:"status"`
	DurationMs float64  `json:"duration_ms"`
	Error      string   `json:"error,omitempty"`
	HookErrors []string `json:"hook_errors,omitempty"`
}

// WriteJSONReport 把运行结果以机器可读的 JSON 写入 w。
// 逐场景给出状态、耗时、失败步骤与错误详情。
func WriteJSONReport(w io.Writer, res *RunResult) error {
	rep := report{Cancelled: res.Cancelled}
	for _, s := range res.Scenarios {
		rs := reportScenario{
			Feature:      s.Feature,
			FeaturePath:  s.FeaturePath,
			Name:         s.Name,
			Line:         s.Line,
			ExampleIndex: s.ExampleIndex,
			Status:       s.Status,
			SkipReason:   s.SkipReason,
			DurationMs:   float64(s.Duration.Microseconds()) / 1000.0,
			HookErrors:   s.HookErrors,
		}
		for _, st := range s.Steps {
			rs.Steps = append(rs.Steps, reportStep{
				Keyword:    st.Keyword,
				Text:       st.Text,
				Line:       st.Line,
				Status:     st.Status,
				DurationMs: float64(st.Duration.Microseconds()) / 1000.0,
				Error:      st.Error,
				HookErrors: st.HookErrors,
			})
		}
		rep.Scenarios = append(rep.Scenarios, rs)
		rep.Summary.Total++
		switch s.Status {
		case StatusPassed:
			rep.Summary.Passed++
		case StatusFailed:
			rep.Summary.Failed++
		case StatusPending:
			rep.Summary.Pending++
		case StatusUndefined:
			rep.Summary.Undefined++
		case StatusAmbiguous:
			rep.Summary.Ambiguous++
		case StatusSkipped:
			rep.Summary.Skipped++
		case StatusCancelled:
			rep.Summary.Cancelled++
		}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
