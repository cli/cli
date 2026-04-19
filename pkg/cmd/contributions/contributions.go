// Package contributions implements the `gh contributions` command which
// displays a user's GitHub contribution calendar in the terminal.
package contributions

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type hostConfig interface {
	DefaultHost() (string, string)
}

// ContributionsOptions holds the parsed flags and dependencies for the command.
type ContributionsOptions struct {
	HttpClient func() (*http.Client, error)
	HostConfig hostConfig
	IO         *iostreams.IOStreams
	Now        func() time.Time

	User     string
	From     string
	To       string
	Exporter cmdutil.Exporter
}

// NewCmdContributions creates the `gh contributions` command.
func NewCmdContributions(f *cmdutil.Factory, runF func(*ContributionsOptions) error) *cobra.Command {
	opts := &ContributionsOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Now:        time.Now,
	}

	cmd := &cobra.Command{
		Use:   "contributions",
		Short: "Show a user's contribution calendar",
		Long: heredoc.Doc(`
			Display a contribution calendar in the terminal, mirroring the
			heat map shown on a user's GitHub profile.

			By default, the calendar for the authenticated user is shown.
			Pass --user to view contributions for someone else.

			Use --from and --to to constrain the range. Both flags accept
			YYYY-MM-DD or RFC3339 timestamps. The GitHub API allows ranges
			of up to one year.
		`),
		Example: heredoc.Doc(`
			# Show your own contributions over the last year
			$ gh contributions

			# Show another user's contributions
			$ gh contributions --user octocat

			# Restrict to a specific date range
			$ gh contributions --from 2024-01-01 --to 2024-06-30

			# Emit JSON for scripting
			$ gh contributions --json totalContributions,days
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := f.Config()
			if err != nil {
				return err
			}
			opts.HostConfig = cfg.Authentication()

			if runF != nil {
				return runF(opts)
			}
			return contributionsRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.User, "user", "u", "", "GitHub login to show contributions for (defaults to the authenticated user)")
	cmd.Flags().StringVar(&opts.From, "from", "", "Start of the date range (YYYY-MM-DD or RFC3339)")
	cmd.Flags().StringVar(&opts.To, "to", "", "End of the date range (YYYY-MM-DD or RFC3339)")
	cmdutil.AddJSONFlags(cmd, &opts.Exporter, []string{"login", "totalContributions", "from", "to", "days"})

	return cmd
}

func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid date %q: expected YYYY-MM-DD or RFC3339", s)
}

func contributionsRun(opts *ContributionsOptions) error {
	from, err := parseDate(opts.From)
	if err != nil {
		return cmdutil.FlagErrorf("%s", err.Error())
	}
	to, err := parseDate(opts.To)
	if err != nil {
		return cmdutil.FlagErrorf("%s", err.Error())
	}
	if !from.IsZero() && !to.IsZero() && to.Before(from) {
		return cmdutil.FlagErrorf("--to must be on or after --from")
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not create client: %w", err)
	}
	hostname, _ := opts.HostConfig.DefaultHost()

	opts.IO.StartProgressIndicator()
	result, err := fetchContributions(httpClient, hostname, opts.User, from, to)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, exportable(result))
	}

	return renderCalendar(opts.IO, result)
}

// exportable shapes the result for JSON output, flattening days for
// easier consumption.
func exportable(r *ContributionsResult) map[string]any {
	days := make([]ContributionDay, 0)
	for _, w := range r.Calendar.Weeks {
		days = append(days, w.ContributionDays...)
	}
	out := map[string]any{
		"login":              r.Login,
		"totalContributions": r.Calendar.TotalContributions,
		"days":               days,
	}
	if !r.From.IsZero() {
		out["from"] = r.From.Format(time.RFC3339)
	} else {
		out["from"] = nil
	}
	if !r.To.IsZero() {
		out["to"] = r.To.Format(time.RFC3339)
	} else {
		out["to"] = nil
	}
	return out
}

// Hex colors taken from GitHub's dark contribution calendar palette.
var levelHex = map[string]string{
	"NONE":            "39414a",
	"FIRST_QUARTILE":  "0e4429",
	"SECOND_QUARTILE": "006d32",
	"THIRD_QUARTILE":  "26a641",
	"FOURTH_QUARTILE": "39d353",
}

const cellGlyph = "■"

func cell(cs *iostreams.ColorScheme, level string) string {
	hex, ok := levelHex[level]
	if !ok {
		hex = levelHex["NONE"]
	}
	if cs.Enabled && cs.TrueColor {
		r, g, b := hexToRGB(hex)
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm%s\x1b[0m", r, g, b, cellGlyph)
	}
	switch level {
	case "NONE", "":
		return cs.Muted(cellGlyph)
	case "FIRST_QUARTILE", "SECOND_QUARTILE":
		return cs.Green(cellGlyph)
	default:
		return cs.GreenBold(cellGlyph)
	}
}

func hexToRGB(h string) (int, int, int) {
	if len(h) != 6 {
		return 0, 0, 0
	}
	var r, g, b int
	_, _ = fmt.Sscanf(h, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

const dayLabelWidth = 4

func renderCalendar(io *iostreams.IOStreams, r *ContributionsResult) error {
	cs := io.ColorScheme()
	out := io.Out

	weeks := r.Calendar.Weeks
	if width := io.TerminalWidth(); width > 0 {
		// Each week takes 2 columns; reserve dayLabelWidth for the row labels.
		maxWeeks := max((width-dayLabelWidth)/2, 1)
		if len(weeks) > maxWeeks {
			weeks = weeks[len(weeks)-maxWeeks:]
		}
	}

	rangeLabel := "in the last year"
	if !r.From.IsZero() || !r.To.IsZero() {
		fromStr := "..."
		toStr := "..."
		if !r.From.IsZero() {
			fromStr = r.From.Format("2006-01-02")
		}
		if !r.To.IsZero() {
			toStr = r.To.Format("2006-01-02")
		}
		rangeLabel = fmt.Sprintf("from %s to %s", fromStr, toStr)
	}
	who := r.Login
	if who == "" {
		who = "you"
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, " %s %s\n",
		cs.Bold(text.Pluralize(r.Calendar.TotalContributions, "contribution")),
		cs.Mutedf("%s by %s", rangeLabel, who),
	)
	fmt.Fprintln(out)

	if len(weeks) == 0 {
		fmt.Fprintln(out, cs.Muted("No contribution data."))
		return nil
	}

	monthRow := strings.Repeat(" ", dayLabelWidth)
	monthRow += renderMonthLabels(weeks)
	fmt.Fprintln(out, monthRow)

	dayLabels := []string{"", "M", "", "W", "", "F", ""}
	for d := range 7 {
		var sb strings.Builder
		label := dayLabels[d]
		if label == "" {
			sb.WriteString(strings.Repeat(" ", dayLabelWidth))
		} else {
			sb.WriteString(cs.Muted(label))
			sb.WriteString(strings.Repeat(" ", dayLabelWidth-len(label)))
		}
		for _, w := range weeks {
			if d < len(w.ContributionDays) {
				sb.WriteString(cell(cs, w.ContributionDays[d].ContributionLevel))
			} else {
				sb.WriteString(" ")
			}
			sb.WriteString(" ")
		}
		fmt.Fprintln(out, sb.String())
	}

	fmt.Fprintln(out)
	// Legend: "Less ■ ■ ■ ■ ■ More" is 5 + (5*2) + 4 = 19 visible chars.
	const legendWidth = 19
	graphWidth := dayLabelWidth + len(weeks)*2
	padding := max(graphWidth-legendWidth-5, 0)
	var legend strings.Builder
	legend.WriteString(strings.Repeat(" ", padding))
	legend.WriteString(cs.Muted("Less "))
	for _, lvl := range []string{"NONE", "FIRST_QUARTILE", "SECOND_QUARTILE", "THIRD_QUARTILE", "FOURTH_QUARTILE"} {
		legend.WriteString(cell(cs, lvl))
		legend.WriteString(" ")
	}
	legend.WriteString(cs.Muted("More"))
	fmt.Fprintln(out, legend.String())

	return nil
}

// renderMonthLabels writes month abbreviations above the first week of
// each new month. Each week occupies 2 character columns.
func renderMonthLabels(weeks []ContributionWeek) string {
	const colW = 2
	const labelLen = 3
	cols := make([]rune, len(weeks)*colW+labelLen)
	for i := range cols {
		cols[i] = ' '
	}

	var prevMonth time.Month
	for i, w := range weeks {
		if len(w.ContributionDays) == 0 {
			continue
		}
		t, err := time.Parse("2006-01-02", w.ContributionDays[0].Date)
		if err != nil {
			continue
		}
		m := t.Month()
		if m == prevMonth {
			continue
		}
		label := m.String()[:labelLen]
		start := i * colW
		overlaps := false
		for k := 0; k < labelLen && start+k < len(cols); k++ {
			if cols[start+k] != ' ' {
				overlaps = true
				break
			}
		}
		if !overlaps && start+labelLen <= len(cols) {
			for k, ch := range label {
				cols[start+k] = ch
			}
		}
		prevMonth = m
	}
	return strings.TrimRight(string(cols), " ")
}
