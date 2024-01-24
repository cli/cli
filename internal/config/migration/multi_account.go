package migration

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/internal/keyring"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/config"
)

var noTokenError = errors.New("no token found")

type CowardlyRefusalError struct {
	err error
}

func (e CowardlyRefusalError) Error() string {
	// Consider whether we should add a call to action here like "open an issue with the contents of your redacted hosts.yml"
	return fmt.Sprintf("cowardly refusing to continue with multi account migration: %s", e.err.Error())
}

var hostsKey = []string{"hosts"}

type tokenSource struct {
	token     string
	inKeyring bool
}

// This migration exists to take a hosts section of the following structure:
//
//	github.com:
//	  user: williammartin
//	  git_protocol: https
//	  editor: vim
//	github.localhost:
//	  user: monalisa
//	  git_protocol: https
//    oauth_token: xyz
//
// We want this to migrate to something like:
//
// github.com:
//	 user: williammartin
//	 git_protocol: https
//	 editor: vim
//	 users:
//	   williammartin:
//
// github.localhost:
//	 user: monalisa
//	 git_protocol: https
//   oauth_token: xyz
//   users:
//	   monalisa:
//	     oauth_token: xyz
//
// For each hosts, we will create a new key `users` with the value of the host level
// `user` key as a list entry. If there is a host level `oauth_token` we will
// put that under the new user entry, otherwise there will be no value for the
// new user key. No other host level configuration will be copied to the new user.
//
// The reason for this is that we can then add new users under a host.
// Note that we are only copying the config under a new users key, and
// under a specific user. The original config is left alone. This is to
// allow forward compatibility for older versions of gh and also to avoid
// breaking existing users of go-gh which looks at a specific location
// in the config for oauth tokens that are stored insecurely.

type MultiAccount struct {
	// Allow injecting a transport layer in tests.
	Transport http.RoundTripper
}

func (m MultiAccount) PreVersion() string {
	// It is expected that there is no version key since this migration
	// introduces it.
	return ""
}

func (m MultiAccount) PostVersion() string {
	return "1"
}

func (m MultiAccount) Do(c *config.Config) error {
	hostnames, err := c.Keys(hostsKey)
	// [github.com, github.localhost]
	// We wouldn't expect to have a hosts key when this is the first time anyone
	// is logging in with the CLI.
	var keyNotFoundError *config.KeyNotFoundError
	if errors.As(err, &keyNotFoundError) {
		return nil
	}
	if err != nil {
		return CowardlyRefusalError{errors.New("couldn't get hosts configuration")}
	}

	// If there are no hosts then it doesn't matter whether we migrate or not,
	// so lets avoid any confusion and say there's no migration required.
	if len(hostnames) == 0 {
		return nil
	}

	// Otherwise let's get to the business of migrating!
	for _, hostname := range hostnames {
		tokenSource, err := getToken(c, hostname)
		// If no token existed for this host we'll remove the entry from the hosts file
		// by deleting it and moving on to the next one.
		if errors.Is(err, noTokenError) {
			// The only error that can be returned here is the key not existing, which
			// we know can't be true.
			_ = c.Remove(append(hostsKey, hostname))
			continue
		}
		// For any other error we'll error out
		if err != nil {
			return CowardlyRefusalError{fmt.Errorf("couldn't find oauth token for %q: %w", hostname, err)}
		}

		username, err := getUsername(c, hostname, tokenSource.token, m.Transport)
		if err != nil {
			issueURL := "https://github.com/cli/cli/issues/8441"
			return CowardlyRefusalError{fmt.Errorf("couldn't get user name for %q please visit %s for help: %w", hostname, issueURL, err)}
		}

		if err := migrateConfig(c, hostname, username); err != nil {
			return CowardlyRefusalError{fmt.Errorf("couldn't migrate config for %q: %w", hostname, err)}
		}

		if err := migrateToken(hostname, username, tokenSource); err != nil {
			return CowardlyRefusalError{fmt.Errorf("couldn't migrate oauth token for %q: %w", hostname, err)}
		}
	}

	return nil
}

func getToken(c *config.Config, hostname string) (tokenSource, error) {
	if token, _ := c.Get(append(hostsKey, hostname, "oauth_token")); token != "" {
		return tokenSource{token: token, inKeyring: false}, nil
	}
	token, err := keyring.Get(keyringServiceName(hostname), "")

	// If we have an error and it's not relating to there being no token
	// then we'll return the error cause that's really unexpected.
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return tokenSource{}, err
	}

	// Otherwise we'll return a sentinel error
	if err != nil || token == "" {
		return tokenSource{}, noTokenError
	}

	return tokenSource{
		token:     token,
		inKeyring: true,
	}, nil
}

func getUsername(c *config.Config, hostname, token string, transport http.RoundTripper) (string, error) {
	username, _ := c.Get(append(hostsKey, hostname, "user"))
	if username != "" && username != "x-access-token" {
		return username, nil
	}
	opts := ghAPI.ClientOptions{
		Host:      hostname,
		AuthToken: token,
		Transport: transport,
	}
	client, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return "", err
	}
	var query struct {
		Viewer struct {
			Login string
		}
	}
	err = client.Query("CurrentUser", &query, nil)
	if err != nil {
		return "", err
	}
	return query.Viewer.Login, nil
}

func migrateToken(hostname, username string, tokenSource tokenSource) error {
	// If token is not currently stored in the keyring do not migrate it,
	// as it is being stored in the config and is being handled when migrating the config.
	if !tokenSource.inKeyring {
		return nil
	}
	return keyring.Set(keyringServiceName(hostname), username, tokenSource.token)
}

func migrateConfig(c *config.Config, hostname, username string) error {
	// Set the user key in case it was previously an anonymous user.
	c.Set(append(hostsKey, hostname, "user"), username)
	// Create the username key with an empty value so it will be
	// written even if there are no keys set under it.
	c.Set(append(hostsKey, hostname, "users", username), "")

	insecureToken, err := c.Get(append(hostsKey, hostname, "oauth_token"))
	var keyNotFoundError *config.KeyNotFoundError
	// If there is no token then we're done here
	if errors.As(err, &keyNotFoundError) {
		return nil
	}

	// If there's another error (current implementation doesn't have any other error but we'll be defensive)
	// then bubble something up.
	if err != nil {
		return fmt.Errorf("couldn't get oauth token for %s: %s", hostname, err)
	}

	// Otherwise we'll set the token under the new key
	c.Set(append(hostsKey, hostname, "users", username, "oauth_token"), insecureToken)
	return nil
}

func keyringServiceName(hostname string) string {
	return "gh:" + hostname
}
