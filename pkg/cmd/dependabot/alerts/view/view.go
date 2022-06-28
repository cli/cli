package view

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/dependabot/alerts/list"
	dependabot "github.com/cli/cli/v2/pkg/cmd/dependabot/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/cli/cli/v2/utils"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/MakeNowJust/heredoc"
	cvss "github.com/goark/go-cvss/v3/metric"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	HttpClient      func() (*http.Client, error)
	Config          func() (config.Config, error)
	IO              *iostreams.IOStreams
	BaseRepo        func() (ghrepo.Interface, error)
	HasRepoOverride bool
	Browser         cmdutil.Browser

	ID  string
	Web bool
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := ViewOptions{
		HttpClient: f.HttpClient,
		Config:     f.Config,
		IO:         f.IOStreams,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "view  [<number> | <url> | <id>]",
		Short: "View Dependabot alert for a repository",
		Long: heredoc.Doc(`
            Display the title, body, and other information about a Dependabot security alert.
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			opts.HasRepoOverride = c.Flags().Changed("repo")

			if len(args) > 0 {
				opts.ID = args[0]
			} else {
				return list.ListRun(&list.ListOptions{
					HttpClient: opts.HttpClient,
					Config:     opts.Config,
					IO:         opts.IO,
					BaseRepo:   opts.BaseRepo,
					Browser:    opts.Browser,
					State:      "all",
					Web:        opts.Web,
				})
			}

			if runF != nil {
				return runF(&opts)
			}
			return viewRun(&opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open configuration in the browser")

	return cmd
}

func viewRun(opts *ViewOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	client, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}

	var alert *dependabot.VulnerabilityAlert
	var fetchError error

	opts.IO.StartProgressIndicator()
	if urlRepo, alertNumber, err := parseURL(opts.ID); err == nil {
		if opts.HasRepoOverride && !ghrepo.IsSame(repo, *urlRepo) {
			return fmt.Errorf("incompatible alert URL for repo %s", ghrepo.FullName(repo))
		}

		alert, fetchError = dependabot.FetchRepositoryAlertWithMatcher(client, *urlRepo, func(va dependabot.VulnerabilityAlert) bool {
			return va.Number == alertNumber
		})
	} else if alertNumber, err := parseNumber(opts.ID); err == nil {
		alert, fetchError = dependabot.FetchRepositoryAlertWithMatcher(client, repo, func(va dependabot.VulnerabilityAlert) bool {
			return va.Number == alertNumber
		})
	} else {
		alert, fetchError = dependabot.FetchRepositoryAlertWithMatcher(client, repo, func(va dependabot.VulnerabilityAlert) bool {
			for _, identifier := range alert.SecurityVulnerability.Advisory.Identifiers {
				if identifier.Value == opts.ID {
					return true
				}
			}

			return false
		})
	}
	opts.IO.StopProgressIndicator()

	if fetchError != nil {
		return fmt.Errorf("could not fetch alert: %w", fetchError)
	}

	if alert == nil {
		return fmt.Errorf("no alert found for %s", opts.ID)
	}

	if opts.Web {
		url := generateRepoSecurityAlertURL(repo, alert)

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", utils.DisplayURL(url))
		}

		return opts.Browser.Browse(url)
	}

	printAlert(opts.IO, alert)

	return nil
}

func formatTime(t time.Time, isTTY bool) string {
	if isTTY {
		return utils.FuzzyAgoAbbr(time.Now(), t)
	} else {
		return t.Format(time.RFC3339)
	}
}

func printAlert(io *iostreams.IOStreams, alert *dependabot.VulnerabilityAlert) {
	out := io.Out
	cs := io.ColorScheme()

	advisory := alert.SecurityVulnerability.Advisory

	fmt.Fprintf(out, "%s #%d\n",
		cs.Bold(alert.SecurityVulnerability.Advisory.Summary),
		alert.Number,
	)

	severity := alert.SecurityVulnerability.Severity
	severityString := cases.Title(language.English).String(severity.String())

	fmt.Fprintf(out, "%s • %s Severity • Created %s",
		cs.ColorFromString(alert.State.Color())(string(alert.State)),
		cs.ColorFromString(severity.Color())(severityString),
		formatTime(alert.CreatedAt, io.IsStdoutTTY()),
	)
	if !alert.DismissedAt.IsZero() {
		fmt.Fprintf(out, " • Dismissed %s",
			formatTime(alert.DismissedAt, io.IsStdoutTTY()),
		)
	}
	fmt.Fprint(out, "\n")

	fmt.Fprint(out, "\n")

	tp := utils.NewTablePrinter(io)
	tp.AddField("Package", nil, cs.Gray)
	tp.AddField("Affected versions", nil, cs.Gray)
	tp.AddField("Patched versions", nil, cs.Gray)
	tp.EndRow()

	ecosystemString := cases.Lower(language.English).String(string(alert.SecurityVulnerability.Package.Ecosystem))
	tp.AddField(fmt.Sprintf("%s (%s)",
		cs.Bold(alert.SecurityVulnerability.Package.Name),
		ecosystemString,
	), nil, nil)
	tp.AddField(alert.SecurityVulnerability.VulnerableVersionRange, nil, nil)
	if alert.SecurityVulnerability.FirstPatchedVersion.Identifier != "" {
		tp.AddField(alert.SecurityVulnerability.FirstPatchedVersion.Identifier, nil, nil)
	} else {
		tp.AddField("N/A", nil, cs.Yellow)
	}
	tp.EndRow()
	_ = tp.Render()

	fmt.Fprint(out, "\n")

	// CVSS Score and Base Metrics
	fmt.Fprintln(out, cs.Bold("CVSS"))
	if bm, err := cvss.NewBase().Decode(advisory.CVSS.VectorString); err == nil {
		tp = utils.NewTablePrinter(io)

		scoreString := strconv.FormatFloat(advisory.CVSS.Score, 'f', 1, 64)
		tp.AddField("Score", nil, cs.Gray)
		tp.AddField(fmt.Sprintf("%s / 10",
			cs.Bold(scoreString),
		), nil, cs.ColorFromString(severity.Color()))
		tp.EndRow()

		tp.AddField("Attack vector", nil, cs.Gray)
		tp.AddField(attackVectorDescription(bm.AV), nil, nil)
		tp.EndRow()

		tp.AddField("Attack complexity", nil, cs.Gray)
		tp.AddField(attackComplexityDescription(bm.AC), nil, nil)
		tp.EndRow()

		tp.AddField("Privilege required", nil, cs.Gray)
		tp.AddField(privilegesRequiredDescription(bm.PR), nil, nil)
		tp.EndRow()

		tp.AddField("User interaction", nil, cs.Gray)
		tp.AddField(userInteractionDescription(bm.UI), nil, nil)
		tp.EndRow()

		tp.AddField("Scope", nil, cs.Gray)
		tp.AddField(scopeDescription(bm.S), nil, nil)
		tp.EndRow()

		tp.AddField("Confidentiality", nil, cs.Gray)
		tp.AddField(confidentialityImpactDescription(bm.C), nil, nil)
		tp.EndRow()

		tp.AddField("Integrity", nil, cs.Gray)
		tp.AddField(integrityImpactDescription(bm.I), nil, nil)
		tp.EndRow()

		tp.AddField("Availability", nil, cs.Gray)
		tp.AddField(availabilityImpactDescription(bm.A), nil, nil)
		tp.EndRow()

		tp.Render()

		fmt.Fprint(out, "\n")
	} else {
		fmt.Fprintln(out, cs.Gray(advisory.CVSS.VectorString))
		fmt.Fprint(out, "\n")
	}

	// Identifiers
	tp = utils.NewTablePrinter(io)
	for _, identifier := range advisory.Identifiers {
		tp.AddField(fmt.Sprintf("%s ID", identifier.Type), nil, cs.Bold)
		tp.AddField(identifier.Value, nil, nil)
		tp.EndRow()
	}
	tp.Render()

	// Body
	var md string
	var err error
	if advisory.Description == "" {
		md = fmt.Sprintf("\n  %s\n\n", cs.Gray("No description provided"))
	} else {
		md, err = markdown.Render(advisory.Description, markdown.WithIO(io), markdown.WithoutIndentation())
		if err != nil {
			md = fmt.Sprintf("\n  %s\n\n", cs.Gray("Could not render description"))
		}
	}
	fmt.Fprintf(out, "\n%s\n", md)

	// Footer
	fmt.Fprintf(out, cs.Gray("View this advisory on GitHub: %s\n"), alert.SecurityVulnerability.Advisory.Permalink)
}

func generateRepoSecurityAlertURL(repo ghrepo.Interface, alert *dependabot.VulnerabilityAlert) string {
	return ghrepo.GenerateRepoURL(repo, "security/dependabot/"+strconv.Itoa(alert.Number))
}

var alertURLRE = regexp.MustCompile(`^/([^/]+)/([^/]+)/security/dependabot/(\d+)`)

func parseURL(alertURL string) (*ghrepo.Interface, int, error) {
	if alertURL == "" {
		return nil, 0, fmt.Errorf("invalid URL: %q", alertURL)
	}

	u, err := url.Parse(alertURL)
	if err != nil {
		return nil, 0, err
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, 0, fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	m := alertURLRE.FindStringSubmatch(u.Path)
	if m == nil {
		return nil, 0, fmt.Errorf("not a security alert URL: %s", alertURL)
	}

	repo := ghrepo.NewWithHost(m[1], m[2], u.Hostname())
	alertNumber, _ := strconv.Atoi(m[3])
	return &repo, alertNumber, nil
}

func parseNumber(alertIdentifier string) (int, error) {
	var number int
	var err error

	if strings.HasPrefix(alertIdentifier, "#") {
		number, err = strconv.Atoi(alertIdentifier[1:])
	} else {
		number, err = strconv.Atoi(alertIdentifier)
	}

	if number < 1 {
		return 0, fmt.Errorf("invalid alert number: %d", number)
	}

	return number, err
}

func attackVectorDescription(av cvss.AttackVector) string {
	switch av {
	case cvss.AttackVectorPhysical:
		return "Physical"
	case cvss.AttackVectorLocal:
		return "Local"
	case cvss.AttackVectorAdjacent:
		return "Adjacent"
	case cvss.AttackVectorNetwork:
		return "Network"
	case cvss.AttackVectorNotDefined:
		return "Not Defined"
	case cvss.AttackVectorUnknown:
		return "Unknown"
	default:
		return av.String()
	}
}

func attackComplexityDescription(ac cvss.AttackComplexity) string {
	switch ac {
	case cvss.AttackComplexityHigh:
		return "High"
	case cvss.AttackComplexityLow:
		return "Low"
	case cvss.AttackComplexityNotDefined:
		return "Not Defined"
	case cvss.AttackComplexityUnknown:
		return "Unknown"
	default:
		return ac.String()
	}
}

func privilegesRequiredDescription(pr cvss.PrivilegesRequired) string {
	switch pr {
	case cvss.PrivilegesRequiredHigh:
		return "High"
	case cvss.PrivilegesRequiredLow:
		return "Low"
	case cvss.PrivilegesRequiredNone:
		return "None"
	case cvss.PrivilegesRequiredNotDefined:
		return "Not Defined"
	case cvss.PrivilegesRequiredUnknown:
		return "Unknown"
	default:
		return pr.String()
	}
}

func userInteractionDescription(ui cvss.UserInteraction) string {
	switch ui {
	case cvss.UserInteractionRequired:
		return "Required"
	case cvss.UserInteractionNotDefined:
		return "Not Defined"
	case cvss.UserInteractionNone:
		return "None"
	case cvss.UserInteractionUnknown:
		return "Unknown"
	default:
		return ui.String()
	}
}

func scopeDescription(s cvss.Scope) string {
	switch s {
	case cvss.ScopeChanged:
		return "Changed"
	case cvss.ScopeUnchanged:
		return "Unchanged"
	case cvss.ScopeUnknown:
		return "Unknown"
	default:
		return s.String()
	}
}

func confidentialityImpactDescription(c cvss.ConfidentialityImpact) string {
	switch c {
	case cvss.ConfidentialityImpactHigh:
		return "High"
	case cvss.ConfidentialityImpactLow:
		return "Low"
	case cvss.ConfidentialityImpactNone:
		return "None"
	case cvss.ConfidentialityImpactNotDefined:
		return "Not Defined"
	case cvss.ConfidentialityImpactUnknown:
		return "Unknown"
	default:
		return c.String()
	}
}

func integrityImpactDescription(i cvss.IntegrityImpact) string {
	switch i {
	case cvss.IntegrityImpactHigh:
		return "High"
	case cvss.IntegrityImpactLow:
		return "Low"
	case cvss.IntegrityImpactNone:
		return "None"
	case cvss.IntegrityImpactUnknown:
		return "Unknown"
	default:
		return i.String()
	}
}

func availabilityImpactDescription(a cvss.AvailabilityImpact) string {
	switch a {
	case cvss.AvailabilityImpactHigh:
		return "High"
	case cvss.AvailabilityImpactLow:
		return "Low"
	case cvss.AvailabilityImpactNotDefined:
		return "Not Defined"
	case cvss.AvailabilityImpactUnknown:
		return "Unknown"
	default:
		return a.String()
	}
}
