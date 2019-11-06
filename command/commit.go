package command

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
)

func init() {
	RootCmd.AddCommand(commitCmd)
}

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Commit the changes in staging.",
	Long: `This command passes through to git to 
commit whatever changes have been added to staging.`,
	RunE: commit,
	FParseErrWhitelist: cobra.FParseErrWhitelist{
		UnknownFlags: true,
	},
}

func commit(cmd *cobra.Command, args []string) error {
	gitCmd := exec.Command("git", os.Args[1:]...)
	gitCmd.Stdin = os.Stdin
	gitCmd.Stdout = os.Stdout
	gitCmd.Stderr = os.Stderr

	err := gitCmd.Run()

	if err != nil {
		return errors.Wrap(err, "git failed")
	}

	return nil
}
