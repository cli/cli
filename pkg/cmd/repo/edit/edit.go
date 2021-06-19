package edit

import (
	"fmt"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
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

			If the current repository is a local git repository and the currently authenticated user has WRITE/ADMIN access to the repository, the command will let the user make changes to the repo settings.

		`, "`"),
		Args: cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			 # update repo description, allow squash merge and delete merged branches automatically
			  $ gh repo edit --description="awesome description" --allow-squash-merge --delete-merged-branch
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

	fmt.Println(isNameAnArg)
	return nil
}
