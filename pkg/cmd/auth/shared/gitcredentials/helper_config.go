package gitcredentials

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/google/shlex"
)

// A HelperConfig is used to configure and inspect the state of git credential helpers.
type HelperConfig struct {
	SelfExecutablePath string
	GitClient          *git.Client
}

// ConfigureOurs sets up the git credential helper chain to use the GitHub CLI credential helper for git repositories
// including gists.
func (hc *HelperConfig) ConfigureOurs(hostname string) error {
	ctx := context.TODO()

	credHelperKeys := []string{
		keyFor(hostname),
	}

	gistHost := strings.TrimSuffix(ghinstance.GistHost(hostname), "/")
	if strings.HasPrefix(gistHost, "gist.") {
		credHelperKeys = append(credHelperKeys, keyFor(gistHost))
	}

	var configErr error

	for _, credHelperKey := range credHelperKeys {
		if configErr != nil {
			break
		}
		// first use a blank value to indicate to git we want to sever the chain of credential helpers
		preConfigureCmd, err := hc.GitClient.Command(ctx, "config", "--global", "--replace-all", credHelperKey, "")
		if err != nil {
			configErr = err
			break
		}
		if _, err = preConfigureCmd.Output(); err != nil {
			configErr = err
			break
		}

		// second configure the actual helper for this host
		configureCmd, err := hc.GitClient.Command(ctx,
			"config", "--global", "--add",
			credHelperKey,
			fmt.Sprintf("!%s auth git-credential", shellQuote(hc.SelfExecutablePath)),
		)
		if err != nil {
			configErr = err
		} else {
			_, configErr = configureCmd.Output()
		}
	}

	return configErr
}

// A Helper represents a git credential helper configuration.
type Helper struct {
	Cmd string
}

// IsConfigured returns true if the helper has a non-empty command, i.e. the git config had an entry
func (h Helper) IsConfigured() bool {
	return h.Cmd != ""
}

// IsOurs returns true if the helper command is the GitHub CLI credential helper
func (h Helper) IsOurs() bool {
	if !strings.HasPrefix(h.Cmd, "!") {
		return false
	}

	args, err := shlex.Split(h.Cmd[1:])
	if err != nil || len(args) == 0 {
		return false
	}

	return strings.TrimSuffix(filepath.Base(args[0]), ".exe") == "gh"
}

// ConfiguredHelper returns the configured git credential helper for a given hostname.
func (hc *HelperConfig) ConfiguredHelper(hostname string) (Helper, error) {
	ctx := context.TODO()

	hostHelperCmd, err := hc.GitClient.Config(ctx, keyFor(hostname))
	if hostHelperCmd != "" {
		// TODO: This is a direct refactoring removing named and naked returns
		// but we should probably look closer at the error handling here
		return Helper{
			Cmd: hostHelperCmd,
		}, err
	}

	globalHelperCmd, err := hc.GitClient.Config(ctx, "credential.helper")
	if globalHelperCmd != "" {
		return Helper{
			Cmd: globalHelperCmd,
		}, err
	}

	return Helper{}, nil
}

func keyFor(hostname string) string {
	host := strings.TrimSuffix(ghinstance.HostPrefix(hostname), "/")
	return fmt.Sprintf("credential.%s.helper", host)
}

func shellQuote(s string) string {
	if strings.ContainsAny(s, " $\\") {
		return "'" + s + "'"
	}
	return s
}
