package protip

import (
	"fmt"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ProtipOptions struct {
	IO *iostreams.IOStreams

	Character string
}

func NewCmdProtip(f *cmdutil.Factory, runF func(*ProtipOptions) error) *cobra.Command {
	opts := &ProtipOptions{
		IO: f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "protip",
		Short: "get a random protip about using gh",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return protipRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Character, "character", "c", "clippy", "What helpful animated character you'd like a protip from")

	return cmd
}

func protipRun(opts *ProtipOptions) error {
	fmt.Println("hey")
	return nil
}
