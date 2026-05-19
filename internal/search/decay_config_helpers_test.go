package search

// Test-only helpers for resetting sync.Once-guarded configuration between test
// cases. These functions must never be called from production code.

import "sync"

// resetDecayConfigForTesting resets the sync.Once for score.go's decay
// configuration so the next call to decayCfg() re-reads the environment.
//
// Test-only: not for production use.
func resetDecayConfigForTesting() {
	decayConfigOnce = sync.Once{}
	resolvedDecay = decayConfig{}
}

// resetDecayIntervalConfigForTesting resets the sync.Once for decay.go's
// interval configuration so the next call to resolveDecayInterval() re-reads
// the environment.
//
// Test-only: not for production use.
func resetDecayIntervalConfigForTesting() {
	decayIntervalOnce = sync.Once{}
	resolvedDecayInterval = 0
}

// resetWeightConfigForTesting resets the sync.Once for score.go's weight
// configuration so the next call to DefaultWeights() re-reads the environment.
//
// Test-only: not for production use.
func resetWeightConfigForTesting() {
	weightConfigOnce = sync.Once{}
	resolvedWeights = weightConfig{}
}
