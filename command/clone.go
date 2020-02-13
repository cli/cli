package command

import (
  "github.com/cli/cli/utils"
  "os/exec"
  "net/url"

  "github.com/spf13/cobra"
)

func init() {
  RootCmd.AddCommand(cloneCmd)

}

var cloneCmd = &cobra.Command{
  Use:   "clone [repo]",
  Short: "Clone repos",
  Args:  cobra.MinimumNArgs(1),
  Long: `Work with GitHub repos.
An repo can be supplied as argument in any of the following formats:
- by repo name, e.g. "OWNER/REPO"; or
- by URL, e.g. "https://github.com/OWNER/REPO".`,
  RunE: cloneRepo,
}

func cloneRepo(cmd *cobra.Command, args []string) error {
  _, err := url.ParseRequestURI(args[0])
  var repoAddress string
  if err != nil {
    repoAddress = "https://github.com/" + args[0]
  } else {
    repoAddress = args[0]
  }

  cloneCmd := exec.Command("git", "clone", repoAddress)
  return utils.PrepareCmd(cloneCmd).Run()

}