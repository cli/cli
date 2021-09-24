package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/github/ghcs/cmd/ghcs"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := ghcs.NewRootCmd()
	if cmd, err := rootCmd.ExecuteC(); err != nil {
		explainError(os.Stderr, err, cmd)
		os.Exit(1)
	}
}

func explainError(w io.Writer, err error, cmd *cobra.Command) {
	if errors.Is(err, ghcs.ErrTokenMissing) {
		fmt.Fprintln(w, "The GITHUB_TOKEN environment variable is required. Create a Personal Access Token at https://github.com/settings/tokens/new?scopes=repo")
		fmt.Fprintln(w, "Make sure to enable SSO for your organizations after creating the token.")
		return
	}
	if errors.Is(err, ghcs.ErrTooManyArgs) {
		_ = cmd.Usage()
		return
	}
}
