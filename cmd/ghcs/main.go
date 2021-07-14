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
	Use:   "ghcs",
	Short: "Codespaces",
	Long:  "Codespaces",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
