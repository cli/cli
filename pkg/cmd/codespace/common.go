package codespace

// This file defines functions common to the entire codespace command set.

import (
	"errors"

	"github.com/spf13/cobra"
)

var ErrTooManyArgs = errors.New("the command accepts no arguments")

func noArgsConstraint(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return ErrTooManyArgs
	}
	return nil
}
