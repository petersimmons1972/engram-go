package main

import "log"

func runAll(cfg *Config) {
	if cfg.RunID == "" {
		cfg.RunID = newRunID()
	}
	log.Printf("all: run-id=%s data=%s workers=%d", cfg.RunID, cfg.DataFile, cfg.Workers)

	log.Println("--- Stage 1: ingest ---")
	runIngest(cfg)

	log.Println("--- Stage 2: run ---")
	runRun(cfg)

	log.Println("--- Stage 3: score ---")
	runScore(cfg)

	log.Printf("all: complete (run-id=%s)", cfg.RunID)
}
