package embedgateway

import (
	"fmt"

	"github.com/petersimmons1972/engram/internal/embedmodel"
)

func ValidateEmbedResponse(vec []float32, reportedModelID string) error {
	if embedmodel.CanonicalName(reportedModelID) != embedmodel.CanonicalBGEM3 {
		return fmt.Errorf("embed response rejected: model %q not in bge-m3 alias set", reportedModelID)
	}
	if len(vec) != embedmodel.RequiredDims {
		return fmt.Errorf("embed response rejected: got %d dims, want %d (model: %q)",
			len(vec), embedmodel.RequiredDims, reportedModelID)
	}
	return nil
}

func validateEmbedResponse(vec []float32, reportedModelID string) error {
	return ValidateEmbedResponse(vec, reportedModelID)
}
