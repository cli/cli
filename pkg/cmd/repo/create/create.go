package create

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type errWithExitCode interface {
	ExitCode() int
}

type iprompter interface {
	Input(string, string) (string, error)
	Select(string, string, []string) (int, error)
	Confirm(string, bool) (bool, error)
}

type CreateOptions struct {
	HttpClient func() (*http.Client, error)
	GitClient  *git.Client
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	Prompter   iprompter
	BackOff    backoff.BackOff

	Name               string
	Description        string
	Homepage           string
	Team               string
	Template           string
	Public             bool
	Private            bool
	Internal           bool
	Visibility         string
	Push               bool
	Clone              bool
	Source             string
	Remote             string
	GitIgnoreTemplate  string
	LicenseTemplate    string
	DisableIssues      bool
	DisableWiki        bool
	Interactive        bool
	IncludeAllBranches bool
	AddReadme          bool
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		Prompter:   f.Prompter,
	}

	var enableIssues bool
	var enableWiki bool

	cmd := &cobra.Command{
		Use:   "create [<name>]",
		Short: "Create a new repository",
		Long: heredoc.Docf(`
			Create a new GitHub repository.

			To create a repository interactively, use %[1]sgh repo create%[1]s with no arguments.

			To create a remote repository non-interactively, supply the repository name and one of %[1]s--public%[1]s, %[1]s--private%[1]s, or %[1]s--internal%[1]s.
			Pass %[1]s--clone%[1]s to clone the new repository locally.

			If the %[1]sOWNER/%[1]s portion of the %[1]sOWNER/REPO%[1]s name argument is omitted, it
			defaults to the name of the authenticating user.

			To create a remote repository from an existing local repository, specify the source directory with %[1]s--source%[1]s.
			By default, the remote repository name will be the name of the source directory.

			Pass %[1]s--push%[1]s to push any local commits to the new repository. If the repo is bare, this will mirror all refs.

			For language or platform .gitignore templates to use with %[1]s--gitignore%[1]s, <https://github.com/github/gitignore>.

			For license keywords to use with %[1]s--license%[1]s, run %[1]sgh repo license list%[1]s or visit <https://choosealicense.com>.

			The repo is created with the configured repository default branch, see <https://docs.github.com/en/account-and-profile/setting-up-and-managing-your-personal-account-on-github/managing-user-account-settings/managing-the-default-branch-name-for-your-repositories>.
		`, "`"),
		Example: heredoc.Doc(`
			# create a repository interactively
			gh repo create

			# create a new remote repository and clone it locally
			gh repo create my-project --public --clone

			# create a new remote repository in a different organization
			gh repo create my-org/my-project --public

			# create a remote repository from the current directory
			gh repo create my-project --private --source=. --remote=upstream
		`),
		Args:    cobra.MaximumNArgs(1),
		Aliases: []string{"new"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}

			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				if !opts.IO.CanPrompt() {
					return cmdutil.FlagErrorf("at least one argument required in non-interactive mode")
				}
				opts.Interactive = true
			} else {
				// exactly one visibility flag required
				if !opts.Public && !opts.Private && !opts.Internal {
					return cmdutil.FlagErrorf("`--public`, `--private`, or `--internal` required when not running interactively")
				}
				err := cmdutil.MutuallyExclusive(
					"expected exactly one of `--public`, `--private`, or `--internal`",
					opts.Public, opts.Private, opts.Internal)
				if err != nil {
					return err
				}

				if opts.Public {
					opts.Visibility = "PUBLIC"
				} else if opts.Private {
					opts.Visibility = "PRIVATE"
				} else {
					opts.Visibility = "INTERNAL"
				}
			}

			if opts.Source == "" {
				if opts.Remote != "" {
					return cmdutil.FlagErrorf("the `--remote` option can only be used with `--source`")
				}
				if opts.Push {
					return cmdutil.FlagErrorf("the `--push` option can only be used with `--source`")
				}
				if opts.Name == "" && !opts.Interactive {
					return cmdutil.FlagErrorf("name argument required to create new remote repository")
				}

			} else if opts.Clone || opts.GitIgnoreTemplate != "" || opts.LicenseTemplate != "" || opts.Template != "" {
				return cmdutil.FlagErrorf("the `--source` option is not supported with `--clone`, `--template`, `--license`, or `--gitignore`")
			}

			if opts.Template != "" && (opts.GitIgnoreTemplate != "" || opts.LicenseTemplate != "") {
				return cmdutil.FlagErrorf(".gitignore and license templates are not added when template is provided")
			}

			if opts.Template != "" && opts.AddReadme {
				return cmdutil.FlagErrorf("the `--add-readme` option is not supported with `--template`")
			}

			if cmd.Flags().Changed("enable-issues") {
				opts.DisableIssues = !enableIssues
			}
			if cmd.Flags().Changed("enable-wiki") {
				opts.DisableWiki = !enableWiki
			}
			if opts.Template != "" && opts.Team != "" {
				return cmdutil.FlagErrorf("the `--template` option is not supported with `--team`")
			}

			if opts.Template == "" && opts.IncludeAllBranches {
				return cmdutil.FlagErrorf("the `--include-all-branches` option is only supported when using `--template`")
			}

			if runF != nil {
				return runF(opts)
			}
			return createRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of the repository")
	cmd.Flags().StringVarP(&opts.Homepage, "homepage", "h", "", "Repository home page `URL`")
	cmd.Flags().StringVarP(&opts.Team, "team", "t", "", "The `name` of the organization team to be granted access")
	cmd.Flags().StringVarP(&opts.Template, "template", "p", "", "Make the new repository based on a template `repository`")
	cmd.Flags().BoolVar(&opts.Public, "public", false, "Make the new repository public")
	cmd.Flags().BoolVar(&opts.Private, "private", false, "Make the new repository private")
	cmd.Flags().BoolVar(&opts.Internal, "internal", false, "Make the new repository internal")
	cmd.Flags().StringVarP(&opts.GitIgnoreTemplate, "gitignore", "g", "", "Specify a gitignore template for the repository")
	cmd.Flags().StringVarP(&opts.LicenseTemplate, "license", "l", "", "Specify an Open Source License for the repository")
	cmd.Flags().StringVarP(&opts.Source, "source", "s", "", "Specify path to local repository to use as source")
	cmd.Flags().StringVarP(&opts.Remote, "remote", "r", "", "Specify remote name for the new repository")
	cmd.Flags().BoolVar(&opts.Push, "push", false, "Push local commits to the new repository")
	cmd.Flags().BoolVarP(&opts.Clone, "clone", "c", false, "Clone the new repository to the current directory")
	cmd.Flags().BoolVar(&opts.DisableIssues, "disable-issues", false, "Disable issues in the new repository")
	cmd.Flags().BoolVar(&opts.DisableWiki, "disable-wiki", false, "Disable wiki in the new repository")
	cmd.Flags().BoolVar(&opts.IncludeAllBranches, "include-all-branches", false, "Include all branches from template repository")
	cmd.Flags().BoolVar(&opts.AddReadme, "add-readme", false, "Add a README file to the new repository")

	// deprecated flags
	cmd.Flags().BoolP("confirm", "y", false, "Skip the confirmation prompt")
	cmd.Flags().BoolVar(&enableIssues, "enable-issues", true, "Enable issues in the new repository")
	cmd.Flags().BoolVar(&enableWiki, "enable-wiki", true, "Enable wiki in the new repository")

	_ = cmd.Flags().MarkDeprecated("confirm", "Pass any argument to skip confirmation prompt")
	_ = cmd.Flags().MarkDeprecated("enable-issues", "Disable issues with `--disable-issues`")
	_ = cmd.Flags().MarkDeprecated("enable-wiki", "Disable wiki with `--disable-wiki`")

	_ = cmd.RegisterFlagCompletionFunc("gitignore", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		cfg, err := opts.Config()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		hostname, _ := cfg.Authentication().DefaultHost()
		results, err := api.RepoGitIgnoreTemplates(httpClient, hostname)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})

	_ = cmd.RegisterFlagCompletionFunc("license", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		cfg, err := opts.Config()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		hostname, _ := cfg.Authentication().DefaultHost()
		licenses, err := api.RepoLicenses(httpClient, hostname)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var results []string
		for _, license := range licenses {
			results = append(results, fmt.Sprintf("%s\t%s", license.Key, license.Name))
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

func createRun(opts *CreateOptions) error {
	if opts.Interactive {
		answer, err := opts.Prompter.Select("What would you like to do?", "", []string{
			"Create a new repository on GitHub from scratch",
			"Create a new repository on GitHub from a template repository",
			"Push an existing local repository to GitHub",
		})
		if err != nil {
			return err
		}
		switch answer {
		case 0:
			return createFromScratch(opts)
		case 1:
			return createFromTemplate(opts)
		case 2:
			return createFromLocal(opts)
		}
	}

	if opts.Source == "" {
		return createFromScratch(opts)
	}
	return createFromLocal(opts)
}

// create new repo on remote host
func createFromScratch(opts *CreateOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var repoToCreate ghrepo.Interface
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	if opts.Interactive {
		opts.Name, opts.Description, opts.Visibility, err = interactiveRepoInfo(httpClient, host, opts.Prompter, "")
		if err != nil {
			return err
		}
		opts.AddReadme, err = opts.Prompter.Confirm("Would you like to add a README file?", false)
		if err != nil {
			return err
		}
		opts.GitIgnoreTemplate, err = interactiveGitIgnore(httpClient, host, opts.Prompter)
		if err != nil {
			return err
		}
		opts.LicenseTemplate, err = interactiveLicense(httpClient, host, opts.Prompter)
		if err != nil {
			return err
		}

		targetRepo := shared.NormalizeRepoName(opts.Name)
		if idx := strings.IndexRune(opts.Name, '/'); idx > 0 {
			targetRepo = opts.Name[0:idx+1] + shared.NormalizeRepoName(opts.Name[idx+1:])
		}
		confirmed, err := opts.Prompter.Confirm(fmt.Sprintf(`This will create "%s" as a %s repository on GitHub. Continue?`, targetRepo, strings.ToLower(opts.Visibility)), true)
		if err != nil {
			return err
		} else if !confirmed {
			return cmdutil.CancelError
		}
	}

	if strings.Contains(opts.Name, "/") {
		var err error
		repoToCreate, err = ghrepo.FromFullName(opts.Name)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	} else {
		repoToCreate = ghrepo.NewWithHost("", opts.Name, host)
	}

	input := repoCreateInput{
		Name:               repoToCreate.RepoName(),
		Visibility:         opts.Visibility,
		OwnerLogin:         repoToCreate.RepoOwner(),
		TeamSlug:           opts.Team,
		Description:        opts.Description,
		HomepageURL:        opts.Homepage,
		HasIssuesEnabled:   !opts.DisableIssues,
		HasWikiEnabled:     !opts.DisableWiki,
		GitIgnoreTemplate:  opts.GitIgnoreTemplate,
		LicenseTemplate:    opts.LicenseTemplate,
		IncludeAllBranches: opts.IncludeAllBranches,
		InitReadme:         opts.AddReadme,
	}

	var templateRepoMainBranch string
	if opts.Template != "" {
		var templateRepo ghrepo.Interface
		apiClient := api.NewClientFromHTTP(httpClient)

		templateRepoName := opts.Template
		if !strings.Contains(templateRepoName, "/") {
			currentUser, err := api.CurrentLoginName(apiClient, host)
			if err != nil {
				return err
			}
			templateRepoName = currentUser + "/" + templateRepoName
		}
		templateRepo, err = ghrepo.FromFullName(templateRepoName)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}

		repo, err := api.GitHubRepo(apiClient, templateRepo)
		if err != nil {
			return err
		}

		input.TemplateRepositoryID = repo.ID
		templateRepoMainBranch = repo.DefaultBranchRef.Name
	}

	repo, err := repoCreate(httpClient, repoToCreate.RepoHost(), input)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	isTTY := opts.IO.IsStdoutTTY()
	if isTTY {
		fmt.Fprintf(opts.IO.Out,
			"%s Created repository %s on GitHub\n  %s\n",
			cs.SuccessIconWithColor(cs.Green),
			ghrepo.FullName(repo),
			repo.URL)
	} else {
		fmt.Fprintln(opts.IO.Out, repo.URL)
	}

	if opts.Interactive {
		var err error
		opts.Clone, err = opts.Prompter.Confirm("Clone the new repository locally?", true)
		if err != nil {
			return err
		}
	}

	if opts.Clone {
		protocol := cfg.GitProtocol(repo.RepoHost()).Value
		remoteURL := ghrepo.FormatRemoteURL(repo, protocol)

		if !opts.AddReadme && opts.LicenseTemplate == "" && opts.GitIgnoreTemplate == "" && opts.Template == "" {
			// cloning empty repository or template
			if err := localInit(opts.GitClient, remoteURL, repo.RepoName()); err != nil {
				return err
			}
		} else if err := cloneWithRetry(opts, remoteURL, templateRepoMainBranch); err != nil {
			return err
		}

	}

	return nil
}

// create new repo on remote host from template repo
func createFromTemplate(opts *CreateOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	opts.Name, opts.Description, opts.Visibility, err = interactiveRepoInfo(httpClient, host, opts.Prompter, "")
	if err != nil {
		return err
	}

	if !strings.Contains(opts.Name, "/") {
		username, _, err := userAndOrgs(httpClient, host)
		if err != nil {
			return err
		}
		opts.Name = fmt.Sprintf("%s/%s", username, opts.Name)
	}
	repoToCreate, err := ghrepo.FromFullName(opts.Name)
	if err != nil {
		return fmt.Errorf("argument error: %w", err)
	}

	templateRepo, err := interactiveRepoTemplate(httpClient, host, repoToCreate.RepoOwner(), opts.Prompter)
	if err != nil {
		return err
	}
	input := repoCreateInput{
		Name:                 repoToCreate.RepoName(),
		Visibility:           opts.Visibility,
		OwnerLogin:           repoToCreate.RepoOwner(),
		TeamSlug:             opts.Team,
		Description:          opts.Description,
		HomepageURL:          opts.Homepage,
		HasIssuesEnabled:     !opts.DisableIssues,
		HasWikiEnabled:       !opts.DisableWiki,
		GitIgnoreTemplate:    opts.GitIgnoreTemplate,
		LicenseTemplate:      opts.LicenseTemplate,
		IncludeAllBranches:   opts.IncludeAllBranches,
		InitReadme:           opts.AddReadme,
		TemplateRepositoryID: templateRepo.ID,
	}
	templateRepoMainBranch := templateRepo.DefaultBranchRef.Name

	targetRepo := shared.NormalizeRepoName(opts.Name)
	if idx := strings.IndexRune(opts.Name, '/'); idx > 0 {
		targetRepo = opts.Name[0:idx+1] + shared.NormalizeRepoName(opts.Name[idx+1:])
	}
	confirmed, err := opts.Prompter.Confirm(fmt.Sprintf(`This will create "%s" as a %s repository on GitHub. Continue?`, targetRepo, strings.ToLower(opts.Visibility)), true)
	if err != nil {
		return err
	} else if !confirmed {
		return cmdutil.CancelError
	}

	repo, err := repoCreate(httpClient, repoToCreate.RepoHost(), input)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.Out,
		"%s Created repository %s on GitHub\n  %s\n",
		cs.SuccessIconWithColor(cs.Green),
		ghrepo.FullName(repo),
		repo.URL)

	opts.Clone, err = opts.Prompter.Confirm("Clone the new repository locally?", true)
	if err != nil {
		return err
	}

	if opts.Clone {
		protocol := cfg.GitProtocol(repo.RepoHost()).Value
		remoteURL := ghrepo.FormatRemoteURL(repo, protocol)

		if err := cloneWithRetry(opts, remoteURL, templateRepoMainBranch); err != nil {
			return err
		}
	}

	return nil
}

// create repo on remote host from existing local repo
func createFromLocal(opts *CreateOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	isTTY := opts.IO.IsStdoutTTY()
	stdout := opts.IO.Out

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	host, _ := cfg.Authentication().DefaultHost()

	if opts.Interactive {
		var err error
		opts.Source, err = opts.Prompter.Input("Path to local repository", ".")
		if err != nil {
			return err
		}
	}

	repoPath := opts.Source
	opts.GitClient.RepoDir = repoPath

	var baseRemote string
	if opts.Remote == "" {
		baseRemote = "origin"
	} else {
		baseRemote = opts.Remote
	}

	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}

	repoType, err := localRepoType(opts.GitClient)
	if err != nil {
		return err
	}
	if repoType == unknown {
		if repoPath == "." {
			return fmt.Errorf("current directory is not a git repository. Run `git init` to initialize it")
		}
		return fmt.Errorf("%s is not a git repository. Run `git -C \"%s\" init` to initialize it", absPath, repoPath)
	}

	committed, err := hasCommits(opts.GitClient)
	if err != nil {
		return err
	}
	if opts.Push {
		// fail immediately if trying to push with no commits
		if !committed {
			return fmt.Errorf("`--push` enabled but no commits found in %s", absPath)
		}
	}

	if opts.Interactive {
		opts.Name, opts.Description, opts.Visibility, err = interactiveRepoInfo(httpClient, host, opts.Prompter, filepath.Base(absPath))
		if err != nil {
			return err
		}
	}

	var repoToCreate ghrepo.Interface

	// repo name will be currdir name or specified
	if opts.Name == "" {
		repoToCreate = ghrepo.NewWithHost("", filepath.Base(absPath), host)
	} else if strings.Contains(opts.Name, "/") {
		var err error
		repoToCreate, err = ghrepo.FromFullName(opts.Name)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	} else {
		repoToCreate = ghrepo.NewWithHost("", opts.Name, host)
	}

	input := repoCreateInput{
		Name:              repoToCreate.RepoName(),
		Visibility:        opts.Visibility,
		OwnerLogin:        repoToCreate.RepoOwner(),
		TeamSlug:          opts.Team,
		Description:       opts.Description,
		HomepageURL:       opts.Homepage,
		HasIssuesEnabled:  !opts.DisableIssues,
		HasWikiEnabled:    !opts.DisableWiki,
		GitIgnoreTemplate: opts.GitIgnoreTemplate,
		LicenseTemplate:   opts.LicenseTemplate,
	}

	repo, err := repoCreate(httpClient, repoToCreate.RepoHost(), input)
	if err != nil {
		return err
	}

	if isTTY {
		fmt.Fprintf(stdout,
			"%s Created repository %s on GitHub\n  %s\n",
			cs.SuccessIconWithColor(cs.Green),
			ghrepo.FullName(repo),
			repo.URL)
	} else {
		fmt.Fprintln(stdout, repo.URL)
	}

	protocol := cfg.GitProtocol(repo.RepoHost()).Value
	remoteURL := ghrepo.FormatRemoteURL(repo, protocol)

	if opts.Interactive {
		addRemote, err := opts.Prompter.Confirm("Add a remote?", true)
		if err != nil {
			return err
		}
		if !addRemote {
			return nil
		}

		baseRemote, err = opts.Prompter.Input("What should the new remote be called?", "origin")
		if err != nil {
			return err
		}
	}

	if err := sourceInit(opts.GitClient, opts.IO, remoteURL, baseRemote); err != nil {
		return err
	}

	// don't prompt for push if there are no commits
	if opts.Interactive && committed {
		msg := fmt.Sprintf("Would you like to push commits from the current branch to %q?", baseRemote)
		if repoType == bare {
			msg = fmt.Sprintf("Would you like to mirror all refs to %q?", baseRemote)
		}

		var err error
		opts.Push, err = opts.Prompter.Confirm(msg, true)
		if err != nil {
			return err
		}
	}

	if opts.Push && repoType == working {
		err := opts.GitClient.Push(context.Background(), baseRemote, "HEAD")
		if err != nil {
			return err
		}

		if isTTY {
			fmt.Fprintf(stdout, "%s Pushed commits to %s\n", cs.SuccessIcon(), remoteURL)
		}
	}

	if opts.Push && repoType == bare {
		cmd, err := opts.GitClient.AuthenticatedCommand(context.Background(), git.AllMatchingCredentialsPattern, "push", baseRemote, "--mirror")
		if err != nil {
			return err
		}
		if err = cmd.Run(); err != nil {
			return err
		}

		if isTTY {
			fmt.Fprintf(stdout, "%s Mirrored all refs to %s\n", cs.SuccessIcon(), remoteURL)
		}
	}

	return nil
}

func cloneWithRetry(opts *CreateOptions, remoteURL, branch string) error {
	// Allow injecting alternative BackOff in tests.
	if opts.BackOff == nil {
		opts.BackOff = backoff.NewConstantBackOff(3 * time.Second)
	}

	var args []string
	if branch != "" {
		args = append(args, "--branch", branch)
	}

	ctx := context.Background()
	return backoff.Retry(func() error {
		stderr := &bytes.Buffer{}
		_, err := opts.GitClient.Clone(ctx, remoteURL, args, git.WithStderr(stderr))

		var execError errWithExitCode
		if errors.As(err, &execError) && execError.ExitCode() == 128 {
			return err
		} else {
			_, _ = io.Copy(opts.IO.ErrOut, stderr)
		}

		return backoff.Permanent(err)
	}, backoff.WithContext(backoff.WithMaxRetries(opts.BackOff, 3), ctx))
}

func sourceInit(gitClient *git.Client, io *iostreams.IOStreams, remoteURL, baseRemote string) error {
	cs := io.ColorScheme()
	remoteAdd, err := gitClient.Command(context.Background(), "remote", "add", baseRemote, remoteURL)
	if err != nil {
		return err
	}
	_, err = remoteAdd.Output()
	if err != nil {
		return fmt.Errorf("%s Unable to add remote %q", cs.FailureIcon(), baseRemote)
	}
	if io.IsStdoutTTY() {
		fmt.Fprintf(io.Out, "%s Added remote %s\n", cs.SuccessIcon(), remoteURL)
	}
	return nil
}

// check if local repository has committed changes
func hasCommits(gitClient *git.Client) (bool, error) {
	hasCommitsCmd, err := gitClient.Command(context.Background(), "rev-parse", "HEAD")
	if err != nil {
		return false, err
	}
	_, err = hasCommitsCmd.Output()
	if err == nil {
		return true, nil
	}

	var execError *exec.ExitError
	if errors.As(err, &execError) {
		exitCode := int(execError.ExitCode())
		if exitCode == 128 {
			return false, nil
		}
		return false, err
	}
	return false, nil
}

type repoType int

const (
	unknown repoType = iota
	working
	bare
)

func localRepoType(gitClient *git.Client) (repoType, error) {
	projectDir, projectDirErr := gitClient.GitDir(context.Background())
	if projectDirErr != nil {
		var execError errWithExitCode
		if errors.As(projectDirErr, &execError) {
			if exitCode := int(execError.ExitCode()); exitCode == 128 {
				return unknown, nil
			}
			return unknown, projectDirErr
		}
	}

	switch projectDir {
	case ".":
		return bare, nil
	case ".git":
		return working, nil
	default:
		return unknown, nil
	}
}

// clone the checkout branch to specified path
func localInit(gitClient *git.Client, remoteURL, path string) error {
	ctx := context.Background()
	gitInit, err := gitClient.Command(ctx, "init", path)
	if err != nil {
		return err
	}
	_, err = gitInit.Output()
	if err != nil {
		return err
	}

	// Copy the client so we do not modify the original client's RepoDir.
	gc := gitClient.Copy()
	gc.RepoDir = path

	gitRemoteAdd, err := gc.Command(ctx, "remote", "add", "origin", remoteURL)
	if err != nil {
		return err
	}
	_, err = gitRemoteAdd.Output()
	if err != nil {
		return err
	}

	return nil
}

func interactiveRepoTemplate(client *http.Client, hostname, owner string, prompter iprompter) (*api.Repository, error) {
	templateRepos, err := listTemplateRepositories(client, hostname, owner)
	if err != nil {
		return nil, err
	}
	if len(templateRepos) == 0 {
		return nil, fmt.Errorf("%s has no template repositories", owner)
	}

	var templates []string
	for _, repo := range templateRepos {
		templates = append(templates, repo.Name)
	}

	selected, err := prompter.Select("Choose a template repository", "", templates)
	if err != nil {
		return nil, err
	}
	return &templateRepos[selected], nil
}

func interactiveGitIgnore(client *http.Client, hostname string, prompter iprompter) (string, error) {
	confirmed, err := prompter.Confirm("Would you like to add a .gitignore?", false)
	if err != nil {
		return "", err
	} else if !confirmed {
		return "", nil
	}

	templates, err := api.RepoGitIgnoreTemplates(client, hostname)
	if err != nil {
		return "", err
	}
	selected, err := prompter.Select("Choose a .gitignore template", "", templates)
	if err != nil {
		return "", err
	}
	return templates[selected], nil
}

func interactiveLicense(client *http.Client, hostname string, prompter iprompter) (string, error) {
	confirmed, err := prompter.Confirm("Would you like to add a license?", false)
	if err != nil {
		return "", err
	} else if !confirmed {
		return "", nil
	}

	licenses, err := api.RepoLicenses(client, hostname)
	if err != nil {
		return "", err
	}
	licenseNames := make([]string, 0, len(licenses))
	for _, license := range licenses {
		licenseNames = append(licenseNames, license.Name)
	}
	selected, err := prompter.Select("Choose a license", "", licenseNames)
	if err != nil {
		return "", err
	}
	return licenses[selected].Key, nil
}

// name, description, and visibility
func interactiveRepoInfo(client *http.Client, hostname string, prompter iprompter, defaultName string) (string, string, string, error) {
	name, owner, err := interactiveRepoNameAndOwner(client, hostname, prompter, defaultName)
	if err != nil {
		return "", "", "", err
	}
	if owner != "" {
		name = fmt.Sprintf("%s/%s", owner, name)
	}

	description, err := prompter.Input("Description", "")
	if err != nil {
		return "", "", "", err
	}

	visibilityOptions := getRepoVisibilityOptions(owner)
	selected, err := prompter.Select("Visibility", "Public", visibilityOptions)
	if err != nil {
		return "", "", "", err
	}

	return name, description, strings.ToUpper(visibilityOptions[selected]), nil
}

func getRepoVisibilityOptions(owner string) []string {
	visibilityOptions := []string{"Public", "Private"}
	// orgs can also create internal repos
	if owner != "" {
		visibilityOptions = append(visibilityOptions, "Internal")
	}
	return visibilityOptions
}

func interactiveRepoNameAndOwner(client *http.Client, hostname string, prompter iprompter, defaultName string) (string, string, error) {
	name, err := prompter.Input("Repository name", defaultName)
	if err != nil {
		return "", "", err
	}

	name, owner, err := splitNameAndOwner(name)
	if err != nil {
		return "", "", err
	}
	if owner != "" {
		// User supplied an explicit owner prefix.
		return name, owner, nil
	}

	username, orgs, err := userAndOrgs(client, hostname)
	if err != nil {
		return "", "", err
	}
	if len(orgs) == 0 {
		// User doesn't belong to any orgs.
		// Leave the owner blank to indicate a personal repo.
		return name, "", nil
	}

	owners := append(orgs, username)
	sort.Strings(owners)
	selected, err := prompter.Select("Repository owner", username, owners)
	if err != nil {
		return "", "", err
	}

	owner = owners[selected]
	if owner == username {
		// Leave the owner blank to indicate a personal repo.
		return name, "", nil
	}
	return name, owner, nil
}

func splitNameAndOwner(name string) (string, string, error) {
	if !strings.Contains(name, "/") {
		return name, "", nil
	}
	repo, err := ghrepo.FromFullName(name)
	if err != nil {
		return "", "", fmt.Errorf("argument error: %w", err)
	}
	return repo.RepoName(), repo.RepoOwner(), nil
}
