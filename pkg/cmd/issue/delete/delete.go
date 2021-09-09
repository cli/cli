package delete

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
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
	apiClient := api.NewClientFromHTTP(httpClient)

	issue, baseRepo, err := shared.IssueFromArg(apiClient, opts.BaseRepo, opts.SelectorArg)
	if err != nil {
		return err
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

	err = api.IssueDelete(apiClient, baseRepo, *issue)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted issue #%d (%s).\n", cs.Red("✔"), issue.Number, issue.Title)

	return nil
}
