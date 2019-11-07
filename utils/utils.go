package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/github/gh-cli/ui"
	"github.com/kballard/go-shellquote"
)

var timeNow = time.Now

func Check(err error) {
	if err != nil {
		ui.Errorln(err)
		os.Exit(1)
	}
}

func ConcatPaths(paths ...string) string {
	return strings.Join(paths, "/")
}

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

func CommandPath(cmd string) (string, error) {
	if runtime.GOOS == "windows" {
		cmd = cmd + ".exe"
	}

	path, err := exec.LookPath(cmd)
	if err != nil {
		return "", err
	}

	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(path)
}

func TimeAgo(t time.Time) string {
	duration := timeNow().Sub(t)
	minutes := duration.Minutes()
	hours := duration.Hours()
	days := hours / 24
	months := days / 30
	years := months / 12

	var val int
	var unit string

	if minutes < 1 {
		return "now"
	} else if hours < 1 {
		val = int(minutes)
		unit = "minute"
	} else if days < 1 {
		val = int(hours)
		unit = "hour"
	} else if months < 1 {
		val = int(days)
		unit = "day"
	} else if years < 1 {
		val = int(months)
		unit = "month"
	} else {
		val = int(years)
		unit = "year"
	}

	var plural string
	if val > 1 {
		plural = "s"
	}
	return fmt.Sprintf("%d %s%s ago", val, unit, plural)
}
