package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/safeexec"
)

type SshContext struct {
	ConfigDir string
	KeygenExe string
}

type KeyPair struct {
	PublicKeyPath  string
	PrivateKeyPath string
}

type KeyAlreadyExistsError struct {
	keyPath string
}

func (err *KeyAlreadyExistsError) Error() string { return "SSH key already exists: " + err.keyPath }

func (c *SshContext) LocalPublicKeys() ([]string, error) {
	sshDir, err := c.sshDir()
	if err != nil {
		return nil, err
	}

	return filepath.Glob(filepath.Join(sshDir, "*.pub"))
}

func (c *SshContext) HasKeygen() bool {
	_, err := c.findKeygen()
	return err == nil
}

func (c *SshContext) GenerateSSHKey(keyName string, passphrase string) (*KeyPair, error) {
	keygenExe, err := c.findKeygen()
	if err != nil {
		return nil, fmt.Errorf("could not find keygen executable")
	}

	sshDir, err := c.sshDir()
	if err != nil {
		return nil, err
	}
	keyFile := filepath.Join(sshDir, keyName)
	keyPair := KeyPair{
		PublicKeyPath:  keyFile + ".pub",
		PrivateKeyPath: keyFile,
	}

	if _, err := os.Stat(keyFile); err == nil {
		// Still return keyPair because the caller might be OK with they - they can check the error with
		return &keyPair, &KeyAlreadyExistsError{keyFile}
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

func (c *SshContext) sshDir() (string, error) {
	if c.ConfigDir != "" {
		return c.ConfigDir, nil
	}
	dir, err := config.HomeDirPath(".ssh")
	if err == nil {
		c.ConfigDir = dir
	}
	return dir, err
}

func (c *SshContext) findKeygen() (string, error) {
	if c.KeygenExe != "" {
		return c.KeygenExe, nil
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
		c.KeygenExe = keygenExe
	}
	return keygenExe, err
}
