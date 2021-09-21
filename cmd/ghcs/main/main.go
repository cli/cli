package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/github/ghcs/cmd/ghcs"
)

func main() {
	rootCmd := ghcs.NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		explainError(os.Stderr, err)
		os.Exit(1)
	}
}

func explainError(w io.Writer, err error) {
	if errors.Is(err, ghcs.ErrTokenMissing) {
		fmt.Fprintln(w, "The GITHUB_TOKEN environment variable is required. Create a Personal Access Token at https://github.com/settings/tokens/new?scopes=repo")
		fmt.Fprintln(w, "Make sure to enable SSO for your organizations after creating the token.")
		return
	}
}
