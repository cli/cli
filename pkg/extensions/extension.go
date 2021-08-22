package extensions

import (
	"io"
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
	Install(url string, stdout, stderr io.Writer) error
	InstallLocal(dir string) error
	Upgrade(name string, force bool, stdout, stderr io.Writer) error
	Remove(name string) error
	Dispatch(args []string, stdin io.Reader, stdout, stderr io.Writer) (bool, error)
}
