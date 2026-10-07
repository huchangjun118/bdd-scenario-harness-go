package bdd

import (
	"fmt"
	"strings"
)

// ParseError 描述一个语法错误，包含文件、行列与可读原因。
type ParseError struct {
	Path string
	Line int
	Col  int
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Line, e.Col, e.Msg)
}

// Parse 解析一份场景描述文件。出现语法错误时返回 *ParseError，
// 且不产出任何 Feature（不会返回半截结果）。
func Parse(path string, src []byte) (*Feature, error) {
	p := &parser{path: path, lines: strings.Split(string(src), "\n")}
	return p.parse()
}

type parser struct {
	path  string
	lines []string
	pos   int // 当前行下标（0 起）
}

func (p *parser) errf(line, col int, format string, args ...any) *ParseError {
	return &ParseError{Path: p.path, Line: line, Col: col, Msg: fmt.Sprintf(format, args...)}
}

// firstCol 返回行内第一个非空白字符的 1 基列号。
func firstCol(s string) int {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return i + 1
		}
	}
	return 1
}

func (p *parser) parse() (*Feature, error) {
	f := &Feature{Path: p.path}
	var pendingTags []string
	pendingTagsLine := 0
	seenFeature := false
	var cur *Scenario
	var lastStep *Step
	var curExamples *Examples

	flushTagsTo := func(dst *[]string) { *dst = append(*dst, pendingTags...); pendingTags = nil }

	for p.pos < len(p.lines) {
		raw := p.lines[p.pos]
		lineNo := p.pos + 1
		trimmed := strings.TrimSpace(raw)
		p.pos++

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		col := firstCol(raw)

		switch {
		case strings.HasPrefix(trimmed, "@"):
			for _, t := range strings.Fields(trimmed) {
				if !strings.HasPrefix(t, "@") || len(t) < 2 {
					return nil, p.errf(lineNo, col, "invalid tag %q: tags look like @name", t)
				}
				if pendingTags == nil {
					pendingTagsLine = lineNo
				}
				pendingTags = append(pendingTags, t[1:])
			}

		case keyword(trimmed, "Feature:"):
			if seenFeature {
				return nil, p.errf(lineNo, col, "duplicate Feature: a file may contain only one feature")
			}
			seenFeature = true
			f.Name = strings.TrimSpace(trimmed[len("Feature:"):])
			if f.Name == "" {
				return nil, p.errf(lineNo, col, "Feature requires a name")
			}
			flushTagsTo(&f.Tags)

		case keyword(trimmed, "Scenario Outline:"), keyword(trimmed, "Scenario Template:"):
			if !seenFeature {
				return nil, p.errf(lineNo, col, "scenario declared before any Feature")
			}
			kw := strings.SplitN(trimmed, ":", 2)[0]
			name := strings.TrimSpace(trimmed[len(kw)+1:])
			if name == "" {
				return nil, p.errf(lineNo, col, "%s requires a name", kw)
			}
			cur = &Scenario{Name: name, Keyword: kw, Line: lineNo, Outline: true, ExampleIndex: -1}
			flushTagsTo(&cur.Tags)
			f.Scenarios = append(f.Scenarios, cur)
			lastStep, curExamples = nil, nil

		case keyword(trimmed, "Scenario:"):
			if !seenFeature {
				return nil, p.errf(lineNo, col, "scenario declared before any Feature")
			}
			name := strings.TrimSpace(trimmed[len("Scenario:"):])
			if name == "" {
				return nil, p.errf(lineNo, col, "Scenario requires a name")
			}
			cur = &Scenario{Name: name, Keyword: "Scenario", Line: lineNo, Outline: false, ExampleIndex: -1}
			flushTagsTo(&cur.Tags)
			f.Scenarios = append(f.Scenarios, cur)
			lastStep, curExamples = nil, nil

		case isStepKeyword(trimmed):
			if cur == nil {
				return nil, p.errf(lineNo, col, "step declared outside of any scenario")
			}
			kw := strings.Fields(trimmed)[0]
			text := strings.TrimSpace(trimmed[len(kw):])
			if kw == "*" {
				text = strings.TrimSpace(trimmed[1:])
			}
			if text == "" {
				return nil, p.errf(lineNo, col, "step %q requires text", kw)
			}
			st := &Step{Keyword: kw, Text: text, Line: lineNo}
			cur.Steps = append(cur.Steps, st)
			lastStep = st

		case keyword(trimmed, "Examples:"):
			if cur == nil || !cur.Outline {
				return nil, p.errf(lineNo, col, "Examples declared outside of a Scenario Outline")
			}
			ex := &Examples{Name: strings.TrimSpace(trimmed[len("Examples:"):]), Line: lineNo}
			flushTagsTo(&ex.Tags)
			cur.Examples = append(cur.Examples, ex)
			curExamples = ex
			lastStep = nil

		case strings.HasPrefix(trimmed, "|"):
			row, err := p.parseTableRow(trimmed, lineNo)
			if err != nil {
				return nil, err
			}
			switch {
			case curExamples != nil:
				if curExamples.Header == nil {
					curExamples.Header = row
				} else {
					if len(row) != len(curExamples.Header) {
						return nil, p.errf(lineNo, col,
							"examples row has %d cells, but header at line %d has %d",
							len(row), curExamples.Line, len(curExamples.Header))
					}
					curExamples.Rows = append(curExamples.Rows, TableRow{Line: lineNo, Cells: row})
				}
			case lastStep != nil:
				if lastStep.Table == nil {
					lastStep.Table = &DataTable{Line: lineNo}
				}
				lastStep.Table.Rows = append(lastStep.Table.Rows, row)
			default:
				return nil, p.errf(lineNo, col, "data table row is not attached to a step or Examples block")
			}

		case strings.HasPrefix(trimmed, `"""`):
			if lastStep == nil {
				return nil, p.errf(lineNo, col, "doc string is not attached to a step")
			}
			ds, err := p.parseDocString(trimmed, lineNo)
			if err != nil {
				return nil, err
			}
			lastStep.DocString = ds

		default:
			// Feature 标题之后、首个场景之前的自由文本视为描述。
			if seenFeature && cur == nil && len(pendingTags) == 0 {
				if f.Description != "" {
					f.Description += "\n"
				}
				f.Description += trimmed
				continue
			}
			return nil, p.errf(lineNo, col, "unexpected content %q: expected a tag, Feature, Scenario, step, Examples, table row or doc string", trimmed)
		}
	}

	if !seenFeature {
		return nil, p.errf(1, 1, "file does not declare a Feature")
	}
	if len(pendingTags) > 0 {
		return nil, p.errf(pendingTagsLine, 1, "tags %v are not attached to any Feature, Scenario or Examples", pendingTags)
	}
	for _, s := range f.Scenarios {
		if s.Outline {
			if len(s.Examples) == 0 {
				return nil, p.errf(s.Line, 1, "Scenario Outline %q has no Examples", s.Name)
			}
			for _, ex := range s.Examples {
				if ex.Header == nil {
					return nil, p.errf(ex.Line, 1, "Examples block is missing its header table row")
				}
			}
		}
	}
	return f, nil
}

// keyword 报告行是否以 "kw" 开头（kw 含结尾冒号）。
func keyword(trimmed, kw string) bool { return strings.HasPrefix(trimmed, kw) }

func isStepKeyword(trimmed string) bool {
	for _, kw := range []string{"Given ", "When ", "Then ", "And ", "But ", "* "} {
		if strings.HasPrefix(trimmed, kw) {
			return true
		}
	}
	return trimmed == "Given" || trimmed == "When" || trimmed == "Then" ||
		trimmed == "And" || trimmed == "But" || trimmed == "*"
}

// parseTableRow 解析 "| a | b |" 形式的行，支持 \| 转义。
func (p *parser) parseTableRow(trimmed string, lineNo int) ([]string, error) {
	body := strings.TrimPrefix(trimmed, "|")
	if !strings.HasSuffix(strings.TrimRight(body, " \t"), "|") {
		return nil, p.errf(lineNo, firstCol(p.lines[lineNo-1]), "table row must end with '|'")
	}
	body = strings.TrimSuffix(strings.TrimRight(body, " \t"), "|")
	var cells []string
	var cur strings.Builder
	escaped := false
	for _, r := range body {
		switch {
		case escaped:
			if r != '|' && r != '\\' {
				cur.WriteByte('\\')
			}
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '|':
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if escaped {
		cur.WriteByte('\\')
	}
	cells = append(cells, strings.TrimSpace(cur.String()))
	return cells, nil
}

// parseDocString 解析 """ 包裹的多行文本，消耗到结束分隔符。
func (p *parser) parseDocString(opening string, lineNo int) (*DocString, error) {
	ctype := strings.TrimSpace(strings.TrimPrefix(opening, `"""`))
	var b strings.Builder
	for p.pos < len(p.lines) {
		raw := p.lines[p.pos]
		p.pos++
		if strings.HasPrefix(strings.TrimSpace(raw), `"""`) {
			return &DocString{ContentType: ctype, Content: b.String(), Line: lineNo}, nil
		}
		b.WriteString(strings.TrimSpace(raw))
		b.WriteByte('\n')
	}
	return nil, p.errf(lineNo, firstCol(p.lines[lineNo-1]), "doc string opened here is never closed with \"\"\"")
}
