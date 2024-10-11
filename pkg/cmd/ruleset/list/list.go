package list

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/ruleset/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser

	Limit          int
	IncludeParents bool
	WebMode        bool
	Organization   string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List rulesets for a repository or organization",
		Long: heredoc.Docf(`
			List GitHub rulesets for a repository or organization.

			If no options are provided, the current repository's rulesets are listed. You can query a different
			repository's rulesets by using the %[1]s--repo%[1]s flag. You can also use the %[1]s--org%[1]s flag to list rulesets
			configured for the provided organization.

			Use the %[1]s--parents%[1]s flag to control whether rulesets configured at higher levels that also apply to the provided
			repository or organization should be returned. The default is %[1]strue%[1]s.
			
			Your access token must have the %[1]sadmin:org%[1]s scope to use the %[1]s--org%[1]s flag, which can be granted by running %[1]sgh auth refresh -s admin:org%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			# List rulesets in the current repository
			$ gh ruleset list

			# List rulesets in a different repository, including those configured at higher levels
			$ gh ruleset list --repo owner/repo --parents

			# List rulesets in an organization
			$ gh ruleset list --org org-name
		`),
		Aliases: []string{"ls"},
		Args:    cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && opts.Organization != "" {
				return cmdutil.FlagErrorf("only one of --repo and --org may be specified")
			}

			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if opts.Limit < 1 {
				return cmdutil.FlagErrorf("invalid limit: %v", opts.Limit)
			}

			if runF != nil {
				return runF(opts)
			}

			return listRun(opts)
		},
	}

	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of rulesets to list")
	cmd.Flags().StringVarP(&opts.Organization, "org", "o", "", "List organization-wide rulesets for the provided organization")
	cmd.Flags().BoolVarP(&opts.IncludeParents, "parents", "p", true, "Whether to include rulesets configured at higher levels that also apply")
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the list of rulesets in the web browser")

	return cmd
}

func listRun(opts *ListOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var repoI ghrepo.Interface

	// only one of the repo or org context is necessary
	if opts.Organization == "" {
		var repoErr error
		repoI, repoErr = opts.BaseRepo()
		if repoErr != nil {
			return repoErr
		}
	}

	hostname, _ := ghauth.DefaultHost()

	if opts.WebMode {
		var rulesetURL string
		if opts.Organization != "" {
			rulesetURL = fmt.Sprintf("%sorganizations/%s/settings/rules", ghinstance.HostPrefix(hostname), opts.Organization)
		} else {
			rulesetURL = ghrepo.GenerateRepoURL(repoI, "rules")
		}

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(rulesetURL))
		}

		return opts.Browser.Browse(rulesetURL)
	}

	var result *shared.RulesetList

	if opts.Organization != "" {
		result, err = shared.ListOrgRulesets(httpClient, opts.Organization, opts.Limit, hostname, opts.IncludeParents)
	} else {
		result, err = shared.ListRepoRulesets(httpClient, repoI, opts.Limit, opts.IncludeParents)
	}

	if err != nil {
		return err
	}

	if result.TotalCount == 0 {
		return shared.NoRulesetsFoundError(opts.Organization, repoI, opts.IncludeParents)
	}

	opts.IO.DetectTerminalTheme()
	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	cs := opts.IO.ColorScheme()

	if opts.IO.IsStdoutTTY() {
		parentsMsg := ""
		if opts.IncludeParents {
			parentsMsg = " and its parents"
		}

		inMsg := fmt.Sprintf("%s%s", shared.EntityName(opts.Organization, repoI), parentsMsg)
		fmt.Fprintf(opts.IO.Out, "\nShowing %d of %d rulesets in %s\n\n", len(result.Rulesets), result.TotalCount, inMsg)
	}

	tp := tableprinter.New(opts.IO, tableprinter.WithHeader("ID", "NAME", "SOURCE", "STATUS", "RULES"))

	for _, rs := range result.Rulesets {
		tp.AddField(strconv.Itoa(rs.DatabaseId), tableprinter.WithColor(cs.Cyan))
		tp.AddField(rs.Name, tableprinter.WithColor(cs.Bold))
		tp.AddField(shared.RulesetSource(rs))
		tp.AddField(strings.ToLower(rs.Enforcement))
		tp.AddField(strconv.Itoa(rs.Rules.TotalCount))
		tp.EndRow()
	}

	return tp.Render()
}
