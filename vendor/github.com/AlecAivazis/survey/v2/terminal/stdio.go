package terminal

import (
	"io"
)

// Stdio is the standard input/output the terminal reads/writes with.
type Stdio struct {
	In  FileReader
	Out FileWriter
	Err io.Writer
}

// FileWriter provides a minimal interface for Stdin.
type FileWriter interface {
	io.Writer
	Fd() uintptr
}

// FileReader provides a minimal interface for Stdout.
type FileReader interface {
	io.Reader
	Fd() uintptr
}
