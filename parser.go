package bdd

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ParseFeature parses one feature file from src. On syntax error it returns
// (nil, *ParseError); it never returns a partially constructed feature.
//
// Supported syntax (a Gherkin subset):
//
//	@tag @another-tag          # tag lines (before Feature/Scenario/Outline/Examples)
//	Feature: name
//	free text description lines
//
//	Background:
//	    Given ...
//
//	@smoke
//	Scenario: name
//	    Given ...
//	    When ...
//	    Then ...
//
//	    # a doc string argument
//	    Given ...
//	        """ json
//	        {"a": 1}
//	        """
//
//	    # a data table argument
//	    Then the totals are
//	        | a | b |
//	        | 1 | 2 |
//
//	Scenario Outline: name
//	    Given a <x>
//	    Examples:
//	        | x |
//	        | 1 |
func ParseFeature(filename string, src []byte) (*Feature, error) {
	p := &parser{file: filename, lines: splitLines(src)}
	feature, err := p.parse()
	if err != nil {
		return nil, err
	}
	return feature, nil
}

type rawLine struct {
	num    int // 1-based
	indent int
	text   string // content after indentation, rtrimmed of trailing whitespace
	raw    string // original line without line ending
}

type parser struct {
	file  string
	lines []rawLine

	pendingTags []Tag
}

func splitLines(src []byte) []rawLine {
	// Normalise CRLF.
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	parts := bytes.Split(src, []byte("\n"))
	out := make([]rawLine, 0, len(parts))
	for i, p := range parts {
		raw := string(p)
		trimmedRight := strings.TrimRight(raw, " \t")
		indent := 0
		for _, r := range trimmedRight {
			if r == ' ' || r == '\t' {
				indent++
			} else {
				break
			}
		}
		out = append(out, rawLine{
			num:    i + 1,
			indent: indent,
			text:   strings.TrimPrefix(trimmedRight, strings.Repeat(" ", 0))[indent:],
			raw:    trimmedRight,
		})
	}
	// Drop a single trailing empty line produced by a final newline.
	if n := len(out); n > 0 && out[n-1].raw == "" {
		out = out[:n-1]
	}
	return out
}

func (p *parser) loc(line, col int) Location {
	return Location{File: p.file, Line: line, Column: col}
}

func (p *parser) err(line, col int, format string, args ...any) error {
	return &ParseError{Location: p.loc(line, col), Message: fmt.Sprintf(format, args...)}
}

// parseState tracks which block we are currently filling.
type parseState int

const (
	stTop parseState = iota
	stBackground
	stScenario
	stOutline
	stExamples
)

func (p *parser) parse() (*Feature, error) {
	var (
		feature   *Feature
		state     = stTop
		descBuf   []string // description collector for the current block
		bg        *Background
		scen      *Scenario
		out       *Outline
		ex        *Examples
		lastStep  **Step
		lastMain  string
		seenBlock bool // any scenario/outline/background seen after feature header
	)

	setLastStep := func(s *Step) {
		sp := s
		lastStep = &sp
	}

	flushDescription := func() {
		switch state {
		case stTop:
			if feature != nil {
				feature.Description = append(feature.Description, descBuf...)
			}
		case stScenario:
			if scen != nil {
				scen.Description = append(scen.Description, descBuf...)
			}
		case stOutline:
			if out != nil {
				out.Description = append(out.Description, descBuf...)
			}
		}
		descBuf = nil
	}

	for i := range p.lines {
		ln := p.lines[i]
		if ln.raw == "" {
			continue
		}
		// Full line comments (but never inside doc strings, handled separately).
		if strings.HasPrefix(ln.text, "#") {
			continue
		}

		col := ln.indent + 1

		// Tag lines.
		if strings.HasPrefix(ln.text, "@") {
			if err := p.collectTags(ln, &state); err != nil {
				return nil, err
			}
			continue
		}

		// Doc strings and tables are only valid as step arguments.
		if isDocStringMarker(ln.text) {
			s := deref(lastStep)
			if s == nil || state != stScenario && state != stOutline && state != stBackground {
				return nil, p.err(ln.num, col, "doc string is not attached to any step")
			}
			content, ctype, endLine, err := p.consumeDocString(i)
			if err != nil {
				return nil, err
			}
			s.DocString = &DocString{Content: content, ContentType: ctype, Location: p.loc(ln.num, col)}
			setLastStep(s)
			i = endLine
			continue
		}
		if strings.HasPrefix(ln.text, "|") {
			row, err := parseTableRow(ln)
			if err != nil {
				return nil, p.err(ln.num, col, "%s", err.Error())
			}
			// Examples table?
			if state == stExamples && ex != nil {
				if len(ex.Header.Cells) == 0 {
					ex.Header = row
				} else {
					if len(row.Cells) != len(ex.Header.Cells) {
						return nil, p.err(ln.num, col,
							"table row has %d cells but header has %d", len(row.Cells), len(ex.Header.Cells))
					}
					ex.Rows = append(ex.Rows, row)
				}
				continue
			}
			s := deref(lastStep)
			if s == nil {
				return nil, p.err(ln.num, col, "data table is not attached to any step or Examples block")
			}
			if s.Table != nil {
				return nil, p.err(ln.num, col, "step already has a data table")
			}
			// Collect contiguous table rows (blank lines allowed between).
			tbl := &DataTable{Rows: []TableRow{row}, Location: p.loc(ln.num, col)}
			end := i
			for j := i + 1; j < len(p.lines); j++ {
				nl := p.lines[j]
				if nl.raw == "" || strings.HasPrefix(nl.text, "#") {
					continue
				}
				if strings.HasPrefix(nl.text, "|") {
					r, err := parseTableRow(nl)
					if err != nil {
						return nil, p.err(nl.num, nl.indent+1, "%s", err.Error())
					}
					if len(r.Cells) != len(row.Cells) {
						return nil, p.err(nl.num, nl.indent+1,
							"table row has %d cells but first row has %d", len(r.Cells), len(row.Cells))
					}
					tbl.Rows = append(tbl.Rows, r)
					end = j
					continue
				}
				break
			}
			s.Table = tbl
			setLastStep(s)
			i = end
			continue
		}

		// Block / step keywords.
		kw, rest, ok := matchKeyword(ln.text)
		if !ok {
			// Free text: treat as description when permitted.
			if state == stTop && feature != nil && !seenBlock {
				descBuf = append(descBuf, strings.TrimSpace(ln.text))
				continue
			}
			if (state == stScenario || state == stOutline) && deref(lastStep) == nil {
				descBuf = append(descBuf, strings.TrimSpace(ln.text))
				continue
			}
			return nil, p.err(ln.num, col, "unexpected text %q: expected a keyword (Feature, Background, Scenario, Scenario Outline, Given/When/Then/And/But or Examples)", ln.text)
		}

		switch kw {
		case "Feature":
			if feature != nil {
				return nil, p.err(ln.num, col, "more than one Feature header in file")
			}
			feature = &Feature{
				Name:       strings.TrimSpace(rest),
				Tags:       p.takeTags(),
				Location:   p.loc(ln.num, col),
				SourceFile: p.file,
			}
			state = stTop
		case "Background":
			flushDescription()
			if feature == nil {
				return nil, p.err(ln.num, col, "Background found before Feature header")
			}
			if bg != nil {
				return nil, p.err(ln.num, col, "more than one Background block")
			}
			if seenBlock {
				return nil, p.err(ln.num, col, "Background must appear before any scenario")
			}
			if tags := p.peekTags(); len(tags) > 0 {
				return nil, p.err(ln.num, col, "tags are not allowed before Background")
			}
			bg = &Background{Keyword: kw, Location: p.loc(ln.num, col)}
			feature.Background = bg
			state = stBackground
			lastStep = nil
			lastMain = "Given"
		case "Scenario", "Example":
			flushDescription()
			if err := p.finishExamples(ex, state, ln); err != nil {
				return nil, err
			}
			if feature == nil {
				return nil, p.err(ln.num, col, "Scenario found before Feature header")
			}
			if len(p.pendingTags) == 0 {
				// nothing
			}
			scen = &Scenario{
				Name:         strings.TrimSpace(rest),
				Tags:         p.takeTags(),
				Location:     p.loc(ln.num, col),
				ExampleIndex: -1,
			}
			feature.Scenarios = append(feature.Scenarios, scen)
			state = stScenario
			lastStep = nil
			lastMain = ""
			seenBlock = true
			ex = nil
		case "Scenario Outline", "Scenario Template":
			flushDescription()
			if err := p.finishExamples(ex, state, ln); err != nil {
				return nil, err
			}
			if feature == nil {
				return nil, p.err(ln.num, col, "Scenario Outline found before Feature header")
			}
			out = &Outline{
				Name:        strings.TrimSpace(rest),
				Tags:        p.takeTags(),
				Location:    p.loc(ln.num, col),
			}
			feature.Outlines = append(feature.Outlines, out)
			state = stOutline
			lastStep = nil
			lastMain = ""
			seenBlock = true
			ex = nil
		case "Examples":
			if state != stOutline || out == nil {
				return nil, p.err(ln.num, col, "Examples can only appear inside a Scenario Outline")
			}
			// Finalise the previous examples block first.
			if ex != nil {
				if err := validateExamples(ex); err != nil {
					return nil, err
				}
			}
			ex = &Examples{
				Tags:     p.takeTags(),
				Keyword:  "Examples",
				Name:     strings.TrimSpace(rest),
				Location: p.loc(ln.num, col),
			}
			out.Examples = append(out.Examples, ex)
			state = stExamples
		case "Given", "When", "Then", "And", "But", "*":
			if state == stExamples {
				return nil, p.err(ln.num, col, "steps are not allowed after Examples; expected table rows")
			}
			if state == stTop {
				return nil, p.err(ln.num, col, "step %q found outside of a scenario or background", kw)
			}
			flushDescription()
			main := kw
			if kw == "And" || kw == "But" || kw == "*" {
				if lastMain == "" {
					main = "Given"
				} else {
					main = lastMain
				}
			}
			lastMain = main
			step := &Step{
				Keyword:  kw,
				Lineage:  main,
				Text:     strings.TrimSpace(rest),
				Location: p.loc(ln.num, col),
			}
			if step.Text == "" {
				return nil, p.err(ln.num, col, "step %q has no text", kw)
			}
			switch state {
			case stBackground:
				bg.Steps = append(bg.Steps, step)
			case stScenario:
				scen.Steps = append(scen.Steps, step)
			case stOutline:
				out.Steps = append(out.Steps, step)
			}
			setLastStep(step)
		default:
			return nil, p.err(ln.num, col, "unsupported keyword %q", kw)
		}
	}

	if feature == nil {
		return nil, p.err(1, 1, "no Feature header found")
	}
	flushDescription()
	if len(p.pendingTags) > 0 {
		t := p.pendingTags[0]
		return nil, &ParseError{Location: t.Location, Message: "dangling tags at end of file with no following block"}
	}
	if err := p.finishExamples(ex, state, rawLine{}); err != nil {
		return nil, err
	}
	// Validate every outline has examples with rows.
	for _, o := range feature.Outlines {
		if len(o.Examples) == 0 {
			return nil, &ParseError{
				Location: o.Location,
				Message:  fmt.Sprintf("scenario outline %q has no Examples table", o.Name),
			}
		}
		for _, e := range o.Examples {
			if err := validateExamples(e); err != nil {
				return nil, err
			}
		}
	}
	return feature, nil
}

func (p *parser) finishExamples(ex *Examples, state parseState, ln rawLine) error {
	if state == stExamples && ex != nil {
		if err := validateExamples(ex); err != nil {
			return err
		}
	}
	return nil
}

func validateExamples(ex *Examples) error {
	if len(ex.Header.Cells) == 0 {
		return &ParseError{Location: ex.Location, Message: fmt.Sprintf("examples %q has no header row", ex.Name)}
	}
	if len(ex.Rows) == 0 {
		return &ParseError{Location: ex.Location, Message: fmt.Sprintf("examples %q has no data rows", ex.Name)}
	}
	seen := map[string]bool{}
	for _, c := range ex.Header.Cells {
		if c == "" {
			return &ParseError{Location: ex.Header.Location, Message: "examples header contains an empty cell"}
		}
		if seen[c] {
			return &ParseError{Location: ex.Header.Location, Message: fmt.Sprintf("duplicate examples column %q", c)}
		}
		seen[c] = true
	}
	for _, r := range ex.Rows {
		for _, c := range r.Cells {
			if c == "" {
				return &ParseError{Location: r.Location, Message: "examples row contains an empty cell"}
			}
		}
	}
	return nil
}

func (p *parser) takeTags() []Tag {
	t := p.pendingTags
	p.pendingTags = nil
	return t
}

func (p *parser) peekTags() []Tag { return p.pendingTags }

func (p *parser) collectTags(ln rawLine, state *parseState) error {
	fields := strings.Fields(ln.text)
	for _, f := range fields {
		if !strings.HasPrefix(f, "@") || f == "@" {
			col := ln.indent + strings.Index(ln.text, f) + 1
			return p.err(ln.num, col, "malformed tag %q: tags must look like @name", f)
		}
		p.pendingTags = append(p.pendingTags, Tag{
			Name:     f,
			Location: p.loc(ln.num, ln.indent+1),
		})
	}
	return nil
}

// consumeDocString reads from the opening marker at startIdx until the closing
// marker. It returns the dedented content, optional content type and the index
// of the closing line.
func (p *parser) consumeDocString(startIdx int) (content, contentType string, endIdx int, err error) {
	open := p.lines[startIdx]
	marker := `"""`
	if strings.HasPrefix(strings.TrimSpace(open.text), "```") {
		marker = "```"
	}
	rest := strings.TrimSpace(open.text)[len(marker):]
	contentType = strings.TrimSpace(rest)
	var lines []string
	for j := startIdx + 1; j < len(p.lines); j++ {
		ln := p.lines[j]
		if strings.TrimSpace(ln.text) == marker {
			return strings.Join(lines, "\n"), contentType, j, nil
		}
		// Strip up to the opening indentation; preserve further indentation.
		s := ln.raw
		if len(s) >= open.indent {
			s = s[open.indent:]
		} else {
			s = strings.TrimLeft(s, " \t")
		}
		lines = append(lines, s)
	}
	return "", "", 0, p.err(open.num, open.indent+1, "unterminated doc string: expected closing %s", marker)
}

func isDocStringMarker(t string) bool {
	ts := strings.TrimSpace(t)
	return strings.HasPrefix(ts, `"""`) || strings.HasPrefix(ts, "```")
}

func parseTableRow(ln rawLine) (TableRow, error) {
	t := strings.TrimSpace(ln.text)
	if !strings.HasPrefix(t, "|") {
		return TableRow{}, fmt.Errorf("malformed table row: must start with '|'")
	}
	if !strings.HasSuffix(t, "|") {
		return TableRow{}, fmt.Errorf("malformed table row: must end with '|'")
	}
	inner := t[1 : len(t)-1]
	parts := strings.Split(inner, "|")
	cells := make([]string, len(parts))
	for i, c := range parts {
		cells[i] = strings.TrimSpace(c)
	}
	return TableRow{Cells: cells, Location: Location{Line: ln.num, Column: ln.indent + 1}}, nil
}

// matchKeyword recognises block and step keywords, returning the canonical
// keyword and the remainder of the line after it (and its colon when needed).
func matchKeyword(text string) (kw, rest string, ok bool) {
	// Order matters: multi-word keywords first.
	for _, k := range []string{
		"Scenario Outline", "Scenario Template",
		"Feature", "Background", "Scenario", "Example", "Examples",
		"Given", "When", "Then", "And", "But",
	} {
		if hasWord(text, k) {
			r := text[len(k):]
			if k == "Feature" || k == "Background" || k == "Scenario" || k == "Example" ||
				k == "Scenario Outline" || k == "Scenario Template" || k == "Examples" {
				r = strings.TrimPrefix(r, ":")
			}
			return k, strings.TrimSpace(r), true
		}
	}
	if strings.HasPrefix(text, "*") {
		r := text[1:]
		if r == "" || r[0] == ' ' || r[0] == '\t' {
			return "*", strings.TrimSpace(r), true
		}
	}
	return "", "", false
}

// hasWord reports whether text starts with keyword followed by ':' (blocks) or
// whitespace (steps), matching case-insensitively.
func hasWord(text, keyword string) bool {
	if len(text) < len(keyword) {
		return false
	}
	if !strings.EqualFold(text[:len(keyword)], keyword) {
		return false
	}
	if len(text) == len(keyword) {
		return false
	}
	c := text[len(keyword)]
	return c == ':' || c == ' ' || c == '\t'
}

func deref(s *Step) *Step {
	if s == nil {
		return nil
	}
	return s
}

var _ = utf8.RuneCountInString
