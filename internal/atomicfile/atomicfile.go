// Package atomicfile replaces files without exposing partial contents.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write replaces path with data after fully writing and syncing a temporary
// file in the same directory. The parent directory must already exist.
func Write(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempPath := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(tempPath)
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := replace(tempPath, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}
