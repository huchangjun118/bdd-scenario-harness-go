package bdd

import "context"

// ScenarioContext is passed to every step handler. State stored on it is
// isolated per scenario and never shared across concurrent scenarios.
type ScenarioContext struct {
	ctx  context.Context
	bag  map[string]any
}

// NewScenarioContext builds an isolated context for one scenario.
func NewScenarioContext(parent context.Context) *ScenarioContext { return nil }

// Context returns the underlying Go context (cancellation aware).
func (c *ScenarioContext) Context() context.Context { return nil }

// Set stores a scenario-local value.
func (c *ScenarioContext) Set(key string, value any) {}

// Get retrieves a scenario-local value.
func (c *ScenarioContext) Get(key string) (any, bool) { return nil, false }

// Pending marks the current step as not implemented.
func (c *ScenarioContext) Pending(reason string) error { return nil }

// Skip skips the current scenario with a manual reason.
func (c *ScenarioContext) Skip(reason string) {}
