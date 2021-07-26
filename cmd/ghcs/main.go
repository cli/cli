package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	Execute()
}

var Version = "DEV"

var rootCmd = &cobra.Command{
	Use:     "ghcs",
	Short:   "Unofficial GitHub Codespaces CLI.",
	Long:    "Unofficial CLI tool to manage and interact with GitHub Codespaces.",
	Version: Version,
}

func Execute() {
	if os.Getenv("GITHUB_TOKEN") == "" {
		fmt.Println("The GITHUB_TOKEN environment variable is required. Create a Personal Access Token at https://github.com/settings/tokens/new?scopes=repo and make sure to enable SSO for the GitHub organization after creating the token.")
		os.Exit(1)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
