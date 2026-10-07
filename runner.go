package bdd

import "context"

// Runner executes expanded scenarios.
type Runner struct{}

// NewRunner builds a runner.
func NewRunner(reg *Registry, cfg Config) (*Runner, error) { return &Runner{}, nil }

// Run executes the given scenarios and returns results in declaration order.
func (r *Runner) Run(ctx context.Context, items []*ExpandedScenario, filter *Filter) []*ScenarioResult {
	return nil
}
