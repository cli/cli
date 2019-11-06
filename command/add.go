package command

import (
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
)

func init() {
	RootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Stage changes for committing.",
	Long: `This command passes through to git to 
add selected changes to staging. They can then 
be committed with gh commit.`,
	RunE: add,
	FParseErrWhitelist: cobra.FParseErrWhitelist{
		UnknownFlags: true,
	},
}

func add(cmd *cobra.Command, args []string) error {
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
