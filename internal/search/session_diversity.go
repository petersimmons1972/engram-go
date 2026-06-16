package search

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/petersimmons1972/engram/internal/types"
)

const sessionDiversityEnvVar = "ENGRAM_SESSION_DIVERSITY_N"

// SessionDiversityNFromEnv returns the per-session cap for the session-diversity
// post-pass. Unset, invalid, or negative values disable the pass and return 0.
func SessionDiversityNFromEnv() int {
	raw := strings.TrimSpace(os.Getenv(sessionDiversityEnvVar))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// shouldApplySessionDiversity reports whether the post-pass can change the
// returned topK window in a meaningful way.
func shouldApplySessionDiversity(results []types.SearchResult, topK, n int) bool {
	if n <= 0 || topK <= 0 || len(results) <= 1 || len(results) <= topK || n >= topK {
		return false
	}

	distinct := make(map[string]struct{}, 2)
	for i, r := range results {
		distinct[sessionDiversityKey(r, i)] = struct{}{}
		if len(distinct) > 1 {
			return true
		}
	}
	return false
}

// applySessionDiversity limits a dominant session from monopolising the topK
// window. It preserves session encounter order from the baseline ranking and
// preserves within-session order from that same ranking.
func applySessionDiversity(results []types.SearchResult, topK, n int) []types.SearchResult {
	if !shouldApplySessionDiversity(results, topK, n) {
		return results
	}

	type sessionBucket struct {
		key     string
		members []types.SearchResult
	}

	bucketIndex := make(map[string]int, len(results))
	buckets := make([]sessionBucket, 0, len(results))
	for i, r := range results {
		key := sessionDiversityKey(r, i)
		idx, ok := bucketIndex[key]
		if !ok {
			idx = len(buckets)
			bucketIndex[key] = idx
			buckets = append(buckets, sessionBucket{key: key})
		}
		buckets[idx].members = append(buckets[idx].members, r)
	}

	out := make([]types.SearchResult, 0, len(results))
	offsets := make([]int, len(buckets))
	for {
		progressed := false
		for i := range buckets {
			taken := 0
			for offsets[i] < len(buckets[i].members) && taken < n {
				out = append(out, buckets[i].members[offsets[i]])
				offsets[i]++
				taken++
				progressed = true
			}
		}
		if !progressed {
			break
		}
	}
	return out
}

func sessionDiversityKey(result types.SearchResult, index int) string {
	if result.Memory != nil {
		if sid := extractSessionID(result.Memory.Tags); sid != "" {
			return sid
		}
		if result.Memory.ID != "" {
			return "\x00session-diversity:" + result.Memory.ID
		}
	}
	return fmt.Sprintf("\x00session-diversity:nil:%d", index)
}
