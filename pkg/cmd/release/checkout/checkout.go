package checkout

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	cliContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CheckoutOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	IO         *iostreams.IOStreams
	Remotes    func() (cliContext.Remotes, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Config     func() (gh.Config, error)

	Force             bool
	RecurseSubmodules bool
	BranchName        string
	TagName           string
}

func NewCmdCheckout(f *cmdutil.Factory, runF func(*CheckoutOptions) error) *cobra.Command {
	opts := &CheckoutOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Remotes:    f.Remotes,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "checkout [<tag>]",
		Short: "Check out a release tag in git",
		Long: heredoc.Doc(`
			Check out a GitHub release tag in your local repository.

			Without a tag name, the latest release in the repository is checked out.
			Use the --repo flag to specify a repository other than the current one.
		`),
		Example: heredoc.Doc(`
			# Checkout the latest release
			$ gh release checkout

			# Checkout a specific release tag
			$ gh release checkout v2.67.0

			# Checkout a release from a non-local repo
			$ gh release checkout v2.67.0 --repo cli/cli

			# Checkout with a custom branch name
			$ gh release checkout v2.67.0 -b my-release-branch
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.TagName = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return checkoutRun(opts)
		},
	}

	// Flags
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Force checkout, resetting any local branch to the release tag")
	cmd.Flags().BoolVarP(&opts.RecurseSubmodules, "recurse-submodules", "", false, "Update all submodules after checkout")
	cmd.Flags().StringVarP(&opts.BranchName, "branch", "b", "", "Local branch name to use (default: tag name)")

	return cmd
}

func checkoutRun(opts *CheckoutOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	ctx := context.Background()
	var release *shared.Release

	if opts.TagName == "" {
		release, err = shared.FetchLatestRelease(ctx, httpClient, baseRepo)
		if err != nil {
			return fmt.Errorf("failed to fetch latest release: %w", err)
		}
	} else {
		release, err = shared.FetchRelease(ctx, httpClient, baseRepo, opts.TagName)
		if err != nil {
			return fmt.Errorf("failed to fetch release %s: %w", opts.TagName, err)
		}
	}

	if strings.HasPrefix(release.TagName, "-") {
		return fmt.Errorf("invalid tag name: %q", release.TagName)
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	protocol := cfg.GitProtocol(baseRepo.RepoHost()).Value

	remotes, err := opts.Remotes()
	if err != nil {
		return err
	}
	baseRemote, _ := remotes.FindByRepo(baseRepo.RepoOwner(), baseRepo.RepoName())
	baseURLOrName := ghrepo.FormatRemoteURL(baseRepo, protocol)
	if baseRemote != nil {
		baseURLOrName = baseRemote.Name
	}

	var cmdQueue [][]string
	localBranch := release.TagName
	if opts.BranchName != "" {
		localBranch = opts.BranchName
	}

	refSpec := fmt.Sprintf("refs/tags/%s", release.TagName)
	cmdQueue = append(cmdQueue, []string{"fetch", baseURLOrName, refSpec, "--no-tags"})

	if localBranchExists(opts.GitClient, localBranch) {
		if opts.IO.IsStdoutTTY() && !opts.Force {
			fmt.Fprintf(opts.IO.Out, "A branch named '%s' already exists. Proceeding may overwrite local changes.\n", localBranch)
			confirmed, err := confirm(opts.IO, "Do you want to proceed? [y/N] ")
			if err != nil {
				return fmt.Errorf("failed to get confirmation: %w", err)
			}
			if !confirmed {
				fmt.Fprintf(opts.IO.Out, "Aborted.\n")
				return nil
			}
		}

		cmdQueue = append(cmdQueue, []string{"checkout", localBranch})
		if opts.Force {
			cmdQueue = append(cmdQueue, []string{"reset", "--hard", refSpec})
		} else {
			cmdQueue = append(cmdQueue, []string{"merge", "--ff-only", refSpec})
		}
	} else {
		cmdQueue = append(cmdQueue, []string{"checkout", "-b", localBranch, refSpec})
	}

	if opts.RecurseSubmodules {
		cmdQueue = append(cmdQueue, []string{"submodule", "sync", "--recursive"})
		cmdQueue = append(cmdQueue, []string{"submodule", "update", "--init", "--recursive"})
	}

	err = executeCmds(opts.GitClient, git.CredentialPatternFromHost(baseRepo.RepoHost()), cmdQueue)
	if err != nil {
		return fmt.Errorf("failed to execute git commands: %w", err)
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "Checked out %s to %s\n", release.TagName, localBranch)
	}

	return nil
}

// localBranchExists checks if a local branch already exists
func localBranchExists(client *git.Client, branch string) bool {
	_, err := client.ShowRefs(context.Background(), []string{"refs/heads/" + branch})
	return err == nil
}

// confirm prompts the user for a yes/no response, defaulting to no
func confirm(io *iostreams.IOStreams, prompt string) (bool, error) {
	if !io.IsStdinTTY() || !io.IsStdoutTTY() {
		return false, nil
	}

	fmt.Fprint(io.Out, prompt)
	reader := bufio.NewReader(io.In)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}

// executeCmds runs a queue of Git commands with appropriate credential handling
func executeCmds(client *git.Client, credentialPattern git.CredentialPattern, cmdQueue [][]string) error {
	for _, args := range cmdQueue {
		var err error
		var cmd *git.Command
		switch args[0] {
		case "submodule":
			cmd, err = client.AuthenticatedCommand(context.Background(), credentialPattern, args...)
		case "fetch":
			cmd, err = client.AuthenticatedCommand(context.Background(), git.AllMatchingCredentialsPattern, args...)
		default:
			cmd, err = client.Command(context.Background(), args...)
		}
		if err != nil {
			return err
		}
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}
