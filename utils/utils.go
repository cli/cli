package utils

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/kballard/go-shellquote"
	md "github.com/vilmibm/go-termd"
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

func normalizeNewlines(d []byte) []byte {
	d = bytes.Replace(d, []byte("\r\n"), []byte("\n"), -1)
	d = bytes.Replace(d, []byte("\r"), []byte("\n"), -1)
	return d
}

func RenderMarkdown(text string) string {
	textB := []byte(text)
	textB = normalizeNewlines(textB)
	mdCompiler := md.Compiler{
		Columns: 100,
		SyntaxHighlighter: md.SyntaxTheme{
			"keyword": md.Style{Color: "#9196ed"},
			"comment": md.Style{
				Color: "#c0c0c2",
			},
			"literal": md.Style{
				Color: "#aaedf7",
			},
			"name": md.Style{
				Color: "#fe8eb5",
			},
		},
	}

	return mdCompiler.Compile(string(textB))
}

func Pluralize(num int, thing string) string {
	if num == 1 {
		return fmt.Sprintf("%d %s", num, thing)
	} else {
		return fmt.Sprintf("%d %ss", num, thing)
	}
}

func FuzzyAgo(ago time.Duration) string {
	if ago < time.Minute {
		return "less than a minute ago"
	}
	if ago < time.Hour {
		return fmt.Sprintf("about %s ago", Pluralize(int(ago.Minutes()), "minute"))
	}
	if ago < 24*time.Hour {
		return fmt.Sprintf("about %s ago", Pluralize(int(ago.Hours()), "hour"))
	}

	return fmt.Sprintf("about %s ago", Pluralize(int(ago.Hours()/24), "day"))
}
