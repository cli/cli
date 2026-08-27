package browser

import (
	"io"
	"os"
	"os/exec"

	ghBrowser "github.com/cli/go-gh/v2/pkg/browser"
	"github.com/cli/go-gh/v2/pkg/config"
)

// Browser opens a URL in a web browser.
type Browser interface {
	Browse(string) error
}

type execFunc func(name string, arg ...string) error

type statFunc func(name string) (os.FileInfo, error)

// fallbackBrowser retries with a specific installed browser when the default
// macOS URL opener fails. `open <url>` goes through Launch Services, and Chrome
// can leave that HTTP/HTTPS mapping pointing at itself even after another
// browser is set as the default. If Chrome is then missing, `open` fails even
// though Edge or Safari is installed. `open -a` targets the .app directly and
// bypasses the broken scheme mapping.
type fallbackBrowser struct {
	inner Browser
	exec  execFunc
	stat  statFunc
	apps  []string
	dirs  []string
}

// New returns a Browser. If launcher, GH_BROWSER, BROWSER, and the browser
// config are all empty, a failed default open is retried against installed
// browsers on macOS.
func New(launcher string, stdout, stderr io.Writer) Browser {
	inner := ghBrowser.New(launcher, stdout, stderr)
	if hasExplicitLauncher(launcher) {
		return inner
	}
	return &fallbackBrowser{
		inner: inner,
		exec: func(name string, arg ...string) error {
			cmd := exec.Command(name, arg...)
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			return cmd.Run()
		},
		stat: os.Stat,
		apps: fallbackApps,
		dirs: applicationDirs(),
	}
}

func (b *fallbackBrowser) Browse(url string) error {
	err := b.inner.Browse(url)
	if err == nil {
		return nil
	}
	if fallbackErr := openFirstInstalledApp(url, b.apps, b.dirs, b.exec, b.stat); fallbackErr == nil {
		return nil
	}
	return err
}

func hasExplicitLauncher(launcher string) bool {
	if launcher != "" {
		return true
	}
	if os.Getenv("GH_BROWSER") != "" || os.Getenv("BROWSER") != "" {
		return true
	}
	cfg, err := config.Read(nil)
	if err == nil {
		if cfgBrowser, _ := cfg.Get([]string{"browser"}); cfgBrowser != "" {
			return true
		}
	}
	return false
}
