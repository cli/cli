package list

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	dependabot "github.com/cli/cli/v2/pkg/cmd/dependabot/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/text"
	"github.com/cli/cli/v2/utils"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    cmdutil.Browser

	Org  string
	Repo string

	State string
	Sort  string
	Order string
	Web   bool
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := ListOptions{
		HttpClient: f.HttpClient,
		Config:     f.Config,
		IO:         f.IOStreams,
		Browser:    f.Browser,
		Sort:       "created",
		Order:      "asc",
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lists Dependabot alerts for a repository",
		Long: heredoc.Doc(`
			Lists Dependabot security alerts configuration for a repository,
			By default, open alerts are shown for the current repository.
		`),
		Example: heredoc.Doc(`
			# list Dependabot Alerts for the current repository
			$ gh dependabot alerts list
			# list all Dependabot Alerts for the octocat/hello repository
			$ gh dependabot alerts list --state all --repo octocat/hello
		`),
		Aliases: []string{"ls"},
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if err := cmdutil.MutuallyExclusive("specify only one of `--repo` or `--org`", opts.Repo != "", opts.Org != ""); err != nil {
				return err
			}

			if runF != nil {
				return runF(&opts)
			}
			return ListRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Org, "org", "o", "", "List alerts for an organization")
	cmdutil.StringEnumFlag(cmd, &opts.Sort, "sort", "", "created", []string{"created", "severity", "number"}, "Sort fetched alerts")
	cmdutil.StringEnumFlag(cmd, &opts.Order, "order", "", "desc", []string{"asc", "desc"}, "Order of alerts returned, ignored unless '--sort' flag is specified")
	cmdutil.StringEnumFlag(cmd, &opts.State, "state", "", "open", []string{"open", "fixed", "dismissed", "all"}, "Filter by state")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open configuration in the browser")

	return cmd
}

func ListRun(opts *ListOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}

	client, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}

	if opts.Web {
		var url string
		if opts.Org != "" {
			url = generateOrganizationDependabotAlertsURL(repo, opts.Org, opts.State, opts.Sort, opts.Order)
		} else {
			url = generateRepositoryDependabotAlertsURL(repo, opts.State, opts.Sort, opts.Order)
		}

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", utils.DisplayURL(url))
		}

		return opts.Browser.Browse(url)
	}

	var alerts []dependabot.VulnerabilityAlert
	opts.IO.StartProgressIndicator()
	if opts.Org != "" {
		alerts, err = dependabot.FetchOrganizationVulnerabilityAlerts(client, repo, opts.Org, opts.State)
	} else {
		alerts, err = dependabot.FetchRepositoryVulnerabilityAlerts(client, repo, opts.State)
	}
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	alertsCount := len(alerts)
	if alertsCount == 0 {
		if opts.Org != "" {
			return cmdutil.NewNoResultsError(fmt.Sprintf("no Dependabot alerts found for organization %s", opts.Org))
		} else {
			return cmdutil.NewNoResultsError(fmt.Sprintf("no Dependabot alerts found in %s", ghrepo.FullName(repo)))
		}
	}

	var data sort.Interface
	switch opts.Sort {
	case "severity":
		data = bySeverity(alerts)
	case "number":
		data = byNumber(alerts)
	case "created":
		data = byCreatedAt(alerts)
	}

	switch opts.Order {
	case "asc":
		sort.Sort(data)
	case "desc":
		sort.Sort(sort.Reverse(data))
	}

	if opts.IO.IsStdoutTTY() {
		var subject string
		switch opts.State {
		case "open":
			subject = "open security alert"
		case "fixed":
			subject = "fixed security alert"
		case "dismissed":
			subject = "dismissed security alert"
		default:
			subject = "security alert"
		}

		if opts.Org != "" {
			fmt.Fprintf(opts.IO.Out, "\nShowing %d of %s for %s\n\n", alertsCount, utils.Pluralize(alertsCount, subject), opts.Org)
		} else {
			fmt.Fprintf(opts.IO.Out, "\nShowing %d of %s in %s org\n\n", alertsCount, utils.Pluralize(alertsCount, subject), ghrepo.FullName(repo))
		}
	}

	printAlerts(opts.IO, alerts, opts.Org != "")

	return nil
}

func generateRepositoryDependabotAlertsURL(repo ghrepo.Interface, state string, sort string, order string) string {
	url := ghrepo.GenerateRepoURL(repo, "security/dependabot")
	if query := formatQuery(state, sort, order); query != "" {
		url = url + "?q=" + query
	}

	return url
}

func generateOrganizationDependabotAlertsURL(repo ghrepo.Interface, org string, state string, sort string, order string) string {
	url := fmt.Sprintf("%sorgs/%s/%s", ghinstance.HostPrefix(repo.RepoHost()), org, "security/alerts/dependabot")
	if query := formatQuery(state, sort, order); query != "" {
		url = url + "?q=" + query
	}

	return url
}

func formatQuery(state string, sort string, order string) string {
	params := url.Values{}
	switch state {
	case "open":
		params.Add("is", "open")
	case "fixed":
		params.Add("is", "closed")
		params.Add("resolution", "fixed")
	case "dismissed":
		params.Add("is", "closed")
	}

	switch sort {
	case "severity":
		params.Add("sort", "severity")
	case "created":
		switch order {
		case "asc":
			params.Add("sort", "oldest")
		case "desc":
			params.Add("sort", "newest")
		}
	}

	if len(params) == 0 {
		return ""
	}

	query := params.Encode()
	query = strings.ReplaceAll(query, "=", url.QueryEscape(":"))
	query = strings.ReplaceAll(query, "&", "+")

	return query
}

func printAlerts(io *iostreams.IOStreams, alerts []dependabot.VulnerabilityAlert, showRepo bool) {
	cs := io.ColorScheme()

	table := utils.NewTablePrinter(io)
	for _, alert := range alerts {
		advisory := alert.SecurityVulnerability.Advisory

		if showRepo {
			table.AddField(alert.Repository.Name, nil, nil)
		}

		number := strconv.Itoa(alert.Number)
		if table.IsTTY() {
			number = "#" + number
			color := cs.ColorFromString(alert.State.Color())
			table.AddField(number, nil, color)
		} else {
			table.AddField(number, nil, nil)
		}

		var id string
		if len(advisory.Identifiers) > 0 {
			id = advisory.Identifiers[0].Value
		} else {
			id = advisory.GhsaId
		}

		severity := alert.SecurityVulnerability.Severity
		if table.IsTTY() {
			color := cs.ColorFromString(severity.Color())
			table.AddField(id, nil, color)
			table.AddField(severity.String(), nil, color)
		} else {
			table.AddField(id, nil, nil)
			table.AddField(severity.String(), nil, nil)
		}

		table.AddField(text.ReplaceExcessiveWhitespace(advisory.Summary), nil, nil)

		if table.IsTTY() {
			now := time.Now()
			ago := now.Sub(advisory.PublishedAt)
			table.AddField(utils.FuzzyAgo(ago), nil, cs.Gray)
		} else {
			table.AddField(advisory.PublishedAt.String(), nil, nil)
		}
		table.EndRow()
	}
	_ = table.Render()
}
