package main

import "log"

func runAll(cfg *Config) int {
	if cfg.RunID == "" {
		cfg.RunID = newRunID()
	}
	log.Printf("all: run-id=%s data=%s workers=%d", cfg.RunID, cfg.DataFile, cfg.Workers)

	log.Println("--- Stage 1: ingest ---")
	runIngestFn(cfg)

	log.Println("--- Stage 2: run ---")
	if exit := runRunFn(cfg); exit != 0 {
		log.Printf("all: run stage failed (run-id=%s exit=%d)", cfg.RunID, exit)
		return exit
	}

	log.Println("--- Stage 3: score ---")
	runScoreFn(cfg)

	log.Printf("all: complete (run-id=%s)", cfg.RunID)
	return 0
}
