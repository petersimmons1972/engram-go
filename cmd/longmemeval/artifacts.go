package main

import (
	"fmt"
	"os"
)

const (
	privateArtifactFileMode os.FileMode = 0o600
	privateArtifactDirMode  os.FileMode = 0o700
)

func createPrivateArtifact(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, privateArtifactFileMode)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(privateArtifactFileMode); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("chmod %s: %w", path, err)
	}
	return f, nil
}
