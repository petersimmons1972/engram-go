package entity

// Entity is a canonical named concept with zero or more aliases.
type Entity struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Project string   `json:"project"`
}

// Relation represents a directed relationship extracted between two entities.
type Relation struct {
	SourceName string  `json:"source_name"`
	TargetName string  `json:"target_name"`
	RelType    string  `json:"rel_type"`
	Strength   float64 `json:"strength"`
}
