package sshkey

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cli/cli/v2/internal/codespaces/rpc"
	"github.com/cli/cli/v2/pkg/ssh"
	"github.com/cli/safeexec"
)

var errKeyFileNotFound = errors.New("SSH key file does not exist")

// Config ensures that the right key pair is returned for a codespace.
type Config struct {
	sshCtx   ssh.Context
	autoKeys *autoKeys
}

func NewConfig(sshCtx ssh.Context) *Config {
	return &Config{
		sshCtx:   sshCtx,
		autoKeys: &autoKeys{ctx: sshCtx},
	}
}

// FindKey evaluates available key pairs and select which should be used to connect to the codespace
// using the precedence rules below. If there is no error, a keypair is always returned and additionally a
// bool flag is returned to specify if the private key need be appended to the ssh arguments (it doesn't need
// to be if the key was selected from a -i argument).
//
// Precedence rules:
// 1. Key which is specified by -i
// 2. Automatic key, if it already exists
// 3. First valid keypair in ssh config (according to ssh -G)
// 4. Automatic key, newly created
func (c *Config) FindKey(ctx context.Context, args []string, profile string) (*ssh.KeyPair, []string, error) {
	customConfigPath := ""
	for i, arg := range args {
		if arg == "-i" {
			if i+1 >= len(args) {
				return nil, args, errors.New("missing value to -i argument")
			}

			privateKeyPath := args[i+1]

			// The --config setup will set the automatic key with -i, but it might not actually be
			// created, so we need to ensure that here
			if automaticPrivateKeyPath, _ := c.autoKeys.privateKeyPath(); automaticPrivateKeyPath == privateKeyPath {
				_, err := c.autoKeys.generate()
				if err != nil {
					return nil, args, fmt.Errorf("generating automatic keypair: %w", err)
				}
			}

			// User manually specified an identity file so just trust it is correct
			return &ssh.KeyPair{
				PrivateKeyPath: privateKeyPath,
				PublicKeyPath:  privateKeyPath + ".pub",
			}, args, nil
		}

		if arg == "-F" && i+1 < len(args) {
			// ssh only pays attention to that last specified -F value, so it's correct to overwrite
			// here
			customConfigPath = args[i+1]
		}
	}

	if autoKeyPair := c.autoKeys.getPair(); autoKeyPair != nil {
		// If the automatic keys already exist, just use them
		return autoKeyPair, addIdentityArg(autoKeyPair, args), nil
	}

	keyPair, err := firstConfiguredKeyPair(ctx, customConfigPath, profile)
	if err != nil {
		if !errors.Is(err, errKeyFileNotFound) {
			return nil, args, fmt.Errorf("checking configured keys: %w", err)
		}

		// no valid key in ssh config, generate one
		keyPair, err = c.autoKeys.generate()
		if err != nil {
			return nil, args, fmt.Errorf("generating automatic keypair: %w", err)
		}
	}

	return keyPair, addIdentityArg(keyPair, args), nil
}

// AutomaticPrivateKeyPath returns the path to the path for the private key from the pair that we
// generate automatically.
func (c *Config) AutomaticPrivateKeyPath() (string, error) {
	return c.autoKeys.privateKeyPath()
}

// firstConfiguredKeyPair reads the effective configuration for a localhost connection and returns
// the first valid key pair which would be tried for authentication
func firstConfiguredKeyPair(
	ctx context.Context,
	customConfigFile string,
	customHost string,
) (*ssh.KeyPair, error) {
	sshExe, err := safeexec.LookPath("ssh")
	if err != nil {
		return nil, fmt.Errorf("could not find ssh executable: %w", err)
	}

	// The -G option tells ssh to output the effective config for the given host, but not connect
	sshGArgs := []string{"-G"}

	if customConfigFile != "" {
		sshGArgs = append(sshGArgs, "-F", customConfigFile)
	}

	if customHost != "" {
		sshGArgs = append(sshGArgs, customHost)
	} else {
		sshGArgs = append(sshGArgs, "localhost")
	}

	sshGCmd := exec.CommandContext(ctx, sshExe, sshGArgs...)
	configBytes, err := sshGCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("could not load ssh configuration: %w", err)
	}

	configLines := strings.Split(string(configBytes), "\n")
	for _, line := range configLines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "identityfile ") {
			privateKeyPath := strings.SplitN(line, " ", 2)[1]

			keypair, err := keyPairFromPrivateKeyPath(privateKeyPath)
			if errors.Is(err, errKeyFileNotFound) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("loading ssh config: %w", err)
			}

			return keypair, nil
		}
	}

	return nil, errKeyFileNotFound
}

// keyPairFromPrivateKeyPath returns the KeyPair with the specified private key if it and the public key both exist
func keyPairFromPrivateKeyPath(path string) (*ssh.KeyPair, error) {
	if strings.HasPrefix(path, "~") {
		userHomeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home dir: %w", err)
		}

		// os.Stat can't handle ~, so convert it to the real path
		path = strings.Replace(path, "~", userHomeDir, 1)
	}

	// The default configuration includes standard keys like id_rsa or id_ed25519,
	// but these may not actually exist
	if _, err := os.Stat(path); err != nil {
		return nil, errKeyFileNotFound
	}

	publicKeyPath := path + ".pub"
	if _, err := os.Stat(publicKeyPath); err != nil {
		return nil, errKeyFileNotFound
	}

	return &ssh.KeyPair{
		PrivateKeyPath: path,
		PublicKeyPath:  publicKeyPath,
	}, nil
}

// addidIdentityArg adds the -i argument to the ssh command line arguments. We do this in order to
// force the identity from the precedence list described on FindKey.
func addIdentityArg(keyPair *ssh.KeyPair, args []string) []string {
	return append([]string{"-i", keyPair.PrivateKeyPath}, args...)
}

func ServerOptions(keyPair *ssh.KeyPair) rpc.StartSSHServerOptions {
	return rpc.StartSSHServerOptions{
		UserPublicKeyFile: keyPair.PublicKeyPath,
	}
}
