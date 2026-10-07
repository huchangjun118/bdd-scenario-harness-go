package bdd

import "strings"

// ExpandOutlines 把 Feature 中的场景大纲按示例展开为独立场景。
// 普通场景原样保留；展开结果的顺序稳定（按文件中出现的顺序），
// 每个展开场景可独立运行、独立报告。
func ExpandOutlines(f *Feature) []*Scenario {
	var out []*Scenario
	for _, s := range f.Scenarios {
		if !s.Outline {
			cp := *s
			cp.Tags = mergeTags(f.Tags, s.Tags)
			out = append(out, &cp)
			continue
		}
		for _, ex := range s.Examples {
			for i, row := range ex.Rows {
				m := map[string]string{}
				for j, h := range ex.Header {
					m[h] = row.Cells[j]
				}
				out = append(out, expandScenario(s, ex, m, i, row.Line, f.Tags))
			}
		}
	}
	return out
}

func expandScenario(s *Scenario, ex *Examples, row map[string]string, idx int, rowLine int, featureTags []string) *Scenario {
	steps := make([]*Step, 0, len(s.Steps))
	for _, st := range s.Steps {
		cp := &Step{
			Keyword: st.Keyword,
			Text:    substitute(st.Text, row),
			Line:    st.Line,
		}
		if st.DocString != nil {
			cp.DocString = &DocString{
				ContentType: st.DocString.ContentType,
				Content:     substitute(st.DocString.Content, row),
				Line:        st.DocString.Line,
			}
		}
		if st.Table != nil {
			t := &DataTable{Line: st.Table.Line}
			for _, r := range st.Table.Rows {
				nr := make([]string, len(r))
				for j, c := range r {
					nr[j] = substitute(c, row)
				}
				t.Rows = append(t.Rows, nr)
			}
			cp.Table = t
		}
		steps = append(steps, cp)
	}
	tags := mergeTags(mergeTags(featureTags, s.Tags), ex.Tags)
	return &Scenario{
		Name:         substitute(s.Name, row),
		Keyword:      s.Keyword,
		Line:         rowLine,
		Tags:         tags,
		Steps:        steps,
		Outline:      true,
		ExampleIndex: idx,
		ExampleRow:   row,
		OutlineLine:  s.Line,
	}
}

// substitute 把 <name> 占位符替换为示例行中的值；未知名列原样保留。
func substitute(text string, row map[string]string) string {
	if !strings.Contains(text, "<") {
		return text
	}
	var b strings.Builder
	for {
		i := strings.IndexByte(text, '<')
		if i < 0 {
			return b.String() + text
		}
		j := strings.IndexByte(text[i:], '>')
		if j < 0 {
			return b.String() + text
		}
		key := text[i+1 : i+j]
		if v, ok := row[key]; ok {
			b.WriteString(text[:i])
			b.WriteString(v)
			text = text[i+j+1:]
		} else {
			b.WriteString(text[:i+1])
			text = text[i+1:]
		}
	}
}

// mergeTags 连接两组标签并去重，保持原有顺序。
func mergeTags(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range append(append([]string{}, a...), b...) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
