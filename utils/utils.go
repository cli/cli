package utils

import (
	"errors"
	"os"
	"os/exec"
	"runtime"

	"github.com/kballard/go-shellquote"
)

func OpenInBrowser(url string) error {
	browser := os.Getenv("BROWSER")
	if browser == "" {
		browser = searchBrowserLauncher(runtime.GOOS)
	} else {
		browser = os.ExpandEnv(browser)
	}

	if browser == "" {
		return errors.New("Please set $BROWSER to a web launcher")
	}

	browserArgs, err := shellquote.Split(browser)
	if err != nil {
		return err
	}

	endingArgs := append(browserArgs[1:], url)
	browseCmd := exec.Command(browserArgs[0], endingArgs...)
	return PrepareCmd(browseCmd).Run()
}

func searchBrowserLauncher(goos string) (browser string) {
	switch goos {
	case "darwin":
		browser = "open"
	case "windows":
		browser = "cmd /c start"
	default:
		candidates := []string{"xdg-open", "cygstart", "x-www-browser", "firefox",
			"opera", "mozilla", "netscape"}
		for _, b := range candidates {
			path, err := exec.LookPath(b)
			if err == nil {
				browser = path
				break
			}
		}
	}

	return browser
}
