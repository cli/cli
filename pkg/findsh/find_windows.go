package findsh

import (
	"os"
	"path/filepath"

	"github.com/cli/safeexec"
)

func Find() (string, error) {
	shPath, shErr := safeexec.LookPath("sh")
	if shErr == nil {
		return shPath, nil
	}

	gitPath, err := safeexec.LookPath("git")
	if err != nil {
		return "", shErr
	}
	gitDir := filepath.Dir(gitPath)

	// regular Git for Windows install
	shPath = filepath.Join(gitDir, "..", "bin", "sh.exe")
	if _, err := os.Stat(shPath); err == nil {
		return filepath.Clean(shPath), nil
	}

	// git as a scoop shim
	shPath = filepath.Join(gitDir, "..", "apps", "git", "current", "bin", "sh.exe")
	if _, err := os.Stat(shPath); err == nil {
		return filepath.Clean(shPath), nil
	}

	return "", shErr
}
