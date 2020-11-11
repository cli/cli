package close

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CloseOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Branch     func() (string, error)

	SelectorArg       string
	DeleteBranch      bool
	DeleteLocalBranch bool
}

func NewCmdClose(f *cmdutil.Factory, runF func(*CloseOptions) error) *cobra.Command {
	opts := &CloseOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "close {<number> | <url> | <branch>}",
		Short: "Close a pull request",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

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
	cmd.Flags().BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "Delete the local and remote branch after close")

	return cmd
}

func closeRun(opts *CloseOptions) error {
	cs := opts.IO.ColorScheme()

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient := api.NewClientFromHTTP(httpClient)

	pr, baseRepo, err := shared.PRFromArgs(apiClient, opts.BaseRepo, nil, nil, opts.SelectorArg)
	if err != nil {
		return err
	}

	if pr.State == "MERGED" {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d (%s) can't be closed because it was already merged", cs.Red("!"), pr.Number, pr.Title)
		return cmdutil.SilentError
	} else if pr.Closed {
		fmt.Fprintf(opts.IO.ErrOut, "%s Pull request #%d (%s) is already closed\n", cs.Yellow("!"), pr.Number, pr.Title)
		return nil
	}

	err = api.PullRequestClose(apiClient, baseRepo, pr)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Closed pull request #%d (%s)\n", cs.Red("✔"), pr.Number, pr.Title)

	crossRepoPR := pr.HeadRepositoryOwner.Login != baseRepo.RepoOwner()

	if opts.DeleteBranch {
		branchSwitchString := ""

		if opts.DeleteLocalBranch && !crossRepoPR {
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
				err = git.CheckoutBranch(branchToSwitchTo)
				if err != nil {
					return err
				}
			}

			localBranchExists := git.HasLocalBranch(pr.HeadRefName)
			if localBranchExists {
				err = git.DeleteLocalBranch(pr.HeadRefName)
				if err != nil {
					err = fmt.Errorf("failed to delete local branch %s: %w", cs.Cyan(pr.HeadRefName), err)
					return err
				}
			}

			if branchToSwitchTo != "" {
				branchSwitchString = fmt.Sprintf(" and switched to branch %s", cs.Cyan(branchToSwitchTo))
			}
		}

		if !crossRepoPR {
			err = api.BranchDeleteRemote(apiClient, baseRepo, pr.HeadRefName)
			if err != nil {
				err = fmt.Errorf("failed to delete remote branch %s: %w", cs.Cyan(pr.HeadRefName), err)
				return err
			}
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted branch %s%s\n", cs.Red("✔"), cs.Cyan(pr.HeadRefName), branchSwitchString)
	}

	return nil
}
