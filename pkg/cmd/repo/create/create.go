package create

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
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
	EnableIssues      bool
	EnableWiki        bool
	Public            bool
	Private           bool
	Internal          bool
	ConfirmSubmit     bool
	GitIgnoreTemplate string
	LicenseTemplate   string
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "create [<name>]",
		Short: "Create a new repository",
		Long: heredoc.Docf(`
			Create a new GitHub repository.

			When the current directory is a local git repository, the new repository will be added
			as the "origin" git remote. Otherwise, the command will prompt to clone the new
			repository into a sub-directory.

			To create a repository non-interactively, supply the following:
			- the name argument;
			- the %[1]s--confirm%[1]s flag;
			- one of %[1]s--public%[1]s, %[1]s--private%[1]s, or %[1]s--internal%[1]s.

			To toggle off %[1]s--enable-issues%[1]s or %[1]s--enable-wiki%[1]s, which are enabled
			by default, use the %[1]s--enable-issues=false%[1]s syntax.
		`, "`"),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# create a repository under your account using the current directory name
			$ git init my-project
			$ cd my-project
			$ gh repo create

			# create a repository with a specific name
			$ gh repo create my-project

			# create a repository in an organization
			$ gh repo create cli/my-project

			# disable issues and wiki
			$ gh repo create --enable-issues=false --enable-wiki=false
	  `),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(`
				A repository can be supplied as an argument in any of the following formats:
				- "OWNER/REPO"
				- by URL, e.g. "https://github.com/OWNER/REPO"
			`),
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}

			if len(args) == 0 && (opts.GitIgnoreTemplate != "" || opts.LicenseTemplate != "") {
				return &cmdutil.FlagError{Err: errors.New(".gitignore and license templates are added only when a specific repository name is passed")}
			}

			if opts.Template != "" && (opts.GitIgnoreTemplate != "" || opts.LicenseTemplate != "") {
				return &cmdutil.FlagError{Err: errors.New(".gitignore and license templates are not added when template is provided")}
			}

			if !opts.IO.CanPrompt() {
				if opts.Name == "" {
					return &cmdutil.FlagError{Err: errors.New("name argument required when not running interactively")}
				}

				if !opts.Internal && !opts.Private && !opts.Public {
					return &cmdutil.FlagError{Err: errors.New("`--public`, `--private`, or `--internal` required when not running interactively")}
				}
			}

			if opts.Template != "" && (opts.Homepage != "" || opts.Team != "" || cmd.Flags().Changed("enable-issues") || cmd.Flags().Changed("enable-wiki")) {
				return &cmdutil.FlagError{Err: errors.New("The `--template` option is not supported with `--homepage`, `--team`, `--enable-issues`, or `--enable-wiki`")}
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
	cmd.Flags().BoolVar(&opts.EnableIssues, "enable-issues", true, "Enable issues in the new repository")
	cmd.Flags().BoolVar(&opts.EnableWiki, "enable-wiki", true, "Enable wiki in the new repository")
	cmd.Flags().BoolVar(&opts.Public, "public", false, "Make the new repository public")
	cmd.Flags().BoolVar(&opts.Private, "private", false, "Make the new repository private")
	cmd.Flags().BoolVar(&opts.Internal, "internal", false, "Make the new repository internal")
	cmd.Flags().BoolVarP(&opts.ConfirmSubmit, "confirm", "y", false, "Skip the confirmation prompt")
	cmd.Flags().StringVarP(&opts.GitIgnoreTemplate, "gitignore", "g", "", "Specify a gitignore template for the repository")
	cmd.Flags().StringVarP(&opts.LicenseTemplate, "license", "l", "", "Specify an Open Source License for the repository")

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
	projectDir, projectDirErr := git.ToplevelDir()
	isNameAnArg := false
	isDescEmpty := opts.Description == ""
	isVisibilityPassed := false
	inLocalRepo := projectDirErr == nil

	if opts.Name != "" {
		isNameAnArg = true
	} else {
		if projectDirErr != nil {
			return projectDirErr
		}
		opts.Name = path.Base(projectDir)
	}

	enabledFlagCount := 0
	visibility := ""
	if opts.Public {
		enabledFlagCount++
		visibility = "PUBLIC"
	}
	if opts.Private {
		enabledFlagCount++
		visibility = "PRIVATE"
	}
	if opts.Internal {
		enabledFlagCount++
		visibility = "INTERNAL"
	}

	if enabledFlagCount > 1 {
		return fmt.Errorf("expected exactly one of `--public`, `--private`, or `--internal` to be true")
	} else if enabledFlagCount == 1 {
		isVisibilityPassed = true
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	var gitIgnoreTemplate, repoLicenseTemplate string

	gitIgnoreTemplate = opts.GitIgnoreTemplate
	repoLicenseTemplate = opts.LicenseTemplate

	// Trigger interactive prompt if name is not passed
	if !isNameAnArg {
		newName, newDesc, newVisibility, err := interactiveRepoCreate(isDescEmpty, isVisibilityPassed, opts.Name)
		if err != nil {
			return err
		}
		if newName != "" {
			opts.Name = newName
		}
		if newDesc != "" {
			opts.Description = newDesc
		}
		if newVisibility != "" {
			visibility = newVisibility
		}

	} else {
		// Go for a prompt only if visibility isn't passed
		if !isVisibilityPassed {
			newVisibility, err := getVisibility()
			if err != nil {
				return nil
			}
			visibility = newVisibility
		}

		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}

		host, err := cfg.DefaultHost()
		if err != nil {
			return err
		}

		// GitIgnore and License templates not added when a template repository
		// is passed, or when the confirm flag is set.
		if opts.Template == "" && opts.IO.CanPrompt() && !opts.ConfirmSubmit {
			if gitIgnoreTemplate == "" {
				gt, err := interactiveGitIgnore(httpClient, host)
				if err != nil {
					return err
				}
				gitIgnoreTemplate = gt
			}
			if repoLicenseTemplate == "" {
				lt, err := interactiveLicense(httpClient, host)
				if err != nil {
					return err
				}
				repoLicenseTemplate = lt
			}
		}
	}

	var repoToCreate ghrepo.Interface

	if strings.Contains(opts.Name, "/") {
		var err error
		repoToCreate, err = ghrepo.FromFullName(opts.Name)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	} else {
		host, err := cfg.DefaultHost()
		if err != nil {
			return err
		}
		repoToCreate = ghrepo.NewWithHost("", opts.Name, host)
	}

	input := repoCreateInput{
		Name:              repoToCreate.RepoName(),
		Visibility:        visibility,
		OwnerLogin:        repoToCreate.RepoOwner(),
		TeamSlug:          opts.Team,
		Description:       opts.Description,
		HomepageURL:       opts.Homepage,
		HasIssuesEnabled:  opts.EnableIssues,
		HasWikiEnabled:    opts.EnableWiki,
		GitIgnoreTemplate: gitIgnoreTemplate,
		LicenseTemplate:   repoLicenseTemplate,
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var templateRepoMainBranch string
	if opts.Template != "" {
		var templateRepo ghrepo.Interface
		apiClient := api.NewClientFromHTTP(httpClient)

		templateRepoName := opts.Template
		if !strings.Contains(templateRepoName, "/") {
			currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
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

	createLocalDirectory := opts.ConfirmSubmit
	if !opts.ConfirmSubmit {
		opts.ConfirmSubmit, err = confirmSubmission(input.Name, input.OwnerLogin, inLocalRepo)
		if err != nil {
			return err
		}
	}

	if opts.ConfirmSubmit {
		repo, err := repoCreate(httpClient, repoToCreate.RepoHost(), input)
		if err != nil {
			return err
		}
		stderr := opts.IO.ErrOut
		stdout := opts.IO.Out
		cs := opts.IO.ColorScheme()
		isTTY := opts.IO.IsStdoutTTY()

		if isTTY {
			fmt.Fprintf(stderr, "%s Created repository %s on GitHub\n", cs.SuccessIconWithColor(cs.Green), ghrepo.FullName(repo))
		} else {
			fmt.Fprintln(stdout, ghrepo.GenerateRepoURL(repo, ""))
		}

		protocol, err := cfg.Get(repo.RepoHost(), "git_protocol")
		if err != nil {
			return err
		}

		remoteURL := ghrepo.FormatRemoteURL(repo, protocol)

		if inLocalRepo {
			_, err = git.AddRemote("origin", remoteURL)
			if err != nil {
				return err
			}
			if isTTY {
				fmt.Fprintf(stderr, "%s Added remote %s\n", cs.SuccessIcon(), remoteURL)
			}
		} else {
			if opts.IO.CanPrompt() {
				if !createLocalDirectory && (gitIgnoreTemplate == "" && repoLicenseTemplate == "") {
					err := prompt.Confirm(fmt.Sprintf(`Create a local project directory for "%s"?`, ghrepo.FullName(repo)), &createLocalDirectory)
					if err != nil {
						return err
					}
				} else if !createLocalDirectory && (gitIgnoreTemplate != "" || repoLicenseTemplate != "") {
					err := prompt.Confirm(fmt.Sprintf(`Clone the remote project directory "%s"?`, ghrepo.FullName(repo)), &createLocalDirectory)
					if err != nil {
						return err
					}
				}
			}
			if createLocalDirectory && (gitIgnoreTemplate == "" && repoLicenseTemplate == "") {
				path := repo.RepoName()
				checkoutBranch := ""
				if opts.Template != "" {
					// NOTE: we cannot read `defaultBranchRef` from the newly created repository as it will
					// be null at this time. Instead, we assume that the main branch name of the new
					// repository will be the same as that of the template repository.
					checkoutBranch = templateRepoMainBranch
				}
				if err := localInit(opts.IO, remoteURL, path, checkoutBranch); err != nil {
					return err
				}
				if isTTY {
					fmt.Fprintf(stderr, "%s Initialized repository in \"%s\"\n", cs.SuccessIcon(), path)
				}
			} else if createLocalDirectory && (gitIgnoreTemplate != "" || repoLicenseTemplate != "") {
				_, err := git.RunClone(remoteURL, []string{})
				if err != nil {
					return err
				}
			}
		}

		return nil
	}
	fmt.Fprintln(opts.IO.Out, "Discarding...")
	return nil
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

func interactiveRepoCreate(isDescEmpty bool, isVisibilityPassed bool, repoName string) (string, string, string, error) {
	qs := []*survey.Question{}

	repoNameQuestion := &survey.Question{
		Name: "repoName",
		Prompt: &survey.Input{
			Message: "Repository name",
			Default: repoName,
		},
	}
	qs = append(qs, repoNameQuestion)

	if isDescEmpty {
		repoDescriptionQuestion := &survey.Question{
			Name: "repoDescription",
			Prompt: &survey.Input{
				Message: "Repository description",
			},
		}

		qs = append(qs, repoDescriptionQuestion)
	}

	if !isVisibilityPassed {
		repoVisibilityQuestion := &survey.Question{
			Name: "repoVisibility",
			Prompt: &survey.Select{
				Message: "Visibility",
				Options: []string{"Public", "Private", "Internal"},
			},
		}
		qs = append(qs, repoVisibilityQuestion)
	}

	answers := struct {
		RepoName        string
		RepoDescription string
		RepoVisibility  string
	}{}

	err := prompt.SurveyAsk(qs, &answers)

	if err != nil {
		return "", "", "", err
	}

	return answers.RepoName, answers.RepoDescription, strings.ToUpper(answers.RepoVisibility), nil
}

func confirmSubmission(repoName string, repoOwner string, inLocalRepo bool) (bool, error) {
	qs := []*survey.Question{}

	promptString := ""
	if inLocalRepo {
		promptString = `This will add an "origin" git remote to your local repository. Continue?`
	} else {
		targetRepo := repoName
		if repoOwner != "" {
			targetRepo = fmt.Sprintf("%s/%s", repoOwner, repoName)
		}
		promptString = fmt.Sprintf(`This will create the "%s" repository on GitHub. Continue?`, targetRepo)
	}

	confirmSubmitQuestion := &survey.Question{
		Name: "confirmSubmit",
		Prompt: &survey.Confirm{
			Message: promptString,
			Default: true,
		},
	}
	qs = append(qs, confirmSubmitQuestion)

	answer := struct {
		ConfirmSubmit bool
	}{}

	err := prompt.SurveyAsk(qs, &answer)
	if err != nil {
		return false, err
	}

	return answer.ConfirmSubmit, nil
}

func getVisibility() (string, error) {
	qs := []*survey.Question{}

	getVisibilityQuestion := &survey.Question{
		Name: "repoVisibility",
		Prompt: &survey.Select{
			Message: "Visibility",
			Options: []string{"Public", "Private", "Internal"},
		},
	}
	qs = append(qs, getVisibilityQuestion)

	answer := struct {
		RepoVisibility string
	}{}

	err := prompt.SurveyAsk(qs, &answer)
	if err != nil {
		return "", err
	}

	return strings.ToUpper(answer.RepoVisibility), nil
}
