package list

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
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

			// if opts.Limit < 1 {
			// 	return cmdutil.FlagErrorf("invalid value for --limit: %v", opts.Limit)
			// }

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of rules to list")
	cmd.Flags().StringVarP(&opts.Organization, "org", "o", "", "List organization-wide rules")
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "List rules in the web browser")

	return cmd
}

type Ruleset struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	Target      string `json:"target"`
	SourceType  string `json:"source_type"`
	Source      string `json:"source"`
	Enforcement string `json:"enforcement"`
	BypassMode  string `json:"bypass_mode"`
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

	if opts.WebMode {
		rulesetURL := ghrepo.GenerateRepoURL(repoI, "settings/rules")
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(rulesetURL))
		}

		return opts.Browser.Browse(rulesetURL)
	}

	result, err := listRepoRulesets(httpClient, repoI, opts.Limit)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()

	tp := tableprinter.New(opts.IO)
	tp.HeaderRow("ID", "NAME" /* "STATUS",*/, "TARGET")

	for _, rs := range result.Rulesets {
		tp.AddField(rs.Id)
		tp.AddField(rs.Name, tableprinter.WithColor(cs.Bold))
		// tp.AddField(strings.ToLower(rs.Enforcement))
		tp.AddField(strings.ToLower(rs.Target))
		tp.EndRow()
	}

	return tp.Render()
}

// func getRulesets()
