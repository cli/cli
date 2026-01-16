package reviewthread

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ResolveOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	ThreadID string
}

func NewCmdResolve(f *cmdutil.Factory, runF func(*ResolveOptions) error) *cobra.Command {
	opts := &ResolveOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "resolve <thread-id>",
		Short: "Resolve a review thread",
		Long: `Resolve a pull request review thread.

The thread ID can be obtained from the GitHub GraphQL API or by using
'gh pr review-thread list' command.`,
		Example: `  # Resolve a review thread
  gh pr review-thread resolve MDEyOlB1bGxSZXF1ZXN0UmV2aWV3VGhyZWFk...`,
		Args: cmdutil.ExactArgs(1, "thread ID required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.ThreadID = args[0]

			if runF != nil {
				return runF(opts)
			}
			return resolveRun(opts)
		},
	}

	return cmd
}

func resolveRun(opts *ResolveOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	err = resolveReviewThread(apiClient, opts.ThreadID)
	if err != nil {
		return fmt.Errorf("failed to resolve thread: %w", err)
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.Out, "%s Resolved review thread\n", cs.SuccessIcon())

	return nil
}

func resolveReviewThread(client *api.Client, threadID string) error {
	query := `
		mutation ResolveReviewThread($threadId: ID!) {
			resolveReviewThread(input: {threadId: $threadId}) {
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
		ResolveReviewThread struct {
			Thread struct {
				ID         string
				IsResolved bool
			}
		}
	}

	return client.GraphQL("", query, variables, &response)
}
