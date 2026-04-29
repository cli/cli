// Package keyring is a simple wrapper that adds timeouts to the zalando/go-keyring package.
package keyring

import (
	"errors"
	"time"

	"github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("secret not found in keyring")

type TimeoutError struct {
	message string
}

func (e *TimeoutError) Error() string {
	return e.message
}

// Set secret in keyring for user.
func Set(service, user, secret string) error {
	ch := make(chan error, 1)
	go func() {
		defer close(ch)
		ch <- keyring.Set(service, user, secret)
	}()
	select {
	case err := <-ch:
		return err
	case <-time.After(60 * time.Second):
		return &TimeoutError{"timeout while trying to set secret in keyring"}
	}
}

// getOverride, when non-nil, intercepts Get calls before they reach the
// underlying provider. It exists solely to let tests simulate per-(service,user)
// failure modes that the upstream mock can only express globally.
var getOverride func(service, user string) (string, error)

// Get secret from keyring given service and user name.
func Get(service, user string) (string, error) {
	if fn := getOverride; fn != nil {
		val, err := fn(service, user)
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return val, err
	}
	ch := make(chan struct {
		val string
		err error
	}, 1)
	go func() {
		defer close(ch)
		val, err := keyring.Get(service, user)
		ch <- struct {
			val string
			err error
		}{val, err}
	}()
	select {
	case res := <-ch:
		if errors.Is(res.err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return res.val, res.err
	case <-time.After(60 * time.Second):
		return "", &TimeoutError{"timeout while trying to get secret from keyring"}
	}
}

// Delete secret from keyring.
func Delete(service, user string) error {
	ch := make(chan error, 1)
	go func() {
		defer close(ch)
		ch <- keyring.Delete(service, user)
	}()
	select {
	case err := <-ch:
		return err
	case <-time.After(60 * time.Second):
		return &TimeoutError{"timeout while trying to delete secret from keyring"}
	}
}

func MockInit() {
	keyring.MockInit()
	getOverride = nil
}

func MockInitWithError(err error) {
	keyring.MockInitWithError(err)
	getOverride = nil
}

// MockGetOverride installs fn as an interceptor for Get calls. Pass nil to
// remove the override. Intended for tests that need per-(service,user) failure
// modes which the upstream mock cannot express.
//
// The override is stored in a package-level variable without synchronization,
// so callers must not use this with t.Parallel(). Either MockInit or
// MockInitWithError will clear the override, preserving the isolation
// contract for tests that reset keyring state at setup; callers should still
// prefer registering t.Cleanup to restore nil promptly when the override is
// only needed for one test.
func MockGetOverride(fn func(service, user string) (string, error)) {
	getOverride = fn
}
