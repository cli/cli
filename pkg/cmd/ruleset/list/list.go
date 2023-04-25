package list

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Config     func() (config.Config, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser

	Limit        int
	WebMode      bool
	Organization string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		Config:     f.Config,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List rulesets for a repository or organization",
		Long: heredoc.Doc(`
			TODO
		`),
		Example: "TODO",
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid value for --limit: %v", opts.Limit)
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of rulesets to list")
	cmd.Flags().StringVarP(&opts.Organization, "org", "o", "", "List organization-wide rulesets")
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "List rulesets in the web browser")

	return cmd
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	repoI, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	hostname, _ := cfg.DefaultHost()

	if opts.WebMode {
		var rulesetURL string
		if opts.Organization != "" {
			rulesetURL = fmt.Sprintf("%sorganizations/%s/settings/rules", ghinstance.HostPrefix(hostname), opts.Organization)
		} else {
			rulesetURL = ghrepo.GenerateRepoURL(repoI, "settings/rules")
		}

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(rulesetURL))
		}

		return opts.Browser.Browse(rulesetURL)
	}

	var result *RulesetList

	if opts.Organization != "" {
		result, err = listOrgRulesets(httpClient, opts.Organization, opts.Limit, hostname)
	} else {
		result, err = listRepoRulesets(httpClient, repoI, opts.Limit)
	}

	if err != nil {
		return err
	}

	var entityName string
	if opts.Organization != "" {
		entityName = opts.Organization
	} else {
		entityName = ghrepo.FullName(repoI)
	}

	if result.TotalCount == 0 {
		msg := fmt.Sprintf("no rulesets found in %s", entityName)
		return cmdutil.NewNoResultsError(msg)
	}

	opts.IO.DetectTerminalTheme()
	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	cs := opts.IO.ColorScheme()

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "\nShowing %d of %d rulesets in %s\n\n", len(result.Rulesets), result.TotalCount, entityName)
	}

	tp := tableprinter.New(opts.IO)
	tp.HeaderRow("ID", "NAME", "STATUS", "TARGET")

	for _, rs := range result.Rulesets {
		tp.AddField(strconv.Itoa(rs.Id))
		tp.AddField(rs.Name, tableprinter.WithColor(cs.Bold))
		tp.AddField(strings.ToLower(rs.Enforcement))
		tp.AddField(strings.ToLower(rs.Target))
		tp.EndRow()
	}

	return tp.Render()
}

// func getRulesets()
