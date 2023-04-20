package iostreams

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	ghTerm "github.com/cli/go-gh/v2/pkg/term"
	"github.com/cli/safeexec"
	"github.com/google/shlex"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
)

const DefaultWidth = 80

// ErrClosedPagerPipe is the error returned when writing to a pager that has been closed.
type ErrClosedPagerPipe struct {
	error
}

type fileWriter interface {
	io.Writer
	Fd() uintptr
}

type fileReader interface {
	io.ReadCloser
	Fd() uintptr
}

type term interface {
	IsTerminalOutput() bool
	IsColorEnabled() bool
	Is256ColorSupported() bool
	IsTrueColorSupported() bool
	Theme() string
	Size() (int, int, error)
}

type IOStreams struct {
	term term

	In     fileReader
	Out    fileWriter
	ErrOut io.Writer

	terminalTheme string

	progressIndicatorEnabled bool
	progressIndicator        *spinner.Spinner
	progressIndicatorMu      sync.Mutex

	alternateScreenBufferEnabled bool
	alternateScreenBufferActive  bool
	alternateScreenBufferMu      sync.Mutex

	stdinTTYOverride  bool
	stdinIsTTY        bool
	stdoutTTYOverride bool
	stdoutIsTTY       bool
	stderrTTYOverride bool
	stderrIsTTY       bool

	colorOverride bool
	colorEnabled  bool

	pagerCommand string
	pagerProcess *os.Process

	neverPrompt bool

	TempFileOverride *os.File
}

func (s *IOStreams) ColorEnabled() bool {
	if s.colorOverride {
		return s.colorEnabled
	}
	return s.term.IsColorEnabled()
}

func (s *IOStreams) ColorSupport256() bool {
	if s.colorOverride {
		return s.colorEnabled
	}
	return s.term.Is256ColorSupported()
}

func (s *IOStreams) HasTrueColor() bool {
	if s.colorOverride {
		return s.colorEnabled
	}
	return s.term.IsTrueColorSupported()
}

// DetectTerminalTheme is a utility to call before starting the output pager so that the terminal background
// can be reliably detected.
func (s *IOStreams) DetectTerminalTheme() {
	if !s.ColorEnabled() || s.pagerProcess != nil {
		s.terminalTheme = "none"
		return
	}

	style := os.Getenv("GLAMOUR_STYLE")
	if style != "" && style != "auto" {
		// ensure GLAMOUR_STYLE takes precedence over "light" and "dark" themes
		s.terminalTheme = "none"
		return
	}

	s.terminalTheme = s.term.Theme()
}

// TerminalTheme returns "light", "dark", or "none" depending on the background color of the terminal.
func (s *IOStreams) TerminalTheme() string {
	if s.terminalTheme == "" {
		s.DetectTerminalTheme()
	}

	return s.terminalTheme
}

func (s *IOStreams) SetColorEnabled(colorEnabled bool) {
	s.colorOverride = true
	s.colorEnabled = colorEnabled
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
	// support GH_FORCE_TTY
	if s.term.IsTerminalOutput() {
		return true
	}
	stdout, ok := s.Out.(*os.File)
	return ok && isCygwinTerminal(stdout.Fd())
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

func (s *IOStreams) SetPager(cmd string) {
	s.pagerCommand = cmd
}

func (s *IOStreams) GetPager() string {
	return s.pagerCommand
}

func (s *IOStreams) StartPager() error {
	if s.pagerCommand == "" || s.pagerCommand == "cat" || !s.IsStdoutTTY() {
		return nil
	}

	pagerArgs, err := shlex.Split(s.pagerCommand)
	if err != nil {
		return err
	}

	pagerEnv := os.Environ()
	for i := len(pagerEnv) - 1; i >= 0; i-- {
		if strings.HasPrefix(pagerEnv[i], "PAGER=") {
			pagerEnv = append(pagerEnv[0:i], pagerEnv[i+1:]...)
		}
	}
	if _, ok := os.LookupEnv("LESS"); !ok {
		pagerEnv = append(pagerEnv, "LESS=FRX")
	}
	if _, ok := os.LookupEnv("LV"); !ok {
		pagerEnv = append(pagerEnv, "LV=-c")
	}

	pagerExe, err := safeexec.LookPath(pagerArgs[0])
	if err != nil {
		return err
	}
	pagerCmd := exec.Command(pagerExe, pagerArgs[1:]...)
	pagerCmd.Env = pagerEnv
	pagerCmd.Stdout = s.Out
	pagerCmd.Stderr = s.ErrOut
	pagedOut, err := pagerCmd.StdinPipe()
	if err != nil {
		return err
	}
	s.Out = &fdWriteCloser{
		fd:          s.Out.Fd(),
		WriteCloser: &pagerWriter{pagedOut},
	}
	err = pagerCmd.Start()
	if err != nil {
		return err
	}
	s.pagerProcess = pagerCmd.Process
	return nil
}

func (s *IOStreams) StopPager() {
	if s.pagerProcess == nil {
		return
	}

	// if a pager was started, we're guaranteed to have a WriteCloser
	_ = s.Out.(io.WriteCloser).Close()
	_, _ = s.pagerProcess.Wait()
	s.pagerProcess = nil
}

func (s *IOStreams) CanPrompt() bool {
	if s.neverPrompt {
		return false
	}

	return s.IsStdinTTY() && s.IsStdoutTTY()
}

func (s *IOStreams) GetNeverPrompt() bool {
	return s.neverPrompt
}

func (s *IOStreams) SetNeverPrompt(v bool) {
	s.neverPrompt = v
}

func (s *IOStreams) StartProgressIndicator() {
	s.StartProgressIndicatorWithLabel("")
}

func (s *IOStreams) StartProgressIndicatorWithLabel(label string) {
	if !s.progressIndicatorEnabled {
		return
	}

	s.progressIndicatorMu.Lock()
	defer s.progressIndicatorMu.Unlock()

	if s.progressIndicator != nil {
		if label == "" {
			s.progressIndicator.Prefix = ""
		} else {
			s.progressIndicator.Prefix = label + " "
		}
		return
	}

	// https://github.com/briandowns/spinner#available-character-sets
	dotStyle := spinner.CharSets[11]
	sp := spinner.New(dotStyle, 120*time.Millisecond, spinner.WithWriter(s.ErrOut), spinner.WithColor("fgCyan"))
	if label != "" {
		sp.Prefix = label + " "
	}

	sp.Start()
	s.progressIndicator = sp
}

func (s *IOStreams) StopProgressIndicator() {
	s.progressIndicatorMu.Lock()
	defer s.progressIndicatorMu.Unlock()
	if s.progressIndicator == nil {
		return
	}
	s.progressIndicator.Stop()
	s.progressIndicator = nil
}

func (s *IOStreams) RunWithProgress(label string, run func() error) error {
	s.StartProgressIndicatorWithLabel(label)
	defer s.StopProgressIndicator()

	return run()
}

func (s *IOStreams) StartAlternateScreenBuffer() {
	if s.alternateScreenBufferEnabled {
		s.alternateScreenBufferMu.Lock()
		defer s.alternateScreenBufferMu.Unlock()

		if _, err := fmt.Fprint(s.Out, "\x1b[?1049h"); err == nil {
			s.alternateScreenBufferActive = true

			ch := make(chan os.Signal, 1)
			signal.Notify(ch, os.Interrupt)

			go func() {
				<-ch
				s.StopAlternateScreenBuffer()

				os.Exit(1)
			}()
		}
	}
}

func (s *IOStreams) StopAlternateScreenBuffer() {
	s.alternateScreenBufferMu.Lock()
	defer s.alternateScreenBufferMu.Unlock()

	if s.alternateScreenBufferActive {
		fmt.Fprint(s.Out, "\x1b[?1049l")
		s.alternateScreenBufferActive = false
	}
}

func (s *IOStreams) SetAlternateScreenBufferEnabled(enabled bool) {
	s.alternateScreenBufferEnabled = enabled
}

func (s *IOStreams) RefreshScreen() {
	if s.IsStdoutTTY() {
		// Move cursor to 0,0
		fmt.Fprint(s.Out, "\x1b[0;0H")
		// Clear from cursor to bottom of screen
		fmt.Fprint(s.Out, "\x1b[J")
	}
}

// TerminalWidth returns the width of the terminal that controls the process
func (s *IOStreams) TerminalWidth() int {
	w, _, err := s.term.Size()
	if err == nil && w > 0 {
		return w
	}
	return DefaultWidth
}

func (s *IOStreams) ColorScheme() *ColorScheme {
	return NewColorScheme(s.ColorEnabled(), s.ColorSupport256(), s.HasTrueColor())
}

func (s *IOStreams) ReadUserFile(fn string) ([]byte, error) {
	var r io.ReadCloser
	if fn == "-" {
		r = s.In
	} else {
		var err error
		r, err = os.Open(fn)
		if err != nil {
			return nil, err
		}
	}
	defer r.Close()
	return io.ReadAll(r)
}

func (s *IOStreams) TempFile(dir, pattern string) (*os.File, error) {
	if s.TempFileOverride != nil {
		return s.TempFileOverride, nil
	}
	return os.CreateTemp(dir, pattern)
}

func System() *IOStreams {
	terminal := ghTerm.FromEnv()

	var stdout fileWriter = os.Stdout
	// On Windows with no virtual terminal processing support, translate ANSI escape
	// sequences to console syscalls
	if colorableStdout := colorable.NewColorable(os.Stdout); colorableStdout != os.Stdout {
		// ensure that the file descriptor of the original stdout is preserved
		stdout = &fdWriter{
			fd:     os.Stdout.Fd(),
			Writer: colorableStdout,
		}
	}

	io := &IOStreams{
		In:           os.Stdin,
		Out:          stdout,
		ErrOut:       colorable.NewColorable(os.Stderr),
		pagerCommand: os.Getenv("PAGER"),
		term:         &terminal,
	}

	stdoutIsTTY := io.IsStdoutTTY()
	stderrIsTTY := io.IsStderrTTY()

	if stdoutIsTTY && stderrIsTTY {
		io.progressIndicatorEnabled = true
	}

	if stdoutIsTTY && hasAlternateScreenBuffer(terminal.IsTrueColorSupported()) {
		io.alternateScreenBufferEnabled = true
	}

	return io
}

type fakeTerm struct{}

func (t fakeTerm) IsTerminalOutput() bool {
	return false
}

func (t fakeTerm) IsColorEnabled() bool {
	return false
}

func (t fakeTerm) Is256ColorSupported() bool {
	return false
}

func (t fakeTerm) IsTrueColorSupported() bool {
	return false
}

func (t fakeTerm) Theme() string {
	return ""
}

func (t fakeTerm) Size() (int, int, error) {
	return 80, -1, nil
}

func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	io := &IOStreams{
		In: &fdReader{
			fd:         0,
			ReadCloser: io.NopCloser(in),
		},
		Out:    &fdWriter{fd: 1, Writer: out},
		ErrOut: errOut,
		term:   &fakeTerm{},
	}
	io.SetStdinTTY(false)
	io.SetStdoutTTY(false)
	io.SetStderrTTY(false)
	return io, in, out, errOut
}

func isTerminal(f *os.File) bool {
	return ghTerm.IsTerminal(f) || isCygwinTerminal(f.Fd())
}

func isCygwinTerminal(fd uintptr) bool {
	return isatty.IsCygwinTerminal(fd)
}

// pagerWriter implements a WriteCloser that wraps all EPIPE errors in an ErrClosedPagerPipe type.
type pagerWriter struct {
	io.WriteCloser
}

func (w *pagerWriter) Write(d []byte) (int, error) {
	n, err := w.WriteCloser.Write(d)
	if err != nil && (errors.Is(err, io.ErrClosedPipe) || isEpipeError(err)) {
		return n, &ErrClosedPagerPipe{err}
	}
	return n, err
}

// fdWriter represents a wrapped stdout Writer that preserves the original file descriptor
type fdWriter struct {
	io.Writer
	fd uintptr
}

func (w *fdWriter) Fd() uintptr {
	return w.fd
}

// fdWriteCloser represents a wrapped stdout Writer that preserves the original file descriptor
type fdWriteCloser struct {
	io.WriteCloser
	fd uintptr
}

func (w *fdWriteCloser) Fd() uintptr {
	return w.fd
}

// fdWriter represents a wrapped stdin ReadCloser that preserves the original file descriptor
type fdReader struct {
	io.ReadCloser
	fd uintptr
}

func (r *fdReader) Fd() uintptr {
	return r.fd
}
