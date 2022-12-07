package close

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CloseOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	IO         *iostreams.IOStreams
	Branch     func() (string, error)

	Finder shared.PRFinder

	SelectorArg       string
	Comment           string
	DeleteBranch      bool
	DeleteLocalBranch bool
}

func NewCmdClose(f *cmdutil.Factory, runF func(*CloseOptions) error) *cobra.Command {
	opts := &CloseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "close {<number> | <url> | <branch>}",
		Short: "Close a pull request",
		Args:  cmdutil.ExactArgs(1, "cannot close pull request: number, url, or branch required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			opts.DeleteLocalBranch = !cmd.Flags().Changed("repo")

			if runF != nil {
				return runF(opts)
			}
			return closeRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Comment, "comment", "c", "", "Leave a closing comment")
	cmd.Flags().BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "Delete the local and remote branch after close")

	return cmd
}

func closeRun(opts *CloseOptions) error {
	cs := opts.IO.ColorScheme()

	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"state", "number", "title", "isCrossRepository", "headRefName"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	if pr.State == "MERGED" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d (%s) can't be closed because it was already merged\n", cs.FailureIcon(), pr.Number, pr.Title)
		return cmdutil.SilentError
	} else if !pr.IsOpen() {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d (%s) is already closed\n", cs.WarningIcon(), pr.Number, pr.Title)
		return nil
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	if opts.Comment != "" {
		commentOpts := &shared.CommentableOptions{
			Body:       opts.Comment,
			HttpClient: opts.HttpClient,
			InputType:  shared.InputTypeInline,
			Quiet:      true,
			RetrieveCommentable: func() (shared.Commentable, ghrepo.Interface, error) {
				return pr, baseRepo, nil
			},
		}
		err := shared.CommentableRun(commentOpts)
		if err != nil {
			return err
		}
	}

	err = api.PullRequestClose(httpClient, baseRepo, pr.ID)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Closed pull request #%d (%s)\n", cs.SuccessIconWithColor(cs.Red), pr.Number, pr.Title)

	if opts.DeleteBranch {
		ctx := context.Background()
		branchSwitchString := ""
		apiClient := api.NewClientFromHTTP(httpClient)
		localBranchExists := opts.GitClient.HasLocalBranch(ctx, pr.HeadRefName)

		if opts.DeleteLocalBranch {
			if localBranchExists {
				currentBranch, err := opts.Branch()
				if err != nil {
					return err
				}

				var branchToSwitchTo string
				if currentBranch == pr.HeadRefName {
					branchToSwitchTo, err = api.RepoDefaultBranch(apiClient, baseRepo)
					if err != nil {
						return err
					}
					err = opts.GitClient.CheckoutBranch(ctx, branchToSwitchTo)
					if err != nil {
						return err
					}
				}

				if err := opts.GitClient.DeleteLocalBranch(ctx, pr.HeadRefName); err != nil {
					return fmt.Errorf("failed to delete local branch %s: %w", cs.Cyan(pr.HeadRefName), err)
				}

				if branchToSwitchTo != "" {
					branchSwitchString = fmt.Sprintf(" and switched to branch %s", cs.Cyan(branchToSwitchTo))
				}
			} else {
				fmt.Fprintf(opts.IO.ErrOut, "%s Skipped deleting the local branch since current directory is not a git repository \n", cs.WarningIcon())
			}
		}

		if pr.IsCrossRepository {
			fmt.Fprintf(opts.IO.ErrOut, "%s Skipped deleting the remote branch of a pull request from fork\n", cs.WarningIcon())
			if !opts.DeleteLocalBranch {
				return nil
			}
		} else {
			if err := api.BranchDeleteRemote(apiClient, baseRepo, pr.HeadRefName); err != nil {
				return fmt.Errorf("failed to delete remote branch %s: %w", cs.Cyan(pr.HeadRefName), err)
			}
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted branch %s%s\n", cs.SuccessIconWithColor(cs.Red), cs.Cyan(pr.HeadRefName), branchSwitchString)
	}

	return nil
}
