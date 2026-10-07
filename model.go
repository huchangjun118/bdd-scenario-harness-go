// Package bdd implements an in-process behaviour-driven test scenario
// orchestration and execution kernel.
//
// It parses Gherkin-style feature descriptions, expands scenario outlines
// with example data, binds steps to registered definitions and executes the
// resulting scenarios deterministically (with optional bounded concurrency),
// producing machine readable reports.
package bdd

import "time"

// Status of an executed step or scenario.
type Status string

const (
	// StatusPassed means the step/scenario executed successfully.
	StatusPassed Status = "passed"
	// StatusFailed means an assertion or handler returned an error.
	StatusFailed Status = "failed"
	// StatusUndefined means no step definition is bound to the step.
	StatusUndefined Status = "undefined"
	// StatusPending means the step is explicitly marked as not implemented.
	StatusPending Status = "pending"
	// StatusSkipped means the step/scenario was not run for a recorded reason.
	StatusSkipped Status = "skipped"
	// StatusCanceled means execution was interrupted by context cancellation.
	StatusCanceled Status = "canceled"
)

// SkipReason explains why a scenario was not executed.
type SkipReason string

const (
	// SkipNone means the scenario was not skipped.
	SkipNone SkipReason = ""
	// SkipFiltered means the scenario did not match the active filters.
	SkipFiltered SkipReason = "filtered"
	// SkipExcluded means the scenario matched an active exclusion filter.
	SkipExcluded SkipReason = "excluded"
	// SkipManual means the scenario (or a preceding step) asked to be skipped.
	SkipManual SkipReason = "manual"
)

// Location identifies a position inside a source file.
type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Tag attached to a feature, scenario or examples block.
type Tag struct {
	Name     string   `json:"name"` // including the leading '@'
	Location Location `json:"location"`
}

// DocString is a multiline string argument attached to a step.
type DocString struct {
	Content     string   `json:"content"`
	ContentType string   `json:"contentType,omitempty"`
	Location    Location `json:"location"`
}

// TableRow is one row of a data table (or examples table).
type TableRow struct {
	Cells    []string `json:"cells"`
	Location Location `json:"location"`
}

// DataTable is a tabular argument attached to a step.
type DataTable struct {
	Rows     []TableRow `json:"rows"`
	Location Location   `json:"location"`
}

// Step is a single When/Then/Given/And/But statement.
type Step struct {
	Keyword   string     `json:"keyword"`
	Text      string     `json:"text"`
	DocString *DocString `json:"docString,omitempty"`
	Table     *DataTable `json:"table,omitempty"`
	Location  Location   `json:"location"`

	// Lineage is the effective keyword chain (used for binding key prefix).
	Lineage string `json:"-"`
}

// Scenario is either a plain scenario or one expanded example of an outline.
type Scenario struct {
	Name         string     `json:"name"`
	Description  []string   `json:"description,omitempty"`
	Tags         []Tag      `json:"tags,omitempty"`
	Steps        []*Step    `json:"steps,omitempty"`
	Location     Location   `json:"location"`
	OutlineName  string     `json:"outlineName,omitempty"`
	ExampleIndex int        `json:"exampleIndex,omitempty"` // -1 when not from an outline
	ExampleRow   *TableRow  `json:"exampleRow,omitempty"`
}

// Examples is one data table attached to a scenario outline.
type Examples struct {
	Tags     []Tag     `json:"tags,omitempty"`
	Keyword  string    `json:"keyword"`
	Name     string    `json:"name,omitempty"`
	Header   TableRow  `json:"header"`
	Rows     []TableRow `json:"rows,omitempty"`
	Location Location  `json:"location"`
}

// Background contains steps executed before every scenario in the feature.
type Background struct {
	Keyword  string  `json:"keyword"`
	Steps    []*Step `json:"steps,omitempty"`
	Location Location `json:"location"`
}

// Feature is the parsed content of one feature file.
type Feature struct {
	Name        string      `json:"name"`
	Description []string    `json:"description,omitempty"`
	Tags        []Tag       `json:"tags,omitempty"`
	Background  *Background `json:"background,omitempty"`
	Scenarios   []*Scenario `json:"scenarios,omitempty"`
	Outlines    []*Outline  `json:"outlines,omitempty"`
	Location    Location    `json:"location"`
	SourceFile  string      `json:"sourceFile,omitempty"`
}

// Outline is a scenario outline before example expansion.
type Outline struct {
	Name        string     `json:"name"`
	Description []string   `json:"description,omitempty"`
	Tags        []Tag      `json:"tags,omitempty"`
	Steps       []*Step    `json:"steps,omitempty"`
	Examples    []*Examples `json:"examples,omitempty"`
	Location    Location   `json:"location"`
}

// ErrorEntry records one failure in the causal chain (original failure first).
type ErrorEntry struct {
	Message  string   `json:"message"`
	Location Location `json:"location,omitempty"`
	Phase    string   `json:"phase,omitempty"` // step | before_hook | after_hook | engine
}

// StepResult is the result of executing one step.
type StepResult struct {
	Keyword   string       `json:"keyword"`
	Text      string       `json:"text"`
	Lineage   string       `json:"lineage"`
	Status    Status       `json:"status"`
	Duration  Duration     `json:"duration"`
	Location  Location     `json:"location"`
	Errors    []ErrorEntry `json:"errors,omitempty"`
	SkipReason SkipReason  `json:"skipReason,omitempty"`
}

// ScenarioResult is the result of executing one (expanded) scenario.
type ScenarioResult struct {
	Name         string       `json:"name"`
	OutlineName  string       `json:"outlineName,omitempty"`
	ExampleIndex int          `json:"exampleIndex"`
	Tags         []string     `json:"tags,omitempty"`
	Location     Location     `json:"location"`
	Status       Status       `json:"status"`
	SkipReason   SkipReason   `json:"skipReason,omitempty"`
	SkipDetail   string       `json:"skipDetail,omitempty"`
	Duration     Duration     `json:"duration"`
	Steps        []*StepResult `json:"steps,omitempty"`
	Errors       []ErrorEntry `json:"errors,omitempty"`
}

// FeatureResult aggregates scenario results from one feature file.
type FeatureResult struct {
	Name    string            `json:"name"`
	File    string            `json:"file"`
	Scenarios []*ScenarioResult `json:"scenarios"`
}

// Report is the full machine readable result of a run.
type Report struct {
	Features []*FeatureResult `json:"features"`
	// Summary counts (order independent).
	Passed    int `json:"passed"`
	Failed    int `json:"failed"`
	Undefined int `json:"undefined"`
	Pending   int `json:"pending"`
	Skipped   int `json:"skipped"`
	Canceled  int `json:"canceled"`
	Total     int `json:"total"`
	// Fatal is set when the run could not start (e.g. binding conflicts).
	Fatal *ErrorEntry `json:"fatal,omitempty"`
}

// Duration is a nanosecond-precision duration, rendered in JSON as an integer
// number of nanoseconds to keep reports exact and comparable.
type Duration time.Duration
