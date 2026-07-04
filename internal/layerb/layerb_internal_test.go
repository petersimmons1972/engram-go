package layerb

import "testing"

func TestStem_PreservesTerminalEOnPluralS(t *testing.T) {
	if got := stem("bikes"); got != "bike" {
		t.Fatalf("stem(\"bikes\") = %q, want %q", got, "bike")
	}
}

func TestStem_NormalizesBakeAndBakedToSameRoot(t *testing.T) {
	if got := stem("bake"); got != "bake" {
		t.Fatalf("stem(\"bake\") = %q, want %q", got, "bake")
	}
	if got := stem("baked"); got != "bake" {
		t.Fatalf("stem(\"baked\") = %q, want %q", got, "bake")
	}
}

func TestStemVariants(t *testing.T) {
	cases := map[string]string{
		"wishes":   "wish",
		"boxes":    "box",
		"classes":  "class",
		"bikes":    "bike",
		"bake":     "bake",
		"baked":    "bake",
		"tried":    "try",
		"walking":  "walk",
		"visited":  "visit",
		"cats":     "cat",
		"go":       "go",
		"goes":     "go",
		"does":     "do",
		"heroes":   "hero",
		"tomatoes": "tomato",
		"echoes":   "echo",
	}
	for input, want := range cases {
		if got := stem(input); got != want {
			t.Fatalf("stem(%q) = %q, want %q", input, got, want)
		}
	}
}
