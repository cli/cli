package logger

import (
	"fmt"

	"github.com/cli/cli/v2/pkg/iostreams"
)

type Logger struct {
	IO      *iostreams.IOStreams
	Quiet   bool
	Verbose bool
}

func (l *Logger) VerbosePrint(msg string) (int, error) {
	if !l.verbose || !opts.IO.IsStdoutTTY() {
		return 0, nil
	}

	return fmt.Fprintf(l.IO.ErrOut, msg)
}

