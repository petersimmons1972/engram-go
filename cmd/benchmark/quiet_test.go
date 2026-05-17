package main

import (
	"bytes"
	"io"
	"testing"
)

// TestBannerWriter — #661: when --quiet is true, banner writes go to io.Discard
// (suppressing the stdout banner that previously made `instinct-benchmark
// --quiet | jq ...` impossible). When --quiet is false, writes pass through
// to the supplied default writer.
func TestBannerWriter(t *testing.T) {
	t.Run("quiet=true discards", func(t *testing.T) {
		var captured bytes.Buffer
		w := bannerWriter(true, &captured)
		n, err := io.WriteString(w, "instinct-benchmark dev\n")
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		if n != len("instinct-benchmark dev\n") {
			t.Errorf("write should report bytes written even when discarding, got %d", n)
		}
		if captured.Len() != 0 {
			t.Errorf("quiet=true should not write to captured buffer, got %q", captured.String())
		}
	})

	t.Run("quiet=false passes through", func(t *testing.T) {
		var captured bytes.Buffer
		w := bannerWriter(false, &captured)
		const msg = "instinct-benchmark dev\n"
		if _, err := io.WriteString(w, msg); err != nil {
			t.Fatalf("write: %v", err)
		}
		if got := captured.String(); got != msg {
			t.Errorf("quiet=false should pass through, got %q want %q", got, msg)
		}
	})
}
