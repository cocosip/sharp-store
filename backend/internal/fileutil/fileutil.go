// Package fileutil holds small filesystem helpers shared by the storage
// backends. It must stay free of cloud SDK imports so every backend package
// can depend on it without widening its dependency set.
package fileutil

import (
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
)

// WriteFile streams reader into a temporary file in the destination directory
// and renames it into place, creating parent directories when missing. A
// failed write leaves an existing destination untouched. It reports copied=true
// once the stream has been written, so a Sync or rename failure after a
// complete copy returns (true, err), matching the backend Download contract.
func WriteFile(destination string, reader io.Reader) (bool, error) {
	directory := filepath.Dir(destination)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return false, err
	}
	temporary := filepath.Join(directory, ".sharp-store-"+rand.Text())
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(temporary)
	}()
	if _, err := io.Copy(file, reader); err != nil {
		return false, err
	}
	if err := file.Sync(); err != nil {
		return true, err
	}
	if err := file.Close(); err != nil {
		return true, err
	}
	if err := os.Rename(temporary, destination); err != nil {
		return true, err
	}
	return true, nil
}
