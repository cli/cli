package pin

import (
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type PinOptions struct {
	HttpClient  func() (*http.Client, error)
	Config      func() (gh.Config, error)
	IO          *iostreams.IOStreams
	BaseRepo    func() (ghrepo.Interface, error)
	SelectorArg string
}

func NewCmdPin(f *cmdutil.Factory, runF func(*PinOptions) error) *cobra.Command {
	opts := &PinOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "pin {<number> | <url>}",
		Short: "Pin a issue",
		Long: heredoc.Doc(`
			Pin an issue to a repository.

			The issue can be specified by issue number or URL.
		`),
		Example: heredoc.Doc(`
			# Pin an issue to the current repository
			$ gh issue pin 23

			# Pin an issue by URL
			$ gh issue pin https://github.com/owner/repo/issues/23

			# Pin an issue to specific repository
			$ gh issue pin 23 --repo owner/repo
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.SelectorArg = args[0]

			if runF != nil {
				return runF(opts)
			}

			return pinRun(opts)
		},
	}

	return cmd
}

func pinRun(opts *PinOptions) error {
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	issue, baseRepo, err := shared.IssueFromArgWithFields(httpClient, opts.BaseRepo, opts.SelectorArg, []string{"id", "number", "title", "isPinned"})
	if err != nil {
		return err
	}

	if issue.IsPinned {
		fmt.Fprintf(opts.IO.ErrOut, "%s Issue %s#%d (%s) is already pinned\n", cs.Yellow("!"), ghrepo.FullName(baseRepo), issue.Number, issue.Title)
		return nil
	}

	err = pinIssue(httpClient, baseRepo, issue)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Pinned issue %s#%d (%s)\n", cs.SuccessIcon(), ghrepo.FullName(baseRepo), issue.Number, issue.Title)

	return nil
}

func pinIssue(httpClient *http.Client, repo ghrepo.Interface, issue *api.Issue) error {
	var mutation struct {
		PinIssue struct {
			Issue struct {
				ID githubv4.ID
			}
		} `graphql:"pinIssue(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.PinIssueInput{
			IssueID: issue.ID,
		},
	}

	gql := api.NewClientFromHTTP(httpClient)

	return gql.Mutate(repo.RepoHost(), "IssuePin", &mutation, variables)
}
