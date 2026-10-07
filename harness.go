package bdd

import "context"

// Harness is the high level facade: parse, expand, validate, run, report.
type Harness struct {
	registry *Registry
	config   Config
}

// New creates a Harness.
func New(cfg Config) *Harness { return &Harness{config: cfg} }

// Registry exposes the step registry for registration.
func (h *Harness) Registry() *Registry { return h.registry }

// RunFiles parses and runs the given feature files.
func (h *Harness) RunFiles(ctx context.Context, files []string) (*Report, error) {
	return &Report{}, nil
}
