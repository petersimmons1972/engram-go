package search

// EmbedRecallClock exposes the internal recall clock only to external tests.
type EmbedRecallClock = embedRecallClock

// SetEmbedRecallClock injects a deterministic clock for timeout tests.
func (e *SearchEngine) SetEmbedRecallClock(clock EmbedRecallClock) {
	if clock == nil {
		panic("search: nil embed recall clock")
	}
	e.embedRecallClock = clock
}
