//go:build pprof
// +build pprof

// Conditional pprof support: register HTTP handlers only when built with -tags=pprof.
package main

import (
	// Register pprof HTTP handlers at /debug/pprof/ when the pprof build tag is active.
	_ "net/http/pprof"
)
