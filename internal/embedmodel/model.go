package embedmodel

import "log/slog"

const (
	CanonicalBGEM3  = "BAAI/bge-m3"
	RequiredDims    = 1024
	PGNotifyChannel = "embed_queue"
	LiveAlias       = "bge-m3-live"
	ReembedAlias    = "bge-m3-reembed"
)

var AcceptedAliases = []string{
	"BAAI/bge-m3",
	"bge-m3",
	"bge-m3-Q8_0.gguf",
	"bge-m3-Q4_K_M.gguf",
	LiveAlias,    // olla routing alias: MI-50 burst embedder (gfx906, priority 50)
	ReembedAlias, // olla routing alias: W6800+leviathan bulk embedder (priority 100)
}

var aliasToCanonical = map[string]string{
	"BAAI/bge-m3":        CanonicalBGEM3,
	"bge-m3":             CanonicalBGEM3,
	"bge-m3-Q8_0.gguf":   CanonicalBGEM3,
	"bge-m3-Q4_K_M.gguf": CanonicalBGEM3,
	LiveAlias:           CanonicalBGEM3, // olla routing alias: MI-50 burst embedder
	ReembedAlias:        CanonicalBGEM3, // olla routing alias: W6800+leviathan bulk embedder
}

func IsLiveAlias(modelID string) bool {
	return modelID == LiveAlias
}

func IsReembedAlias(modelID string) bool {
	return modelID == ReembedAlias
}

// CanonicalName resolves a model alias to its canonical identifier.
// Returns the canonical name, or "" if the alias is not recognised.
// An unrecognised alias is logged at WARN level to surface audit-trail gaps
// when a model is renamed or a new alias is introduced without being registered.
func CanonicalName(modelID string) string {
	c, ok := aliasToCanonical[modelID]
	if !ok {
		slog.Warn("embedmodel: unrecognised model alias", "model_id", modelID)
		return ""
	}
	return c
}
