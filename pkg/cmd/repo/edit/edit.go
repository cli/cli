package edit

import (
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/spf13/cobra"
	"net/http"
	"path"
)

type EditOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams

	Name             string
	Description      string
	Homepage         string
	IsTemplate       bool
	EnableIssues     bool
	EnableWiki       bool
	EnableProjects   bool
	Public           bool
	Private          bool
	Internal         bool
	AllowMergeCommit bool
	AllowSquashMerge bool
	AllowRebaseMerge bool
	DeleteBranchOnMerge bool
	Archive          bool
	ConfirmSubmit    bool
}

func NewCmdEdit(f *cmdutil.Factory, runF func(options *EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "edit [<name>]",
		Short: "Edit repository settings",
		Long: heredoc.Docf(`
			Edit your Github Repo settings

			If the current repository is a local git repository and the currently authenticated user has WRITE/ADMIN access to the repository, the command will let the user make changes to the repo settings.`),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			 # update repo description, allow squash merge and delete merged branches automatically
			  $ gh repo edit --description="awesome description" --allow-squash-merge --delete-merged-branch`),
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

			if runF != nil {
				return runF(opts)
			}
			return editRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of the repository")
	cmd.Flags().StringVarP(&opts.Homepage, "homepage", "h", "", "Repository home page `URL`")
	cmd.Flags().BoolVar(&opts.IsTemplate, "is-template", false, "Make the repository available as a template `repository`")
	cmd.Flags().BoolVar(&opts.EnableIssues, "enable-issues", true, "Enable issues in the repository")
	cmd.Flags().BoolVar(&opts.EnableWiki, "enable-wiki", true, "Enable wiki in the repository")
	cmd.Flags().BoolVar(&opts.Public, "public", false, "Make the repository public")
	cmd.Flags().BoolVar(&opts.Private, "private", false, "Make the repository private")
	cmd.Flags().BoolVar(&opts.Internal, "internal", false, "Make the repository internal")
	cmd.Flags().BoolVar(&opts.AllowMergeCommit, "allow-merge-commit", true, "Enable merge commits")
	cmd.Flags().BoolVar(&opts.AllowSquashMerge, "allow-squash-merge", true, "Enable squash merge")
	cmd.Flags().BoolVar(&opts.AllowRebaseMerge, "allow-rebase-merge", true, "Enable rebase merge")
	cmd.Flags().BoolVar(&opts.DeleteBranchOnMerge, "delete-branch-on-merge", false, "Delete head branch where PRs are merged")
	cmd.Flags().BoolVarP(&opts.ConfirmSubmit, "confirm", "y", false, "Skip the confirmation prompt")

	return cmd
}

func editRun(opts *EditOptions) error {
	projectDir, projectDirErr := git.ToplevelDir()
	isNameAnArg := false

	if opts.Name != "" {
		isNameAnArg = true
	} else {
		if projectDirErr != nil {
			return projectDirErr
		}
		opts.Name = path.Base(projectDir)
	}

	if !isNameAnArg {
		editOpts, err := interactiveRepoEdit()
		if err != nil {
			return err
		}

		fmt.Println(editOpts)
	}

	return nil
}

func interactiveRepoEdit() (*EditOptions, error) {
	qs := []*survey.Question{}

	repoNameQuestion := &survey.Question{
		Name: "repoName",
		Prompt: &survey.Input{
			Message: "Repository name",
			Default: "cli",
		},
	}
	qs = append(qs, repoNameQuestion)

	repoDescriptionQuestion := &survey.Question{
		Name: "repoDescription",
		Prompt: &survey.Input{
			Message: "Repository description",
			Default: "Github's Official command line tool",
		},
	}

	qs = append(qs, repoDescriptionQuestion)

	repoHomePageURLQuestion := &survey.Question{
		Name: "repoURL",
		Prompt: &survey.Input{
			Message: "Repository Homepage URL",
			Default: "cli.github.com",
		},
	}

	qs = append(qs, repoHomePageURLQuestion)

	repoVisibilityQuestion := &survey.Question{
		Name: "repoVisibility",
		Prompt: &survey.Select{
			Message: "Visibility",
			Options: []string{"Public", "Private", "Internal"},
		},
	}
	qs = append(qs, repoVisibilityQuestion)

	mergeOptionsQuestion := &survey.Question{
		Name: "mergeOption",
		Prompt: &survey.Select{
			Message: "Choose a merge option",
			Options: []string{"Allow Merge Commits", "Allow Squash Merging", "Allow Rebase Merging"},
		},
	}
	qs = append(qs, mergeOptionsQuestion)

	templateRepoQuestion := &survey.Question{
		Name: "isTemplateRepo",
		Prompt: &survey.Confirm{
			Message: "Convert into a template repository?",
			Default: false,
		},
	}

	qs = append(qs, templateRepoQuestion)

	autoDeleteBranchQuestion := &survey.Question{
		Name: "autoDeleteBranch",
		Prompt: &survey.Confirm{
			Message: "Automatically delete head branches after merging?",
			Default: false,
		},
	}

	qs = append(qs, autoDeleteBranchQuestion)

	archiveRepoQuestion := &survey.Question{
		Name: "archiveRepo",
		Prompt: &survey.Confirm{
			Message: "Archive Repository",
			Default: false,
		},
	}

	qs = append(qs, archiveRepoQuestion)

	answers := struct {
		RepoName         string
		RepoDescription  string
		RepoURL          string
		RepoVisibility   string
		MergeOption      string
		IsTemplateRepo   bool
		AutoDeleteBranch bool
		ArchiveRepo      bool
	}{}

	err := prompt.SurveyAsk(qs, &answers)
	if err != nil {
		return nil, err
	}

	return &EditOptions{
		Name:             answers.RepoName,
		Description:      answers.RepoDescription,
		Homepage:         answers.RepoURL,
		IsTemplate:       answers.IsTemplateRepo,
		EnableIssues:     false,
		EnableWiki:       false,
		EnableProjects:   false,
		Public:           false,
		Private:          false,
		Internal:         false,
		AllowMergeCommit: true,
		AllowSquashMerge: false,
		AllowRebaseMerge: false,
		DeleteBranchOnMerge: false,
		Archive:          false,
		ConfirmSubmit:    false,
	}, nil
}
