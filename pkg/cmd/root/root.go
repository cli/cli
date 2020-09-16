package root

import (
	"fmt"
	"net/http"
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
	"github.com/cli/cli/pkg/cmd/factory"
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
				Open an issue using 'gh issue create -R cli/cli'
			`),
			"help:environment": heredoc.Doc(`
				See 'gh help environment' for the list of supported environment variables.
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

	// Child commands
	cmd.AddCommand(aliasCmd.NewCmdAlias(f))
	cmd.AddCommand(authCmd.NewCmdAuth(f))
	cmd.AddCommand(configCmd.NewCmdConfig(f))
	cmd.AddCommand(creditsCmd.NewCmdCredits(f, nil))
	cmd.AddCommand(gistCmd.NewCmdGist(f))
	cmd.AddCommand(NewCmdCompletion(f.IOStreams))

	// Help topics
	cmd.AddCommand(NewHelpTopic("environment"))

	// the `api` command should not inherit any extra HTTP headers
	bareHTTPCmdFactory := *f
	bareHTTPCmdFactory.HttpClient = func() (*http.Client, error) {
		cfg, err := bareHTTPCmdFactory.Config()
		if err != nil {
			return nil, err
		}
		return factory.NewHTTPClient(bareHTTPCmdFactory.IOStreams, cfg, version, false), nil
	}

	cmd.AddCommand(apiCmd.NewCmdApi(&bareHTTPCmdFactory, nil))

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
		baseRepo, err := repoContext.BaseRepo(f.IOStreams)
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
