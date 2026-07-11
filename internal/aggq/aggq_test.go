package aggq

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCompositionVectors(t *testing.T) {
	t.Parallel()

	// This frozen corpus makes classifier changes deliberate: update the vectors
	// in both fleet repositories when intentionally changing behavior.
	type vector struct {
		QuestionID string `json:"question_id"`
		Question   string `json:"question"`
		Expected   bool   `json:"expected"`
	}

	data, err := os.ReadFile("testdata/composition_vectors.json")
	if err != nil {
		t.Fatal(err)
	}

	vectors := []vector{}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 500 {
		t.Fatalf("loaded %d composition vectors, want 500", len(vectors))
	}

	for _, vector := range vectors {
		vector := vector
		t.Run(vector.QuestionID, func(t *testing.T) {
			t.Parallel()
			if got := IsMultiFactComposition(vector.Question); got != vector.Expected {
				t.Errorf("IsMultiFactComposition(%q) = %t, want %t", vector.Question, got, vector.Expected)
			}
		})
	}
}
