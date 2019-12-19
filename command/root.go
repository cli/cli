package command

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/utils"

	"github.com/spf13/cobra"
)

// Version is dynamically set at build time in the Makefile
var Version = "DEV"

// BuildDate is dynamically set at build time in the Makefile
var BuildDate = "YYYY-MM-DD"

func init() {
	RootCmd.Version = fmt.Sprintf("%s (%s)", strings.TrimPrefix(Version, "v"), BuildDate)
	RootCmd.AddCommand(versionCmd)

	RootCmd.PersistentFlags().StringP("repo", "R", "", "Select another repository using the `OWNER/REPO` format")
	RootCmd.PersistentFlags().Bool("help", false, "Show help for command")
	RootCmd.Flags().Bool("version", false, "Print gh version")
	// TODO:
	// RootCmd.PersistentFlags().BoolP("verbose", "V", false, "enable verbose output")

	RootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return FlagError{err}
	})
}

// FlagError is the kind of error raised in flag processing
type FlagError struct {
	error
}

// RootCmd is the entry point of command-line execution
var RootCmd = &cobra.Command{
	Use:   "gh",
	Short: "GitHub CLI",
	Long: `Work seamlessly with GitHub from the command line.

GitHub CLI is in early stages of development, and we'd love to hear your
feedback at <https://forms.gle/pBt3kujJi7nXZmcM6>`,

	SilenceErrors: true,
	SilenceUsage:  true,
}

var versionCmd = &cobra.Command{
	Use:    "version",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gh version %s\n", RootCmd.Version)
	},
}

// overriden in tests
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
	opts := []api.ClientOption{
		api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)),
	}
	if c, err := context.ParseDefaultConfig(); err == nil {
		opts = append(opts, api.AddHeader("Authorization", fmt.Sprintf("token %s", c.Token)))
	}
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, api.VerboseLog(os.Stderr))
	}
	return api.NewClient(opts...), nil
}

func contextForCommand(cmd *cobra.Command) context.Context {
	ctx := initContext()
	if repo, err := cmd.Flags().GetString("repo"); err == nil && repo != "" {
		ctx.SetBaseRepo(repo)
		ctx.SetBranch("master")
	}
	return ctx
}

// overriden in tests
var apiClientForContext = func(ctx context.Context) (*api.Client, error) {
	token, err := ctx.AuthToken()
	if err != nil {
		return nil, err
	}
	opts := []api.ClientOption{
		api.AddHeader("Authorization", fmt.Sprintf("token %s", token)),
		api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)),
		// antiope-preview: Checks
		// shadow-cat-preview: Draft pull requests
		api.AddHeader("Accept", "application/vnd.github.antiope-preview+json, application/vnd.github.shadow-cat-preview"),
		api.AddHeader("GraphQL-Features", "pe_mobile"),
	}
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, api.VerboseLog(os.Stderr))
	}
	return api.NewClient(opts...), nil
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
