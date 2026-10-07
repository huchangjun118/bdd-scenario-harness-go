package bdd

import "testing"

func mustParse(t *testing.T, src string) *Feature {
	t.Helper()
	f, err := Parse("t.feature", []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return f
}

func TestExpandOutlines(t *testing.T) {
	f := mustParse(t, `Feature: f
  Scenario: plain
    Given x
  Scenario Outline: calc <a>+<b>
    Given sum of <a> and <b>
    Then result is <sum>
      | extra |
      | <a>   |
    Examples:
      | a | b | sum |
      | 1 | 2 | 3   |
      | 4 | 5 | 9   |
`)
	out := ExpandOutlines(f)
	if len(out) != 3 {
		t.Fatalf("expanded = %d, want 3", len(out))
	}
	if out[0].Name != "plain" || out[0].ExampleIndex != -1 {
		t.Errorf("plain scenario changed: %+v", out[0])
	}
	s1, s2 := out[1], out[2]
	if s1.Name != "calc 1+2" || s2.Name != "calc 4+5" {
		t.Errorf("names = %q, %q", s1.Name, s2.Name)
	}
	if s1.Steps[0].Text != "sum of 1 and 2" {
		t.Errorf("step text = %q", s1.Steps[0].Text)
	}
	if s1.Steps[1].Table.Rows[1][0] != "1" {
		t.Errorf("table substitution = %v", s1.Steps[1].Table.Rows)
	}
	if s1.ExampleIndex != 0 || s2.ExampleIndex != 1 {
		t.Errorf("example indexes = %d, %d", s1.ExampleIndex, s2.ExampleIndex)
	}
	if s1.ExampleRow["sum"] != "3" || s2.ExampleRow["sum"] != "9" {
		t.Errorf("example rows = %v, %v", s1.ExampleRow, s2.ExampleRow)
	}
	// 展开不得修改原大纲
	if f.Scenarios[1].Steps[0].Text != "sum of <a> and <b>" {
		t.Errorf("outline mutated: %q", f.Scenarios[1].Steps[0].Text)
	}
}

func TestExpandOutlinesDocString(t *testing.T) {
	f := mustParse(t, `Feature: f
  Scenario Outline: s
    Given payload
      """
      hello <who>
      """
    Examples:
      | who   |
      | world |
`)
	out := ExpandOutlines(f)
	if got := out[0].Steps[0].DocString.Content; got != "hello world\n" {
		t.Errorf("docstring = %q", got)
	}
}

func TestExpandInheritsFeatureAndExamplesTags(t *testing.T) {
	f := mustParse(t, `@feature-tag
Feature: f
  @sc-tag
  Scenario Outline: s
    Given x
    @ex-tag
    Examples:
      | a |
      | 1 |
  @plain-tag
  Scenario: p
    Given x
`)
	out := ExpandOutlines(f)
	want := []string{"feature-tag", "sc-tag", "ex-tag"}
	got := out[0].Tags
	if len(got) != len(want) {
		t.Fatalf("tags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tags = %v, want %v", got, want)
		}
	}
	plain := out[1].Tags
	if len(plain) != 2 || plain[0] != "feature-tag" || plain[1] != "plain-tag" {
		t.Errorf("plain tags = %v", plain)
	}
}
