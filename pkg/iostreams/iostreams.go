package iostreams

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"golang.org/x/crypto/ssh/terminal"
)

type IOStreams struct {
	In     io.ReadCloser
	Out    io.Writer
	ErrOut io.Writer

	colorEnabled bool

	stdinTTYOverride  bool
	stdinIsTTY        bool
	stdoutTTYOverride bool
	stdoutIsTTY       bool
	stderrTTYOverride bool
	stderrIsTTY       bool
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

func (s *IOStreams) SetStdoutTTY(isTTY bool) {
	s.stdoutTTYOverride = true
	s.stdoutIsTTY = isTTY
}

func (s *IOStreams) IsStdoutTTY() bool {
	if s.stdoutTTYOverride {
		return s.stdoutIsTTY
	}
	if stdout, ok := s.Out.(*os.File); ok {
		return isTerminal(stdout)
	}
	return false
}

func (s *IOStreams) SetStderrTTY(isTTY bool) {
	s.stderrTTYOverride = true
	s.stderrIsTTY = isTTY
}

func (s *IOStreams) IsStderrTTY() bool {
	if s.stderrTTYOverride {
		return s.stderrIsTTY
	}
	if stderr, ok := s.ErrOut.(*os.File); ok {
		return isTerminal(stderr)
	}
	return false
}

func (s *IOStreams) TerminalWidth() int {
	defaultWidth := 80
	if s.stdoutTTYOverride {
		return defaultWidth
	}

	if w, _, err := terminalSize(s.Out); err == nil {
		return w
	}

	if isCygwinTerminal(s.Out) {
		tputCmd := exec.Command("tput", "cols")
		tputCmd.Stdin = os.Stdin
		if out, err := tputCmd.Output(); err == nil {
			if w, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
				return w
			}
		}
	}

	return defaultWidth
}

func System() *IOStreams {
	return &IOStreams{
		In:           os.Stdin,
		Out:          colorable.NewColorable(os.Stdout),
		ErrOut:       colorable.NewColorable(os.Stderr),
		colorEnabled: os.Getenv("NO_COLOR") == "" && isTerminal(os.Stdout),
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

func isCygwinTerminal(w io.Writer) bool {
	if f, isFile := w.(*os.File); isFile {
		return isatty.IsCygwinTerminal(f.Fd())
	}
	return false
}

func terminalSize(w io.Writer) (int, int, error) {
	if f, isFile := w.(*os.File); isFile {
		return terminal.GetSize(int(f.Fd()))
	}
	return 0, 0, fmt.Errorf("%v is not a file", w)
}
