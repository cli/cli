package command

import (
	"github.com/github/gh-cli/version"
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:     "gh",
	Short:   "GitHub CLI",
	Long:    `Do things with GitHub from your terminal`,
	Version: version.Version,
}
