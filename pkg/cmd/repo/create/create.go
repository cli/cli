package create

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
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
	Push              bool
	Clone             bool
	Source            string
	Remote            string
	GitIgnoreTemplate string
	LicenseTemplate   string
	DisableIssues     bool
	DisableWiki       bool
	Interactive       bool

	ConfirmSubmit bool
	EnableIssues  bool
	EnableWiki    bool
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

			To create a repository interactively, use %[1]sgh repo create%[1]s with no arguments.

			To create a new remote repository, supply the repository name 
			and one of %[1]s--public%[1]s, %[1]s--private%[1]s, or %[1]s--internal%[1]s.
			Toggle %[1]s--clone%[1]s to clone the new repository locally.

			To create a remote repository from an existing local repository, 
			specify the source directory with %[1]s--source%[1]s.
		`, "`"),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}

			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				if !opts.IO.CanPrompt() {
					return &cmdutil.FlagError{Err: errors.New("at least one argument required in non-interactive mode")}
				}
				opts.Interactive = true
			} else if !opts.Internal && !opts.Private && !opts.Public {
				// always require visibility
				return &cmdutil.FlagError{
					Err: errors.New("`--public`, `--private`, or `--internal` required when not running interactively")}
			}

			if opts.Source != "" && (opts.Clone || opts.GitIgnoreTemplate != "" || opts.LicenseTemplate != "") {
				// which options allowed with --source?
				return &cmdutil.FlagError{Err: errors.New("the `--scope` option is not supported with `--clone`, `--template`, or `--gitignore`")}
			}

			if opts.Source == "" {
				if opts.Remote != "" {
					return &cmdutil.FlagError{Err: errors.New("the `--remote` option can only be used with `--scope`")}
				}
				if opts.Name == "" && !opts.Interactive {
					return &cmdutil.FlagError{Err: errors.New("name argument required to create new remote repository")}
				}
			}

			if opts.Template != "" && (opts.GitIgnoreTemplate != "" || opts.LicenseTemplate != "") {
				return &cmdutil.FlagError{Err: errors.New(".gitignore and license templates are not added when template is provided")}
			}

			issuesDisabled := cmd.Flags().Changed("enable-issues") || opts.DisableIssues
			wikiDisabled := cmd.Flags().Changed("enable-wiki") || opts.DisableWiki
			if opts.Template != "" && (opts.Homepage != "" || opts.Team != "" || issuesDisabled || wikiDisabled) {
				return &cmdutil.FlagError{
					Err: errors.New("the `--template` option is not supported with `--homepage`, `--team`, `--enable-issues`, or `--enable-wiki`")}
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
	cmd.Flags().BoolVarP(&opts.ConfirmSubmit, "confirm", "y", false, "Skip the confirmation prompt")
	cmd.Flags().BoolVar(&opts.EnableIssues, "enable-issues", true, "Enable issues in the new repository")
	cmd.Flags().BoolVar(&opts.EnableWiki, "enable-wiki", true, "Enable wiki in the new repository")

	cmd.Flags().MarkDeprecated("confirm", "Pass any argument to skip confirmation prompt")
	cmd.Flags().MarkDeprecated("enable-issues", "Disable issues with `--disable-issues`")
	cmd.Flags().MarkDeprecated("enable-wiki", "Disable wiki with `--disable-wiki`")

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
	return nil
}
