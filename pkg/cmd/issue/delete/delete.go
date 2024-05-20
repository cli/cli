package delete

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Prompter   iprompter

	SelectorArg string
	Confirmed   bool
}

type iprompter interface {
	ConfirmDeletion(string) error
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "delete {<number> | <url>}",
		Short: "Delete issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Confirmed, "confirm", false, "confirm deletion without prompting")
	_ = cmd.Flags().MarkDeprecated("confirm", "use `--yes` instead")
	cmd.Flags().BoolVar(&opts.Confirmed, "yes", false, "confirm deletion without prompting")
	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	issue, baseRepo, err := shared.IssueFromArgWithFields(httpClient, opts.BaseRepo, opts.SelectorArg, []string{"id", "number", "title"})
	if err != nil {
		return err
	}
	if issue.IsPullRequest() {
		return fmt.Errorf("issue %s#%d is a pull request and cannot be deleted", ghrepo.FullName(baseRepo), issue.Number)
	}

	// When executed in an interactive shell, require confirmation, unless
	// already provided. Otherwise skip confirmation.
	if opts.IO.CanPrompt() && !opts.Confirmed {
		cs := opts.IO.ColorScheme()
		fmt.Printf("%s Deleted issues cannot be recovered.\n", cs.WarningIcon())
		err := opts.Prompter.ConfirmDeletion(fmt.Sprintf("%d", issue.Number))
		if err != nil {
			return err
		}
	}

	if err := apiDelete(httpClient, baseRepo, issue.ID); err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted issue %s#%d (%s).\n", cs.Red("✔"), ghrepo.FullName(baseRepo), issue.Number, issue.Title)
	}

	return nil
}

func apiDelete(httpClient *http.Client, repo ghrepo.Interface, issueID string) error {
	var mutation struct {
		DeleteIssue struct {
			Repository struct {
				ID githubv4.ID
			}
		} `graphql:"deleteIssue(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.DeleteIssueInput{
			IssueID: issueID,
		},
	}

	gql := api.NewClientFromHTTP(httpClient)
	return gql.Mutate(repo.RepoHost(), "IssueDelete", &mutation, variables)
}
