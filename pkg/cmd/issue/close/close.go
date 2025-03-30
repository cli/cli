package close

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/shared"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/spf13/cobra"
)

type CloseOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	SelectorArg string
	Comment     string
	Reason      string

	Detector fd.Detector
}

func NewCmdClose(f *cmdutil.Factory, runF func(*CloseOptions) error) *cobra.Command {
	opts := &CloseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "close {<number> | <url>}",
		Short: "Close issue",
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
			return closeRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Comment, "comment", "c", "", "Leave a closing comment")
	cmdutil.StringEnumFlag(cmd, &opts.Reason, "reason", "r", "", []string{"completed", "not planned"}, "Reason for closing")

	return cmd
}

func closeRun(opts *CloseOptions) error {
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	issue, baseRepo, err := shared.IssueFromArgWithFields(httpClient, opts.BaseRepo, opts.SelectorArg, []string{"id", "number", "title", "state"})
	if err != nil {
		return err
	}

	if issue.State == "CLOSED" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Issue %s#%d (%s) is already closed\n", cs.Yellow("!"), ghrepo.FullName(baseRepo), issue.Number, issue.Title)
		return nil
	}

	if opts.Comment != "" {
		commentOpts := &prShared.CommentableOptions{
			Body:       opts.Comment,
			HttpClient: opts.HttpClient,
			InputType:  prShared.InputTypeInline,
			Quiet:      true,
			RetrieveCommentable: func() (prShared.Commentable, ghrepo.Interface, error) {
				return issue, baseRepo, nil
			},
		}
		err := prShared.CommentableRun(commentOpts)
		if err != nil {
			return err
		}
	}

	err = apiClose(httpClient, baseRepo, issue, opts.Detector, opts.Reason)
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Closed issue %s#%d (%s)\n", cs.SuccessIconWithColor(cs.Red), ghrepo.FullName(baseRepo), issue.Number, issue.Title)

	return nil
}

func apiClose(httpClient *http.Client, repo ghrepo.Interface, issue *api.Issue, detector fd.Detector, reason string) error {
	if issue.IsPullRequest() {
		return api.PullRequestClose(httpClient, repo, issue.ID)
	}

	if reason != "" {
		if detector == nil {
			cachedClient := api.NewCachedHTTPClient(httpClient, time.Hour*24)
			detector = fd.NewDetector(cachedClient, repo.RepoHost())
		}
		features, err := detector.IssueFeatures()
		if err != nil {
			return err
		}
		if !features.StateReason {
			// If StateReason is not supported silently close issue without setting StateReason.
			reason = ""
		}
	}

	switch reason {
	case "":
		// If no reason is specified do not set it.
	case "not planned":
		reason = "NOT_PLANNED"
	default:
		reason = "COMPLETED"
	}

	var mutation struct {
		CloseIssue struct {
			Issue struct {
				ID githubv4.ID
			}
		} `graphql:"closeIssue(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": CloseIssueInput{
			IssueID:     issue.ID,
			StateReason: reason,
		},
	}

	gql := api.NewClientFromHTTP(httpClient)
	return gql.Mutate(repo.RepoHost(), "IssueClose", &mutation, variables)
}

type CloseIssueInput struct {
	IssueID     string `json:"issueId"`
	StateReason string `json:"stateReason,omitempty"`
}
