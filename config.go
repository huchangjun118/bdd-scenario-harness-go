package bdd

// Config configures a harness run.
type Config struct {
	// Parallelism is the maximum number of scenarios executed concurrently.
	// Values <= 1 mean sequential execution.
	Parallelism int
	// TagFilter selects scenarios using a tag expression (e.g. "@smoke and not @slow").
	TagFilter string
	// ExcludeTags skips scenarios carrying any of these tags.
	ExcludeTags []string
	// NameFilter selects scenarios whose name matches this regular expression.
	NameFilter string
	// Strict makes pending/undefined steps fail the run with a non-zero exit code.
	Strict bool
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config { return Config{} }
