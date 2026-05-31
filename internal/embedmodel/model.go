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
}

var aliasToCanonical = map[string]string{
	"BAAI/bge-m3":        CanonicalBGEM3,
	"bge-m3":             CanonicalBGEM3,
	"bge-m3-Q8_0.gguf":   CanonicalBGEM3,
	"bge-m3-Q4_K_M.gguf": CanonicalBGEM3,
}

func CanonicalName(modelID string) string {
	return aliasToCanonical[modelID]
}
