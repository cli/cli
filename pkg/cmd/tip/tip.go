package tip

import (
	"fmt"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type TipOptions struct {
	IO *iostreams.IOStreams

	Character string
}

func NewCmdTip(f *cmdutil.Factory, runF func(*TipOptions) error) *cobra.Command {
	opts := &TipOptions{
		IO: f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "tip",
		Short: "get a random tip about using gh",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return tipRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Character, "character", "c", "clippy", "What helpful animated character you'd like a tip from")

	return cmd
}

func tipRun(opts *TipOptions) error {
	fmt.Println("hey")
	return nil
}
