package output

import (
	"fmt"
	"io"
	"sync"
)

// NewLogger returns a Logger that will write to the given stdout/stderr writers.
// Disable the Logger to prevent it from writing to stdout in a TTY environment.
func NewLogger(stdout, stderr io.Writer, disabled bool) *Logger {
	return &Logger{
		out:     stdout,
		errout:  stderr,
		enabled: !disabled && isTTY(stdout),
	}
}

// Logger writes to the given stdout/stderr writers.
// If not enabled, Print functions will noop but Error functions will continue
// to write to the stderr writer.
type Logger struct {
	mu      sync.Mutex // guards the writers
	out     io.Writer
	errout  io.Writer
	enabled bool
}

// Print writes the arguments to the stdout writer.
func (l *Logger) Print(v ...interface{}) (int, error) {
	if !l.enabled {
		return 0, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	return fmt.Fprint(l.out, v...)
}

// Println writes the arguments to the stdout writer with a newline at the end.
func (l *Logger) Println(v ...interface{}) (int, error) {
	if !l.enabled {
		return 0, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	return fmt.Fprintln(l.out, v...)
}

// Printf writes the formatted arguments to the stdout writer.
func (l *Logger) Printf(f string, v ...interface{}) (int, error) {
	if !l.enabled {
		return 0, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	return fmt.Fprintf(l.out, f, v...)
}

// Errorf writes the formatted arguments to the stderr writer.
func (l *Logger) Errorf(f string, v ...interface{}) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return fmt.Fprintf(l.errout, f, v...)
}

// Errorln writes the arguments to the stderr writer with a newline at the end.
func (l *Logger) Errorln(v ...interface{}) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return fmt.Fprintln(l.errout, v...)
}
