package bdd

import (
	"strings"
	"testing"
)

func TestParseValidFeature(t *testing.T) {
	src := `@fast
Feature: shopping cart
  @web
  Scenario: add item
    Given a cart
    When I add an item
    Then the count is 1
`
	f, err := Parse("cart.feature", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Name != "shopping cart" {
		t.Errorf("feature name = %q", f.Name)
	}
	if len(f.Tags) != 1 || f.Tags[0] != "fast" {
		t.Errorf("feature tags = %v", f.Tags)
	}
	if len(f.Scenarios) != 1 {
		t.Fatalf("scenarios = %d", len(f.Scenarios))
	}
	s := f.Scenarios[0]
	if s.Name != "add item" || len(s.Steps) != 3 {
		t.Errorf("scenario = %+v", s)
	}
	if s.Steps[0].Keyword != "Given" || s.Steps[0].Text != "a cart" {
		t.Errorf("step0 = %+v", s.Steps[0])
	}
	if s.Line != 4 {
		t.Errorf("scenario line = %d, want 4", s.Line)
	}
}

func TestParseDocStringAndTable(t *testing.T) {
	src := `Feature: f
  Scenario: s
    Given a doc
      """json
      {"a": 1}
      """
    When a table
      | name | age |
      | ann  | 30  |
`
	f, err := Parse("f.feature", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	st := f.Scenarios[0].Steps[0]
	if st.DocString == nil || st.DocString.ContentType != "json" {
		t.Fatalf("docstring = %+v", st.DocString)
	}
	if !strings.Contains(st.DocString.Content, `{"a": 1}`) {
		t.Errorf("docstring content = %q", st.DocString.Content)
	}
	tbl := f.Scenarios[0].Steps[1].Table
	if tbl == nil || len(tbl.Rows) != 2 || tbl.Rows[1][1] != "30" {
		t.Errorf("table = %+v", tbl)
	}
}

func TestParseOutline(t *testing.T) {
	src := `Feature: f
  Scenario Outline: eat
    Given there are <start> cucumbers
    Then I have <left> left
    Examples: small
      | start | left |
      | 5     | 4    |
      | 3     | 2    |
`
	f, err := Parse("f.feature", []byte(src))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := f.Scenarios[0]
	if !s.Outline || len(s.Examples) != 1 {
		t.Fatalf("outline = %+v", s)
	}
	ex := s.Examples[0]
	if ex.Name != "small" {
		t.Errorf("examples name = %q", ex.Name)
	}
	if len(ex.Header) != 2 || len(ex.Rows) != 2 || ex.Rows[1].Cells[0] != "3" {
		t.Errorf("examples = %+v", ex)
	}
}

// 语法错误必须带文件、行、列，且不产出 Feature。
func TestParseSyntaxErrors(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantLine int
		wantMsg  string
	}{
		{"no feature", "Scenario: s\n", 1, "Feature"},
		{"step outside scenario", "Feature: f\n  Given x\n", 2, "outside"},
		{"empty scenario name", "Feature: f\n  Scenario:\n", 2, "requires a name"},
		{"unclosed docstring", "Feature: f\n  Scenario: s\n    Given x\n      \"\"\"\n      text\n", 4, "never closed"},
		{"examples row width", "Feature: f\n  Scenario Outline: s\n    Given <a>\n    Examples:\n      | a | b |\n      | 1 |\n", 6, "header"},
		{"outline without examples", "Feature: f\n  Scenario Outline: s\n    Given <a>\n", 2, "no Examples"},
		{"stray table", "Feature: f\n  Scenario: s\n    | a |\n", 3, "not attached"},
		{"unexpected content", "Feature: f\n  Scenario: s\n    Given x\n  bogus line\n", 4, "unexpected"},
		{"duplicate feature", "Feature: a\nFeature: b\n", 2, "only one"},
		{"dangling tags", "Feature: f\n  Scenario: s\n    Given x\n  @lonely\n", 4, "not attached"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := Parse("bad.feature", []byte(tc.src))
			if err == nil {
				t.Fatalf("expected error, got feature %+v", f)
			}
			if f != nil {
				t.Errorf("parse failure must not yield partial feature, got %+v", f)
			}
			pe, ok := err.(*ParseError)
			if !ok {
				t.Fatalf("error type = %T, want *ParseError", err)
			}
			if pe.Path != "bad.feature" {
				t.Errorf("path = %q", pe.Path)
			}
			if pe.Line != tc.wantLine {
				t.Errorf("line = %d, want %d (err: %v)", pe.Line, tc.wantLine, pe)
			}
			if pe.Col < 1 {
				t.Errorf("col = %d, want >= 1", pe.Col)
			}
			if !strings.Contains(pe.Msg, tc.wantMsg) {
				t.Errorf("msg = %q, want substring %q", pe.Msg, tc.wantMsg)
			}
		})
	}
}

func TestParseFeatureDescription(t *testing.T) {
	f, err := Parse("f.feature", []byte(`Feature: f
  first line
  second line
  Scenario: s
    Given x
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Description != "first line\nsecond line" {
		t.Errorf("description = %q", f.Description)
	}
}
