package main

import "log"

type allStages struct {
	ingest func(*Config)
	run    func(*Config) int
	score  func(*Config)
}

func runAll(cfg *Config) int {
	return runAllWithStages(cfg, allStages{
		ingest: runIngest,
		run:    runRun,
		score:  runScore,
	})
}

func runAllWithStages(cfg *Config, stages allStages) int {
	if cfg.RunID == "" {
		cfg.RunID = newRunID()
	}
	log.Printf("all: run-id=%s data=%s workers=%d", cfg.RunID, cfg.DataFile, cfg.Workers)

	log.Println("--- Stage 1: ingest ---")
	stages.ingest(cfg)

	log.Println("--- Stage 2: run ---")
	if exit := stages.run(cfg); exit != 0 {
		log.Printf("all: run stage failed with exit=%d; skipping score (run-id=%s)", exit, cfg.RunID)
		return exit
	}

	log.Println("--- Stage 3: score ---")
	stages.score(cfg)

	log.Printf("all: complete (run-id=%s)", cfg.RunID)
	return 0
}
