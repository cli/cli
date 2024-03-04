package logger

import (
	"fmt"
	"log"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
)

type Logger struct {
	ColorScheme *iostreams.ColorScheme
	IO      *iostreams.IOStreams
	quiet   bool
	verbose bool
}

func NewLogger(isQuiet, isVerbose bool) *Logger {
	io := iostreams.System()
	colorScheme := io.ColorScheme()
	return &Logger{
		ColorScheme: colorScheme,
		IO:          io,
		quiet:       isQuiet,
		verbose:     isVerbose,
	}
}

// NewDefaultLogger returns a Logger that with the default logging settings
func NewDefaultLogger() *Logger {
	isQuiet := false
	isVerbose := false

	return NewLogger(isQuiet, isVerbose)
}

// Printf writes the formatted arguments to the stdout writer.
func (l *Logger) Printf(f string, v ...interface{}) (int, error) {
	if l.quiet || !l.IO.IsStdoutTTY() {
		return 0, nil
	}
	return fmt.Fprintf(l.IO.ErrOut, f, v...)
}

// Println writes the arguments to the stdout writer with a newline at the end.
func (l *Logger) Println(v ...interface{}) (int, error) {
	if l.quiet || !l.IO.IsStdoutTTY() {
		return 0, nil
	}
	return fmt.Fprintln(l.IO.ErrOut, v...)
}

func (l *Logger) VerbosePrint(msg string) (int, error) {
	if !l.verbose || !l.IO.IsStdoutTTY() {
		return 0, nil
	}

	return fmt.Fprintf(l.IO.ErrOut, msg)
}

func (l *Logger) VerbosePrintf(f string, v ...interface{}) (int, error) {
	if !l.verbose || !l.IO.IsStdoutTTY() {
		return 0, nil
	}

	return fmt.Fprintf(l.IO.ErrOut, f, v...)
}

func (l *Logger) PrintTableToStdOut(headers []string, rows [][]string) {
	if rows == nil {
		return
	}
	t := tableprinter.New(l.IO.Out, l.IO.IsStdoutTTY(), l.IO.TerminalWidth())

	if headers != nil {
		// Print the header row in green
		t.AddHeader(headers, tableprinter.WithColor(l.ColorScheme.Green))
	}

	for _, row := range rows {
		for _, field := range row {
			t.AddField(field, tableprinter.WithTruncate(nil))
		}
		t.EndRow()
	}

	if err := t.Render(); err != nil {
		log.Fatal(err)
	}
}
