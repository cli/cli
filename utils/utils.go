package utils

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"

	md "github.com/github/gh-cli/markdown"
	"github.com/kballard/go-shellquote"
)

var mdCompiler md.Compiler

func init() {
	mdCompiler = md.Compiler{
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

func normalizeNewlines(d []byte) []byte {
	// from https://github.com/MichaelMure/go-term-markdown/issues/1#issuecomment-570702862
	// replace CR LF \r\n (windows) with LF \n (unix)
	d = bytes.Replace(d, []byte{13, 10}, []byte{10}, -1)
	// replace CF \r (mac) with LF \n (unix)
	d = bytes.Replace(d, []byte{13}, []byte{10}, -1)
	return d
}

func RenderMarkdown(text string) string {
	textB := []byte(text)
	textB = normalizeNewlines(textB)

	return mdCompiler.Compile(string(textB))
}
