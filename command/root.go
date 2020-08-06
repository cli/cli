package command

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	apiCmd "github.com/cli/cli/pkg/cmd/api"
	authCmd "github.com/cli/cli/pkg/cmd/auth"
	authLoginCmd "github.com/cli/cli/pkg/cmd/auth/login"
	gistCreateCmd "github.com/cli/cli/pkg/cmd/gist/create"
	prCheckoutCmd "github.com/cli/cli/pkg/cmd/pr/checkout"
	prDiffCmd "github.com/cli/cli/pkg/cmd/pr/diff"
	prReviewCmd "github.com/cli/cli/pkg/cmd/pr/review"
	repoCmd "github.com/cli/cli/pkg/cmd/repo"
	repoCloneCmd "github.com/cli/cli/pkg/cmd/repo/clone"
	repoCreateCmd "github.com/cli/cli/pkg/cmd/repo/create"
	creditsCmd "github.com/cli/cli/pkg/cmd/repo/credits"
	repoForkCmd "github.com/cli/cli/pkg/cmd/repo/fork"
	repoViewCmd "github.com/cli/cli/pkg/cmd/repo/view"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/google/shlex"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Version is dynamically set by the toolchain or overridden by the Makefile.
var Version = "DEV"

// BuildDate is dynamically set at build time in the Makefile.
var BuildDate = "" // YYYY-MM-DD

var versionOutput = ""

var defaultStreams *iostreams.IOStreams

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

	RootCmd.PersistentFlags().Bool("help", false, "Show help for command")
	RootCmd.Flags().Bool("version", false, "Show gh version")
	// TODO:
	// RootCmd.PersistentFlags().BoolP("verbose", "V", false, "enable verbose output")

	RootCmd.SetHelpFunc(rootHelpFunc)
	RootCmd.SetUsageFunc(rootUsageFunc)

	RootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		if err == pflag.ErrHelp {
			return err
		}
		return &cmdutil.FlagError{Err: err}
	})

	defaultStreams = iostreams.System()

	// TODO: iron out how a factory incorporates context
	cmdFactory := &cmdutil.Factory{
		IOStreams: defaultStreams,
		HttpClient: func() (*http.Client, error) {
			// TODO: decouple from `context`
			ctx := context.New()
			cfg, err := ctx.Config()
			if err != nil {
				return nil, err
			}

			// TODO: avoid setting Accept header for `api` command
			return httpClient(defaultStreams, cfg, true), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			// TODO: decouple from `context`
			ctx := context.New()
			return ctx.BaseRepo()
		},
		Remotes: func() (context.Remotes, error) {
			ctx := context.New()
			return ctx.Remotes()
		},
		Config: func() (config.Config, error) {
			cfg, err := config.ParseDefaultConfig()
			if errors.Is(err, os.ErrNotExist) {
				cfg = config.NewBlankConfig()
			} else if err != nil {
				return nil, err
			}
			return cfg, nil
		},
		Branch: func() (string, error) {
			currentBranch, err := git.CurrentBranch()
			if err != nil {
				return "", fmt.Errorf("could not determine current branch: %w", err)
			}
			return currentBranch, nil
		},
	}
	RootCmd.AddCommand(apiCmd.NewCmdApi(cmdFactory, nil))

	gistCmd := &cobra.Command{
		Use:   "gist",
		Short: "Create gists",
		Long:  `Work with GitHub gists.`,
	}
	RootCmd.AddCommand(gistCmd)
	gistCmd.AddCommand(gistCreateCmd.NewCmdCreate(cmdFactory, nil))

	RootCmd.AddCommand(authCmd.Cmd)
	authCmd.Cmd.AddCommand(authLoginCmd.NewCmdLogin(cmdFactory, nil))

	resolvedBaseRepo := func() (ghrepo.Interface, error) {
		httpClient, err := cmdFactory.HttpClient()
		if err != nil {
			return nil, err
		}

		apiClient := api.NewClientFromHTTP(httpClient)

		ctx := context.New()
		remotes, err := ctx.Remotes()
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

	repoResolvingCmdFactory := *cmdFactory

	repoResolvingCmdFactory.BaseRepo = resolvedBaseRepo

	RootCmd.AddCommand(repoCmd.Cmd)
	repoCmd.Cmd.AddCommand(repoViewCmd.NewCmdView(&repoResolvingCmdFactory, nil))
	repoCmd.Cmd.AddCommand(repoForkCmd.NewCmdFork(&repoResolvingCmdFactory, nil))
	repoCmd.Cmd.AddCommand(repoCloneCmd.NewCmdClone(cmdFactory, nil))
	repoCmd.Cmd.AddCommand(repoCreateCmd.NewCmdCreate(cmdFactory, nil))
	repoCmd.Cmd.AddCommand(creditsCmd.NewCmdRepoCredits(&repoResolvingCmdFactory, nil))

	prCmd.AddCommand(prReviewCmd.NewCmdReview(&repoResolvingCmdFactory, nil))
	prCmd.AddCommand(prDiffCmd.NewCmdDiff(&repoResolvingCmdFactory, nil))
	prCmd.AddCommand(prCheckoutCmd.NewCmdCheckout(&repoResolvingCmdFactory, nil))

	RootCmd.AddCommand(creditsCmd.NewCmdCredits(cmdFactory, nil))
}

// RootCmd is the entry point of command-line execution
var RootCmd = &cobra.Command{
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
			Fill out our feedback form https://forms.gle/umxd3h31c7aMQFKG7
			Open an issue using “gh issue create -R cli/cli”
		`),
		"help:environment": heredoc.Doc(`
			GITHUB_TOKEN: an authentication token for API requests. Setting this avoids being
			prompted to authenticate and overrides any previously stored credentials.

			GH_REPO: specify the GitHub repository in "OWNER/REPO" format for commands that
			otherwise operate on a local repository.

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
	return ctx
}

// BasicClient returns an API client for github.com only that borrows from but
// does not depend on user configuration
func BasicClient() (*api.Client, error) {
	var opts []api.ClientOption
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, apiVerboseLog())
	}
	opts = append(opts, api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)))

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		if c, err := config.ParseDefaultConfig(); err == nil {
			token, _ = c.Get(ghinstance.Default(), "oauth_token")
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

// generic authenticated HTTP client for commands
func httpClient(io *iostreams.IOStreams, cfg config.Config, setAccept bool) *http.Client {
	var opts []api.ClientOption
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, apiVerboseLog())
	}

	opts = append(opts,
		api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)),
		api.AddHeaderFunc("Authorization", func(req *http.Request) (string, error) {
			if token := os.Getenv("GITHUB_TOKEN"); token != "" {
				return fmt.Sprintf("token %s", token), nil
			}

			hostname := ghinstance.NormalizeHostname(req.URL.Hostname())
			token, err := cfg.Get(hostname, "oauth_token")
			if token == "" {
				var notFound *config.NotFoundError
				// TODO: check if stdout is TTY too
				if errors.As(err, &notFound) && io.IsStdinTTY() {
					// interactive OAuth flow
					token, err = config.AuthFlowWithConfig(cfg, hostname, "Notice: authentication required")
				}
				if err != nil {
					return "", err
				}
				if token == "" {
					// TODO: instruct user how to manually authenticate
					return "", fmt.Errorf("authentication required for %s", hostname)
				}
			}

			return fmt.Sprintf("token %s", token), nil
		}),
	)

	if setAccept {
		opts = append(opts,
			api.AddHeaderFunc("Accept", func(req *http.Request) (string, error) {
				// antiope-preview: Checks
				accept := "application/vnd.github.antiope-preview+json"
				if ghinstance.IsEnterprise(req.URL.Hostname()) {
					// shadow-cat-preview: Draft pull requests
					accept += ", application/vnd.github.shadow-cat-preview"
				}
				return accept, nil
			}),
		)
	}

	return api.NewHTTPClient(opts...)
}

// LEGACY; overridden in tests
var apiClientForContext = func(ctx context.Context) (*api.Client, error) {
	cfg, err := ctx.Config()
	if err != nil {
		return nil, err
	}

	http := httpClient(defaultStreams, cfg, true)
	return api.NewClientFromHTTP(http), nil
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
	repo, _ := cmd.Flags().GetString("repo")
	if repo != "" {
		baseRepo, err := ghrepo.FromFullName(repo)
		if err != nil {
			return nil, fmt.Errorf("argument error: %w", err)
		}
		return baseRepo, nil
	}

	remotes, err := ctx.Remotes()
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

// TODO there is a parallel implementation for isolated commands
func formatRemoteURL(cmd *cobra.Command, repo ghrepo.Interface) string {
	ctx := contextForCommand(cmd)

	var protocol string
	cfg, err := ctx.Config()
	if err != nil {
		fmt.Fprintf(colorableErr(cmd), "%s failed to load config: %s. using defaults\n", utils.Yellow("!"), err)
	} else {
		protocol, _ = cfg.Get(repo.RepoHost(), "git_protocol")
	}

	if protocol == "ssh" {
		return fmt.Sprintf("git@%s:%s/%s.git", repo.RepoHost(), repo.RepoOwner(), repo.RepoName())
	}

	return fmt.Sprintf("https://%s/%s/%s.git", repo.RepoHost(), repo.RepoOwner(), repo.RepoName())
}

// TODO there is a parallel implementation for isolated commands
func determineEditor(cmd *cobra.Command) (string, error) {
	editorCommand := os.Getenv("GH_EDITOR")
	if editorCommand == "" {
		ctx := contextForCommand(cmd)
		cfg, err := ctx.Config()
		if err != nil {
			return "", fmt.Errorf("could not read config: %w", err)
		}
		// TODO: consider supporting setting an editor per GHE host
		editorCommand, _ = cfg.Get(ghinstance.Default(), "editor")
	}

	return editorCommand, nil
}

func ExecuteShellAlias(args []string) error {
	externalCmd := exec.Command(args[0], args[1:]...)
	externalCmd.Stderr = os.Stderr
	externalCmd.Stdout = os.Stdout
	externalCmd.Stdin = os.Stdin
	preparedCmd := run.PrepareCmd(externalCmd)

	return preparedCmd.Run()
}

var findSh = func() (string, error) {
	shPath, err := exec.LookPath("sh")
	if err == nil {
		return shPath, nil
	}

	if runtime.GOOS == "windows" {
		winNotFoundErr := errors.New("unable to locate sh to execute the shell alias with. The sh.exe interpreter is typically distributed with Git for Windows.")
		// We can try and find a sh executable in a Git for Windows install
		gitPath, err := exec.LookPath("git")
		if err != nil {
			return "", winNotFoundErr
		}

		shPath = filepath.Join(filepath.Dir(gitPath), "..", "bin", "sh.exe")
		_, err = os.Stat(shPath)
		if err != nil {
			return "", winNotFoundErr
		}

		return shPath, nil
	}

	return "", errors.New("unable to locate sh to execute shell alias with")
}

// ExpandAlias processes argv to see if it should be rewritten according to a user's aliases. The
// second return value indicates whether the alias should be executed in a new shell process instead
// of running gh itself.
func ExpandAlias(args []string) (expanded []string, isShell bool, err error) {
	err = nil
	isShell = false
	expanded = []string{}

	if len(args) < 2 {
		// the command is lacking a subcommand
		return
	}

	ctx := initContext()
	cfg, err := ctx.Config()
	if err != nil {
		return
	}
	aliases, err := cfg.Aliases()
	if err != nil {
		return
	}

	expansion, ok := aliases.Get(args[1])
	if ok {
		if strings.HasPrefix(expansion, "!") {
			isShell = true
			shPath, shErr := findSh()
			if shErr != nil {
				err = shErr
				return
			}

			expanded = []string{shPath, "-c", expansion[1:]}

			if len(args[2:]) > 0 {
				expanded = append(expanded, "--")
				expanded = append(expanded, args[2:]...)
			}

			return
		}

		extraArgs := []string{}
		for i, a := range args[2:] {
			if !strings.Contains(expansion, "$") {
				extraArgs = append(extraArgs, a)
			} else {
				expansion = strings.ReplaceAll(expansion, fmt.Sprintf("$%d", i+1), a)
			}
		}
		lingeringRE := regexp.MustCompile(`\$\d`)
		if lingeringRE.MatchString(expansion) {
			err = fmt.Errorf("not enough arguments for alias: %s", expansion)
			return
		}

		var newArgs []string
		newArgs, err = shlex.Split(expansion)
		if err != nil {
			return
		}

		expanded = append(newArgs, extraArgs...)
		return
	}

	expanded = args[1:]
	return
}

func connectedToTerminal(cmd *cobra.Command) bool {
	return utils.IsTerminal(cmd.InOrStdin()) && utils.IsTerminal(cmd.OutOrStdout())
}
