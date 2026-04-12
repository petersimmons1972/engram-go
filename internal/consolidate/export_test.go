package consolidate

// export_test.go exposes unexported pure functions for external test packages.
// These functions require no database — they operate on strings only.

// IsContradiction exposes isContradiction for unit testing.
var IsContradiction = isContradiction
