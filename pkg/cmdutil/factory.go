package cmdutil

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type Factory struct {
	IOStreams *iostreams.IOStreams
	Prompter  prompter.Prompter
	Browser   browser.Browser
	GitClient *git.Client

	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Remotes    func() (context.Remotes, error)
	Config     func() (config.Config, error)
	Branch     func() (string, error)

	ExtensionManager extensions.ExtensionManager
	ExecutableName   string
}

// Executable is the path to the currently invoked binary
func (f *Factory) Executable() string {
	ghPath := os.Getenv("GH_PATH")
	if ghPath != "" {
		return ghPath
	}
	if !strings.ContainsRune(f.ExecutableName, os.PathSeparator) {
		f.ExecutableName = executable(f.ExecutableName)
	}
	return f.ExecutableName
}

// Finds the location of the executable for the current process as it's found in PATH, respecting symlinks.
// If the process couldn't determine its location, return fallbackName. If the executable wasn't found in
// PATH, return the absolute location to the program.
//
// The idea is that the result of this function is callable in the future and refers to the same
// installation of gh, even across upgrades. This is needed primarily for Homebrew, which installs software
// under a location such as `/usr/local/Cellar/gh/1.13.1/bin/gh` and symlinks it from `/usr/local/bin/gh`.
// When the version is upgraded, Homebrew will often delete older versions, but keep the symlink. Because of
// this, we want to refer to the `gh` binary as `/usr/local/bin/gh` and not as its internal Homebrew
// location.
//
// None of this would be needed if we could just refer to GitHub CLI as `gh`, i.e. without using an absolute
// path. However, for some reason Homebrew does not include `/usr/local/bin` in PATH when it invokes git
// commands to update its taps. If `gh` (no path) is being used as git credential helper, as set up by `gh
// auth login`, running `brew update` will print out authentication errors as git is unable to locate
// Homebrew-installed `gh`.
func executable(fallbackName string) string {
	exe, err := os.Executable()
	if err != nil {
		return fallbackName
	}

	base := filepath.Base(exe)
	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		p, err := filepath.Abs(filepath.Join(dir, base))
		if err != nil {
			continue
		}
		f, err := os.Lstat(p)
		if err != nil {
			continue
		}

		if p == exe {
			return p
		} else if f.Mode()&os.ModeSymlink != 0 {
			if t, err := os.Readlink(p); err == nil && t == exe {
				return p
			}
		}
	}

	return exe
}
