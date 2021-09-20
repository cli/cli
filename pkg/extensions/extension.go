package extensions

import (
	"io"
	"net/http"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
)

//go:generate moq -rm -out extension_mock.go . Extension
type Extension interface {
	Name() string
	Path() string
	URL() string
	IsLocal() bool
	UpdateAvailable() bool
}

//go:generate moq -rm -out manager_mock.go . ExtensionManager
type ExtensionManager interface {
	List(includeMetadata bool) []Extension
	Install(*http.Client, ghrepo.Interface, *iostreams.IOStreams, config.Config) error
	InstallBin(*http.Client, ghrepo.Interface) error
	InstallGit(string, io.Writer, io.Writer) error
	InstallLocal(dir string) error
	Upgrade(name string, force bool, stdout, stderr io.Writer) error
	Remove(name string) error
	Dispatch(args []string, stdin io.Reader, stdout, stderr io.Writer) (bool, error)
	Create(name string) error
}
