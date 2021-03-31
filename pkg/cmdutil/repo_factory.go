package cmdutil

import (
	"fmt"
	"strings"

	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
)

type fallbackFunc func() (string, error)

// NewRepo extracts the GitHub repsoitory information from the following formated strings:
// * NAME
// * OWNER/NAME
// * HOST/OWNER/NAME
// * Full URL
// NewRepo falls back to provided fall back functions when HOST or OWNER are not specified in the string
// If HOST is not specified and hostFallbackFunc is nil then use "" as HOST
// If OWNER is not specified and ownerFallbackFunc is nil then use "" as OWNER
func NewRepo(nwo string, hostFallbackFunc fallbackFunc, ownerFallbackFunc fallbackFunc) (ghrepo.Interface, error) {
	if git.IsURL(nwo) {
		u, err := git.ParseURL(nwo)
		if err != nil {
			return nil, err
		}
		return ghrepo.FromURL(u)
	}

	parts := strings.SplitN(nwo, "/", 4)
	for _, p := range parts {
		if len(p) == 0 {
			return nil, fmt.Errorf(`expected the "[HOST/]OWNER/REPO" format, got %q`, nwo)
		}
	}
	l := len(parts)

	if l == 0 || l == 4 {
		return nil, fmt.Errorf(`expected the "[HOST/]OWNER/REPO" format, got %q`, nwo)
	}

	if l == 3 {
		return ghrepo.NewWithHost(parts[1], parts[2], parts[0]), nil
	}

	var err error
	var host string
	if hostFallbackFunc != nil {
		host, err = hostFallbackFunc()
		if err != nil {
			return nil, err
		}
	}

	var owner string
	var name string
	if l == 1 {
		name = parts[0]
		if ownerFallbackFunc != nil {
			owner, err = ownerFallbackFunc()
			if err != nil {
				return nil, err
			}
		}
	}

	if l == 2 {
		owner = parts[0]
		name = parts[1]
	}

	return ghrepo.NewWithHost(owner, name, host), nil
}
