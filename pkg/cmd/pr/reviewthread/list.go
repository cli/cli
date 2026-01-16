package reviewthread

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Finder shared.PRFinder

	SelectorArg    string
	UnresolvedOnly bool
}

type ReviewThread struct {
	ID         string
	IsResolved bool
	Path       string
	Line       int
	Body       string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "list [<number> | <url> | <branch>]",
		Short: "List review threads for a pull request",
		Long: `List review threads for a pull request.

Without an argument, the pull request that belongs to the current branch is used.`,
		Example: `  # List all review threads for current PR
  gh pr review-thread list

  # List unresolved review threads for PR #123
  gh pr review-thread list 123 --unresolved`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.UnresolvedOnly, "unresolved", false, "Show only unresolved threads")

	return cmd
}

func listRun(opts *ListOptions) error {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	threads, err := listReviewThreads(apiClient, baseRepo, pr.Number, opts.UnresolvedOnly)
	if err != nil {
		return fmt.Errorf("failed to list threads: %w", err)
	}

	if len(threads) == 0 {
		cs := opts.IO.ColorScheme()
		if opts.UnresolvedOnly {
			fmt.Fprintf(opts.IO.Out, "%s No unresolved review threads found\n", cs.SuccessIcon())
		} else {
			fmt.Fprintf(opts.IO.Out, "%s No review threads found\n", cs.WarningIcon())
		}
		return nil
	}

	cs := opts.IO.ColorScheme()
	for _, thread := range threads {
		status := "unresolved"
		if thread.IsResolved {
			status = "resolved"
		}
		fmt.Fprintf(opts.IO.Out, "%s %s %s:%d\n",
			cs.Bold(thread.ID),
			cs.Gray(fmt.Sprintf("[%s]", status)),
			thread.Path,
			thread.Line,
		)
		if thread.Body != "" {
			fmt.Fprintf(opts.IO.Out, "  %s\n", thread.Body)
		}
	}

	return nil
}

func listReviewThreads(client *api.Client, repo ghrepo.Interface, prNumber int, unresolvedOnly bool) ([]ReviewThread, error) {
	query := `
		query ListReviewThreads($owner: String!, $repo: String!, $number: Int!) {
			repository(owner: $owner, name: $repo) {
				pullRequest(number: $number) {
					reviewThreads(first: 100) {
						nodes {
							id
							isResolved
							comments(first: 1) {
								nodes {
									path
									line
									body
								}
							}
						}
					}
				}
			}
		}
	`

	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"repo":   repo.RepoName(),
		"number": prNumber,
	}

	var response struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []struct {
						ID         string
						IsResolved bool
						Comments   struct {
							Nodes []struct {
								Path string
								Line int
								Body string
							}
						}
					}
				}
			}
		}
	}

	err := client.GraphQL(repo.RepoHost(), query, variables, &response)
	if err != nil {
		return nil, err
	}

	var threads []ReviewThread
	for _, node := range response.Repository.PullRequest.ReviewThreads.Nodes {
		if unresolvedOnly && node.IsResolved {
			continue
		}

		thread := ReviewThread{
			ID:         node.ID,
			IsResolved: node.IsResolved,
		}

		if len(node.Comments.Nodes) > 0 {
			thread.Path = node.Comments.Nodes[0].Path
			thread.Line = node.Comments.Nodes[0].Line
			thread.Body = node.Comments.Nodes[0].Body
		}

		threads = append(threads, thread)
	}

	return threads, nil
}
