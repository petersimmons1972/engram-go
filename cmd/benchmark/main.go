package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/petersimmons1972/engram/internal/cache"
	"github.com/petersimmons1972/engram/internal/manifest"
	"github.com/petersimmons1972/engram/internal/ollama"
	"github.com/petersimmons1972/engram/internal/reporter"
	"github.com/petersimmons1972/engram/internal/runner"
	"github.com/petersimmons1972/engram/internal/scorer"
	"github.com/petersimmons1972/engram/internal/types"
	"github.com/petersimmons1972/engram/internal/vram"
)

// Version is injected at build time via -ldflags "-X main.Version=$(git describe --tags --always)"
var Version = "dev"

// vramHeadroom reserves 20% of detected VRAM for the OS, display, and other processes.
const vramHeadroom = 0.8

func main() {
	var (
		manifestPath  = flag.String("manifest", "models/candidates.yaml", "path to candidates.yaml")
		fixturePath   = flag.String("fixture", "testdata/sample.jsonl", "event fixture file")
		resultsPath   = flag.String("results", "docs/benchmark-results.json", "output results JSON")
		docsPath      = flag.String("docs", "docs/models.md", "output documentation markdown")
		svgPath       = flag.String("svg", "docs/assets/svg/model-tiers.svg", "output SVG diagram")
		ollamaURL     = flag.String("ollama-url", "http://localhost:11434", "Ollama base URL")
		singleModel   = flag.String("only-model", "", "run only this model (exact manifest name)")
		numRuns       = flag.Int("runs", 3, "inference runs per model")
		dryRun        = flag.Bool("dry-run", false, "validate manifest only, skip inference")
		force         = flag.Bool("force", false, "bypass result cache")
		useLiveBuffer = flag.Bool("use-live-buffer", false, "use ~/.local/state/instinct/buffer.jsonl instead of fixture")
		quiet         = flag.Bool("quiet", false, "suppress per-model progress output on stderr")
	)
	flag.Parse()

	fmt.Printf("instinct-benchmark %s\n", Version)

	m, err := manifest.Load(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Manifest: %d included, %d excluded\n", len(m.Included()), len(m.Excluded()))

	if *dryRun {
		fmt.Println("Dry run complete — manifest valid.")
		os.Exit(0)
	}

	eventSource := *fixturePath
	if *useLiveBuffer {
		liveBuffer := filepath.Join(os.Getenv("HOME"), ".local/state/instinct/buffer.jsonl")
		if _, err := os.Stat(liveBuffer); err == nil {
			eventSource = liveBuffer
			fmt.Printf("Using live buffer: %s\n", liveBuffer)
		} else {
			fmt.Printf("Live buffer not found, falling back to fixture: %s\n", *fixturePath)
		}
	}

	gpuInfo := vram.Detect()
	fmt.Printf("GPU: %s\n", gpuInfo.Label)
	maxVRAM := gpuInfo.GB * vramHeadroom

	client := ollama.NewClient(*ollamaURL)
	ctx := context.Background()

	ollamaVersion, err := client.Version(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot reach Ollama at %s: %v\n", *ollamaURL, err)
		os.Exit(1)
	}
	fmt.Printf("Ollama: %s\n\n", ollamaVersion)

	fixtureData, err := os.ReadFile(eventSource)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read event source %s: %v\n", eventSource, err)
		os.Exit(1)
	}
	promptHash := sha256.Sum256([]byte(runner.SystemPrompt))
	cacheKeyBase := fmt.Sprintf("%x:%s", promptHash[:8], string(fixtureData))

	resultCache := cache.New(*resultsPath)

	models := m.Included()
	if *singleModel != "" {
		var filtered []manifest.Model
		for _, model := range models {
			if model.Name == *singleModel {
				filtered = append(filtered, model)
			}
		}
		if len(filtered) == 0 {
			fmt.Fprintf(os.Stderr, "error: model %q not found in manifest\n", *singleModel)
			os.Exit(1)
		}
		models = filtered
	}

	var allResults []types.ModelResult
	completedCount := 0

	for _, model := range models {
		if !*quiet {
			fmt.Fprintf(os.Stderr, "  %-35s", model.Name)
		}

		if model.VRAMGB > maxVRAM {
			result := types.ModelResult{
				Model:  model.Name,
				VRAMGB: model.VRAMGB,
				Tier:   model.Tier,
				Vendor: model.Vendor,
				Score: types.Score{
					Verdict:       types.VerdictSkippedVRAM,
					VerdictReason: fmt.Sprintf("requires %.1fGB VRAM, available %.1fGB (with headroom)", model.VRAMGB, maxVRAM),
				},
			}
			if !*quiet {
				fmt.Fprintf(os.Stderr, "SKIP (%.1fGB VRAM > %.1fGB available)\n", model.VRAMGB, maxVRAM)
			}
			allResults = append(allResults, result)
			continue
		}

		cacheKey := cacheKeyFor(cacheKeyBase, ollamaVersion, model.Name)
		if !*force {
			if cached, ok, err := resultCache.Read(model.Name, cacheKey, 24*time.Hour); err == nil && ok {
				if !*quiet {
					fmt.Fprintf(os.Stderr, "cached  verdict=%-15s composite=%.2f\n", cached.Score.Verdict, cached.Score.Composite)
				}
				allResults = append(allResults, cached)
				completedCount++
				continue
			}
		}

		runResult, err := runner.Run(ctx, client, model, eventSource, *numRuns)
		if err != nil {
			if !*quiet {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			}
			allResults = append(allResults, types.ModelResult{
				Model:  model.Name,
				VRAMGB: model.VRAMGB,
				Tier:   model.Tier,
				Vendor: model.Vendor,
				Score: types.Score{
					Verdict:       types.VerdictFailed,
					VerdictReason: fmt.Sprintf("runner error: %v", err),
				},
			})
			continue
		}

		score := scorer.Score(runResult)
		modelResult := types.ModelResult{
			Model:  model.Name,
			VRAMGB: model.VRAMGB,
			Tier:   model.Tier,
			Vendor: model.Vendor,
			Score:  score,
		}

		if !*quiet {
			fmt.Fprintf(os.Stderr, "%-15s patterns=%-3d latency=%s\n",
				score.Verdict, score.ValidPatterns, score.AvgLatency.Std().Round(time.Second))
		}

		if err := resultCache.Write(model.Name, cacheKey, modelResult); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: cache write failed for %s: %v\n", model.Name, err)
		}
		allResults = append(allResults, modelResult)
		completedCount++
	}

	if completedCount == 0 {
		fmt.Fprintln(os.Stderr, "error: no models completed")
		os.Exit(2)
	}

	info := reporter.RunInfo{
		OllamaVersion: ollamaVersion,
		OS:            runtime.GOOS + "/" + runtime.GOARCH,
		GPU:           gpuInfo,
		BinaryVersion: Version,
		Timestamp:     time.Now().UTC(),
	}

	md := reporter.RenderMarkdown(allResults, m, info)
	if err := os.WriteFile(*docsPath, []byte(md), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "writing docs: %v\n", err)
	} else {
		fmt.Printf("\nWrote %s\n", *docsPath)
	}

	svgContent, err := reporter.RenderSVG(allResults)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rendering SVG: %v\n", err)
	} else {
		if err := os.MkdirAll(filepath.Dir(*svgPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "creating SVG dir: %v\n", err)
		} else if err := os.WriteFile(*svgPath, []byte(svgContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "writing SVG: %v\n", err)
		} else {
			fmt.Printf("Wrote %s\n", *svgPath)
		}
	}

	// Write full results JSON
	allData, err := json.MarshalIndent(allResults, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: marshalling results: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*resultsPath, allData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: writing results to %s: %v\n", *resultsPath, err)
		os.Exit(1)
	}

	fmt.Printf("Done. Results cached at %s\n", *resultsPath)
}

func cacheKeyFor(base, ollamaVersion, modelName string) string {
	h := sha256.Sum256([]byte(base + ":" + ollamaVersion + ":" + modelName))
	return fmt.Sprintf("%x", h[:16])
}
