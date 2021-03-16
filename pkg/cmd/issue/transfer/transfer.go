package transfer

import (
	"fmt"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/issue/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
	"net/http"
)

type TransferOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	IssueSelector    string
	DestRepoSelector string
}

func NewCmdTransfer(f *cmdutil.Factory, runF func(*TransferOptions) error) *cobra.Command {
	opts := TransferOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "transfer {<number> | <url>} <destination-repo>",
		Short: "Transfer issue",
		Args:  cmdutil.ExactArgs(2, "issue and destination repository are required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo
			opts.IssueSelector = args[0]
			opts.DestRepoSelector = args[1]

			if runF != nil {
				return runF(&opts)
			}

			return transferRun(&opts)
		},
	}

	return cmd
}

func transferRun(opts *TransferOptions) error {
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(client)

	issue, _, err := shared.IssueFromArg(apiClient, opts.BaseRepo, opts.IssueSelector)
	if err != nil {
		return err
	}

	var destRepo *api.Repository

	dRepo, err := ghrepo.FromFullName(opts.DestRepoSelector)
	if err != nil {
		return err
	}

	destRepo, err = api.GitHubRepo(apiClient, dRepo)
	if err != nil {
		return err
	}

	url, err := issueTransfer(apiClient, destRepo, *issue)
	if err != nil {
		return err
	}

	completionMessage := fmt.Sprintf("Issue transferred to %s", url)

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Out, "%s %s\n", cs.SuccessIconWithColor(cs.Green), completionMessage)
	}

	return nil
}

func issueTransfer(client *api.Client, destRepo *api.Repository, issue api.Issue) (string, error) {
	mutation := `
	mutation transferIssue($input:TransferIssueInput!){
		transferIssue(input: $input){
			issue {
				url
			}
		}
	}
	`

	type response struct {
		TransferIssue struct {
			Issue struct {
				URL string
			}
		}
	}

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"issueId":      issue.ID,
			"repositoryId": destRepo.ID,
		},
	}

	var resp response

	err := client.GraphQL(destRepo.RepoHost(), mutation, variables, &resp)
	if err != nil {
		return "", err
	}
	return resp.TransferIssue.Issue.URL, nil
}
