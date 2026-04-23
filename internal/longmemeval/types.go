// Package longmemeval implements the LongMemEval benchmark harness for engram-go.
package longmemeval

import (
	"encoding/json"
	"fmt"
)

// flexString unmarshals JSON strings or numbers as a Go string.
// LongMemEval has 32/500 questions with numeric answers (e.g. 120).
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*f = flexString(n.String())
		return nil
	}
	return fmt.Errorf("flexString: cannot unmarshal %s", b)
}

// Item is one entry from the LongMemEval dataset JSON file.
type Item struct {
	QuestionID         string     `json:"question_id"`
	QuestionType       string     `json:"question_type"`
	Question           string     `json:"question"`
	Answer             flexString `json:"answer"`
	QuestionDate       string   `json:"question_date"`
	HaystackSessionIDs []string `json:"haystack_session_ids"`
	HaystackDates      []string `json:"haystack_dates"`
	HaystackSessions   [][]Turn `json:"haystack_sessions"`
	AnswerSessionIDs   []string `json:"answer_session_ids"`
}

// Turn is one exchange within a haystack session.
type Turn struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	HasAnswer bool   `json:"has_answer,omitempty"`
}

// IngestEntry is one line written to checkpoint-ingest.jsonl.
type IngestEntry struct {
	QuestionID   string            `json:"question_id"`
	Project      string            `json:"project"`
	SessionCount int               `json:"session_count"`
	MemoryMap    map[string]string `json:"memory_map"` // memory_id → session_id
	Status       string            `json:"status"`     // "done" | "error"
	Error        string            `json:"error,omitempty"`
}

// RunEntry is one line written to checkpoint-run.jsonl.
type RunEntry struct {
	QuestionID   string   `json:"question_id"`
	Hypothesis   string   `json:"hypothesis"`
	RetrievedIDs []string `json:"retrieved_ids"` // memory IDs in ranked order
	Status       string   `json:"status"`
	Error        string   `json:"error,omitempty"`
}

// ScoreEntry is one line written to checkpoint-score.jsonl.
type ScoreEntry struct {
	QuestionID   string `json:"question_id"`
	QuestionType string `json:"question_type"`
	Hypothesis   string `json:"hypothesis"`
	ScoreLabel   string `json:"score_label"` // CORRECT | PARTIALLY_CORRECT | INCORRECT
	Explanation  string `json:"explanation"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

// HypothesisLine is one line in the LongMemEval-compatible hypotheses.jsonl output.
type HypothesisLine struct {
	QuestionID string `json:"question_id"`
	Hypothesis string `json:"hypothesis"`
}

// RetrievalMetrics holds session-level retrieval metrics for one question.
type RetrievalMetrics struct {
	RecallAll5  float64 `json:"recall_all@5"`
	NDCGAny5    float64 `json:"ndcg_any@5"`
	RecallAll10 float64 `json:"recall_all@10"`
	NDCGAny10   float64 `json:"ndcg_any@10"`
}

// RetrievalLogSession is the intermediate type for JSON marshaling.
type RetrievalLogSession struct {
	Session RetrievalMetrics `json:"session"`
}

// RetrievalLogMetrics wraps the session metrics for JSON marshaling.
type RetrievalLogMetrics struct {
	Metrics RetrievalLogSession `json:"metrics"`
}

// RetrievalLogEntry is one line in retrieval_log.jsonl (LongMemEval-compatible).
type RetrievalLogEntry struct {
	QuestionID       string              `json:"question_id"`
	RetrievalResults RetrievalLogMetrics `json:"retrieval_results"`
}
