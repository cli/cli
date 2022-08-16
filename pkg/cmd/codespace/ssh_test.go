package codespace

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/ssh"
)

func TestPendingOperationDisallowsSSH(t *testing.T) {
	app := testingSSHApp()

	if err := app.SSH(context.Background(), []string{}, sshOptions{codespace: "disabledCodespace"}); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func TestAutomaticSSHKeyPairs(t *testing.T) {
	tests := []struct {
		// These files exist when calling setupAutomaticSSHKeys
		existingFiles []string
		// These files should exist after setupAutomaticSSHKeys finishes
		wantFinalFiles []string
	}{
		// Basic case: no existing keys, they should be created
		{
			nil,
			[]string{automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
		// Basic case: keys already exist
		{
			[]string{automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
			[]string{automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
		// Backward compatibility: both old keys exist, they should be renamed
		{
			[]string{automaticPrivateKeyNameOld, automaticPrivateKeyNameOld + ".pub"},
			[]string{automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
		// Backward compatibility: old private key exists but not the public key, the new keys should be created
		{
			[]string{automaticPrivateKeyNameOld},
			[]string{automaticPrivateKeyNameOld, automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
		// Backward compatibility: old public key exists but not the private key, the new keys should be created
		{
			[]string{automaticPrivateKeyNameOld + ".pub"},
			[]string{automaticPrivateKeyNameOld + ".pub", automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
		// Backward compatibility (edge case): files exist which contains old key name as a substring, the new keys should be created
		{
			[]string{"foo" + automaticPrivateKeyNameOld + ".pub", "foo" + automaticPrivateKeyNameOld},
			[]string{"foo" + automaticPrivateKeyNameOld + ".pub", "foo" + automaticPrivateKeyNameOld, automaticPrivateKeyName, automaticPrivateKeyName + ".pub"},
		},
	}

	for _, tt := range tests {
		dir := t.TempDir()

		sshContext := ssh.Context{
			ConfigDir: dir,
		}

		for _, file := range tt.existingFiles {
			f, err := os.Create(filepath.Join(dir, file))
			if err != nil {
				t.Errorf("Failed to setup test files: %v", err)
			}
			// If the file isn't closed here windows will have errors about file already in use
			f.Close()
		}

		keyPair, err := setupAutomaticSSHKeys(sshContext)
		if err != nil {
			t.Errorf("Unexpected error from setupAutomaticSSHKeys: %v", err)
		}
		if keyPair == nil {
			t.Fatal("Unexpected nil KeyPair from setupAutomaticSSHKeys")
		}
		if !strings.HasSuffix(keyPair.PrivateKeyPath, automaticPrivateKeyName) {
			t.Errorf("Expected private key path %v, got %v", automaticPrivateKeyName, keyPair.PrivateKeyPath)
		}
		if !strings.HasSuffix(keyPair.PublicKeyPath, automaticPrivateKeyName+".pub") {
			t.Errorf("Expected public key path %v, got %v", automaticPrivateKeyName+".pub", keyPair.PublicKeyPath)
		}

		// Check that all the expected files are present
		for _, file := range tt.wantFinalFiles {
			if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
				t.Errorf("Want file %q to exist after setupAutomaticSSHKeys but it doesn't", file)
			}
		}

		// Check that no unexpected files are present
		allExistingFiles, err := ioutil.ReadDir(dir)
		if err != nil {
			t.Errorf("Failed to list files in test directory: %v", err)
		}
		for _, file := range allExistingFiles {
			filename := file.Name()
			isWantedFile := false
			for _, wantedFile := range tt.wantFinalFiles {
				if filename == wantedFile {
					isWantedFile = true
					break
				}
			}

			if !isWantedFile {
				t.Errorf("Unexpected file %q exists after setupAutomaticSSHKeys", filename)
			}
		}
	}
}

func TestUseAutomaticSSHKeysIdentityFileArg(t *testing.T) {
	tests := []struct {
		identityFileArg string
		wantResult      bool
	}{
		{"custom-private-key", false},
		{automaticPrivateKeyName, true},
		{"", false}, // Edge case check for missing arg value
	}

	for _, tt := range tests {
		t.Logf("%v", tt.identityFileArg)

		args := []string{"-i"}
		if tt.identityFileArg != "" {
			args = append(args, tt.identityFileArg)
		}

		result, err := useAutomaticSSHKeys(context.Background(), ssh.Context{}, nil, args, sshOptions{})

		if err != nil {
			t.Errorf("Unexpected error from useAutomaticSSHKeys: %v", err)
		}

		if result != tt.wantResult {
			t.Errorf("Want useAutomaticSSHKeys to be %v, got %v", tt.wantResult, result)
		}
	}
}

func TestUseAutomaticSSHKeysWithAutoKeysExist(t *testing.T) {
	dir := t.TempDir()

	f, err := os.Create(path.Join(dir, automaticPrivateKeyName))
	if err != nil {
		t.Errorf("Failed to create test private key: %v", err)
	}
	f.Close()

	f, err = os.Create(path.Join(dir, automaticPrivateKeyName+".pub"))
	if err != nil {
		t.Errorf("Failed to create test public key: %v", err)
	}
	f.Close()

	sshContext := ssh.Context{
		ConfigDir: dir,
	}
	result, err := useAutomaticSSHKeys(context.Background(), sshContext, nil, nil, sshOptions{})

	if err != nil {
		t.Errorf("Unexpected error from useAutomaticSSHKeys: %v", err)
	}

	if result != true {
		t.Errorf("Want useAutomaticSSHKeys to be true, got false")
	}
}

func TestHasUploadedPublicKeyForConfig(t *testing.T) {
	type testLocalKeyPair struct {
		privateKeyFile   string
		publicKeyContent string
	}

	tests := []struct {
		apiAuthorizedPublicKeys []string
		localKeyPairs           []testLocalKeyPair
		wantResult              bool
	}{
		// Failure tests
		{
			// No API keys and no local keys
			wantResult: false,
		},
		{
			// Has API keys, but no local keys
			apiAuthorizedPublicKeys: []string{"ssh-rsa test-key"},
			wantResult:              false,
		},
		{
			// No API keys, but has local keys
			localKeyPairs: []testLocalKeyPair{{"keyfile", "ssh-rsa test-key"}},
			wantResult:    false,
		},
		{
			// API keys and local keys, but not matching
			apiAuthorizedPublicKeys: []string{"ssh-rsa test-api-key"},
			localKeyPairs:           []testLocalKeyPair{{"keyfile", "ssh-rsa test-local-key"}},
			wantResult:              false,
		},

		// Successful tests
		{
			apiAuthorizedPublicKeys: []string{"ssh-rsa test-key"},
			localKeyPairs:           []testLocalKeyPair{{"keyfile", "ssh-rsa test-key"}},
			wantResult:              true,
		},
		{
			apiAuthorizedPublicKeys: []string{"ssh-rsa test-key-1", "ssh-rsa test-key-2"},
			localKeyPairs:           []testLocalKeyPair{{"keyfile1", "ssh-rsa test-key-1"}},
			wantResult:              true,
		},
		{
			apiAuthorizedPublicKeys: []string{"ssh-rsa test-key-1"},
			localKeyPairs:           []testLocalKeyPair{{"keyfile1", "ssh-rsa test-key-1"}, {"keyfile2", "ssh-rsa test-key-2"}},
			wantResult:              true,
		},
		{
			apiAuthorizedPublicKeys: []string{"ssh-rsa test-key-1", "ssh-rsa test-key-2"},
			localKeyPairs:           []testLocalKeyPair{{"keyfile3", "ssh-rsa test-key-3"}, {"keyfile2", "ssh-rsa test-key-2"}},
			wantResult:              true,
		},

		// Extra case - local key contain comments
		{
			apiAuthorizedPublicKeys: []string{"ssh-rsa test-key-1", "ssh-rsa test-key"},
			localKeyPairs:           []testLocalKeyPair{{"keyfile3", "ssh-rsa test-key a comment on the key"}},
			wantResult:              true,
		},
	}

	for _, tt := range tests {
		t.Logf("%+v", tt)

		mockApi := &apiClientMock{
			GetUserFunc: func(ctx context.Context) (*api.User, error) {
				return &api.User{Login: "test"}, nil
			},
			AuthorizedKeysFunc: func(_ context.Context, _ string) ([]string, error) {
				return tt.apiAuthorizedPublicKeys, nil
			},
		}

		dir := t.TempDir()
		configPath := path.Join(dir, "test-config")

		configContent := ""
		for _, pair := range tt.localKeyPairs {
			configContent += fmt.Sprintf("IdentityFile %s\n", path.Join(dir, pair.privateKeyFile))

			err := os.WriteFile(path.Join(dir, pair.privateKeyFile+".pub"), []byte(pair.publicKeyContent), 0666)
			if err != nil {
				t.Fatalf("could not write test public key file %v", err)
			}
		}

		err := os.WriteFile(configPath, []byte(configContent), 0666)
		if err != nil {
			t.Fatalf("could not write test config %v", err)
		}

		result, err := hasUploadedPublicKeyForConfig(context.Background(), mockApi, configPath, "")
		if err != nil {
			t.Errorf("Unexpected error from hasUploadedPublicKeyForConfig: %v", err)
		}

		if result != tt.wantResult {
			t.Errorf("Want hasUploadedPublicKeyForConfig to be %v, got %v", tt.wantResult, result)
		}
	}
}

func testingSSHApp() *App {
	user := &api.User{Login: "monalisa"}
	disabledCodespace := &api.Codespace{
		Name:                           "disabledCodespace",
		PendingOperation:               true,
		PendingOperationDisabledReason: "Some pending operation",
	}
	apiMock := &apiClientMock{
		GetCodespaceFunc: func(_ context.Context, name string, _ bool) (*api.Codespace, error) {
			if name == "disabledCodespace" {
				return disabledCodespace, nil
			}
			return nil, nil
		},
		GetUserFunc: func(_ context.Context) (*api.User, error) {
			return user, nil
		},
		AuthorizedKeysFunc: func(_ context.Context, _ string) ([]string, error) {
			return []string{}, nil
		},
	}

	ios, _, _, _ := iostreams.Test()
	return NewApp(ios, nil, apiMock, nil)
}
