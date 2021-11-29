package root

import (
	"net/http"
	"os"
	"sync"

	"github.com/MakeNowJust/heredoc"
	codespacesAPI "github.com/cli/cli/v2/internal/codespaces/api"
	actionsCmd "github.com/cli/cli/v2/pkg/cmd/actions"
	aliasCmd "github.com/cli/cli/v2/pkg/cmd/alias"
	apiCmd "github.com/cli/cli/v2/pkg/cmd/api"
	authCmd "github.com/cli/cli/v2/pkg/cmd/auth"
	browseCmd "github.com/cli/cli/v2/pkg/cmd/browse"
	codespaceCmd "github.com/cli/cli/v2/pkg/cmd/codespace"
	completionCmd "github.com/cli/cli/v2/pkg/cmd/completion"
	configCmd "github.com/cli/cli/v2/pkg/cmd/config"
	extensionCmd "github.com/cli/cli/v2/pkg/cmd/extension"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	gistCmd "github.com/cli/cli/v2/pkg/cmd/gist"
	gpgKeyCmd "github.com/cli/cli/v2/pkg/cmd/gpg-key"
	issueCmd "github.com/cli/cli/v2/pkg/cmd/issue"
	prCmd "github.com/cli/cli/v2/pkg/cmd/pr"
	releaseCmd "github.com/cli/cli/v2/pkg/cmd/release"
	repoCmd "github.com/cli/cli/v2/pkg/cmd/repo"
	creditsCmd "github.com/cli/cli/v2/pkg/cmd/repo/credits"
	runCmd "github.com/cli/cli/v2/pkg/cmd/run"
	secretCmd "github.com/cli/cli/v2/pkg/cmd/secret"
	sshKeyCmd "github.com/cli/cli/v2/pkg/cmd/ssh-key"
	versionCmd "github.com/cli/cli/v2/pkg/cmd/version"
	workflowCmd "github.com/cli/cli/v2/pkg/cmd/workflow"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
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
				Open an issue using 'gh issue create -R github.com/cli/cli'
			`),
			"help:environment": heredoc.Doc(`
				See 'gh help environment' for the list of supported environment variables.
			`),
		},
	}

	cmd.SetOut(f.IOStreams.Out)
	cmd.SetErr(f.IOStreams.ErrOut)

	cmd.PersistentFlags().Bool("help", false, "Show help for command")
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		rootHelpFunc(f, cmd, args)
	})
	cmd.SetUsageFunc(rootUsageFunc)
	cmd.SetFlagErrorFunc(rootFlagErrorFunc)

	formattedVersion := versionCmd.Format(version, buildDate)
	cmd.SetVersionTemplate(formattedVersion)
	cmd.Version = formattedVersion
	cmd.Flags().Bool("version", false, "Show gh version")

	// Child commands
	cmd.AddCommand(versionCmd.NewCmdVersion(f, version, buildDate))
	cmd.AddCommand(actionsCmd.NewCmdActions(f))
	cmd.AddCommand(aliasCmd.NewCmdAlias(f))
	cmd.AddCommand(authCmd.NewCmdAuth(f))
	cmd.AddCommand(configCmd.NewCmdConfig(f))
	cmd.AddCommand(creditsCmd.NewCmdCredits(f, nil))
	cmd.AddCommand(gistCmd.NewCmdGist(f))
	cmd.AddCommand(gpgKeyCmd.NewCmdGPGKey(f))
	cmd.AddCommand(completionCmd.NewCmdCompletion(f.IOStreams))
	cmd.AddCommand(extensionCmd.NewCmdExtension(f))
	cmd.AddCommand(secretCmd.NewCmdSecret(f))
	cmd.AddCommand(sshKeyCmd.NewCmdSSHKey(f))
	cmd.AddCommand(newCodespaceCmd(f))

	// the `api` command should not inherit any extra HTTP headers
	bareHTTPCmdFactory := *f
	bareHTTPCmdFactory.HttpClient = bareHTTPClient(f, version)

	cmd.AddCommand(apiCmd.NewCmdApi(&bareHTTPCmdFactory, nil))

	// below here at the commands that require the "intelligent" BaseRepo resolver
	repoResolvingCmdFactory := *f
	repoResolvingCmdFactory.BaseRepo = factory.SmartBaseRepoFunc(f)

	cmd.AddCommand(browseCmd.NewCmdBrowse(&repoResolvingCmdFactory, nil))
	cmd.AddCommand(prCmd.NewCmdPR(&repoResolvingCmdFactory))
	cmd.AddCommand(issueCmd.NewCmdIssue(&repoResolvingCmdFactory))
	cmd.AddCommand(releaseCmd.NewCmdRelease(&repoResolvingCmdFactory))
	cmd.AddCommand(repoCmd.NewCmdRepo(&repoResolvingCmdFactory))
	cmd.AddCommand(runCmd.NewCmdRun(&repoResolvingCmdFactory))
	cmd.AddCommand(workflowCmd.NewCmdWorkflow(&repoResolvingCmdFactory))

	// Help topics
	cmd.AddCommand(NewHelpTopic("environment"))
	cmd.AddCommand(NewHelpTopic("formatting"))
	cmd.AddCommand(NewHelpTopic("mintty"))
	referenceCmd := NewHelpTopic("reference")
	referenceCmd.SetHelpFunc(referenceHelpFn(f.IOStreams))
	cmd.AddCommand(referenceCmd)

	cmdutil.DisableAuthCheck(cmd)

	// this needs to appear last:
	referenceCmd.Long = referenceLong(cmd)
	return cmd
}

func bareHTTPClient(f *cmdutil.Factory, version string) func() (*http.Client, error) {
	return func() (*http.Client, error) {
		cfg, err := f.Config()
		if err != nil {
			return nil, err
		}
		return factory.NewHTTPClient(f.IOStreams, cfg, version, false)
	}
}

func newCodespaceCmd(f *cmdutil.Factory) *cobra.Command {
	serverURL := os.Getenv("GITHUB_SERVER_URL")
	apiURL := os.Getenv("GITHUB_API_URL")
	vscsURL := os.Getenv("INTERNAL_VSCS_TARGET_URL")
	app := codespaceCmd.NewApp(
		f.IOStreams,
		codespacesAPI.New(
			serverURL,
			apiURL,
			vscsURL,
			&lazyLoadedHTTPClient{factory: f},
		),
	)
	cmd := codespaceCmd.NewRootCmd(app)
	cmd.Use = "codespace"
	cmd.Aliases = []string{"cs"}
	cmd.Annotations = map[string]string{"IsCore": "true"}
	return cmd
}

type lazyLoadedHTTPClient struct {
	factory *cmdutil.Factory

	httpClientMu sync.RWMutex // guards httpClient
	httpClient   *http.Client
}

func (l *lazyLoadedHTTPClient) Do(req *http.Request) (*http.Response, error) {
	l.httpClientMu.RLock()
	httpClient := l.httpClient
	l.httpClientMu.RUnlock()

	if httpClient == nil {
		var err error
		l.httpClientMu.Lock()
		l.httpClient, err = l.factory.HttpClient()
		l.httpClientMu.Unlock()
		if err != nil {
			return nil, err
		}
	}

	return l.httpClient.Do(req)
}
