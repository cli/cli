package reviewthread

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type UnresolveOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	ThreadID string
}

func NewCmdUnresolve(f *cmdutil.Factory, runF func(*UnresolveOptions) error) *cobra.Command {
	opts := &UnresolveOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "unresolve <thread-id>",
		Short: "Unresolve a review thread",
		Long: `Unresolve a pull request review thread that was previously resolved.

The thread ID can be obtained from the GitHub GraphQL API or by using
'gh pr review-thread list' command.`,
		Example: `  # Unresolve a review thread
  gh pr review-thread unresolve MDEyOlB1bGxSZXF1ZXN0UmV2aWV3VGhyZWFk...`,
		Args: cmdutil.ExactArgs(1, "thread ID required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ThreadID = args[0]

			if runF != nil {
				return runF(opts)
			}
			return unresolveRun(opts)
		},
	}

	return cmd
}

func unresolveRun(opts *UnresolveOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	err = unresolveReviewThread(apiClient, opts.ThreadID)
	if err != nil {
		return fmt.Errorf("failed to unresolve thread: %w", err)
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.Out, "%s Unresolved review thread\n", cs.SuccessIcon())

	return nil
}

func unresolveReviewThread(client *api.Client, threadID string) error {
	query := `
		mutation UnresolveReviewThread($threadId: ID!) {
			unresolveReviewThread(input: {threadId: $threadId}) {
				thread {
					id
					isResolved
				}
			}
		}
	`

	variables := map[string]interface{}{
		"threadId": threadID,
	}

	var response struct {
		UnresolveReviewThread struct {
			Thread struct {
				ID         string
				IsResolved bool
			}
		}
	}

	return client.GraphQL("", query, variables, &response)
}
