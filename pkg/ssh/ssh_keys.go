package ssh

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/safeexec"
)

type Context struct {
	ConfigDir string
	KeygenExe string
}

type KeyPair struct {
	PublicKeyPath  string
	PrivateKeyPath string
}

var ErrKeyAlreadyExists = errors.New("SSH key already exists")

func (c *Context) LocalPublicKeys() ([]string, error) {
	sshDir, err := c.sshDir()
	if err != nil {
		return nil, err
	}

	return filepath.Glob(filepath.Join(sshDir, "*.pub"))
}

func (c *Context) HasPrivateKey(keyName string) bool {
	sshDir, err := c.sshDir()
	if err != nil {
		return false
	}

	keyFile := filepath.Join(sshDir, keyName)
	_, err = os.Stat(keyFile)

	return err == nil
}

func (c *Context) HasAnyMatchingPrivateKey(publicKeys []string) (bool, error) {
	keygenExe, err := c.findKeygen()
	if err != nil {
		return false, fmt.Errorf("could not find ssh-keygen: %w", err)
	}

	sshDir, err := c.sshDir()
	if err != nil {
		// If .ssh doesn't exist it's not an error, there are just no keys to match
		return false, nil
	}

	files, err := ioutil.ReadDir(sshDir)
	if err != nil {
		return false, fmt.Errorf("could not read .ssh directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fullPath := filepath.Join(sshDir, file.Name())
		keygenCmd := exec.Command(keygenExe, "-y", "-f", fullPath)

		keygenRunnable := run.PrepareCmd(keygenCmd)
		outputBytes, err := keygenRunnable.Output()
		if err != nil {
			// Just move on, it probably wasn't a key file
			continue
		}

		fullPublicKey := string(outputBytes)
		typeAndKey := strings.SplitN(fullPublicKey, " ", 3)[:2]
		publicKeyWithoutComment := strings.Join(typeAndKey, " ")

		for _, publicKey := range publicKeys {
			if publicKeyWithoutComment == publicKey {
				return true, nil
			}
		}
	}

	return false, nil
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

	sshDir, err := c.sshDir()
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

func (c *Context) sshDir() (string, error) {
	if c.ConfigDir != "" {
		return c.ConfigDir, nil
	}
	dir, err := config.HomeDirPath(".ssh")
	if err == nil {
		c.ConfigDir = dir
	}
	return dir, err
}

func (c *Context) findKeygen() (string, error) {
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
