package sshkey

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/pkg/ssh"
)

// In 2.13.0 codespaces ssh and scp commands started automatically generating key pairs named
// 'codespaces' and 'codespaces.pub' which could collide with suggested the ssh config also named
// 'codespaces'. We now use 'codespaces.auto' and 'codespaces.auto.pub' in order to avoid that
// collision.
const automaticPrivateKeyNameOld = "codespaces"
const automaticPrivateKeyName = "codespaces.auto"

// autokeys	handles the automatic generation of SSH key pairs for the codespaces cli
type autoKeys struct {
	ctx ssh.Context
}

// generate creates a new SSH key pair if it doesn't already exist.
func (a *autoKeys) generate() (*ssh.KeyPair, error) {
	keyPair := a.upgrade()
	if keyPair != nil {
		return keyPair, nil
	}

	keyPair, err := a.ctx.GenerateSSHKey(automaticPrivateKeyName, "")
	if err != nil && !errors.Is(err, ssh.ErrKeyAlreadyExists) {
		return nil, err
	}

	return keyPair, nil
}

func (a *autoKeys) privateKeyPath() (string, error) {
	sshDir, err := a.ctx.SshDir()
	if err != nil {
		return "", err
	}

	return path.Join(sshDir, automaticPrivateKeyName), nil
}

// upgrade handles backward compatibility with the old keypair names.
//
// If the old public and private keys both exist they are renamed to the new name. The return value
// is non-nil only if the rename happens.
func (a *autoKeys) upgrade() *ssh.KeyPair {
	publicKeys, err := a.ctx.LocalPublicKeys()
	if err != nil {
		return nil
	}

	for _, publicKey := range publicKeys {
		if filepath.Base(publicKey) != automaticPrivateKeyNameOld+".pub" {
			continue
		}

		privateKey := strings.TrimSuffix(publicKey, ".pub")
		_, err := os.Stat(privateKey)
		if err != nil {
			continue
		}

		// Both old public and private keys exist, rename them to the new name

		sshDir := filepath.Dir(publicKey)

		publicKeyNew := filepath.Join(sshDir, automaticPrivateKeyName+".pub")
		err = os.Rename(publicKey, publicKeyNew)
		if err != nil {
			return nil
		}

		privateKeyNew := filepath.Join(sshDir, automaticPrivateKeyName)
		err = os.Rename(privateKey, privateKeyNew)
		if err != nil {
			return nil
		}

		keyPair := &ssh.KeyPair{
			PublicKeyPath:  publicKeyNew,
			PrivateKeyPath: privateKeyNew,
		}

		return keyPair
	}

	return nil
}

// getPair returns the paths to the automatic key pair files, if they both exist
func (a *autoKeys) getPair() *ssh.KeyPair {
	publicKeys, err := a.ctx.LocalPublicKeys()
	if err != nil {
		// The error would be that the .ssh dir doesn't exist, which just means that the keypair
		// also doesn't exist
		return nil
	}

	for _, publicKey := range publicKeys {
		if filepath.Base(publicKey) != automaticPrivateKeyName+".pub" {
			continue
		}

		privateKey := strings.TrimSuffix(publicKey, ".pub")

		_, err := os.Stat(privateKey)
		if err == nil {
			return &ssh.KeyPair{
				PrivateKeyPath: privateKey,
				PublicKeyPath:  publicKey,
			}
		}
	}

	return nil
}
