package sshkey

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/ssh"
)

func Test_GenerateAutomaticSSHKeys(t *testing.T) {
	tests := []struct {
		name string
		// These files exist when calling generate
		existing []string
		// These files should exist after generate finishes
		wanted []string
	}{
		{
			name:     "creates new keys when they don't exist",
			existing: nil,
			wanted:   []string{automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
		{
			name:     "doesn't create new keys when they already exist",
			existing: []string{automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
			wanted:   []string{automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
		{
			name:     "renames old keys to new names",
			existing: []string{automaticPrivateKeyNameOld, automaticPrivateKeyNameOld + ".pub"},
			wanted:   []string{automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
		{
			name:     "creates new keys when old private key exists but not the public key",
			existing: []string{automaticPrivateKeyNameOld},
			wanted:   []string{automaticPrivateKeyNameOld, automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
		{
			name:     "creates new keys when old public key exists but not the private key",
			existing: []string{automaticPrivateKeyNameOld + ".pub"},
			wanted:   []string{automaticPrivateKeyNameOld + ".pub", automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
		{
			name:     "creates new keys when files exist which contain old key name as a substring",
			existing: []string{"foo" + automaticPrivateKeyNameOld + ".pub", "foo" + automaticPrivateKeyNameOld},
			wanted:   []string{"foo" + automaticPrivateKeyNameOld + ".pub", "foo" + automaticPrivateKeyNameOld, automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			sshContext := ssh.NewContextForTests(dir, "")
			auto := autoKeys{ctx: sshContext}

			for _, file := range tt.existing {
				f, err := os.Create(filepath.Join(dir, file))
				if err != nil {
					t.Fatalf("Failed to setup test files: %v", err)
				}
				// If the file isn't closed here windows will have errors about file already in use
				f.Close()
			}

			keyPair, err := auto.generate()
			if err != nil {
				t.Fatalf("Unexpected error from generate: %v", err)
			}
			if keyPair == nil {
				t.Fatal("Unexpected nil KeyPair from generate")
			}
			if !strings.HasSuffix(keyPair.PrivateKeyPath, automaticPrivateKeyName) {
				t.Fatalf("Expected private key path %v, got %v", automaticPrivateKeyName, keyPair.PrivateKeyPath)
			}
			if !strings.HasSuffix(keyPair.PublicKeyPath, automaticPrivateKeyName+".pub") {
				t.Fatalf("Expected public key path %v, got %v", automaticPrivateKeyName+".pub", keyPair.PublicKeyPath)
			}

			// Check that all the expected files are present
			for _, file := range tt.wanted {
				if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
					t.Fatalf("Want file %q to exist after generate but it doesn't", file)
				}
			}

			// Check that no unexpected files are present
			allExistingFiles, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("Failed to list files in test directory: %v", err)
			}
			for _, file := range allExistingFiles {
				filename := file.Name()

				if !slices.Contains(tt.wanted, filename) {
					t.Fatalf("Unexpected file %q exists after generate", filename)
				}
			}
		})
	}
}
