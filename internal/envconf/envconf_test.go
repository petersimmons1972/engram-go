package envconf_test

import (
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/envconf"
)

func TestFloat_Unset(t *testing.T) {
	t.Setenv("ENGRAM_TEST_FLOAT", "")
	if got := envconf.Float("ENGRAM_TEST_FLOAT", 1.5); got != 1.5 {
		t.Fatalf("unset: want 1.5, got %v", got)
	}
}

func TestFloat_Valid(t *testing.T) {
	t.Setenv("ENGRAM_TEST_FLOAT", "3.14")
	if got := envconf.Float("ENGRAM_TEST_FLOAT", 1.0); got != 3.14 {
		t.Fatalf("valid: want 3.14, got %v", got)
	}
}

func TestFloat_Malformed(t *testing.T) {
	t.Setenv("ENGRAM_TEST_FLOAT", "not-a-number")
	if got := envconf.Float("ENGRAM_TEST_FLOAT", 2.0); got != 2.0 {
		t.Fatalf("malformed: want default 2.0, got %v", got)
	}
}

func TestFloatBounded_InRange(t *testing.T) {
	t.Setenv("ENGRAM_TEST_BOUNDED", "0.5")
	if got := envconf.FloatBounded("ENGRAM_TEST_BOUNDED", 0.75, 0.0, 1.0); got != 0.5 {
		t.Fatalf("in-range: want 0.5, got %v", got)
	}
}

func TestFloatBounded_BelowMin(t *testing.T) {
	t.Setenv("ENGRAM_TEST_BOUNDED", "-0.1")
	if got := envconf.FloatBounded("ENGRAM_TEST_BOUNDED", 0.75, 0.0, 1.0); got != 0.75 {
		t.Fatalf("below-min: want default 0.75, got %v", got)
	}
}

func TestFloatBounded_AboveMax(t *testing.T) {
	t.Setenv("ENGRAM_TEST_BOUNDED", "1.5")
	if got := envconf.FloatBounded("ENGRAM_TEST_BOUNDED", 0.75, 0.0, 1.0); got != 0.75 {
		t.Fatalf("above-max: want default 0.75, got %v", got)
	}
}

func TestFloatBounded_Unset(t *testing.T) {
	t.Setenv("ENGRAM_TEST_BOUNDED", "")
	if got := envconf.FloatBounded("ENGRAM_TEST_BOUNDED", 0.75, 0.0, 1.0); got != 0.75 {
		t.Fatalf("unset: want default 0.75, got %v", got)
	}
}

func TestDurationHours_Valid(t *testing.T) {
	t.Setenv("ENGRAM_TEST_HOURS", "2.5")
	want := time.Duration(float64(time.Hour) * 2.5)
	if got := envconf.DurationHours("ENGRAM_TEST_HOURS", time.Hour); got != want {
		t.Fatalf("valid: want %v, got %v", want, got)
	}
}

func TestDurationHours_Unset(t *testing.T) {
	t.Setenv("ENGRAM_TEST_HOURS", "")
	def := 8 * time.Hour
	if got := envconf.DurationHours("ENGRAM_TEST_HOURS", def); got != def {
		t.Fatalf("unset: want %v, got %v", def, got)
	}
}

func TestDurationHours_NonPositive(t *testing.T) {
	t.Setenv("ENGRAM_TEST_HOURS", "-1")
	def := 8 * time.Hour
	if got := envconf.DurationHours("ENGRAM_TEST_HOURS", def); got != def {
		t.Fatalf("non-positive: want default %v, got %v", def, got)
	}
}

func TestDurationHours_Malformed(t *testing.T) {
	t.Setenv("ENGRAM_TEST_HOURS", "bad")
	def := 8 * time.Hour
	if got := envconf.DurationHours("ENGRAM_TEST_HOURS", def); got != def {
		t.Fatalf("malformed: want default %v, got %v", def, got)
	}
}

func TestInt_Unset(t *testing.T) {
	t.Setenv("ENGRAM_TEST_INT", "")
	if got := envconf.Int("ENGRAM_TEST_INT", 42); got != 42 {
		t.Fatalf("unset: want 42, got %v", got)
	}
}

func TestInt_Valid(t *testing.T) {
	t.Setenv("ENGRAM_TEST_INT", "7")
	if got := envconf.Int("ENGRAM_TEST_INT", 42); got != 7 {
		t.Fatalf("valid: want 7, got %v", got)
	}
}

func TestInt_Malformed(t *testing.T) {
	t.Setenv("ENGRAM_TEST_INT", "not-a-number")
	if got := envconf.Int("ENGRAM_TEST_INT", 99); got != 99 {
		t.Fatalf("malformed: want default 99, got %v", got)
	}
}

func TestString_Unset(t *testing.T) {
	t.Setenv("ENGRAM_TEST_STRING", "")
	if got := envconf.String("ENGRAM_TEST_STRING", "default"); got != "default" {
		t.Fatalf("unset: want 'default', got %q", got)
	}
}

func TestString_Set(t *testing.T) {
	t.Setenv("ENGRAM_TEST_STRING", "hello")
	if got := envconf.String("ENGRAM_TEST_STRING", "default"); got != "hello" {
		t.Fatalf("set: want 'hello', got %q", got)
	}
}
