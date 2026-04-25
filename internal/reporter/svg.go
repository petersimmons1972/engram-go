package reporter

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/petersimmons1972/engram/internal/types"
)

// Visual identity: instinct — Cassandre geometric streamline moderne
// Palette: #0D1117 bg, #4FAAFF accent, #E6EDF3 text, #1E3A5F structure
// Canvas: 750x420

const svgTmpl = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 750 420">
  <rect width="750" height="420" fill="#0D1117"/>
  <text x="375" y="36" font-family="'Helvetica Neue',Arial,sans-serif" font-size="13"
    fill="#4FAAFF" text-anchor="middle" letter-spacing="0.15em">MODEL COMPATIBILITY</text>
{{- range $i, $tier := .Tiers}}
  <rect x="40" y="{{$tier.Y}}" width="670" height="60" fill="#1E3A5F" opacity="0.3"/>
  <text x="50" y="{{add $tier.Y 20}}" font-family="'Helvetica Neue',Arial,sans-serif"
    font-size="10" fill="#E6EDF3" letter-spacing="0.1em">{{$tier.Label}}</text>
{{- range $j, $model := $tier.Models}}
  <line x1="{{$model.X}}" y1="{{add $tier.Y 30}}" x2="375" y2="380"
    stroke="{{$model.Color}}" stroke-width="1" opacity="{{$model.Opacity}}"/>
  <circle cx="{{$model.X}}" cy="{{add $tier.Y 30}}" r="3" fill="{{$model.Color}}" opacity="{{$model.Opacity}}"/>
  <text x="{{$model.X}}" y="{{add $tier.Y 50}}" font-family="'Helvetica Neue',Arial,sans-serif"
    font-size="8" fill="#E6EDF3" text-anchor="middle" opacity="{{$model.Opacity}}">{{$model.ShortName}}</text>
{{- end}}
{{- end}}
  <polygon points="375,370 365,388 385,388" fill="#4FAAFF" opacity="0.9"/>
  <text x="375" y="410" font-family="'Helvetica Neue',Arial,sans-serif"
    font-size="9" fill="#4FAAFF" text-anchor="middle" letter-spacing="0.1em">INSTINCT</text>
</svg>`

type svgModel struct {
	Name      string
	ShortName string
	X         int
	Color     string
	Opacity   string
}

type svgTier struct {
	Label  string
	Y      int
	Models []svgModel
}

type svgData struct {
	Tiers []svgTier
}

// RenderSVG produces the docs/assets/svg/model-tiers.svg content from scored
// results. Models are bucketed by tier into horizontal rows; recommended models
// use the accent colour at full opacity, others are dimmed.
func RenderSVG(results []types.ModelResult) (string, error) {
	tiers := []struct {
		label string
		y     int
	}{
		{"≤2GB", 60},
		{"4-6GB", 140},
		{"7-10GB", 220},
		{"11-16GB", 300},
	}

	var data svgData
	for _, tier := range tiers {
		st := svgTier{Label: tier.label, Y: tier.y}
		var models []types.ModelResult
		for _, r := range results {
			if r.Tier == tier.label {
				models = append(models, r)
			}
		}
		if len(models) == 0 {
			data.Tiers = append(data.Tiers, st)
			continue
		}
		spacing := 670 / (len(models) + 1)
		for j, m := range models {
			color := "#4FAAFF"
			if m.Score.Verdict != types.VerdictRecommended {
				color = "#1E3A5F"
			}
			opacity := fmt.Sprintf("%.1f", 0.3+(m.Score.Composite/10.0))
			if m.Score.Verdict == types.VerdictRecommended {
				opacity = "0.9"
			}
			short := m.Model
			if idx := strings.Index(short, ":"); idx > 0 {
				short = short[:idx]
			}
			st.Models = append(st.Models, svgModel{
				Name:      m.Model,
				ShortName: short,
				X:         40 + spacing*(j+1),
				Color:     color,
				Opacity:   opacity,
			})
		}
		data.Tiers = append(data.Tiers, st)
	}

	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}
	tmpl := template.Must(template.New("svg").Funcs(funcMap).Parse(svgTmpl))
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("svg render: %w", err)
	}
	return b.String(), nil
}
