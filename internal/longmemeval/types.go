package longmemeval

type Item struct {
	QuestionID         string   `json:"question_id"`
	QuestionType       string   `json:"question_type"`
	Question           string   `json:"question"`
	Answer             any      `json:"answer"`
	QuestionDate       string   `json:"question_date"`
	HaystackSessionIDs []string `json:"haystack_session_ids"`
	HaystackDates      []string `json:"haystack_dates"`
	HaystackSessions   [][]Turn `json:"haystack_sessions"`
	AnswerSessionIDs   []string `json:"answer_session_ids"`
}

type Turn struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	HasAnswer bool   `json:"has_answer,omitempty"`
}

type IngestEntry struct {
	QuestionID   string            `json:"question_id"`
	Project      string            `json:"project"`
	SessionCount int               `json:"session_count"`
	MemoryMap    map[string]string `json:"memory_map"`
	Status       string            `json:"status"`
	Error        string            `json:"error,omitempty"`
}

type RunEntry struct {
	QuestionID   string   `json:"question_id"`
	Hypothesis   string   `json:"hypothesis"`
	RetrievedIDs []string `json:"retrieved_ids"`
	Status       string   `json:"status"`
	Error        string   `json:"error,omitempty"`
}

type ScoreEntry struct {
	QuestionID   string `json:"question_id"`
	QuestionType string `json:"question_type"`
	Hypothesis    string `json:"hypothesis"`
	ScoreLabel    string `json:"score_label"`
	Explanation   string `json:"explanation"`
	Status        string `json:"status"`
	Error         string `json:"error,omitempty"`
}

type HypothesisLine struct {
	QuestionID string `json:"question_id"`
	Hypothesis string `json:"hypothesis"`
}

type RetrievalMetrics struct {
	RecallAll5  float64 `json:"recall_all@5"`
	NDCGAny5    float64 `json:"ndcg_any@5"`
	RecallAll10 float64 `json:"recall_all@10"`
	NDCGAny10   float64 `json:"ndcg_any@10"`
}

type RetrievalLogEntry struct {
	QuestionID string `json:"question_id"`
	RetrievalResults struct {
		Metrics struct {
			Session RetrievalMetrics `json:"session"`
		} `json:"metrics"`
	} `json:"retrieval_results"`
}
