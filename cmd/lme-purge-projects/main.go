package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/petersimmons1972/engram/internal/longmemeval"
)

func main() {
	listFile := os.Args[1]
	workers := 4
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &workers)
	}

	serverURL := os.Getenv("ENGRAM_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8788"
	}
	apiKey := os.Getenv("ENGRAM_API_KEY")

	f, err := os.Open(listFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open list: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	var projects []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			projects = append(projects, line)
		}
	}
	total := len(projects)
	fmt.Printf("Loaded %d projects, workers=%d\n", total, workers)

	ctx := context.Background()

	// Create worker pool — each worker gets its own MCP connection
	work := make(chan string, workers*2)
	var deleted, failed atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client, err := longmemeval.Connect(ctx, serverURL, apiKey)
			if err != nil {
				fmt.Fprintf(os.Stderr, "worker %d connect: %v\n", workerID, err)
				// drain channel
				for range work {}
				return
			}
			defer client.Close()

			for proj := range work {
				if err := client.DeleteProjectConfirmed(ctx, proj); err != nil {
					fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", proj, err)
					failed.Add(1)
				} else {
					d := deleted.Add(1)
					if d%100 == 0 {
						elapsed := time.Since(start)
						rate := float64(d) / elapsed.Seconds()
						remaining := float64(total) - float64(d)
						fmt.Printf("Progress: %d/%d deleted, %d failed, %.1f/s, ~%.0fs remaining\n",
							d, total, failed.Load(), rate, remaining/rate)
					}
				}
			}
		}(i)
	}

	for _, proj := range projects {
		work <- proj
	}
	close(work)
	wg.Wait()

	fmt.Printf("\nDone: %d deleted, %d failed in %s\n",
		deleted.Load(), failed.Load(), time.Since(start).Round(time.Second))
}
