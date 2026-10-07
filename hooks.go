package bdd

// HookFunc is a scenario or step hook.
type HookFunc func(ctx *ScenarioContext) error

// hooks holds ordered hook lists.
type hooks struct{}
