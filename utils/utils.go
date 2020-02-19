package utils

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// OpenInBrowser opens the url in a web browser based on OS and $BROWSER environment variable
func OpenInBrowser(url string) error {
	browseCmd, err := browser.Command(url)
	if err != nil {
		return err
	}
	return PrepareCmd(browseCmd).Run()
}

func normalizeNewlines(s string) string {
	s = strings.Replace(s, "\r\n", "\n", -1)
	s = strings.Replace(s, "\r", "\n", -1)
	return s
}

func RenderMarkdown(text string) (string, error) {
	return glamour.Render(normalizeNewlines(text), "dark")
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

func GetTitle(cmd *cobra.Command, cmdType string, limit int, matchCount int, baseRepo *ghrepo.Interface) string {
	userSetFlagCounter := 0
	limitSet := false

	cmd.Flags().Visit(func(f *pflag.Flag) {
		userSetFlagCounter += 1
		if f.Name == "limit" {
			limitSet = true
		}
	})

	title := "\n%s in %s\n\n"
	if matchCount == 0 {
		msg := fmt.Sprintf("There are no open %ss", cmdType)

		if userSetFlagCounter > 0 {
			msg = fmt.Sprintf("No %ss match your search", cmdType)
		}
		return fmt.Sprintf(title, msg, ghrepo.FullName(*baseRepo))
	}

	if (!limitSet && userSetFlagCounter > 0) || (userSetFlagCounter > 1) {
		title = "\n%s match your search in %s\n\n"
	}

	out := fmt.Sprintf(title, Pluralize(matchCount, cmdType), ghrepo.FullName(*baseRepo))

	if limit < matchCount {
		out = out + fmt.Sprintln(Gray(fmt.Sprintf("Showing %d/%d results\n", limit, matchCount)))
	}

	return out
}
