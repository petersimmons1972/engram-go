package embed

import (
	"encoding/json"
	"errors"
	"fmt"
)

var ErrPermanent = errors.New("embed: permanent failure")

type PermanentError struct {
	Code        string
	Stored      string
	Current     string
	Remediation string
	Wrapped     error
}

func (e *PermanentError) Error() string {
	return fmt.Sprintf("embed permanent failure: %s (remediation: %s)", e.Code, e.Remediation)
}

func (e *PermanentError) Unwrap() []error {
	result := []error{ErrPermanent}
	if e.Wrapped != nil {
		result = append(result, e.Wrapped)
	}
	return result
}

func (e *PermanentError) MarshalJSON() ([]byte, error) {
	m := map[string]string{
		"code":        e.Code,
		"stored":      e.Stored,
		"current":     e.Current,
		"remediation": e.Remediation,
	}
	return json.Marshal(m)
}
