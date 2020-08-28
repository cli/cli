package root

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/ghrepo"
	aliasCmd "github.com/cli/cli/pkg/cmd/alias"
	apiCmd "github.com/cli/cli/pkg/cmd/api"
	authCmd "github.com/cli/cli/pkg/cmd/auth"
	configCmd "github.com/cli/cli/pkg/cmd/config"
	gistCmd "github.com/cli/cli/pkg/cmd/gist"
	issueCmd "github.com/cli/cli/pkg/cmd/issue"
	prCmd "github.com/cli/cli/pkg/cmd/pr"
	releaseCmd "github.com/cli/cli/pkg/cmd/release"
	repoCmd "github.com/cli/cli/pkg/cmd/repo"
	creditsCmd "github.com/cli/cli/pkg/cmd/repo/credits"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func NewCmdRoot(f *cmdutil.Factory, version, buildDate string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gh <command> <subcommand> [flags]",
		Short: "GitHub CLI",
		Long:  `Work seamlessly with GitHub from the command line.`,

		SilenceErrors: true,
		SilenceUsage:  true,
		Example: heredoc.Doc(`
			$ gh issue create
			$ gh repo clone cli/cli
			$ gh pr checkout 321
		`),
		Annotations: map[string]string{
			"help:feedback": heredoc.Doc(`
				Open an issue using “gh issue create -R cli/cli”
			`),
			"help:environment": heredoc.Doc(`
				GITHUB_TOKEN: an authentication token for API requests. Setting this avoids being
				prompted to authenticate and overrides any previously stored credentials.
	
				GH_REPO: specify the GitHub repository in the "[HOST/]OWNER/REPO" format for commands
				that otherwise operate on a local repository.

				GH_HOST: specify the GitHub hostname for commands that would otherwise assume
				the "github.com" host when not in a context of an existing repository.
	
				GH_EDITOR, GIT_EDITOR, VISUAL, EDITOR (in order of precedence): the editor tool to use
				for authoring text.
	
				BROWSER: the web browser to use for opening links.
	
				DEBUG: set to any value to enable verbose output to standard error. Include values "api"
				or "oauth" to print detailed information about HTTP requests or authentication flow.
	
				GLAMOUR_STYLE: the style to use for rendering Markdown. See
				https://github.com/charmbracelet/glamour#styles
	
				NO_COLOR: avoid printing ANSI escape sequences for color output.
			`),
		},
	}

	version = strings.TrimPrefix(version, "v")
	if buildDate == "" {
		cmd.Version = version
	} else {
		cmd.Version = fmt.Sprintf("%s (%s)", version, buildDate)
	}
	versionOutput := fmt.Sprintf("gh version %s\n%s\n", cmd.Version, changelogURL(version))
	cmd.AddCommand(&cobra.Command{
		Use:    "version",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(versionOutput)
		},
	})
	cmd.SetVersionTemplate(versionOutput)
	cmd.Flags().Bool("version", false, "Show gh version")

	cmd.SetOut(f.IOStreams.Out)
	cmd.SetErr(f.IOStreams.ErrOut)

	cmd.PersistentFlags().Bool("help", false, "Show help for command")
	cmd.SetHelpFunc(rootHelpFunc)
	cmd.SetUsageFunc(rootUsageFunc)

	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		if err == pflag.ErrHelp {
			return err
		}
		return &cmdutil.FlagError{Err: err}
	})

	cmdutil.DisableAuthCheck(cmd)

	// CHILD COMMANDS

	cmd.AddCommand(aliasCmd.NewCmdAlias(f))
	cmd.AddCommand(apiCmd.NewCmdApi(f, nil))
	cmd.AddCommand(authCmd.NewCmdAuth(f))
	cmd.AddCommand(configCmd.NewCmdConfig(f))
	cmd.AddCommand(creditsCmd.NewCmdCredits(f, nil))
	cmd.AddCommand(gistCmd.NewCmdGist(f))
	cmd.AddCommand(NewCmdCompletion(f.IOStreams))

	// below here at the commands that require the "intelligent" BaseRepo resolver
	repoResolvingCmdFactory := *f
	repoResolvingCmdFactory.BaseRepo = resolvedBaseRepo(f)

	cmd.AddCommand(prCmd.NewCmdPR(&repoResolvingCmdFactory))
	cmd.AddCommand(issueCmd.NewCmdIssue(&repoResolvingCmdFactory))
	cmd.AddCommand(releaseCmd.NewCmdRelease(&repoResolvingCmdFactory))
	cmd.AddCommand(repoCmd.NewCmdRepo(&repoResolvingCmdFactory))

	return cmd
}

func resolvedBaseRepo(f *cmdutil.Factory) func() (ghrepo.Interface, error) {
	return func() (ghrepo.Interface, error) {
		httpClient, err := f.HttpClient()
		if err != nil {
			return nil, err
		}

		apiClient := api.NewClientFromHTTP(httpClient)

		remotes, err := f.Remotes()
		if err != nil {
			return nil, err
		}
		repoContext, err := context.ResolveRemotesToRepos(remotes, apiClient, "")
		if err != nil {
			return nil, err
		}
		baseRepo, err := repoContext.BaseRepo()
		if err != nil {
			return nil, err
		}

		return baseRepo, nil
	}
}

func changelogURL(version string) string {
	path := "https://github.com/cli/cli"
	r := regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[\w.]+)?$`)
	if !r.MatchString(version) {
		return fmt.Sprintf("%s/releases/latest", path)
	}

	url := fmt.Sprintf("%s/releases/tag/v%s", path, strings.TrimPrefix(version, "v"))
	return url
}
