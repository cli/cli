package identity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kballard/go-shellquote"
)

// SSHCommand resolves the user's connection command underneath gh's managed
// dispatcher. Override reads the original environment captured by --account.
// Only the default OpenSSH command is supported; custom connection commands are refused.
func (s *Store) SSHCommand(dir string, override bool) ([]string, error) {
	commandEnv, variantEnv := "GIT_SSH_COMMAND", "GIT_SSH_VARIANT"
	if override {
		commandEnv, variantEnv = "GH_IDENTITY_SSH_COMMAND", "GH_IDENTITY_SSH_VARIANT"
	}
	if os.Getenv(commandEnv) != "" || os.Getenv("GIT_SSH") != "" {
		return nil, errors.New("custom SSH commands are not supported for identity routing; your SSH configuration has not been changed")
	}
	if variant := os.Getenv(variantEnv); variant != "" && variant != "ssh" {
		return nil, errors.New("custom SSH variants are not supported for identity routing")
	}
	for _, key := range []string{"core.sshCommand", "ssh.variant"} {
		value, err := s.underlyingGitValue(dir, key)
		if err != nil {
			return nil, err
		}
		if value != "" {
			return nil, fmt.Errorf("custom %s is not supported for identity routing; your SSH configuration has not been changed", key)
		}
	}
	return []string{"ssh"}, nil
}

func isSSHDispatcher(command []string) bool {
	return len(command) == 3 && command[1] == "auth" && command[2] == "git-ssh"
}

func (s *Store) underlyingGitValue(dir, key string) (string, error) {
	out, err := gitOptional(dir, "config", "--includes", "--show-origin", "--null", "--get-all", key)
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", nil
	}
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	if len(fields)%2 != 0 {
		return "", fmt.Errorf("Git did not report origins for %s", key)
	}
	value := ""
	for i := 0; i < len(fields); i += 2 {
		if sameConfigFile(strings.TrimPrefix(fields[i], "file:"), s.authConfigPath()) {
			switch key {
			case "core.sshCommand":
				command, err := shellquote.Split(fields[i+1])
				if err != nil || !isSSHDispatcher(command) {
					return "", errors.New("gh's managed SSH command has been edited; refusing to replace it")
				}
			case "ssh.variant":
				if fields[i+1] != "ssh" {
					return "", errors.New("gh's managed SSH variant has been edited; refusing to replace it")
				}
			}
			continue
		}
		value = fields[i+1]
	}
	return value, nil
}

func sameConfigFile(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	first, err := os.Stat(a)
	if err != nil {
		return false
	}
	second, err := os.Stat(b)
	return err == nil && os.SameFile(first, second)
}
