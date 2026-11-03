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
// It supports literal OpenSSH arguments, not arbitrary shell programs.
func (s *Store) SSHCommand(dir string, override bool) ([]string, error) {
	commandEnv, variantEnv := "GIT_SSH_COMMAND", "GIT_SSH_VARIANT"
	if override {
		commandEnv, variantEnv = "GH_IDENTITY_SSH_COMMAND", "GH_IDENTITY_SSH_VARIANT"
	}
	variant := os.Getenv(variantEnv)
	if variant == "" {
		var err error
		variant, err = s.underlyingGitValue(dir, "ssh.variant")
		if err != nil {
			return nil, err
		}
	}
	if variant != "" && variant != "auto" && variant != "ssh" {
		return nil, fmt.Errorf("identity SSH routing does not support SSH variant %q; your existing SSH configuration has not been changed", variant)
	}
	value := os.Getenv(commandEnv)
	if value == "" {
		var err error
		value, err = s.underlyingGitValue(dir, "core.sshCommand")
		if err != nil {
			return nil, err
		}
	}
	var command []string
	if value == "" {
		if executable := os.Getenv("GIT_SSH"); executable != "" {
			command = []string{executable}
		} else {
			command = []string{"ssh"}
		}
	}
	if command == nil {
		if strings.ContainsAny(value, "\r\n$`;&|<>") {
			return nil, errors.New("identity routing cannot safely preserve SSH shell expressions; your existing SSH configuration has not been changed")
		}
		var err error
		command, err = shellquote.Split(value)
		if err != nil {
			return nil, fmt.Errorf("cannot parse SSH command: %w", err)
		}
	}
	if isSSHDispatcher(command) {
		return nil, errors.New("SSH command would recursively invoke gh auth git-ssh; configure the underlying connection command instead")
	}
	if len(command) == 0 {
		return nil, errors.New("SSH connection command is empty")
	}
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(command[0])), ".exe")
	if name != "ssh" && variant != "ssh" {
		return nil, errors.New("the configured SSH command has unknown argument conventions; set ssh.variant=ssh only if it accepts OpenSSH arguments and honors the supplied destination")
	}
	return command, nil
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
