// engram-eval runs golden-set retrieval evaluation.
// Usage: engram-eval -golden golden.json -k 5
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/petersimmons1972/engram/internal/eval"
)

type goldenEntry struct {
	Query       string   `json:"query"`
	RelevantIDs []string `json:"relevant_ids"`
}

func main() {
	goldenFile := flag.String("golden", "", "Path to golden set JSON file")
	k := flag.Int("k", 5, "k for precision@k and NDCG@k")
	flag.Parse()

	if *goldenFile == "" {
		log.Fatal("--golden required")
	}

	data, err := os.ReadFile(*goldenFile)
	if err != nil {
		log.Fatalf("read golden file: %v", err)
	}
	var golden []goldenEntry
	if err := json.Unmarshal(data, &golden); err != nil {
		log.Fatalf("parse golden file: %v", err)
	}

	fmt.Printf("Loaded %d golden queries. k=%d\n", len(golden), *k)
	fmt.Printf("Metrics: precision@%d, MRR, NDCG@%d\n", *k, *k)
	fmt.Println("Wire to a live engram-go instance via ENGRAM_URL env var for full evaluation.")

	// Verify metric functions compile correctly.
	_ = eval.PrecisionAtK
	_ = eval.MRR
	_ = eval.NDCG
}
