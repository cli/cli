package utils

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/charmbracelet/glamour"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/browser"
)

// OpenInBrowser opens the url in a web browser based on OS and $BROWSER environment variable
func OpenInBrowser(url string) error {
	browseCmd, err := browser.Command(url)
	if err != nil {
		return err
	}
	return run.PrepareCmd(browseCmd).Run()
}

func RenderMarkdown(text string) (string, error) {
	style := "notty"
	// TODO: make color an input parameter
	if isColorEnabled() {
		style = "auto"
	}

	// Glamour rendering preserves carriage return characters in code blocks, but
	// we need to ensure that no such characters are present in the output.
	text = strings.ReplaceAll(text, "\r\n", "\n")

	tr, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		// glamour.WithBaseURL(""),  // TODO: make configurable
		// glamour.WithWordWrap(80), // TODO: make configurable
	)
	if err != nil {
		return "", err
	}

	return tr.Render(text)
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

func Humanize(s string) string {
	// Replaces - and _ with spaces.
	replace := "_-"
	h := func(r rune) rune {
		if strings.ContainsRune(replace, r) {
			return ' '
		}
		return r
	}

	return strings.Map(h, s)
}

// We do this so we can stub out the spinner in tests -- it made things really flakey. this is not
// an elegant solution.
var StartSpinner = func(s *spinner.Spinner) {
	s.Start()
}

var StopSpinner = func(s *spinner.Spinner) {
	s.Stop()
}

func Spinner(w io.Writer) *spinner.Spinner {
	return spinner.New(spinner.CharSets[11], 400*time.Millisecond, spinner.WithWriter(w))
}
