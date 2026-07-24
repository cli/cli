package extensions

import (
	"io"

	"github.com/cli/cli/v2/internal/ghrepo"
)

type ExtTemplateType int

const (
	GitTemplateType      ExtTemplateType = 0
	GoBinTemplateType    ExtTemplateType = 1
	OtherBinTemplateType ExtTemplateType = 2
)

//go:generate moq -rm -out extension_mock.go . Extension
type Extension interface {
	Name() string // Extension Name without gh-
	Path() string // Path to executable
	URL() string
	CurrentVersion() string
	LatestVersion() string
	IsPinned() bool
	UpdateAvailable() bool
	IsBinary() bool
	IsLocal() bool
	Owner() string
}

// UpgradeOptions configures how installed extensions are upgraded.
type UpgradeOptions struct {
	// Force upgrades the extension even when it is pinned or already up to date.
	Force bool
	// LatestPreRelease upgrades to the most recent release, including pre-releases,
	// selected by version order. Only supported for binary extensions.
	LatestPreRelease bool
	// PinVersion, when set, upgrades to a specific release tag and pins the
	// extension to it. Only supported for a single named binary extension.
	PinVersion string
}

//go:generate moq -rm -out manager_mock.go . ExtensionManager
type ExtensionManager interface {
	List() []Extension
	Install(ghrepo.Interface, string) error
	InstallLocal(dir string) error
	Upgrade(name string, opts UpgradeOptions) error
	Remove(name string) error
	Dispatch(args []string, stdin io.Reader, stdout, stderr io.Writer) (bool, error)
	Create(name string, tmplType ExtTemplateType) error
	EnableDryRunMode()
	UpdateDir(name string) string
}
