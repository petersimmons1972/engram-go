package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/petersimmons1972/engram/internal/db"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// NewSharedPool runs all pending migrations via runMigrations.
	pool, err := db.NewSharedPool(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect + migrate: %v", err)
	}
	pool.Close()
	log.Println("migration complete")
}
