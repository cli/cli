package gitcredentials

import (
	"fmt"
	"strings"

	"github.com/cli/cli/v2/internal/ghinstance"
)

type FakeHelperConfig struct {
	SelfExecutablePath string
	Helpers            map[string]Helper
}

// ConfigureOurs sets up the git credential helper chain to use the GitHub CLI credential helper for git repositories
// including gists.
func (hc *FakeHelperConfig) ConfigureOurs(hostname string) error {
	credHelperKeys := []string{
		keyFor(hostname),
	}

	gistHost := strings.TrimSuffix(ghinstance.GistHost(hostname), "/")
	if strings.HasPrefix(gistHost, "gist.") {
		credHelperKeys = append(credHelperKeys, keyFor(gistHost))
	}

	for _, credHelperKey := range credHelperKeys {
		hc.Helpers[credHelperKey] = Helper{
			Cmd: fmt.Sprintf("!%s auth git-credential", shellQuote(hc.SelfExecutablePath)),
		}
	}

	return nil
}

// ConfiguredHelper returns the configured git credential helper for a given hostname.
func (hc *FakeHelperConfig) ConfiguredHelper(hostname string) (Helper, error) {
	helper, ok := hc.Helpers[keyFor(hostname)]
	if ok {
		return helper, nil
	}

	helper, ok = hc.Helpers["credential.helper"]
	if ok {
		return helper, nil
	}

	return Helper{}, nil
}
