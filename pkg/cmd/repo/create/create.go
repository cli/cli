package create

import (
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams

	Name              string
	Description       string
	Homepage          string
	Team              string
	Template          string
	Public            bool
	Private           bool
	Internal          bool
	Visibility        string
	Push              bool
	Clone             bool
	Source            string
	Remote            string
	GitIgnoreTemplate string
	LicenseTemplate   string
	DisableIssues     bool
	DisableWiki       bool
	Interactive       bool
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
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

			To create a remote repository from an existing local repository, specify the source directory with %[1]s--source%[1]s. 
			By default, the remote repository name will be the name of the source directory. 
			Pass %[1]s--push%[1]s to push any local commits to the new repository.
		`, "`"),
		Example: heredoc.Doc(`
			# create a repository interactively 
			gh repo create

			# create a new remote repository and clone it locally
			gh repo create my-project --public --clone

			# create a remote repository from the current directory
			gh repo create my-project --private --source=. --remote=upstream
		`),
		Args: cobra.MaximumNArgs(1),
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

			if cmd.Flags().Changed("enable-issues") {
				opts.DisableIssues = !enableIssues
			}
			if cmd.Flags().Changed("enable-wiki") {
				opts.DisableWiki = !enableWiki
			}
			if opts.Template != "" && (opts.Homepage != "" || opts.Team != "" || opts.DisableIssues || opts.DisableWiki) {
				return cmdutil.FlagErrorf("the `--template` option is not supported with `--homepage`, `--team`, `--disable-issues`, or `--disable-wiki`")
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
		hostname, err := cfg.DefaultHost()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		results, err := listGitIgnoreTemplates(httpClient, hostname)
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
		hostname, err := cfg.DefaultHost()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		licenses, err := listLicenseTemplates(httpClient, hostname)
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
	fromScratch := opts.Source == ""

	if opts.Interactive {
		var selectedMode string
		modeOptions := []string{
			"Create a new repository on GitHub from scratch",
			"Push an existing local repository to GitHub",
		}
		if err := prompt.SurveyAskOne(&survey.Select{
			Message: "What would you like to do?",
			Options: modeOptions,
		}, &selectedMode); err != nil {
			return err
		}
		fromScratch = selectedMode == modeOptions[0]
	}

	if fromScratch {
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

	host, err := cfg.DefaultHost()
	if err != nil {
		return err
	}

	if opts.Interactive {
		opts.Name, opts.Description, opts.Visibility, err = interactiveRepoInfo("")
		if err != nil {
			return err
		}
		opts.GitIgnoreTemplate, err = interactiveGitIgnore(httpClient, host)
		if err != nil {
			return err
		}
		opts.LicenseTemplate, err = interactiveLicense(httpClient, host)
		if err != nil {
			return err
		}

		if err := confirmSubmission(opts.Name, opts.Visibility); err != nil {
			return err
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
			"%s Created repository %s on GitHub\n",
			cs.SuccessIconWithColor(cs.Green),
			ghrepo.FullName(repo))
	} else {
		fmt.Fprintln(opts.IO.Out, repo.URL)
	}

	if opts.Interactive {
		cloneQuestion := &survey.Confirm{
			Message: "Clone the new repository locally?",
			Default: true,
		}
		err = prompt.SurveyAskOne(cloneQuestion, &opts.Clone)
		if err != nil {
			return err
		}
	}

	if opts.Clone {
		protocol, err := cfg.Get(repo.RepoHost(), "git_protocol")
		if err != nil {
			return err
		}

		remoteURL := ghrepo.FormatRemoteURL(repo, protocol)

		if opts.LicenseTemplate == "" && opts.GitIgnoreTemplate == "" {
			// cloning empty repository or template
			checkoutBranch := ""
			if opts.Template != "" {
				// use the template's default branch
				checkoutBranch = templateRepoMainBranch
			}
			if err := localInit(opts.IO, remoteURL, repo.RepoName(), checkoutBranch); err != nil {
				return err
			}
		} else if _, err := git.RunClone(remoteURL, []string{}); err != nil {
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
	host, err := cfg.DefaultHost()
	if err != nil {
		return err
	}

	if opts.Interactive {
		opts.Source, err = interactiveSource()
		if err != nil {
			return err
		}
	}

	repoPath := opts.Source

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

	isRepo, err := isLocalRepo(repoPath)
	if err != nil {
		return err
	}
	if !isRepo {
		if repoPath == "." {
			return fmt.Errorf("current directory is not a git repository. Run `git init` to initalize it")
		}
		return fmt.Errorf("%s is not a git repository. Run `git -C %s init` to initialize it", absPath, repoPath)
	}

	committed, err := hasCommits(repoPath)
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
		opts.Name, opts.Description, opts.Visibility, err = interactiveRepoInfo(filepath.Base(absPath))
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
			"%s Created repository %s on GitHub\n",
			cs.SuccessIconWithColor(cs.Green),
			ghrepo.FullName(repo))
	} else {
		fmt.Fprintln(stdout, repo.URL)
	}

	protocol, err := cfg.Get(repo.RepoHost(), "git_protocol")
	if err != nil {
		return err
	}

	remoteURL := ghrepo.FormatRemoteURL(repo, protocol)

	if opts.Interactive {
		var addRemote bool
		remoteQuesiton := &survey.Confirm{
			Message: `Add a remote?`,
			Default: true,
		}
		err = prompt.SurveyAskOne(remoteQuesiton, &addRemote)
		if err != nil {
			return err
		}

		if !addRemote {
			return nil
		}

		pushQuestion := &survey.Input{
			Message: "What should the new remote be called?",
			Default: "origin",
		}
		err = prompt.SurveyAskOne(pushQuestion, &baseRemote)
		if err != nil {
			return err
		}
	}

	if err := sourceInit(opts.IO, remoteURL, baseRemote, repoPath); err != nil {
		return err
	}

	// don't prompt for push if there's no commits
	if opts.Interactive && committed {
		pushQuestion := &survey.Confirm{
			Message: fmt.Sprintf(`Would you like to push commits from the current branch to the %q?`, baseRemote),
			Default: true,
		}
		err = prompt.SurveyAskOne(pushQuestion, &opts.Push)
		if err != nil {
			return err
		}
	}

	if opts.Push {
		repoPush, err := git.GitCommand("-C", repoPath, "push", "-u", baseRemote, "HEAD")
		if err != nil {
			return err
		}
		err = run.PrepareCmd(repoPush).Run()
		if err != nil {
			return err
		}

		if isTTY {
			fmt.Fprintf(stdout, "%s Pushed commits to %s\n", cs.SuccessIcon(), remoteURL)
		}
	}
	return nil
}

func sourceInit(io *iostreams.IOStreams, remoteURL, baseRemote, repoPath string) error {
	cs := io.ColorScheme()
	isTTY := io.IsStdoutTTY()
	stdout := io.Out

	remoteAdd, err := git.GitCommand("-C", repoPath, "remote", "add", baseRemote, remoteURL)
	if err != nil {
		return err
	}

	err = run.PrepareCmd(remoteAdd).Run()
	if err != nil {
		return fmt.Errorf("%s Unable to add remote %q", cs.FailureIcon(), baseRemote)
	}
	if isTTY {
		fmt.Fprintf(stdout, "%s Added remote %s\n", cs.SuccessIcon(), remoteURL)
	}
	return nil
}

// check if local repository has commited changes
func hasCommits(repoPath string) (bool, error) {
	hasCommitsCmd, err := git.GitCommand("-C", repoPath, "rev-parse", "HEAD")
	if err != nil {
		return false, err
	}
	prepareCmd := run.PrepareCmd(hasCommitsCmd)
	err = prepareCmd.Run()
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

// check if path is the top level directory of a git repo
func isLocalRepo(repoPath string) (bool, error) {
	projectDir, projectDirErr := git.GetDirFromPath(repoPath)
	if projectDirErr != nil {
		var execError *exec.ExitError
		if errors.As(projectDirErr, &execError) {
			if exitCode := int(execError.ExitCode()); exitCode == 128 {
				return false, nil
			}
			return false, projectDirErr
		}
	}
	if projectDir != ".git" {
		return false, nil
	}
	return true, nil
}

// clone the checkout branch to specified path
func localInit(io *iostreams.IOStreams, remoteURL, path, checkoutBranch string) error {
	gitInit, err := git.GitCommand("init", path)
	if err != nil {
		return err
	}
	isTTY := io.IsStdoutTTY()
	if isTTY {
		gitInit.Stdout = io.Out
	}
	gitInit.Stderr = io.ErrOut
	err = run.PrepareCmd(gitInit).Run()
	if err != nil {
		return err
	}

	gitRemoteAdd, err := git.GitCommand("-C", path, "remote", "add", "origin", remoteURL)
	if err != nil {
		return err
	}
	gitRemoteAdd.Stdout = io.Out
	gitRemoteAdd.Stderr = io.ErrOut
	err = run.PrepareCmd(gitRemoteAdd).Run()
	if err != nil {
		return err
	}

	if checkoutBranch == "" {
		return nil
	}

	gitFetch, err := git.GitCommand("-C", path, "fetch", "origin", fmt.Sprintf("+refs/heads/%[1]s:refs/remotes/origin/%[1]s", checkoutBranch))
	if err != nil {
		return err
	}
	gitFetch.Stdout = io.Out
	gitFetch.Stderr = io.ErrOut
	err = run.PrepareCmd(gitFetch).Run()
	if err != nil {
		return err
	}

	gitCheckout, err := git.GitCommand("-C", path, "checkout", checkoutBranch)
	if err != nil {
		return err
	}
	gitCheckout.Stdout = io.Out
	gitCheckout.Stderr = io.ErrOut
	return run.PrepareCmd(gitCheckout).Run()
}

func interactiveGitIgnore(client *http.Client, hostname string) (string, error) {
	var addGitIgnore bool
	var addGitIgnoreSurvey []*survey.Question

	addGitIgnoreQuestion := &survey.Question{
		Name: "addGitIgnore",
		Prompt: &survey.Confirm{
			Message: "Would you like to add a .gitignore?",
			Default: false,
		},
	}

	addGitIgnoreSurvey = append(addGitIgnoreSurvey, addGitIgnoreQuestion)
	err := prompt.SurveyAsk(addGitIgnoreSurvey, &addGitIgnore)
	if err != nil {
		return "", err
	}

	var wantedIgnoreTemplate string

	if addGitIgnore {
		var gitIg []*survey.Question

		gitIgnoretemplates, err := listGitIgnoreTemplates(client, hostname)
		if err != nil {
			return "", err
		}
		gitIgnoreQuestion := &survey.Question{
			Name: "chooseGitIgnore",
			Prompt: &survey.Select{
				Message: "Choose a .gitignore template",
				Options: gitIgnoretemplates,
			},
		}
		gitIg = append(gitIg, gitIgnoreQuestion)
		err = prompt.SurveyAsk(gitIg, &wantedIgnoreTemplate)
		if err != nil {
			return "", err
		}

	}

	return wantedIgnoreTemplate, nil
}

func interactiveLicense(client *http.Client, hostname string) (string, error) {
	var addLicense bool
	var addLicenseSurvey []*survey.Question
	var wantedLicense string

	addLicenseQuestion := &survey.Question{
		Name: "addLicense",
		Prompt: &survey.Confirm{
			Message: "Would you like to add a license?",
			Default: false,
		},
	}

	addLicenseSurvey = append(addLicenseSurvey, addLicenseQuestion)
	err := prompt.SurveyAsk(addLicenseSurvey, &addLicense)
	if err != nil {
		return "", err
	}

	licenseKey := map[string]string{}

	if addLicense {
		licenseTemplates, err := listLicenseTemplates(client, hostname)
		if err != nil {
			return "", err
		}
		var licenseNames []string
		for _, l := range licenseTemplates {
			licenseNames = append(licenseNames, l.Name)
			licenseKey[l.Name] = l.Key
		}
		var licenseQs []*survey.Question

		licenseQuestion := &survey.Question{
			Name: "chooseLicense",
			Prompt: &survey.Select{
				Message: "Choose a license",
				Options: licenseNames,
			},
		}
		licenseQs = append(licenseQs, licenseQuestion)
		err = prompt.SurveyAsk(licenseQs, &wantedLicense)
		if err != nil {
			return "", err
		}
		return licenseKey[wantedLicense], nil
	}
	return "", nil
}

// name, description, and visibility
func interactiveRepoInfo(defaultName string) (string, string, string, error) {
	qs := []*survey.Question{
		{
			Name: "repoName",
			Prompt: &survey.Input{
				Message: "Repository name",
				Default: defaultName,
			},
		},
		{
			Name:   "repoDescription",
			Prompt: &survey.Input{Message: "Description"},
		},
		{
			Name: "repoVisibility",
			Prompt: &survey.Select{
				Message: "Visibility",
				Options: []string{"Public", "Private", "Internal"},
			},
		}}

	answer := struct {
		RepoName        string
		RepoDescription string
		RepoVisibility  string
	}{}

	err := prompt.SurveyAsk(qs, &answer)
	if err != nil {
		return "", "", "", err
	}

	return answer.RepoName, answer.RepoDescription, strings.ToUpper(answer.RepoVisibility), nil
}

func interactiveSource() (string, error) {
	var sourcePath string
	sourcePrompt := &survey.Input{
		Message: "Path to local repository",
		Default: "."}

	err := prompt.SurveyAskOne(sourcePrompt, &sourcePath)
	if err != nil {
		return "", err
	}
	return sourcePath, nil
}

func confirmSubmission(repoWithOwner, visibility string) error {
	targetRepo := normalizeRepoName(repoWithOwner)
	if idx := strings.IndexRune(repoWithOwner, '/'); idx > 0 {
		targetRepo = repoWithOwner[0:idx+1] + normalizeRepoName(repoWithOwner[idx+1:])
	}
	var answer struct {
		ConfirmSubmit bool
	}
	err := prompt.SurveyAsk([]*survey.Question{{
		Name: "confirmSubmit",
		Prompt: &survey.Confirm{
			Message: fmt.Sprintf(`This will create "%s" as a %s repository on GitHub. Continue?`, targetRepo, strings.ToLower(visibility)),
			Default: true,
		},
	}}, &answer)
	if err != nil {
		return err
	}
	if !answer.ConfirmSubmit {
		return cmdutil.CancelError
	}
	return nil
}

// normalizeRepoName takes in the repo name the user inputted and normalizes it using the same logic as GitHub (GitHub.com/new)
func normalizeRepoName(repoName string) string {
	return strings.TrimSuffix(regexp.MustCompile(`[^\w._-]+`).ReplaceAllString(repoName, "-"), ".git")
}
