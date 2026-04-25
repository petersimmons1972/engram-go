package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const maxIncluded = 25

type Model struct {
	Name      string  `yaml:"name"`
	ParamsB   float64 `yaml:"params_b"`
	VRAMGB    float64 `yaml:"vram_gb"`
	Tier      string  `yaml:"tier"`
	Vendor    string  `yaml:"vendor"`
	Family    string  `yaml:"family"`
	Instruct  bool    `yaml:"instruct"`
	Include   bool    `yaml:"include"`
	Rationale string  `yaml:"rationale"`
	Notes     *string `yaml:"notes"` // nil when not applicable; Rationale is always required
}

type Manifest struct {
	Models []Model `yaml:"models"`
}

func (m *Manifest) Included() []Model {
	var out []Model
	for _, model := range m.Models {
		if model.Include {
			out = append(out, model)
		}
	}
	return out
}

func (m *Manifest) Excluded() []Model {
	var out []Model
	for _, model := range m.Models {
		if !model.Include {
			out = append(out, model)
		}
	}
	return out
}

func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	seen := map[string]bool{}
	includedCount := 0
	for _, model := range m.Models {
		if seen[model.Name] {
			return nil, fmt.Errorf("duplicate model name: %q", model.Name)
		}
		seen[model.Name] = true

		if !model.Instruct && model.Include {
			return nil, fmt.Errorf("model %q: instruct must be true for included models (base models cannot follow structured prompts)", model.Name)
		}
		if model.VRAMGB > 16.0 && model.Include {
			return nil, fmt.Errorf("model %q: vram_gb %.1f exceeds 16GB limit for included models", model.Name, model.VRAMGB)
		}
		if model.Include {
			if model.Rationale == "" {
				return nil, fmt.Errorf("model %q: rationale is required for included models", model.Name)
			}
			includedCount++
		}
	}
	if includedCount > maxIncluded {
		return nil, fmt.Errorf("manifest has %d included models (max %d); add models as include: false or raise the cap", includedCount, maxIncluded)
	}
	return &m, nil
}
