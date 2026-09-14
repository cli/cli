package login

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const tokenUser = "x-access-token"

type config interface {
	ActiveTokenWithRefresh(string) (gh.Credential, gh.RefreshStatus, error)
	ActiveUser(string) (string, error)
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
	authtypeCapability := false
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
		} else if key == "capability[]" {
			if value == "authtype" {
				authtypeCapability = true
			}
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
	// ActiveTokenWithRefresh refreshes an expired or near-expiry short-lived token before
	// handing it to git; for non-refreshable tokens it is a no-op that just returns them.
	cred, refreshStatus, _ := cfg.ActiveTokenWithRefresh(lookupHost)
	if cred.Token == "" && strings.HasPrefix(lookupHost, "gist.") {
		lookupHost = strings.TrimPrefix(lookupHost, "gist.")
		cred, refreshStatus, _ = cfg.ActiveTokenWithRefresh(lookupHost)
	}

	// A rejected refresh token can only be recovered by re-authenticating. Notify the user on
	// stderr, which git surfaces, and hand git no credential rather than one that is already dead.
	if refreshStatus == gh.RefreshStatusExpired {
		fmt.Fprintf(opts.IO.ErrOut, "the token for %s has expired; please run 'gh auth login' to re-authenticate\n", lookupHost)
		return cmdutil.SilentError
	}

	gotToken, source := cred.Token, cred.Source

	if strings.HasSuffix(source, "_TOKEN") {
		gotUser = tokenUser
	} else {
		gotUser, _ = cfg.ActiveUser(lookupHost)
		if gotUser == "" {
			gotUser = tokenUser
		}
	}

	if gotUser == "" || gotToken == "" {
		return cmdutil.SilentError
	}

	if wants["username"] != "" && gotUser != tokenUser && !strings.EqualFold(wants["username"], gotUser) {
		return cmdutil.SilentError
	}

	// For a short-lived, refreshable token we want to stop a caching helper chained in front of gh
	// from persisting it, since a cached copy goes stale as soon as the token is refreshed or
	// replaced out of band (for example by gh auth refresh or gh auth login). git offers exactly
	// one mechanism for this: under the authtype capability (git 2.46 and newer) a helper may mark a
	// credential ephemeral, and caching helpers such as git-credential-cache then refuse to store
	// it. See https://git-scm.com/docs/git-credential/2.46.0#IOFMT for the ephemeral attribute. We
	// deliberately do not use password_expiry_utc instead: it only makes a cache drop a
	// naturally expired token, does nothing for out-of-band replacement, and advertising a
	// far-future expiry would make a cache trust a stale token even longer than its own TTL. When a
	// token is rejected git also sends erase to every helper and re-consults gh on the next call, so
	// staleness self-heals regardless.
	//
	// Whether a caching helper sits in front of gh is the user's own git configuration; gh only
	// configures itself as the credential helper for its hosts, so this is an uncommon setup.
	if authtypeCapability && cred.IsRefreshable() {
		// Express the credential through the opaque credential field so we can also mark it
		// ephemeral. authtype=Basic with base64(user:token) produces the same Authorization header
		// as the username/password form, so this is byte-identical Basic auth on the wire.
		encoded := base64.StdEncoding.EncodeToString([]byte(gotUser + ":" + gotToken))
		fmt.Fprint(opts.IO.Out, "capability[]=authtype\n")
		fmt.Fprint(opts.IO.Out, "protocol=https\n")
		fmt.Fprintf(opts.IO.Out, "host=%s\n", wants["host"])
		fmt.Fprintf(opts.IO.Out, "username=%s\n", gotUser)
		fmt.Fprint(opts.IO.Out, "authtype=Basic\n")
		fmt.Fprintf(opts.IO.Out, "credential=%s\n", encoded)
		fmt.Fprint(opts.IO.Out, "ephemeral=1\n")
		return nil
	}

	if cred.IsRefreshable() {
		// Older git does not offer the authtype capability, so we cannot mark the token ephemeral.
		// Warn in case a caching helper is chained in front of gh and would serve a stale token.
		fmt.Fprint(opts.IO.ErrOut, "WARNING: gh: this git version cannot mark the short-lived token as non-cacheable; upgrade to git 2.46 or newer, or avoid a credential caching helper, to prevent a stale token being reused\n")
	}

	fmt.Fprint(opts.IO.Out, "protocol=https\n")
	fmt.Fprintf(opts.IO.Out, "host=%s\n", wants["host"])
	fmt.Fprintf(opts.IO.Out, "username=%s\n", gotUser)
	fmt.Fprintf(opts.IO.Out, "password=%s\n", gotToken)

	return nil
}
