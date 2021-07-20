package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// ghcs create
// ghcs connect
// ghcs delete
// ghcs list
func main() {
	Execute()
}

var rootCmd = &cobra.Command{
	Use:     "ghcs",
	Short:   "Codespaces",
	Long:    "Codespaces",
	Version: "0.5.1",
}

func Execute() {
	if os.Getenv("GITHUB_TOKEN") == "" {
		fmt.Println("The GITHUB_TOKEN environment variable is required. Create a Personal Access Token with org SSO access at https://github.com/settings/tokens/new.")
		os.Exit(1)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
