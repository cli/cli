package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/v2/cmd/ghcs"
	"github.com/cli/cli/v2/cmd/ghcs/output"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	rootCmd := ghcs.NewRootCmd(ghcs.NewApp(
		output.NewLogger(os.Stdout, os.Stderr, false),
		api.New(token, http.DefaultClient),
	))

	// Require GITHUB_TOKEN through a Cobra pre-run hook so that Cobra's help system for commands can still
	// function without the token set.
	oldPreRun := rootCmd.PersistentPreRunE
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if token == "" {
			return errTokenMissing
		}
		if oldPreRun != nil {
			return oldPreRun(cmd, args)
		}
		return nil
	}

	if cmd, err := rootCmd.ExecuteC(); err != nil {
		explainError(os.Stderr, err, cmd)
		os.Exit(1)
	}
}

var errTokenMissing = errors.New("GITHUB_TOKEN is missing")

func explainError(w io.Writer, err error, cmd *cobra.Command) {
	if errors.Is(err, errTokenMissing) {
		fmt.Fprintln(w, "The GITHUB_TOKEN environment variable is required. Create a Personal Access Token at https://github.com/settings/tokens/new?scopes=repo")
		fmt.Fprintln(w, "Make sure to enable SSO for your organizations after creating the token.")
		return
	}
	if errors.Is(err, ghcs.ErrTooManyArgs) {
		_ = cmd.Usage()
		return
	}
}
