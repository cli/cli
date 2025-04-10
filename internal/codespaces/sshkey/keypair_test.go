package sshkey

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/ssh"
)

func Test_Config_FindKey(t *testing.T) {
	// This string will be substituted in sshArgs for test cases
	// This is to work around the temp test ssh dir not being known until the test is executing
	substituteSSHDir := "SUB_SSH_DIR"

	tests := []struct {
		name               string
		sshDirFiles        []string
		sshConfigKeys      []string
		args               []string
		profile            string
		wantedPair         *ssh.KeyPair
		wantedShouldAddArg bool
	}{
		// -i tests
		{
			name:       "respects -i",
			args:       []string{"-i", "custom-private-key"},
			wantedPair: &ssh.KeyPair{PrivateKeyPath: "custom-private-key", PublicKeyPath: "custom-private-key.pub"},
		},
		{
			name:       "respects -i with full path",
			args:       []string{"-i", path.Join(substituteSSHDir, automaticPrivateKeyName)},
			wantedPair: &ssh.KeyPair{PrivateKeyPath: automaticPrivateKeyName, PublicKeyPath: automaticPrivateKeyName + ".pub"},
		},
		{
			// Edge case check for missing arg value
			name: "detects when -i value is missing",
			args: []string{"-i"},
		},

		// Auto key exists tests
		{
			name:               "detects and uses existing automatic key",
			sshDirFiles:        []string{automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
			wantedPair:         &ssh.KeyPair{PrivateKeyPath: automaticPrivateKeyName, PublicKeyPath: automaticPrivateKeyName + ".pub"},
			wantedShouldAddArg: true,
		},
		{
			name:               "detects and uses existing automatic key when other keys exist",
			sshDirFiles:        []string{automaticPrivateKeyName, automaticPrivateKeyName + ".pub", "custom-private-key", "custom-private-key.pub"},
			wantedPair:         &ssh.KeyPair{PrivateKeyPath: automaticPrivateKeyName, PublicKeyPath: automaticPrivateKeyName + ".pub"},
			wantedShouldAddArg: true,
		},

		// SSH config tests
		{
			name:               "detects and uses existing key from ssh config",
			sshDirFiles:        []string{"custom-private-key", "custom-private-key.pub"},
			sshConfigKeys:      []string{"custom-private-key"},
			wantedPair:         &ssh.KeyPair{PrivateKeyPath: "custom-private-key", PublicKeyPath: "custom-private-key.pub"},
			wantedShouldAddArg: true,
		},
		{
			// 2 pairs, but only 1 is configured
			name:               "detects and uses existing key from ssh config with multiple keys",
			sshDirFiles:        []string{"custom-private-key", "custom-private-key.pub", "custom-private-key-2", "custom-private-key-2.pub"},
			sshConfigKeys:      []string{"custom-private-key-2"},
			wantedPair:         &ssh.KeyPair{PrivateKeyPath: "custom-private-key-2", PublicKeyPath: "custom-private-key-2.pub"},
			wantedShouldAddArg: true,
		},
		{
			// 2 pairs, but only 1 has both public and private
			name:               "detects and uses existing key from ssh config when only one has both public and private",
			sshDirFiles:        []string{"custom-private-key", "custom-private-key-2", "custom-private-key-2.pub"},
			sshConfigKeys:      []string{"custom-private-key", "custom-private-key-2"},
			wantedPair:         &ssh.KeyPair{PrivateKeyPath: "custom-private-key-2", PublicKeyPath: "custom-private-key-2.pub"},
			wantedShouldAddArg: true,
		},

		// Automatic key tests
		{
			name:               "creates automatic key when no keys exist",
			wantedPair:         &ssh.KeyPair{PrivateKeyPath: automaticPrivateKeyName, PublicKeyPath: automaticPrivateKeyName + ".pub"},
			wantedShouldAddArg: true,
		},
		{
			// Renames old key pair to new
			name:               "renames old automatic key pair to new",
			sshDirFiles:        []string{automaticPrivateKeyNameOld, automaticPrivateKeyNameOld + ".pub"},
			wantedPair:         &ssh.KeyPair{PrivateKeyPath: automaticPrivateKeyName, PublicKeyPath: automaticPrivateKeyName + ".pub"},
			wantedShouldAddArg: true,
		},
		{
			// Other key is configured, but doesn't exist
			name:               "creates automatic key when other key is configured but doesn't exist",
			sshConfigKeys:      []string{"custom-private-key"},
			wantedPair:         &ssh.KeyPair{PrivateKeyPath: automaticPrivateKeyName, PublicKeyPath: automaticPrivateKeyName + ".pub"},
			wantedShouldAddArg: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sshDir := t.TempDir()
			sshContext := ssh.NewContextForTests(sshDir, "")

			for _, file := range tt.sshDirFiles {
				f, err := os.Create(filepath.Join(sshDir, file))
				if err != nil {
					t.Fatalf("Failed to create test ssh dir file %q: %v", file, err)
				}
				f.Close()
			}

			configPath := filepath.Join(sshDir, "test-config")

			// Seed the config with a non-existent key so that the default config won't apply
			configContent := "IdentityFile dummy\n"

			for _, key := range tt.sshConfigKeys {
				configContent += fmt.Sprintf("IdentityFile %s\n", filepath.Join(sshDir, key))
			}

			err := os.WriteFile(configPath, []byte(configContent), 0666)
			if err != nil {
				t.Fatalf("could not write test config %v", err)
			}

			var subbedSSHArgs []string
			for _, arg := range tt.args {
				subbedSSHArgs = append(subbedSSHArgs, strings.Replace(arg, substituteSSHDir, sshDir, -1))
			}

			tt.args = append([]string{"-F", configPath}, subbedSSHArgs...)

			config := NewConfig(sshContext)
			gotKeyPair, gotArgs, err := config.FindKey(context.Background(), tt.args, tt.profile)

			if tt.wantedPair == nil {
				if err == nil {
					t.Fatal("Expected error from FindKey but got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("Unexpected error from FindKey: %v", err)
			}

			if gotKeyPair == nil {
				t.Fatal("Expected non-nil result from FindKey but got nil")
			}

			if tt.wantedShouldAddArg && gotArgs[0] != "-i" {
				t.Fatalf("Got wrong list of args from FindKey, wanted include `-i` got %v", gotArgs)
			}

			// Strip the dir (sshDir) from the gotKeyPair paths so that they match wantKeyPair (which doesn't know the directory)
			gotKeyPairJustFileNames := &ssh.KeyPair{
				PrivateKeyPath: filepath.Base(gotKeyPair.PrivateKeyPath),
				PublicKeyPath:  filepath.Base(gotKeyPair.PublicKeyPath),
			}

			if fmt.Sprintf("%v", gotKeyPairJustFileNames) != fmt.Sprintf("%v", tt.wantedPair) {
				t.Fatalf("Want FindKey result to be %v, got %v", tt.wantedPair, gotKeyPairJustFileNames)
			}

			// If the automatic key pair is selected, it needs to exist no matter what
			if strings.Contains(tt.wantedPair.PrivateKeyPath, automaticPrivateKeyName) {
				if _, err := os.Stat(gotKeyPair.PrivateKeyPath); err != nil {
					t.Fatalf("Expected automatic key pair private key to exist, but it did not")
				}

				if _, err := os.Stat(gotKeyPair.PublicKeyPath); err != nil {
					t.Fatalf("Expected automatic key pair public key to exist, but it did not")
				}
			}
		})
	}
}
