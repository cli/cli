package rename

import (
	"testing"
)

func TestNewCmdRename(t * testing.T) {
	tests := []struct {
		name string 
		tty bool
		input string
		output RenameOptions
		wantErr bool
		errMsg string
	}{
		{
			name: "no argument",
			tty: true,
			input: "",
			output: RenameOptions{},
		}, 
		{
			name: "argument",
			tty: true,
			input: "cli/cli",
			output: RenameOptions{
				DestArg: "cli comand-line-interface",
			},
		},
		{
			name: "incorrect argument",
			tty: true,
			input: "",
			output: RenameOptions{
				DestArg: "cli ",
			},
		},
	}
}

