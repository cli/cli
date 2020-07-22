package iostreams

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
)

type IOStreams struct {
	In     io.ReadCloser
	Out    io.Writer
	ErrOut io.Writer

	colorEnabled bool

	stdinTTYOverride bool
	stdinIsTTY       bool
}

func (s *IOStreams) ColorEnabled() bool {
	return s.colorEnabled
}

func (s *IOStreams) SetStdinTTY(isTTY bool) {
	s.stdinTTYOverride = true
	s.stdinIsTTY = isTTY
}

func (s *IOStreams) IsStdinTTY() bool {
	if s.stdinTTYOverride {
		return s.stdinIsTTY
	}
	if stdin, ok := s.In.(*os.File); ok {
		return isTerminal(stdin)
	}
	return false
}

func System() *IOStreams {
	var out io.Writer = os.Stdout
	var colorEnabled bool
	if os.Getenv("NO_COLOR") == "" && isTerminal(os.Stdout) {
		out = colorable.NewColorable(os.Stdout)
		colorEnabled = true
	}

	return &IOStreams{
		In:           os.Stdin,
		Out:          out,
		ErrOut:       os.Stderr,
		colorEnabled: colorEnabled,
	}
}

func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	return &IOStreams{
		In:     ioutil.NopCloser(in),
		Out:    out,
		ErrOut: errOut,
	}, in, out, errOut
}

func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}
