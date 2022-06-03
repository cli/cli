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

type SshKeyPair struct {
	PublicKeyPath  string
	PrivateKeyPath string
}

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

func (c *SshContext) GenerateSSHKey(keyName string, errorOnExists bool, promptPassphrase func() (string, error)) (SshKeyPair, error) {
	keygenExe, err := c.findKeygen()
	if err != nil {
		// TODO: is there a nicer way to do this default SshKeyPair?
		return SshKeyPair{}, fmt.Errorf("could not find keygen executable")
	}

	sshDir, err := c.sshDir()
	if err != nil {
		return SshKeyPair{}, err
	}
	keyFile := filepath.Join(sshDir, keyName)
	keyPair := SshKeyPair{
		PublicKeyPath:  keyFile + ".pub",
		PrivateKeyPath: keyFile,
	}

	if _, err := os.Stat(keyFile); err == nil {
		if errorOnExists {
			return SshKeyPair{}, fmt.Errorf("refusing to overwrite file %s", keyFile)
		} else {
			return keyPair, nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(keyFile), 0711); err != nil {
		return SshKeyPair{}, err
	}

	var sshPassphrase string
	if promptPassphrase != nil {
		sshPassphrase, err = promptPassphrase()
	}

	// TOOD: sshLabel was never set, so should -C just be removed?
	keygenCmd := exec.Command(keygenExe, "-t", "ed25519", "-C", "", "-N", sshPassphrase, "-f", keyFile)
	return keyPair, run.PrepareCmd(keygenCmd).Run()
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
