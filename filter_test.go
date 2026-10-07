package bdd

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

const filterFeature = `Feature: f
  @web @fast
  Scenario: login works
    Given ok
  @web @slow
  Scenario: upload works
    Given ok
  @api
  Scenario: token refresh
    Given ok
  @web @skip
  Scenario: legacy page
    Given ok
`

func runFiltered(t *testing.T, f Filter) *RunResult {
	t.Helper()
	feat, err := Parse("f.feature", []byte(filterFeature))
	if err != nil {
		t.Fatal(err)
	}
	reg := registryWith("ok", func(ctx *StepContext, a ...any) error { return nil })
	r := &Runner{Registry: reg, Filter: f}
	return r.Run(context.Background(), []*Feature{feat})
}

func statuses(res *RunResult) map[string]ScenarioResult {
	m := map[string]ScenarioResult{}
	for _, s := range res.Scenarios {
		m[s.Name] = s
	}
	return m
}

func TestFilterByTags(t *testing.T) {
	res := runFiltered(t, Filter{IncludeTags: []string{"web"}, ExcludeTags: []string{"slow"}})
	m := statuses(res)
	if m["login works"].Status != StatusPassed {
		t.Errorf("login = %s", m["login works"].Status)
	}
	// 被排除标签命中 -> excluded，区别于未选中
	if m["upload works"].Status != StatusSkipped || m["upload works"].SkipReason != ReasonExcluded {
		t.Errorf("upload = %s/%s", m["upload works"].Status, m["upload works"].SkipReason)
	}
	// 不满足 include -> not_selected
	if m["token refresh"].Status != StatusSkipped || m["token refresh"].SkipReason != ReasonNotSelected {
		t.Errorf("token = %s/%s", m["token refresh"].Status, m["token refresh"].SkipReason)
	}
	// 主动 @skip -> skipped
	if m["legacy page"].Status != StatusSkipped || m["legacy page"].SkipReason != ReasonSkipped {
		t.Errorf("legacy = %s/%s", m["legacy page"].Status, m["legacy page"].SkipReason)
	}
}

func TestFilterByName(t *testing.T) {
	res := runFiltered(t, Filter{NameContains: "WORKS"})
	m := statuses(res)
	if m["login works"].Status != StatusPassed || m["token refresh"].SkipReason != ReasonNotSelected {
		t.Errorf("name filter wrong: %+v", m)
	}
}

func TestSkipTagWithoutFilter(t *testing.T) {
	res := runFiltered(t, Filter{})
	m := statuses(res)
	if m["legacy page"].Status != StatusSkipped || m["legacy page"].SkipReason != ReasonSkipped {
		t.Errorf("legacy = %s/%s", m["legacy page"].Status, m["legacy page"].SkipReason)
	}
	if m["login works"].Status != StatusPassed {
		t.Errorf("login = %s", m["login works"].Status)
	}
}

func TestJSONReport(t *testing.T) {
	reg := registryWith(
		"ok", func(ctx *StepContext, a ...any) error { return nil },
		"boom", func(ctx *StepContext, a ...any) error { return ErrPending },
	)
	feat, err := Parse("f.feature", []byte(`Feature: f
  Scenario: good
    Given ok
  Scenario: bad
    Given boom
`))
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{Registry: reg}
	res := r.Run(context.Background(), []*Feature{feat})

	var buf bytes.Buffer
	if err := WriteJSONReport(&buf, res); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Cancelled bool `json:"cancelled"`
		Summary   struct {
			Total, Passed, Pending int
		} `json:"summary"`
		Scenarios []struct {
			Name   string `json:"name"`
			Status Status `json:"status"`
			Steps  []struct {
				Text   string `json:"text"`
				Status Status `json:"status"`
				Error  string `json:"error"`
			} `json:"steps"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if decoded.Summary.Total != 2 || decoded.Summary.Passed != 1 || decoded.Summary.Pending != 1 {
		t.Errorf("summary = %+v", decoded.Summary)
	}
	if decoded.Scenarios[1].Steps[0].Status != StatusPending ||
		decoded.Scenarios[1].Steps[0].Error == "" {
		t.Errorf("pending step detail = %+v", decoded.Scenarios[1].Steps[0])
	}
}
