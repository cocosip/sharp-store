// Package fileutil holds small filesystem helpers shared by the storage
// backends. It must stay free of cloud SDK imports so every backend package
// can depend on it without widening its dependency set.
package fileutil

import (
	"io"
	"os"
	"path/filepath"
)

// WriteFile streams reader into destination, creating parent directories when
// missing. It reports copied=true once the stream has been written, so a Sync
// failure after a complete copy returns (true, err), matching the backend
// Download contract.
func WriteFile(destination string, reader io.Reader) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return false, err
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	if _, err := io.Copy(file, reader); err != nil {
		return false, err
	}
	return true, file.Sync()
}
