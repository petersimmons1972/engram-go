package reporter_test

import (
	"strings"
	"testing"
	"time"

	"github.com/petersimmons1972/engram/internal/reporter"
	"github.com/petersimmons1972/engram/internal/types"
)

func TestRenderSVG_ContainsExpected(t *testing.T) {
	results := []types.ModelResult{
		{Model: "mistral:7b", VRAMGB: 4.5, Tier: "4-6GB", Score: types.Score{
			Verdict: types.VerdictRecommended, Composite: 7.44,
			AvgLatency: types.Duration(13 * time.Second),
		}},
	}
	svg, err := reporter.RenderSVG(results)
	if err != nil {
		t.Fatalf("RenderSVG: %v", err)
	}
	if !strings.Contains(svg, `viewBox="0 0 750 420"`) {
		t.Error("SVG should have correct viewBox")
	}
	if !strings.Contains(svg, "#4FAAFF") {
		t.Error("SVG should use accent colour #4FAAFF")
	}
	if !strings.Contains(svg, "#0D1117") {
		t.Error("SVG should use background colour #0D1117")
	}
}

// TestSVGTemplateEscapesHTML verifies that html/template properly escapes
// potentially dangerous content in model names and other fields. This prevents
// XSS attacks when SVG content is embedded in HTML.
func TestSVGTemplateEscapesHTML(t *testing.T) {
	// Inject an HTML/script payload in the model name to verify escaping
	results := []types.ModelResult{
		{Model: `<script>alert(1)</script>`, VRAMGB: 4.5, Tier: "4-6GB", Score: types.Score{
			Verdict: types.VerdictRecommended, Composite: 7.44,
			AvgLatency: types.Duration(13 * time.Second),
		}},
	}
	svg, err := reporter.RenderSVG(results)
	if err != nil {
		t.Fatalf("RenderSVG: %v", err)
	}
	// The script tag should be HTML-escaped
	if strings.Contains(svg, "<script>") {
		t.Error("SVG should escape <script> to &lt;script&gt;")
	}
	if !strings.Contains(svg, "&lt;script&gt;") {
		t.Error("SVG should contain escaped &lt;script&gt;")
	}
}
