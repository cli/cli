package output

import (
	"fmt"
	"io"
)

func NewLogger(stdout, stderr io.Writer, disabled bool) *Logger {
	return &Logger{
		out:     stdout,
		errout:  stderr,
		enabled: !disabled && isTTY(stdout),
	}
}

type Logger struct {
	out     io.Writer
	errout  io.Writer
	enabled bool
}

func (l *Logger) Print(v ...interface{}) (int, error) {
	if !l.enabled {
		return 0, nil
	}
	return fmt.Fprint(l.out, v...)
}

func (l *Logger) Println(v ...interface{}) (int, error) {
	if !l.enabled {
		return 0, nil
	}
	return fmt.Fprintln(l.out, v...)
}

func (l *Logger) Printf(f string, v ...interface{}) (int, error) {
	if !l.enabled {
		return 0, nil
	}
	return fmt.Fprintf(l.out, f, v...)
}

func (l *Logger) Errorf(f string, v ...interface{}) (int, error) {
	return fmt.Fprintf(l.errout, f, v...)
}
