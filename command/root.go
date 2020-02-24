package command

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"

	"github.com/spf13/cobra"
)

// Version is dynamically set by the toolchain or overriden by the Makefile.
var Version = "DEV"

// BuildDate is dynamically set at build time in the Makefile.
var BuildDate = "" // YYYY-MM-DD

var versionOutput = ""

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

	RootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &FlagError{Err: err}
	})
}

// FlagError is the kind of error raised in flag processing
type FlagError struct {
	Err error
}

func (fe FlagError) Error() string {
	return fe.Err.Error()
}

func (fe FlagError) Unwrap() error {
	return fe.Err
}

// RootCmd is the entry point of command-line execution
var RootCmd = &cobra.Command{
	Use:   "gh",
	Short: "GitHub CLI",
	Long: `Work seamlessly with GitHub from the command line.

GitHub CLI is in early stages of development, and we'd love to hear your
feedback at <https://forms.gle/umxd3h31c7aMQFKG7>`,

	SilenceErrors: true,
	SilenceUsage:  true,
}

var versionCmd = &cobra.Command{
	Use:    "version",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(versionOutput)
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

// BasicClient returns an API client that borrows from but does not depend on
// user configuration
func BasicClient() (*api.Client, error) {
	opts := []api.ClientOption{}
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, apiVerboseLog())
	}
	opts = append(opts, api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)))
	if c, err := context.ParseDefaultConfig(); err == nil {
		opts = append(opts, api.AddHeader("Authorization", fmt.Sprintf("token %s", c.Token)))
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

// overridden in tests
var apiClientForContext = func(ctx context.Context) (*api.Client, error) {
	token, err := ctx.AuthToken()
	if err != nil {
		return nil, err
	}
	opts := []api.ClientOption{}
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, apiVerboseLog())
	}
	opts = append(opts,
		api.AddHeader("Authorization", fmt.Sprintf("token %s", token)),
		api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)),
		// antiope-preview: Checks
		// shadow-cat-preview: Draft pull requests
		api.AddHeader("Accept", "application/vnd.github.antiope-preview+json, application/vnd.github.shadow-cat-preview"),
		api.AddHeader("GraphQL-Features", "pe_mobile"),
	)

	return api.NewClient(opts...), nil
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

func determineBaseRepo(cmd *cobra.Command, ctx context.Context) (*ghrepo.Interface, error) {
	apiClient, err := apiClientForContext(ctx)
	if err != nil {
		return nil, err
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

	var baseRepo ghrepo.Interface
	baseRepo, err = repoContext.BaseRepo()
	if err != nil {
		return nil, err
	}

	return &baseRepo, nil
}
