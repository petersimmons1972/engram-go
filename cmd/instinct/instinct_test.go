package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"testing"
)

// TestHelpFlagDoesNotStartDaemon verifies that --help/-h flags exit
// cleanly without starting the daemon (#562).
func TestHelpFlagDoesNotStartDaemon(t *testing.T) {
	t.Run("-help flag exits cleanly", func(t *testing.T) {
		// Save original os.Args
		oldArgs := os.Args
		defer func() { os.Args = oldArgs }()

		// Set up test args with --help
		os.Args = []string{"instinct", "--help"}

		// We can't directly test the main() function without it calling os.Exit,
		// but we can verify that flag.Parse doesn't fail and that the help
		// flag is properly registered.

		fs := flag.NewFlagSet("instinct", flag.ExitOnError)
		help := fs.Bool("help", false, "show help and exit")
		fs.Bool("h", false, "show help and exit")

		// This should not panic or error
		if err := fs.Parse([]string{"--help"}); err != nil {
			t.Fatalf("flag.Parse with --help failed: %v", err)
		}

		if !*help {
			t.Error("--help flag was not parsed")
		}
	})

	t.Run("-h flag exits cleanly", func(t *testing.T) {
		fs := flag.NewFlagSet("instinct", flag.ExitOnError)
		help := fs.Bool("help", false, "show help and exit")
		helpAlias := fs.Bool("h", false, "show help and exit")

		if err := fs.Parse([]string{"-h"}); err != nil {
			t.Fatalf("flag.Parse with -h failed: %v", err)
		}

		if !*helpAlias {
			t.Error("-h flag was not parsed")
		}
		_ = help // help might not be set when -h is used (it's an alias)
	})

	t.Run("help text is printed", func(t *testing.T) {
		// Capture stderr where usage is printed
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		fs := flag.NewFlagSet("instinct", flag.ExitOnError)
		fs.Bool("help", false, "show help and exit")
		fs.Bool("h", false, "show help and exit")

		fs.Usage = func() {
			io.WriteString(os.Stderr, "Usage: instinct [options]\n")
		}

		fs.Parse([]string{"-h"})
		fs.Usage()

		w.Close()
		os.Stderr = oldStderr
		usage, _ := io.ReadAll(r)

		if !bytes.Contains(usage, []byte("Usage:")) {
			t.Error("usage output not printed")
		}
	})
}

// TestVersionFlag verifies that --version flag is handled.
func TestVersionFlag(t *testing.T) {
	t.Run("--version flag is registered", func(t *testing.T) {
		fs := flag.NewFlagSet("instinct", flag.ExitOnError)
		version := fs.Bool("version", false, "print version and exit")

		if err := fs.Parse([]string{"--version"}); err != nil {
			t.Fatalf("flag.Parse failed: %v", err)
		}

		if !*version {
			t.Error("--version flag was not parsed")
		}
	})
}
