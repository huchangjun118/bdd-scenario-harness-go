package bdd

import "context"

// StepFunc is a registered step handler.
type StepFunc func(ctx *ScenarioContext) error

// Registry holds step definitions and hooks.
type Registry struct{}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Register adds a step definition; pattern is a regex with typed placeholders.
func (r *Registry) Register(pattern string, fn StepFunc) error { return nil }

// MustRegister registers or panics.
func (r *Registry) MustRegister(pattern string, fn StepFunc) {}

// validateBindings checks every expanded scenario for unbound/ambiguous steps.
func validateBindings(reg *Registry, scenarios []*ExpandedScenario) error { return nil }

var _ = context.Background
