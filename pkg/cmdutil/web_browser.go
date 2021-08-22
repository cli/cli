package cmdutil

import (
	"io"
	"os/exec"

	"github.com/cli/browser"
	"github.com/cli/safeexec"
	"github.com/google/shlex"
)

func NewBrowser(launcher string, stdout, stderr io.Writer) Browser {
	return &webBrowser{
		launcher: launcher,
		stdout:   stdout,
		stderr:   stderr,
	}
}

type webBrowser struct {
	launcher string
	stdout   io.Writer
	stderr   io.Writer
}

func (b *webBrowser) Browse(url string) error {
	if b.launcher != "" {
		launcherArgs, err := shlex.Split(b.launcher)
		if err != nil {
			return err
		}
		launcherExe, err := safeexec.LookPath(launcherArgs[0])
		if err != nil {
			return err
		}
		args := append(launcherArgs[1:], url)
		cmd := exec.Command(launcherExe, args...)
		cmd.Stdout = b.stdout
		cmd.Stderr = b.stderr
		return cmd.Run()
	}

	return browser.OpenURL(url)
}

type TestBrowser struct {
	urls []string
}

func (b *TestBrowser) Browse(url string) error {
	b.urls = append(b.urls, url)
	return nil
}

func (b *TestBrowser) BrowsedURL() string {
	if len(b.urls) > 0 {
		return b.urls[0]
	}
	return ""
}

type _testing interface {
	Errorf(string, ...interface{})
	Helper()
}

func (b *TestBrowser) Verify(t _testing, url string) {
	t.Helper()
	if url != "" {
		switch len(b.urls) {
		case 0:
			t.Errorf("expected browser to open URL %q, but it was never invoked", url)
		case 1:
			if url != b.urls[0] {
				t.Errorf("expected browser to open URL %q, got %q", url, b.urls[0])
			}
		default:
			t.Errorf("expected browser to open one URL, but was invoked %d times", len(b.urls))
		}
	} else if len(b.urls) > 0 {
		t.Errorf("expected no browser to open, but was invoked %d times: %v", len(b.urls), b.urls)
	}
}
