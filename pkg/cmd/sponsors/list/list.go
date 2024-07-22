package list

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type ListOptions struct {
	Username string
	Sponsors []string
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List sponsors for a user",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Username = args[0]
			opts.Sponsors = []string{"GitHub"}

			if runF != nil {
				return runF(opts)
			}

			fmt.Println(opts.Sponsors)
			return nil
		},
	}

	return cmd
}
