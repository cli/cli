package delete

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	SelectorArg string
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
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
		return fmt.Errorf("issue #%d is a pull request and cannot be deleted", issue.Number)
	}

	// When executed in an interactive shell, require confirmation. Otherwise skip confirmation.
	if opts.IO.CanPrompt() {
		answer := ""
		err = prompt.SurveyAskOne(
			&survey.Input{
				Message: fmt.Sprintf("You're going to delete issue #%d. This action cannot be reversed. To confirm, type the issue number:", issue.Number),
			},
			&answer,
		)
		if err != nil {
			return err
		}
		answerInt, err := strconv.Atoi(answer)
		if err != nil || answerInt != issue.Number {
			fmt.Fprintf(opts.IO.Out, "Issue #%d was not deleted.\n", issue.Number)
			return nil
		}
	}

	if err := apiDelete(httpClient, baseRepo, issue.ID); err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted issue #%d (%s).\n", cs.Red("✔"), issue.Number, issue.Title)
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

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)
	return gql.MutateNamed(context.Background(), "IssueDelete", &mutation, variables)
}
