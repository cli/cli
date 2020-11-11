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

	Name          string
	Description   string
	Homepage      string
	Team          string
	Template      string
	EnableIssues  bool
	EnableWiki    bool
	Public        bool
	Private       bool
	Internal      bool
	ConfirmSubmit bool
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
		Long:  `Create a new GitHub repository.`,
		Args:  cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# create a repository under your account using the current directory name
			$ gh repo create

			# create a repository with a specific name
			$ gh repo create my-project

			# create a repository in an organization
			$ gh repo create cli/my-project
	  `),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(
				`A repository can be supplied as an argument in any of the following formats:
           - <OWNER/REPO>
           - by URL, e.g. "https://github.com/OWNER/REPO"`),
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}

			if !opts.IO.CanPrompt() {
				if opts.Name == "" {
					return &cmdutil.FlagError{Err: errors.New("name argument required when not running interactively")}
				}

				if !opts.Internal && !opts.Private && !opts.Public {
					return &cmdutil.FlagError{Err: errors.New("--public, --private, or --internal required when not running interactively")}
				}
			}

			if runF != nil {
				return runF(opts)
			}

			if opts.Template != "" && (opts.Homepage != "" || opts.Team != "" || !opts.EnableIssues || !opts.EnableWiki) {
				return &cmdutil.FlagError{Err: errors.New(`The '--template' option is not supported with '--homepage, --team, --enable-issues or --enable-wiki'`)}
			}

			return createRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of repository")
	cmd.Flags().StringVarP(&opts.Homepage, "homepage", "h", "", "Repository home page URL")
	cmd.Flags().StringVarP(&opts.Team, "team", "t", "", "The name of the organization team to be granted access")
	cmd.Flags().StringVarP(&opts.Template, "template", "p", "", "Make the new repository based on a template repository")
	cmd.Flags().BoolVar(&opts.EnableIssues, "enable-issues", true, "Enable issues in the new repository")
	cmd.Flags().BoolVar(&opts.EnableWiki, "enable-wiki", true, "Enable wiki in the new repository")
	cmd.Flags().BoolVar(&opts.Public, "public", false, "Make the new repository public")
	cmd.Flags().BoolVar(&opts.Private, "private", false, "Make the new repository private")
	cmd.Flags().BoolVar(&opts.Internal, "internal", false, "Make the new repository internal")
	cmd.Flags().BoolVarP(&opts.ConfirmSubmit, "confirm", "y", false, "Confirm the submission directly")

	return cmd
}

func createRun(opts *CreateOptions) error {
	projectDir, projectDirErr := git.ToplevelDir()
	isNameAnArg := false
	isDescEmpty := opts.Description == ""
	isVisibilityPassed := false

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
		return fmt.Errorf("expected exactly one of --public, --private, or --internal to be true")
	} else if enabledFlagCount == 1 {
		isVisibilityPassed = true
	}

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
	}

	var repoToCreate ghrepo.Interface

	if strings.Contains(opts.Name, "/") {
		var err error
		repoToCreate, err = ghrepo.FromFullName(opts.Name)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	} else {
		repoToCreate = ghrepo.New("", opts.Name)
	}

	// Find template repo ID
	if opts.Template != "" {
		httpClient, err := opts.HttpClient()
		if err != nil {
			return err
		}

		var toClone ghrepo.Interface
		apiClient := api.NewClientFromHTTP(httpClient)

		cloneURL := opts.Template
		if !strings.Contains(cloneURL, "/") {
			currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
			if err != nil {
				return err
			}
			cloneURL = currentUser + "/" + cloneURL
		}
		toClone, err = ghrepo.FromFullName(cloneURL)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}

		repo, err := api.GitHubRepo(apiClient, toClone)
		if err != nil {
			return err
		}

		opts.Template = repo.ID
	}

	input := repoCreateInput{
		Name:             repoToCreate.RepoName(),
		Visibility:       visibility,
		OwnerID:          repoToCreate.RepoOwner(),
		TeamID:           opts.Team,
		Description:      opts.Description,
		HomepageURL:      opts.Homepage,
		HasIssuesEnabled: opts.EnableIssues,
		HasWikiEnabled:   opts.EnableWiki,
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	createLocalDirectory := opts.ConfirmSubmit
	if !opts.ConfirmSubmit {
		opts.ConfirmSubmit, err = confirmSubmission(input.Name, input.OwnerID, &opts.ConfirmSubmit)
		if err != nil {
			return err
		}
	}

	if opts.ConfirmSubmit {
		repo, err := repoCreate(httpClient, repoToCreate.RepoHost(), input, opts.Template)
		if err != nil {
			return err
		}

		stderr := opts.IO.ErrOut
		stdout := opts.IO.Out
		cs := opts.IO.ColorScheme()
		isTTY := opts.IO.IsStdoutTTY()

		if isTTY {
			fmt.Fprintf(stderr, "%s Created repository %s on GitHub\n", cs.SuccessIcon(), ghrepo.FullName(repo))
		} else {
			fmt.Fprintln(stdout, repo.URL)
		}

		// TODO This is overly wordy and I'd like to streamline this.
		cfg, err := opts.Config()
		if err != nil {
			return err
		}
		protocol, err := cfg.Get(repo.RepoHost(), "git_protocol")
		if err != nil {
			return err
		}
		remoteURL := ghrepo.FormatRemoteURL(repo, protocol)

		if projectDirErr == nil {
			_, err = git.AddRemote("origin", remoteURL)
			if err != nil {
				return err
			}
			if isTTY {
				fmt.Fprintf(stderr, "%s Added remote %s\n", cs.SuccessIcon(), remoteURL)
			}
		} else if opts.IO.CanPrompt() {
			doSetup := createLocalDirectory
			if !doSetup {
				err := prompt.Confirm(fmt.Sprintf("Create a local project directory for %s?", ghrepo.FullName(repo)), &doSetup)
				if err != nil {
					return err
				}
			}
			if doSetup {
				path := repo.Name

				gitInit, err := git.GitCommand("init", path)
				if err != nil {
					return err
				}
				gitInit.Stdout = stdout
				gitInit.Stderr = stderr
				err = run.PrepareCmd(gitInit).Run()
				if err != nil {
					return err
				}
				gitRemoteAdd, err := git.GitCommand("-C", path, "remote", "add", "origin", remoteURL)
				if err != nil {
					return err
				}
				gitRemoteAdd.Stdout = stdout
				gitRemoteAdd.Stderr = stderr
				err = run.PrepareCmd(gitRemoteAdd).Run()
				if err != nil {
					return err
				}

				fmt.Fprintf(stderr, "%s Initialized repository in './%s/'\n", cs.SuccessIcon(), path)
			}
		}

		return nil
	}
	fmt.Fprintln(opts.IO.Out, "Discarding...")
	return nil
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

func confirmSubmission(repoName string, repoOwner string, isConfirmFlagPassed *bool) (bool, error) {
	qs := []*survey.Question{}

	promptString := ""
	if repoOwner != "" {
		promptString = fmt.Sprintf("This will create '%s/%s' in your current directory. Continue? ", repoOwner, repoName)
	} else {
		promptString = fmt.Sprintf("This will create '%s' in your current directory. Continue? ", repoName)
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
