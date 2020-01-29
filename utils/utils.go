package utils

import (
	"bytes"
	"fmt"
	"time"

	"github.com/cli/cli/pkg/browser"
	md "github.com/vilmibm/go-termd"
)

// OpenInBrowser opens the url in a web browser based on OS and $BROWSER environment variable
func OpenInBrowser(url string) error {
	browseCmd, err := browser.Command(url)
	if err != nil {
		return err
	}
	return PrepareCmd(browseCmd).Run()
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

func fmtDuration(amount int, unit string) string {
	return fmt.Sprintf("about %s ago", Pluralize(amount, unit))
}

func FuzzyAgo(ago time.Duration) string {
	if ago < time.Minute {
		return "less than a minute ago"
	}
	if ago < time.Hour {
		return fmtDuration(int(ago.Minutes()), "minute")
	}
	if ago < 24*time.Hour {
		return fmtDuration(int(ago.Hours()), "hour")
	}
	if ago < 30*24*time.Hour {
		return fmtDuration(int(ago.Hours())/24, "day")
	}
	if ago < 365*24*time.Hour {
		return fmtDuration(int(ago.Hours())/24/30, "month")
	}

	return fmtDuration(int(ago.Hours()/24/365), "year")
}
