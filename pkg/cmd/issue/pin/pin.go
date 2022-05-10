package pin

import (
	"context"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type PinOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	SelectorArg string
	Remove      bool
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
			Pin a Github issue to a repository.

			The issue can be specified by issue number or URL.
		`),

		Example: heredoc.Doc(`
			# Pin and Unpin issues to the current repository
			$ gh issue pin 23
			$ gh issue pin 23 --remove

			# Pin and Unpin issues by URL
			$ gh issue pin https://github.com/owner/repo/issues/23
			$ gh issue pin https://github.com/owner/repo/issues/23 --remove

			# Override the default repository
			$ gh issue pin 23 --repo owner/repo
			$ gh issue pin 23 --repo owner/repo --remove
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return pinRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Remove, "remove", false, "remove the issue from the list of pinned issues")

	return cmd
}

func pinRun(opts *PinOptions) (err error) {
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return
	}

	issue, baseRepo, err := shared.IssueFromArgWithFields(httpClient, opts.BaseRepo, opts.SelectorArg, []string{"id", "number", "title", "isPinned"})
	if err != nil {
		return
	}

	if issue.IsPinned && !opts.Remove {
		fmt.Fprintf(opts.IO.ErrOut, "%s Issue #%d (%s) is already pinned to %s\n", cs.Yellow("!"), issue.Number, issue.Title, ghrepo.FullName(baseRepo))
		return
	}

	if !issue.IsPinned && opts.Remove {
		fmt.Fprintf(opts.IO.ErrOut, "%s Issue #%d (%s) is not pinned to %s\n", cs.Yellow("!"), issue.Number, issue.Title, ghrepo.FullName(baseRepo))
		return
	}

	if opts.Remove {
		err = apiUnpin(httpClient, baseRepo, issue)
		if err != nil {
			return
		}

		fmt.Fprintf(opts.IO.ErrOut, "%s Issue #%d (%s) was unpinned from %s\n", cs.SuccessIconWithColor(cs.Red), issue.Number, issue.Title, ghrepo.FullName(baseRepo))
	} else {
		err = apiPin(httpClient, baseRepo, issue)
		if err != nil {
			return
		}

		fmt.Fprintf(opts.IO.ErrOut, "%s Issue #%d (%s) pinned to %s\n", cs.SuccessIcon(), issue.Number, issue.Title, ghrepo.FullName(baseRepo))
	}

	return
}

func apiPin(httpClient *http.Client, repo ghrepo.Interface, issue *api.Issue) error {

	var Mutation struct {
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

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)

	return gql.MutateNamed(context.Background(), "IssuePin", &Mutation, variables)
}

func apiUnpin(httpClient *http.Client, repo ghrepo.Interface, issue *api.Issue) error {

	var Mutation struct {
		UnpinIssue struct {
			Issue struct {
				ID githubv4.ID
			}
		} `graphql:"unpinIssue(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.UnpinIssueInput{
			IssueID: issue.ID,
		},
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)

	return gql.MutateNamed(context.Background(), "IssueUnpin", &Mutation, variables)
}
