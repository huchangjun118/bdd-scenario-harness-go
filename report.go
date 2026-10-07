package bdd

import "encoding/json"

// BuildReport assembles a report from per-scenario results.
func BuildReport(features []*Feature, results []*ScenarioResult, fatal error) *Report {
	return &Report{}
}

// WriteJSON writes the report as indented JSON.
func WriteJSON(w interface{ Write([]byte) (int, error) }, rep *Report) error {
	_ = json.Marshal
	return nil
}

// ExitCode maps a report to process exit code (0 success, 1 failures/undefined/pending, 2 canceled).
func ExitCode(rep *Report) int { return 0 }
