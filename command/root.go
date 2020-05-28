package command

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	apiCmd "github.com/cli/cli/pkg/cmd/api"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TODO these are sprinkled across command, context, config, and ghrepo
const defaultHostname = "github.com"

// Version is dynamically set by the toolchain or overridden by the Makefile.
var Version = "DEV"

// BuildDate is dynamically set at build time in the Makefile.
var BuildDate = "" // YYYY-MM-DD

var versionOutput = ""
var cobraDefaultHelpFunc func(*cobra.Command, []string)

func init() {
	if Version == "DEV" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}
	Version = strings.TrimPrefix(Version, "v")
	if BuildDate == "" {
		RootCmd.Version = Version
	} else {
		RootCmd.Version = fmt.Sprintf("%s (%s)", Version, BuildDate)
	}
	versionOutput = fmt.Sprintf("gh version %s\n%s\n", RootCmd.Version, changelogURL(Version))
	RootCmd.AddCommand(versionCmd)
	RootCmd.SetVersionTemplate(versionOutput)

	RootCmd.PersistentFlags().StringP("repo", "R", "", "Select another repository using the `OWNER/REPO` format")
	RootCmd.PersistentFlags().Bool("help", false, "Show help for command")
	RootCmd.Flags().Bool("version", false, "Show gh version")
	// TODO:
	// RootCmd.PersistentFlags().BoolP("verbose", "V", false, "enable verbose output")

	cobraDefaultHelpFunc = RootCmd.HelpFunc()
	RootCmd.SetHelpFunc(rootHelpFunc)

	RootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		if err == pflag.ErrHelp {
			return err
		}
		return &cmdutil.FlagError{Err: err}
	})

	// TODO: iron out how a factory incorporates context
	cmdFactory := &cmdutil.Factory{
		IOStreams: iostreams.System(),
		HttpClient: func() (*http.Client, error) {
			token := os.Getenv("GITHUB_TOKEN")
			if len(token) == 0 {
				ctx := context.New()
				var err error
				token, err = ctx.AuthToken()
				if err != nil {
					return nil, err
				}
			}
			return httpClient(token), nil
		},
	}
	RootCmd.AddCommand(apiCmd.NewCmdApi(cmdFactory, nil))
}

// RootCmd is the entry point of command-line execution
var RootCmd = &cobra.Command{
	Use:   "gh <command> <subcommand> [flags]",
	Short: "GitHub CLI",
	Long:  `Work seamlessly with GitHub from the command line.`,

	SilenceErrors: true,
	SilenceUsage:  true,
}

var versionCmd = &cobra.Command{
	Use:    "version",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(versionOutput)
	},
}

// overridden in tests
var initContext = func() context.Context {
	ctx := context.New()
	if repo := os.Getenv("GH_REPO"); repo != "" {
		ctx.SetBaseRepo(repo)
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		ctx.SetAuthToken(token)
	}
	return ctx
}

// BasicClient returns an API client that borrows from but does not depend on
// user configuration
func BasicClient() (*api.Client, error) {
	var opts []api.ClientOption
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, apiVerboseLog())
	}
	opts = append(opts, api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)))

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		if c, err := config.ParseDefaultConfig(); err == nil {
			token, _ = c.Get(defaultHostname, "oauth_token")
		}
	}
	if token != "" {
		opts = append(opts, api.AddHeader("Authorization", fmt.Sprintf("token %s", token)))
	}
	return api.NewClient(opts...), nil
}

func contextForCommand(cmd *cobra.Command) context.Context {
	ctx := initContext()
	if repo, err := cmd.Flags().GetString("repo"); err == nil && repo != "" {
		ctx.SetBaseRepo(repo)
	}
	return ctx
}

// for cmdutil-powered commands
func httpClient(token string) *http.Client {
	var opts []api.ClientOption
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, apiVerboseLog())
	}
	opts = append(opts,
		api.AddHeader("Authorization", fmt.Sprintf("token %s", token)),
		api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)),
	)
	return api.NewHTTPClient(opts...)
}

// overridden in tests
var apiClientForContext = func(ctx context.Context) (*api.Client, error) {
	token, err := ctx.AuthToken()
	if err != nil {
		return nil, err
	}

	var opts []api.ClientOption
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, apiVerboseLog())
	}

	getAuthValue := func() string {
		return fmt.Sprintf("token %s", token)
	}

	tokenFromEnv := func() bool {
		return os.Getenv("GITHUB_TOKEN") == token
	}

	checkScopesFunc := func(appID string) error {
		if config.IsGitHubApp(appID) && !tokenFromEnv() && utils.IsTerminal(os.Stdin) && utils.IsTerminal(os.Stderr) {
			newToken, loginHandle, err := config.AuthFlow("Notice: additional authorization required")
			if err != nil {
				return err
			}
			cfg, err := ctx.Config()
			if err != nil {
				return err
			}
			_ = cfg.Set(defaultHostname, "oauth_token", newToken)
			_ = cfg.Set(defaultHostname, "user", loginHandle)
			// update config file on disk
			err = cfg.Write()
			if err != nil {
				return err
			}
			// update configuration in memory
			token = newToken
			config.AuthFlowComplete()
		} else {
			fmt.Fprintln(os.Stderr, "Warning: gh now requires the `read:org` OAuth scope.")
			fmt.Fprintln(os.Stderr, "Visit https://github.com/settings/tokens and edit your token to enable `read:org`")
			if tokenFromEnv() {
				fmt.Fprintln(os.Stderr, "or generate a new token for the GITHUB_TOKEN environment variable")
			} else {
				fmt.Fprintln(os.Stderr, "or generate a new token and paste it via `gh config set -h github.com oauth_token MYTOKEN`")
			}
		}
		return nil
	}

	opts = append(opts,
		api.CheckScopes("read:org", checkScopesFunc),
		api.AddHeaderFunc("Authorization", getAuthValue),
		api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)),
		// antiope-preview: Checks
		api.AddHeader("Accept", "application/vnd.github.antiope-preview+json"),
	)

	return api.NewClient(opts...), nil
}

var ensureScopes = func(ctx context.Context, client *api.Client, wantedScopes ...string) (*api.Client, error) {
	hasScopes, appID, err := client.HasScopes(wantedScopes...)
	if err != nil {
		return client, err
	}

	if hasScopes {
		return client, nil
	}

	tokenFromEnv := len(os.Getenv("GITHUB_TOKEN")) > 0

	if config.IsGitHubApp(appID) && !tokenFromEnv && utils.IsTerminal(os.Stdin) && utils.IsTerminal(os.Stderr) {
		newToken, loginHandle, err := config.AuthFlow("Notice: additional authorization required")
		if err != nil {
			return client, err
		}
		cfg, err := ctx.Config()
		if err != nil {
			return client, err
		}
		_ = cfg.Set(defaultHostname, "oauth_token", newToken)
		_ = cfg.Set(defaultHostname, "user", loginHandle)
		// update config file on disk
		err = cfg.Write()
		if err != nil {
			return client, err
		}
		// update configuration in memory
		config.AuthFlowComplete()
		reloadedClient, err := apiClientForContext(ctx)
		if err != nil {
			return client, err
		}

		return reloadedClient, nil
	} else {
		fmt.Fprintln(os.Stderr, fmt.Sprintf("Warning: gh now requires %s OAuth scopes.", wantedScopes))
		fmt.Fprintln(os.Stderr, fmt.Sprintf("Visit https://github.com/settings/tokens and edit your token to enable %s", wantedScopes))
		if tokenFromEnv {
			fmt.Fprintln(os.Stderr, "or generate a new token for the GITHUB_TOKEN environment variable")
		} else {
			fmt.Fprintln(os.Stderr, "or generate a new token and paste it via `gh config set -h github.com oauth_token MYTOKEN`")
		}
		return client, errors.New("Unable to reauthenticate")
	}

}

func apiVerboseLog() api.ClientOption {
	logTraffic := strings.Contains(os.Getenv("DEBUG"), "api")
	colorize := utils.IsTerminal(os.Stderr)
	return api.VerboseLog(utils.NewColorable(os.Stderr), logTraffic, colorize)
}

func colorableOut(cmd *cobra.Command) io.Writer {
	out := cmd.OutOrStdout()
	if outFile, isFile := out.(*os.File); isFile {
		return utils.NewColorable(outFile)
	}
	return out
}

func colorableErr(cmd *cobra.Command) io.Writer {
	err := cmd.ErrOrStderr()
	if outFile, isFile := err.(*os.File); isFile {
		return utils.NewColorable(outFile)
	}
	return err
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

func determineBaseRepo(apiClient *api.Client, cmd *cobra.Command, ctx context.Context) (ghrepo.Interface, error) {
	repo, err := cmd.Flags().GetString("repo")
	if err == nil && repo != "" {
		baseRepo, err := ghrepo.FromFullName(repo)
		if err != nil {
			return nil, fmt.Errorf("argument error: %w", err)
		}
		return baseRepo, nil
	}

	baseOverride, err := cmd.Flags().GetString("repo")
	if err != nil {
		return nil, err
	}

	remotes, err := ctx.Remotes()
	if err != nil {
		return nil, err
	}

	repoContext, err := context.ResolveRemotesToRepos(remotes, apiClient, baseOverride)
	if err != nil {
		return nil, err
	}

	baseRepo, err := repoContext.BaseRepo()
	if err != nil {
		return nil, err
	}

	return baseRepo, nil
}

func rootHelpFunc(command *cobra.Command, args []string) {
	if command != RootCmd {
		// Display helpful error message in case subcommand name was mistyped.
		// This matches Cobra's behavior for root command, which Cobra
		// confusingly doesn't apply to nested commands.
		if command.Parent() == RootCmd && len(args) >= 2 {
			if command.SuggestionsMinimumDistance <= 0 {
				command.SuggestionsMinimumDistance = 2
			}
			candidates := command.SuggestionsFor(args[1])

			errOut := command.OutOrStderr()
			fmt.Fprintf(errOut, "unknown command %q for %q\n", args[1], "gh "+args[0])

			if len(candidates) > 0 {
				fmt.Fprint(errOut, "\nDid you mean this?\n")
				for _, c := range candidates {
					fmt.Fprintf(errOut, "\t%s\n", c)
				}
				fmt.Fprint(errOut, "\n")
			}

			oldOut := command.OutOrStdout()
			command.SetOut(errOut)
			defer command.SetOut(oldOut)
		}
		cobraDefaultHelpFunc(command, args)
		return
	}

	type helpEntry struct {
		Title string
		Body  string
	}

	coreCommandNames := []string{"issue", "pr", "repo"}
	var coreCommands []string
	var additionalCommands []string
	for _, c := range command.Commands() {
		if c.Short == "" {
			continue
		}
		s := "  " + rpad(c.Name()+":", c.NamePadding()) + c.Short
		if includes(coreCommandNames, c.Name()) {
			coreCommands = append(coreCommands, s)
		} else if c != creditsCmd {
			additionalCommands = append(additionalCommands, s)
		}
	}

	helpEntries := []helpEntry{
		{
			"",
			command.Long},
		{"USAGE", command.Use},
		{"CORE COMMANDS", strings.Join(coreCommands, "\n")},
		{"ADDITIONAL COMMANDS", strings.Join(additionalCommands, "\n")},
		{"FLAGS", strings.TrimRight(command.LocalFlags().FlagUsages(), "\n")},
		{"EXAMPLES", `
  $ gh issue create
  $ gh repo clone
  $ gh pr checkout 321`},
		{"LEARN MORE", `
  Use "gh <command> <subcommand> --help" for more information about a command.
  Read the manual at <http://cli.github.com/manual>`},
		{"FEEDBACK", `
  Fill out our feedback form <https://forms.gle/umxd3h31c7aMQFKG7>
  Open an issue using “gh issue create -R cli/cli”`},
	}

	out := colorableOut(command)
	for _, e := range helpEntries {
		if e.Title != "" {
			fmt.Fprintln(out, utils.Bold(e.Title))
		}
		fmt.Fprintln(out, strings.TrimLeft(e.Body, "\n")+"\n")
	}
}

// rpad adds padding to the right of a string.
func rpad(s string, padding int) string {
	template := fmt.Sprintf("%%-%ds ", padding)
	return fmt.Sprintf(template, s)
}

func includes(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}

func formatRemoteURL(cmd *cobra.Command, fullRepoName string) string {
	ctx := contextForCommand(cmd)

	protocol := "https"
	cfg, err := ctx.Config()
	if err != nil {
		fmt.Fprintf(colorableErr(cmd), "%s failed to load config: %s. using defaults\n", utils.Yellow("!"), err)
	} else {
		cfgProtocol, _ := cfg.Get(defaultHostname, "git_protocol")
		protocol = cfgProtocol
	}

	if protocol == "ssh" {
		return fmt.Sprintf("git@%s:%s.git", defaultHostname, fullRepoName)
	}

	return fmt.Sprintf("https://%s/%s.git", defaultHostname, fullRepoName)
}

func determineEditor(cmd *cobra.Command) (string, error) {
	editorCommand := os.Getenv("GH_EDITOR")
	if editorCommand == "" {
		ctx := contextForCommand(cmd)
		cfg, err := ctx.Config()
		if err != nil {
			return "", fmt.Errorf("could not read config: %w", err)
		}
		editorCommand, _ = cfg.Get(defaultHostname, "editor")
	}

	return editorCommand, nil
}
