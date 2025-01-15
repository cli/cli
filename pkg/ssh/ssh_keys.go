package ssh

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/safeexec"
)

type Context struct {
	configDir string
	keygenExe string
}

// NewContextForTests creates a new `ssh.Context` with internal properties set to the
// specified values. It should only be used to inject test-specific setup.
func NewContextForTests(configDir, keygenExe string) Context {
	return Context{
		configDir,
		keygenExe,
	}
}

type KeyPair struct {
	PublicKeyPath  string
	PrivateKeyPath string
}

var ErrKeyAlreadyExists = errors.New("SSH key already exists")

func (c *Context) LocalPublicKeys() ([]string, error) {
	sshDir, err := c.SshDir()
	if err != nil {
		return nil, err
	}

	return filepath.Glob(filepath.Join(sshDir, "*.pub"))
}

func (c *Context) HasKeygen() bool {
	_, err := c.findKeygen()
	return err == nil
}

func (c *Context) GenerateSSHKey(keyName string, passphrase string) (*KeyPair, error) {
	keygenExe, err := c.findKeygen()
	if err != nil {
		return nil, err
	}

	sshDir, err := c.SshDir()
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(sshDir, 0700)
	if err != nil {
		return nil, fmt.Errorf("could not create .ssh directory: %w", err)
	}

	keyFile := filepath.Join(sshDir, keyName)
	keyPair := KeyPair{
		PublicKeyPath:  keyFile + ".pub",
		PrivateKeyPath: keyFile,
	}

	if _, err := os.Stat(keyFile); err == nil {
		// Still return keyPair because the caller might be OK with this - they can check the error with errors.Is(err, ErrKeyAlreadyExists)
		return &keyPair, ErrKeyAlreadyExists
	}

	if err := os.MkdirAll(filepath.Dir(keyFile), 0711); err != nil {
		return nil, err
	}

	keygenCmd := exec.Command(keygenExe, "-t", "ed25519", "-C", "", "-N", passphrase, "-f", keyFile)
	err = run.PrepareCmd(keygenCmd).Run()
	if err != nil {
		return nil, err
	}

	return &keyPair, nil
}

func (c *Context) SshDir() (string, error) {
	if c.configDir != "" {
		return c.configDir, nil
	}
	dir, err := config.HomeDirPath(".ssh")
	if err == nil {
		c.configDir = dir
	}
	return dir, err
}

func (c *Context) findKeygen() (string, error) {
	if c.keygenExe != "" {
		return c.keygenExe, nil
	}

	keygenExe, err := safeexec.LookPath("ssh-keygen")
	if err != nil && runtime.GOOS == "windows" {
		// We can try and find ssh-keygen in a Git for Windows install
		if gitPath, err := safeexec.LookPath("git"); err == nil {
			gitKeygen := filepath.Join(filepath.Dir(gitPath), "..", "usr", "bin", "ssh-keygen.exe")
			if _, err = os.Stat(gitKeygen); err == nil {
				return gitKeygen, nil
			}
		}
	}

	if err == nil {
		c.keygenExe = keygenExe
	}
	return keygenExe, err
}
