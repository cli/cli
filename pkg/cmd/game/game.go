package game

import (
	mergeconflictCmd "github.com/cli/cli/pkg/cmd/game/mergeconflict"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmdGame(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "game <command>",
		Short:  "Play games in your terminal",
		Hidden: true,
	}

	cmd.AddCommand(mergeconflictCmd.NewCmdMergeconflict(f, nil))

	return cmd
}
