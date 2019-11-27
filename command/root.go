package command

import (
	"fmt"
	"os"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/context"

	"github.com/spf13/cobra"
)

// Version is dynamically set at build time in the Makefile
var Version = "DEV"

// BuildDate is dynamically set at build time in the Makefile
var BuildDate = "YYYY-MM-DD"

func init() {
	RootCmd.Version = fmt.Sprintf("%s (%s)", Version, BuildDate)
	RootCmd.PersistentFlags().StringP("repo", "R", "", "Current GitHub repository")
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
	Long: `Work seamlessly with GitHub from the command line`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

// overriden in tests
var initContext = func() context.Context {
	ctx := context.New()
	if repo := os.Getenv("GH_REPO"); repo != "" {
		ctx.SetBaseRepo(repo)
	}
	return ctx
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
