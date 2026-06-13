package embedmodel

const (
	CanonicalBGEM3  = "BAAI/bge-m3"
	RequiredDims    = 1024
	PGNotifyChannel = "embed_queue"
)

var AcceptedAliases = []string{
	"BAAI/bge-m3",
	"bge-m3",
	"bge-m3-Q8_0.gguf",
	"bge-m3-Q4_K_M.gguf",
	"bge-m3-live",    // olla routing alias: MI-50 burst embedder (gfx906, priority 50)
	"bge-m3-reembed", // olla routing alias: W6800+leviathan bulk embedder (priority 100)
}

var aliasToCanonical = map[string]string{
	"BAAI/bge-m3":        CanonicalBGEM3,
	"bge-m3":             CanonicalBGEM3,
	"bge-m3-Q8_0.gguf":   CanonicalBGEM3,
	"bge-m3-Q4_K_M.gguf": CanonicalBGEM3,
	"bge-m3-live":        CanonicalBGEM3, // olla routing alias: MI-50 burst embedder
	"bge-m3-reembed":     CanonicalBGEM3, // olla routing alias: W6800+leviathan bulk embedder
}

func CanonicalName(modelID string) string {
	return aliasToCanonical[modelID]
}
