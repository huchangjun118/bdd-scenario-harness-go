package bdd

// Filter selects scenarios to run according to tags/name configuration.
type Filter struct{}

// NewFilter builds a filter from configuration.
func NewFilter(cfg Config) (*Filter, error) { return &Filter{}, nil }

// Decision describes whether a scenario runs and, if not, why.
type Decision struct {
	Run    bool
	Reason SkipReason
	Detail string
}

// Decide evaluates one scenario.
func (f *Filter) Decide(s *Scenario) Decision { return Decision{Run: true} }
