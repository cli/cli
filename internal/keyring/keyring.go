// Package keyring is a simple wrapper that adds timeouts to the zalando/go-keyring package.
package keyring

import (
	"time"

	"github.com/99designs/keyring"
)

type TimeoutError struct {
	message string
}

func (e *TimeoutError) Error() string {
	return e.message
}

var GetKeyring = func(service string) (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{ServiceName: service})
}

func MockInit() {
	keyrings := make(map[string]keyring.Keyring)
	GetKeyring = func(service string) (keyring.Keyring, error) {
		kr, ok := keyrings[service]
		if !ok {
			kr = keyring.NewArrayKeyring(nil)
			keyrings[service] = kr
		}
		return kr, nil
	}
}

// Set secret in keyring for user.
func Set(service, user, secret string) error {
	kr, err := GetKeyring(service)
	if err != nil {
		return err
	}
	ch := make(chan error, 1)
	go func() {
		defer close(ch)
		ch <- kr.Set(keyring.Item{Key: user, Data: []byte(secret)})
	}()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		return &TimeoutError{"timeout while trying to set secret in keyring"}
	}
}

// Get secret from keyring given service and user name.
func Get(service, user string) (string, error) {
	kr, err := GetKeyring(service)
	if err != nil {
		return "", err
	}
	ch := make(chan struct {
		val string
		err error
	}, 1)
	go func() {
		defer close(ch)
		val, err := kr.Get(user)
		ch <- struct {
			val string
			err error
		}{string(val.Data), err}
	}()
	select {
	case res := <-ch:
		return res.val, res.err
	case <-time.After(3 * time.Second):
		return "", &TimeoutError{"timeout while trying to get secret from keyring"}
	}
}

// Delete secret from keyring.
func Delete(service, user string) error {
	kr, err := GetKeyring(service)
	if err != nil {
		return err
	}
	ch := make(chan error, 1)
	go func() {
		defer close(ch)
		ch <- kr.Remove(user)
	}()
	select {
	case err := <-ch:
		return err
	case <-time.After(3 * time.Second):
		return &TimeoutError{"timeout while trying to delete secret from keyring"}
	}
}
