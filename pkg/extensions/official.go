package extensions

import (
	"strings"

	"github.com/cli/cli/v2/internal/ghrepo"
)

// OfficialExtension describes a GitHub-owned CLI extension that can be
// suggested to users when they invoke an unknown command.
type OfficialExtension struct {
	Name  string
	Owner string
	Repo  string
}

// Repository returns a ghrepo.Interface pinned to github.com so that GHES
// users install from github.com rather than their enterprise host.
func (e *OfficialExtension) Repository() ghrepo.Interface {
	return ghrepo.NewWithHost(e.Owner, e.Repo, "github.com")
}

// OfficialExtensions is the registry of GitHub-owned extensions that gh will
// offer to install when the user invokes the corresponding command name.
var OfficialExtensions = []OfficialExtension{
	{Name: "aw", Owner: "github", Repo: "gh-aw"},
	{Name: "stack", Owner: "github", Repo: "gh-stack"},
}

// IsOfficial reports whether the given extension command name and owner
// match an entry in the OfficialExtensions registry. Owner must be
// checked alongside name because a user may have installed a third-party
// extension that happens to share a name with one of ours (e.g.
// `someuser/gh-stack` predates `github/gh-stack` becoming official).
// Owner will be empty for local extensions, in which case the extension
// is treated as non-official.
//
// Comparison is case-sensitive: on case-sensitive filesystems a user can
// install a private extension whose name differs only in casing (e.g.
// `gh-STACK`), and we must not treat that as official. Owner comparison
// is case-insensitive because GitHub usernames and organization names
// are themselves case-insensitive.
func IsOfficial(name, owner string) bool {
	if owner == "" {
		return false
	}
	for _, ext := range OfficialExtensions {
		if ext.Name == name && strings.EqualFold(ext.Owner, owner) {
			return true
		}
	}
	return false
}
