package vram

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type Info struct {
	GB     float64
	Source string // "nvidia" | "amd" | "apple" | "fallback"
	Label  string // human description e.g. "NVIDIA RTX 3090 (24GB)"
}

func Fallback() Info {
	return Info{GB: 8.0, Source: "fallback", Label: "unknown GPU (assumed 8GB)"}
}

// Detect probes GPU VRAM. Never returns an error — falls back to 8GB.
func Detect() Info {
	if info, ok := probeNvidia(); ok {
		return info
	}
	if info, ok := probeAMD(); ok {
		return info
	}
	if runtime.GOOS == "darwin" {
		if info, ok := probeApple(); ok {
			return info
		}
	}
	return Fallback()
}

func probeNvidia() (Info, bool) {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=name,memory.total",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return Info{}, false
	}
	line := strings.TrimSpace(string(out))
	parts := strings.Split(line, ", ")
	if len(parts) < 2 {
		return Info{}, false
	}
	mb, err := strconv.ParseFloat(strings.TrimSpace(parts[len(parts)-1]), 64)
	if err != nil {
		return Info{}, false
	}
	name := strings.Join(parts[:len(parts)-1], ", ")
	gb := mb / 1024.0
	return Info{
		GB:     gb,
		Source: "nvidia",
		Label:  fmt.Sprintf("%s (%.0fGB)", name, gb),
	}, true
}

// ParseNvidiaMiB parses "8192 MiB\n" → 8.0, true. Exported for testing.
func ParseNvidiaMiB(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, " MiB")
	mb, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return mb / 1024.0, true
}

func probeAMD() (Info, bool) {
	out, err := exec.Command("rocm-smi", "--showmeminfo", "vram", "--json").Output()
	if err != nil {
		return Info{}, false
	}
	// rocm-smi JSON: {"card0": {"VRAM Total Memory (B)": "17163091968", ...}}
	_, after, ok := strings.Cut(string(out), `"VRAM Total Memory (B)": "`)
	if !ok {
		return Info{}, false
	}
	value, _, ok := strings.Cut(after, `"`)
	if !ok {
		return Info{}, false
	}
	totalBytes, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return Info{}, false
	}
	gb := totalBytes / (1024 * 1024 * 1024)
	return Info{GB: gb, Source: "amd", Label: fmt.Sprintf("AMD GPU (%.0fGB)", gb)}, true
}

func probeApple() (Info, bool) {
	// Unified memory: use 50% of total RAM as safe VRAM estimate.
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return Info{}, false
	}
	totalBytes, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return Info{}, false
	}
	gb := (totalBytes / 2) / (1024 * 1024 * 1024)
	return Info{
		GB:     gb,
		Source: "apple",
		Label:  fmt.Sprintf("Apple Silicon unified memory (%.0fGB allocated)", gb),
	}, true
}
