package longmemeval

import (
	"reflect"
	"testing"
)

func TestSubjectNPQuery_StripsPreferenceFraming(t *testing.T) {
	cases := []struct {
		question string
		want     string
	}{
		{
			question: "Can you recommend a hotel for my upcoming trip to Miami?",
			want:     "a hotel for my upcoming trip to Miami",
		},
		{
			question: "What do you think about electric bikes?",
			want:     "electric bikes",
		},
		{
			question: "Do you prefer tea or coffee?",
			want:     "tea or coffee",
		},
	}

	for _, tc := range cases {
		if got := SubjectNPQuery(tc.question); got != tc.want {
			t.Fatalf("SubjectNPQuery(%q) = %q, want %q", tc.question, got, tc.want)
		}
	}
}

func TestDualPreferenceRecall_DisabledIsBaseline(t *testing.T) {
	question := "Can you recommend a hotel for my upcoming trip to Miami?"
	baselineQuery := PreferenceRecallQuery(question)
	want := []RecallResult{
		{ID: "m1", Score: 0.91},
		{ID: "m2", Score: 0.52},
	}

	var calls []string
	got, err := RecallForQuestion(question, baselineQuery, RunOpts{}, func(query string) ([]RecallResult, error) {
		calls = append(calls, query)
		return want, nil
	})
	if err != nil {
		t.Fatalf("RecallForQuestion: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{baselineQuery}) {
		t.Fatalf("recall calls = %v, want [%q]", calls, baselineQuery)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("results = %#v, want %#v", got, want)
	}
}

func TestDualPreferenceRecall_GateSkipsNonPreference(t *testing.T) {
	question := "When did I visit Paris?"
	baselineQuery := question

	var calls []string
	_, err := RecallForQuestion(question, baselineQuery, RunOpts{DualPreferenceRecall: true}, func(query string) ([]RecallResult, error) {
		calls = append(calls, query)
		return []RecallResult{{ID: "m1", Score: 0.5}}, nil
	})
	if err != nil {
		t.Fatalf("RecallForQuestion: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{baselineQuery}) {
		t.Fatalf("recall calls = %v, want [%q]", calls, baselineQuery)
	}
}

func TestDualPreferenceRecall_UnionDedup(t *testing.T) {
	question := "Can you recommend a hotel for my upcoming trip to Miami?"
	baselineQuery := PreferenceRecallQuery(question)

	var calls []string
	got, err := RecallForQuestion(question, baselineQuery, RunOpts{DualPreferenceRecall: true}, func(query string) ([]RecallResult, error) {
		calls = append(calls, query)
		switch query {
		case SubjectNPQuery(question):
			return []RecallResult{
				{ID: "subject-only", Score: 0.70},
				{ID: "both", Score: 0.40},
			}, nil
		case baselineQuery:
			return []RecallResult{
				{ID: "generic-only", Score: 0.80},
				{ID: "both", Score: 0.90},
			}, nil
		default:
			t.Fatalf("unexpected query %q", query)
			return nil, nil
		}
	})
	if err != nil {
		t.Fatalf("RecallForQuestion: %v", err)
	}

	wantCalls := []string{SubjectNPQuery(question), baselineQuery}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("recall calls = %v, want %v", calls, wantCalls)
	}

	want := []RecallResult{
		{ID: "both", Score: 0.90},
		{ID: "generic-only", Score: 0.80},
		{ID: "subject-only", Score: 0.70},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("results = %#v, want %#v", got, want)
	}
}

func TestDualPreferenceRecall_SmokeFull3Items(t *testing.T) {
	questions := []string{
		"Can you recommend a hotel for my upcoming trip to Miami?",
		"What do you think about electric bikes?",
		"Do you prefer tea or coffee?",
	}

	for _, question := range questions {
		baselineQuery := PreferenceRecallQuery(question)
		got, err := RecallForQuestion(question, baselineQuery, RunOpts{DualPreferenceRecall: true}, func(query string) ([]RecallResult, error) {
			switch query {
			case SubjectNPQuery(question):
				return []RecallResult{{ID: question + "-subject", Score: 0.7}}, nil
			case baselineQuery:
				return []RecallResult{{ID: question + "-generic", Score: 0.8}}, nil
			default:
				t.Fatalf("unexpected query %q", query)
				return nil, nil
			}
		})
		if err != nil {
			t.Fatalf("RecallForQuestion(%q): %v", question, err)
		}
		if len(got) != 2 {
			t.Fatalf("RecallForQuestion(%q) returned %d results, want 2", question, len(got))
		}
	}
}
