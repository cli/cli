package checkout

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	HttpClient     func() (*http.Client, error)
	GitClient      *git.Client
	IO             *iostreams.IOStreams
	Remotes        func() (cliContext.Remotes, error)
	BaseRepo       func() (ghrepo.Interface, error)
	Config         func() (gh.Config, error)
	IsLocalGitRepo bool

	Force             bool
	Yes               bool
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
		Short: "Check out a release tag",
		Long: heredoc.Doc(`
			Check out a GitHub release tag.

			In a local repository, checks out the specified release tag.
			With --repo, clones the specified repository to a directory named '<reponame>-<releasetag>' if not in a matching Git repository.
			Without a tag, checks out the latest release.
		`),
		Example: heredoc.Doc(`
			# Checkout the latest release in the current repo
			$ gh release checkout

			# Checkout a specific tag in the current repo
			$ gh release checkout v1.2.3

			# Clone and checkout a release from an external repo
			$ gh release checkout v1.2.3 --repo owner/repo

			# Clone with a custom branch name
			$ gh release checkout v1.2.3 -b my-branch --repo owner/repo
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.TagName = args[0]
			}

			if !opts.IO.CanPrompt() && opts.TagName == "" {
				return cmdutil.FlagErrorf("release tag argument required when not running interactively")
			}

			_, err := opts.Remotes()
			opts.IsLocalGitRepo = err == nil
			if !opts.IsLocalGitRepo && !cmd.Flags().Changed("repo") {
				return fmt.Errorf("not in a Git repository and no --repo specified.\nPlease run from a Git repository or use --repo to specify a repository to checkout release")
			}

			if runF != nil {
				return runF(opts)
			}
			return checkoutRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Force checkout, resetting any local branch to the release tag")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Skip confirmation prompt when overwriting an existing directory or branch")
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

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	protocol := cfg.GitProtocol(baseRepo.RepoHost()).Value
	remoteURL := ghrepo.FormatRemoteURL(baseRepo, protocol)

	var baseRemote *cliContext.Remote
	var repoMatches bool
	if opts.IsLocalGitRepo {
		remotes, err := opts.Remotes()
		if err != nil {
			return fmt.Errorf("unexpected failure checking remotes: %w", err)
		}
		baseRemote, _ = remotes.FindByRepo(baseRepo.RepoOwner(), baseRepo.RepoName())
		repoMatches = baseRemote != nil
	}

	if opts.IsLocalGitRepo && !repoMatches {
		remotes, _ := opts.Remotes()
		currentRepo := "unknown"
		if len(remotes) > 0 {
			currentRepo = fmt.Sprintf("%s/%s", remotes[0].RepoOwner(), remotes[0].RepoName())
		}
		return fmt.Errorf("--repo %s doesn't match the current repository (%s).\nRun outside a Git repo to checkout release, or omit --repo to use the current repo", ghrepo.FullName(baseRepo), currentRepo)
	}

	var release *shared.Release
	fetchMessage := fmt.Sprintf("Fetching release info for %s...", ghrepo.FullName(baseRepo))
	if opts.IO.IsStdoutTTY() {
		opts.IO.StartProgressIndicatorWithLabel(fetchMessage)
	}
	if opts.TagName == "" {
		release, err = shared.FetchLatestRelease(ctx, httpClient, baseRepo)
	} else {
		release, err = shared.FetchRelease(ctx, httpClient, baseRepo, opts.TagName)
	}
	if opts.IO.IsStdoutTTY() {
		opts.IO.StopProgressIndicator()
	}
	if err != nil {
		return fmt.Errorf("failed to fetch release %s: %w", opts.TagName, err)
	}

	localBranch := release.TagName
	if opts.BranchName != "" {
		localBranch = opts.BranchName
	}

	cs := opts.IO.ColorScheme()

	if !opts.IsLocalGitRepo || !repoMatches {
		targetDir := fmt.Sprintf("%s-%s", baseRepo.RepoName(), release.TagName)
		if err := handleExistingDir(opts, targetDir, baseRepo, release.TagName, cs); err != nil {
			return err
		}

		cloneArgs := []string{"--branch", release.TagName}
		if opts.RecurseSubmodules {
			cloneArgs = append(cloneArgs, "--recurse-submodules")
		}
		fetchMessage = fmt.Sprintf("Fetching %s@%s...", ghrepo.FullName(baseRepo), release.TagName)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		if opts.IO.IsStdoutTTY() {
			opts.IO.StartProgressIndicatorWithLabel(fetchMessage)
		}
		defaultTarget, err := opts.GitClient.Clone(ctx, remoteURL, cloneArgs, git.WithStdout(stdout), git.WithStderr(stderr))
		if opts.IO.IsStdoutTTY() {
			opts.IO.StopProgressIndicator()
		}
		if err != nil {
			_, _ = io.Copy(opts.IO.ErrOut, stderr)
			return fmt.Errorf("failed to clone repository: %w", err)
		}
		if defaultTarget != targetDir {
			if err := os.Rename(defaultTarget, targetDir); err != nil {
				return fmt.Errorf("failed to rename cloned directory from %s to %s: %w", defaultTarget, targetDir, err)
			}
		}

		absTargetDir, err := filepath.Abs(targetDir)
		if err != nil {
			return fmt.Errorf("failed to get absolute path of cloned directory: %w", err)
		}
		cmd, err := opts.GitClient.Command(ctx, "checkout", "-b", localBranch)
		if err != nil {
			return fmt.Errorf("failed to prepare checkout command: %w", err)
		}
		cmd.Dir = absTargetDir
		cmd.Stdout = nil
		cmd.Stderr = opts.IO.ErrOut
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create branch after clone: %w", err)
		}

		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.Out, "%s Cloned %s@%s to %s\n", cs.SuccessIconWithColor(cs.Green), ghrepo.FullName(baseRepo), release.TagName, targetDir)
		} else {
			fmt.Fprintln(opts.IO.Out, targetDir)
		}
		return nil
	}

	baseURLOrName := baseRemote.Name
	var cmdQueue [][]string
	refSpec := fmt.Sprintf("refs/tags/%s", release.TagName)
	fetchMessage = fmt.Sprintf("Fetching %s@%s...", ghrepo.FullName(baseRepo), release.TagName)
	if opts.IO.IsStdoutTTY() {
		opts.IO.StartProgressIndicatorWithLabel(fetchMessage)
	}
	err = executeCmds(opts.GitClient, git.CredentialPatternFromHost(baseRepo.RepoHost()), [][]string{{"fetch", baseURLOrName, refSpec, "--no-tags"}})
	if opts.IO.IsStdoutTTY() {
		opts.IO.StopProgressIndicator()
	}
	if err != nil {
		return fmt.Errorf("failed to fetch release tag: %w", err)
	}

	if localBranchExists(opts.GitClient, localBranch) {
		if err := handleExistingBranch(opts, localBranch, release.TagName, cs); err != nil {
			return err
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
		cmdQueue = append(cmdQueue, []string{"submodule", "sync", "--recursive"}, []string{"submodule", "update", "--init", "--recursive"})
	}

	err = executeCmds(opts.GitClient, git.CredentialPatternFromHost(baseRepo.RepoHost()), cmdQueue)
	if err != nil {
		return fmt.Errorf("failed to execute git commands: %w", err)
	}

	if opts.IO.IsStdoutTTY() {
		fmt.Fprintf(opts.IO.Out, "%s Checked out %s to %s\n", cs.SuccessIconWithColor(cs.Green), release.TagName, localBranch)
	}
	return nil
}

// handleExistingDir checks and handles an existing directory before cloning
func handleExistingDir(opts *CheckoutOptions, targetDir string, baseRepo ghrepo.Interface, tagName string, cs *iostreams.ColorScheme) error {
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		return nil
	}
	if !opts.IO.CanPrompt() || opts.Yes {
		return os.RemoveAll(targetDir)
	}
	prompt := fmt.Sprintf("Directory '%s' already exists. Overwrite by cloning %s@%s? [y/N] ", targetDir, ghrepo.FullName(baseRepo), tagName)
	confirmed, err := confirm(opts.IO, prompt)
	if err != nil {
		return fmt.Errorf("failed to get confirmation: %w", err)
	}
	if !confirmed {
		fmt.Fprintf(opts.IO.Out, "%s Checkout aborted\n", cs.Yellow("!"))
		return cmdutil.SilentError
	}
	return os.RemoveAll(targetDir)
}

// handleExistingBranch checks and handles an existing branch before checkout
func handleExistingBranch(opts *CheckoutOptions, branch, tagName string, cs *iostreams.ColorScheme) error {
	if opts.Force || opts.Yes || !opts.IO.CanPrompt() {
		return nil
	}
	prompt := fmt.Sprintf("Branch '%s' already exists. Overwrite with %s? [y/N] ", branch, tagName)
	confirmed, err := confirm(opts.IO, prompt)
	if err != nil {
		return fmt.Errorf("failed to get confirmation: %w", err)
	}
	if !confirmed {
		fmt.Fprintf(opts.IO.Out, "%s Checkout aborted\n", cs.Yellow("!"))
		return cmdutil.SilentError
	}
	return nil
}

// localBranchExists checks if a local branch exists
func localBranchExists(client *git.Client, branch string) bool {
	_, err := client.ShowRefs(context.Background(), []string{"refs/heads/" + branch})
	return err == nil
}

// confirm prompts the user for a yes/no response, defaulting to no
func confirm(io *iostreams.IOStreams, prompt string) (bool, error) {
	if !io.CanPrompt() {
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

// executeCmds runs a queue of Git commands with output suppression
func executeCmds(client *git.Client, credentialPattern git.CredentialPattern, cmdQueue [][]string) error {
	ctx := context.Background()
	for _, args := range cmdQueue {
		var cmd *git.Command
		var err error
		switch args[0] {
		case "submodule":
			cmd, err = client.AuthenticatedCommand(ctx, credentialPattern, args...)
		case "fetch":
			cmd, err = client.AuthenticatedCommand(ctx, git.AllMatchingCredentialsPattern, args...)
		default:
			cmd, err = client.Command(ctx, args...)
		}
		if err != nil {
			return err
		}
		cmd.Stdout = nil
		cmd.Stderr = &bytes.Buffer{}
		if err := cmd.Run(); err != nil {
			_, _ = io.Copy(os.Stderr, cmd.Stderr.(*bytes.Buffer))
			return err
		}
	}
	return nil
}
