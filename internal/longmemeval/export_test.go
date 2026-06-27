// export_test.go exposes internal symbols to the longmemeval_test package.
// This file is only compiled during testing.
package longmemeval

// GenerateParaphrasesWith is the testable variant of GenerateParaphrases that
// accepts an injected TextGenerator instead of hard-coding GenerateHaiku.
// Used by tests to inject a fake LLM without spawning a real claude process.
var GenerateParaphrasesWith = generateParaphrasesWith
