package login

import (
	"bufio"
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const tokenUser = "x-access-token"

type config interface {
	ActiveToken(string) (string, string)
	ActiveUser(string) (string, error)
	TokenForUser(hostname, user string) (string, string, error)
}

type CredentialOptions struct {
	IO     *iostreams.IOStreams
	Config func() (config, error)

	Operation string
}

func NewCmdCredential(f *cmdutil.Factory, runF func(*CredentialOptions) error) *cobra.Command {
	opts := &CredentialOptions{
		IO: f.IOStreams,
		Config: func() (config, error) {
			cfg, err := f.Config()
			if err != nil {
				return nil, err
			}
			return cfg.Authentication(), nil
		},
	}

	cmd := &cobra.Command{
		Use:    "git-credential",
		Args:   cobra.ExactArgs(1),
		Short:  "Implements git credential helper protocol",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Operation = args[0]

			if runF != nil {
				return runF(opts)
			}
			return helperRun(opts)
		},
	}

	return cmd
}

func helperRun(opts *CredentialOptions) error {
	if opts.Operation == "store" {
		// We pretend to implement the "store" operation, but do nothing since we already have a cached token.
		return nil
	}

	if opts.Operation == "erase" {
		// We pretend to implement the "erase" operation, but do nothing since we don't want git to cause user to be logged out.
		return nil
	}

	if opts.Operation != "get" {
		return fmt.Errorf("gh auth git-credential: %q operation not supported", opts.Operation)
	}

	wants := map[string]string{}

	s := bufio.NewScanner(opts.IO.In)
	for s.Scan() {
		line := s.Text()
		if line == "" {
			break
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) < 2 {
			continue
		}
		key, value := parts[0], parts[1]
		if key == "url" {
			u, err := url.Parse(value)
			if err != nil {
				return err
			}
			wants["protocol"] = u.Scheme
			wants["host"] = u.Host
			wants["path"] = u.Path
			wants["username"] = u.User.Username()
			wants["password"], _ = u.User.Password()
		} else {
			wants[key] = value
		}
	}
	if err := s.Err(); err != nil {
		return err
	}

	if wants["protocol"] != "https" {
		return cmdutil.SilentError
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	lookupHost := wants["host"]
	var gotUser string
	var gotToken string
	var source string

	// Helper function to try both the original host and the gist-prefixed variant
	tryBothHosts := func(host string, lookup func(string) (string, string, error)) (string, string, error) {
		token, src, err := lookup(host)
		if err == nil && token != "" {
			return token, src, nil
		}
		// If the host starts with "gist.", try without the prefix
		if strings.HasPrefix(host, "gist.") {
			strippedHost := strings.TrimPrefix(host, "gist.")
			fallbackToken, fallbackSrc, fallbackErr := lookup(strippedHost)
			if fallbackErr == nil && fallbackToken != "" {
				return fallbackToken, fallbackSrc, nil
			}
			return fallbackToken, fallbackSrc, fallbackErr
		}
		return token, src, err
	}

	// Helper to wrap ActiveToken for use with tryBothHosts
	activeTokenLookup := func(host string) (string, string, error) {
		token, src := cfg.ActiveToken(host)
		if token == "" {
			return "", "", fmt.Errorf("no token")
		}
		return token, src, nil
	}

	// Check if environment variables provide a token (highest priority)
	envToken, envSource, _ := tryBothHosts(lookupHost, activeTokenLookup)

	// If environment token is found, use it regardless of username
	if strings.HasSuffix(envSource, "_TOKEN") {
		gotToken = envToken
		source = envSource
		gotUser = tokenUser
	} else if wants["username"] != "" {
		// No environment token, look up token for specific user
		var err error
		gotToken, source, err = tryBothHosts(lookupHost, func(host string) (string, string, error) {
			return cfg.TokenForUser(host, wants["username"])
		})
		if err == nil {
			gotUser = wants["username"]
		}

		// If user-specific token lookup failed, fall back to active token/user
		// We intentionally discard the error here since an empty token will be
		// caught by the validation check below (line 189)
		if gotToken == "" {
			gotToken, source, _ = tryBothHosts(lookupHost, activeTokenLookup)
		}
	} else {
		// No username provided, use active token
		gotToken = envToken
		source = envSource
	}

	// Determine the username based on token source
	if gotUser == "" {
		if strings.HasSuffix(source, "_TOKEN") {
			gotUser = tokenUser
		} else {
			gotUser, _ = cfg.ActiveUser(lookupHost)
			if gotUser == "" && strings.HasPrefix(lookupHost, "gist.") {
				strippedHost := strings.TrimPrefix(lookupHost, "gist.")
				gotUser, _ = cfg.ActiveUser(strippedHost)
			}
			if gotUser == "" {
				gotUser = tokenUser
			}
		}
	}

	if gotUser == "" || gotToken == "" {
		return cmdutil.SilentError
	}

	fmt.Fprint(opts.IO.Out, "protocol=https\n")
	fmt.Fprintf(opts.IO.Out, "host=%s\n", wants["host"])
	fmt.Fprintf(opts.IO.Out, "username=%s\n", gotUser)
	fmt.Fprintf(opts.IO.Out, "password=%s\n", gotToken)

	return nil
}
