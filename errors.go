package bdd

import "fmt"

// ParseError describes a syntax error in a feature file with a precise
// file/line/column location and a human readable reason.
type ParseError struct {
	Location Location
	Message  string
}

func (e *ParseError) Error() string {
	if e.Location.File != "" {
		if e.Location.Column > 0 {
			return fmt.Sprintf("%s:%d:%d: %s", e.Location.File, e.Location.Line, e.Location.Column, e.Message)
		}
		return fmt.Sprintf("%s:%d: %s", e.Location.File, e.Location.Line, e.Message)
	}
	return e.Message
}

// BindingError describes a problem discovered before execution: duplicate
// step patterns, or steps that are unbound / ambiguously bound.
type BindingError struct {
	// Conflicts is one entry per offending scenario step.
	Conflicts []BindingConflict
}

// BindingConflict records a single step that cannot be bound uniquely.
type BindingConflict struct {
	Scenario string
	Step     *Step
	Kind     BindingKind
	Matches  []string // patterns involved (for ambiguity)
}

// BindingKind classifies a binding conflict.
type BindingKind string

const (
	// BindingUndefined means no registered pattern matches the step.
	BindingUndefined BindingKind = "undefined"
	// BindingAmbiguous means multiple registered patterns match the step.
	BindingAmbiguous BindingKind = "ambiguous"
)

func (e *BindingError) Error() string {
	if e == nil || len(e.Conflicts) == 0 {
		return "binding error"
	}
	msg := "step binding problems detected before execution:\n"
	for _, c := range e.Conflicts {
		loc := c.Step.Location
		switch c.Kind {
		case BindingUndefined:
			msg += fmt.Sprintf("  - %s:%d: undefined step in scenario %q: %s %s\n",
				loc.File, loc.Line, c.Scenario, c.Step.Keyword, c.Step.Text)
		case BindingAmbiguous:
			msg += fmt.Sprintf("  - %s:%d: ambiguous step in scenario %q: %s %s matches %v\n",
				loc.File, loc.Line, c.Scenario, c.Step.Keyword, c.Step.Text, c.Matches)
		}
	}
	return msg
}

// ErrPending is returned (or thrown) by steps that are deliberately not yet
// implemented. It maps to the pending status, distinct from failure.
var ErrPending = fmt.Errorf("step is pending implementation")

// PendingError carries an optional reason for a pending step.
type PendingError struct {
	Reason string
}

func (e *PendingError) Error() string {
	if e.Reason == "" {
		return ErrPending.Error()
	}
	return "pending: " + e.Reason
}

// Is makes PendingError satisfy errors.Is(err, ErrPending).
func (e *PendingError) Is(target error) bool { return target == ErrPending }

// ErrSkipped signals a manual skip from within a step.
var ErrSkipped = fmt.Errorf("scenario skipped")

// SkipError carries the reason for a manual skip.
type SkipError struct {
	Reason string
}

func (e *SkipError) Error() string {
	if e.Reason == "" {
		return ErrSkipped.Error()
	}
	return "skipped: " + e.Reason
}

// Is makes SkipError satisfy errors.Is(err, ErrSkipped).
func (e *SkipError) Is(target error) bool { return target == ErrSkipped }
