package db

import "testing"

// TestEscapeLike verifies that escapeLike correctly escapes all three PostgreSQL
// LIKE metacharacters and handles combined and edge cases.
func TestEscapeLike(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "plain string — no metacharacters",
			input: "auth",
			want:  "auth",
		},
		{
			name:  "percent wildcard",
			input: "test%",
			want:  `test\%`,
		},
		{
			name:  "underscore wildcard",
			input: "t_st",
			want:  `t\_st`,
		},
		{
			name:  "backslash escape character",
			input: `C:\Users`,
			want:  `C:\\Users`,
		},
		{
			name:  "combined metacharacters",
			input: `100% _done_ \win`,
			want:  `100\% \_done\_ \\win`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := escapeLike(tc.input)
			if got != tc.want {
				t.Errorf("escapeLike(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}
