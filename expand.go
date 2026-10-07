package bdd

// ExpandedScenario is one scenario plus the background it must run.
type ExpandedScenario struct {
	Scenario   *Scenario
	Background *Background
}

// ExpandFeature expands all outlines into concrete scenarios, preserving
// declaration order (plain scenarios and expanded examples interleaved).
func ExpandFeature(f *Feature) ([]*ExpandedScenario, error) {
	return nil, nil
}
