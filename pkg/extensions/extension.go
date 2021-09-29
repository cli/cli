package extensions

import (
	"io"

	"github.com/cli/cli/v2/internal/ghrepo"
)

//go:generate moq -rm -out extension_mock.go . Extension
type Extension interface {
	Name() string // Extension Name without gh-
	Path() string // Path to executable
	URL() string
	IsLocal() bool
	UpdateAvailable() bool
}

//go:generate moq -rm -out manager_mock.go . ExtensionManager
type ExtensionManager interface {
	List(includeMetadata bool) []Extension
	Install(ghrepo.Interface) error
	InstallLocal(dir string) error
	Upgrade(name string, force bool) error
	Remove(name string) error
	Dispatch(args []string, stdin io.Reader, stdout, stderr io.Writer) (bool, error)
	Create(name string) error
}
