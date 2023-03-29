package root

import (
	"net/http"
	"os"
	"sync"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
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
	labelCmd "github.com/cli/cli/v2/pkg/cmd/label"
	orgCmd "github.com/cli/cli/v2/pkg/cmd/org"
	prCmd "github.com/cli/cli/v2/pkg/cmd/pr"
	releaseCmd "github.com/cli/cli/v2/pkg/cmd/release"
	repoCmd "github.com/cli/cli/v2/pkg/cmd/repo"
	creditsCmd "github.com/cli/cli/v2/pkg/cmd/repo/credits"
	runCmd "github.com/cli/cli/v2/pkg/cmd/run"
	searchCmd "github.com/cli/cli/v2/pkg/cmd/search"
	secretCmd "github.com/cli/cli/v2/pkg/cmd/secret"
	sshKeyCmd "github.com/cli/cli/v2/pkg/cmd/ssh-key"
	statusCmd "github.com/cli/cli/v2/pkg/cmd/status"
	variableCmd "github.com/cli/cli/v2/pkg/cmd/variable"
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

		Example: heredoc.Doc(`
			$ gh issue create
			$ gh repo clone cli/cli
			$ gh pr checkout 321
		`),
		Annotations: map[string]string{
			"versionInfo": versionCmd.Format(version, buildDate),
		},
	}

	// cmd.SetOut(f.IOStreams.Out)    // can't use due to https://github.com/spf13/cobra/issues/1708
	// cmd.SetErr(f.IOStreams.ErrOut) // just let it default to os.Stderr instead

	cmd.PersistentFlags().Bool("help", false, "Show help for command")

	// override Cobra's default behaviors unless an opt-out has been set
	if os.Getenv("GH_COBRA") == "" {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true

		// this --version flag is checked in rootHelpFunc
		cmd.Flags().Bool("version", false, "Show gh version")

		cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
			rootHelpFunc(f, c, args)
		})
		cmd.SetUsageFunc(func(c *cobra.Command) error {
			return rootUsageFunc(f.IOStreams.ErrOut, c)
		})
		cmd.SetFlagErrorFunc(rootFlagErrorFunc)
	}

	cmd.AddGroup(&cobra.Group{
		ID:    "core",
		Title: "Core commands",
	})
	cmd.AddGroup(&cobra.Group{
		ID:    "actions",
		Title: "GitHub Actions commands",
	})

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
	cmd.AddCommand(searchCmd.NewCmdSearch(f))
	cmd.AddCommand(secretCmd.NewCmdSecret(f))
	cmd.AddCommand(variableCmd.NewCmdVariable(f))
	cmd.AddCommand(sshKeyCmd.NewCmdSSHKey(f))
	cmd.AddCommand(statusCmd.NewCmdStatus(f, nil))
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
	cmd.AddCommand(orgCmd.NewCmdOrg(&repoResolvingCmdFactory))
	cmd.AddCommand(issueCmd.NewCmdIssue(&repoResolvingCmdFactory))
	cmd.AddCommand(releaseCmd.NewCmdRelease(&repoResolvingCmdFactory))
	cmd.AddCommand(repoCmd.NewCmdRepo(&repoResolvingCmdFactory))
	cmd.AddCommand(runCmd.NewCmdRun(&repoResolvingCmdFactory))
	cmd.AddCommand(workflowCmd.NewCmdWorkflow(&repoResolvingCmdFactory))
	cmd.AddCommand(labelCmd.NewCmdLabel(&repoResolvingCmdFactory))

	// Help topics
	cmd.AddCommand(NewHelpTopic(f.IOStreams, "environment"))
	cmd.AddCommand(NewHelpTopic(f.IOStreams, "formatting"))
	cmd.AddCommand(NewHelpTopic(f.IOStreams, "mintty"))
	cmd.AddCommand(NewHelpTopic(f.IOStreams, "exit-codes"))
	referenceCmd := NewHelpTopic(f.IOStreams, "reference")
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
		opts := api.HTTPClientOptions{
			AppVersion:        version,
			Config:            cfg.Authentication(),
			Log:               f.IOStreams.ErrOut,
			LogColorize:       f.IOStreams.ColorEnabled(),
			SkipAcceptHeaders: true,
		}
		return api.NewHTTPClient(opts)
	}
}

func newCodespaceCmd(f *cmdutil.Factory) *cobra.Command {
	serverURL := os.Getenv("GITHUB_SERVER_URL")
	apiURL := os.Getenv("GITHUB_API_URL")
	vscsURL := os.Getenv("INTERNAL_VSCS_TARGET_URL")
	app := codespaceCmd.NewApp(
		f.IOStreams,
		f,
		codespacesAPI.New(
			serverURL,
			apiURL,
			vscsURL,
			&lazyLoadedHTTPClient{factory: f},
		),
		f.Browser,
		f.Remotes,
	)
	cmd := codespaceCmd.NewRootCmd(app)
	cmd.Use = "codespace"
	cmd.Aliases = []string{"cs"}
	cmd.GroupID = "core"
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
