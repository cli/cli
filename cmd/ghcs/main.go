package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		explainError(os.Stderr, err)
		os.Exit(1)
	}
}

var version = "DEV"

var rootCmd = &cobra.Command{
	Use:           "ghcs",
	SilenceUsage:  true,  // don't print usage message after each error (see #80)
	SilenceErrors: false, // print errors automatically so that main need not
	Long: `Unofficial CLI tool to manage GitHub Codespaces.

Running commands requires the GITHUB_TOKEN environment variable to be set to a
token to access the GitHub API with.`,
	Version: version,

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("GITHUB_TOKEN") == "" {
			return tokenError
		}
		return nil
	},
}

var tokenError = errors.New("GITHUB_TOKEN is missing")

func explainError(w io.Writer, err error) {
	if errors.Is(err, tokenError) {
		fmt.Fprintln(w, "The GITHUB_TOKEN environment variable is required. Create a Personal Access Token at https://github.com/settings/tokens/new?scopes=repo")
		fmt.Fprintln(w, "Make sure to enable SSO for your organizations after creating the token.")
		return
	}
}
