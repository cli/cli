package iostreams

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/cli/safeexec"
	"github.com/google/shlex"
	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

const DefaultWidth = 80

// ErrClosedPagerPipe is the error returned when writing to a pager that has been closed.
type ErrClosedPagerPipe struct {
	error
}

type IOStreams struct {
	In     io.ReadCloser
	Out    io.Writer
	ErrOut io.Writer

	// the original (non-colorable) output stream
	originalOut   io.Writer
	colorEnabled  bool
	is256enabled  bool
	hasTrueColor  bool
	terminalTheme string

	progressIndicatorEnabled bool
	progressIndicator        *spinner.Spinner
	progressIndicatorMu      sync.Mutex

	stdinTTYOverride  bool
	stdinIsTTY        bool
	stdoutTTYOverride bool
	stdoutIsTTY       bool
	stderrTTYOverride bool
	stderrIsTTY       bool
	termWidthOverride int
	ttySize           func() (int, int, error)

	pagerCommand string
	pagerProcess *os.Process

	neverPrompt bool

	TempFileOverride *os.File
}

func (s *IOStreams) ColorEnabled() bool {
	return s.colorEnabled
}

func (s *IOStreams) ColorSupport256() bool {
	return s.is256enabled
}

func (s *IOStreams) HasTrueColor() bool {
	return s.hasTrueColor
}

// DetectTerminalTheme is a utility to call before starting the output pager so that the terminal background
// can be reliably detected.
func (s *IOStreams) DetectTerminalTheme() {
	if !s.ColorEnabled() {
		s.terminalTheme = "none"
		return
	}

	if s.pagerProcess != nil {
		s.terminalTheme = "none"
		return
	}

	style := os.Getenv("GLAMOUR_STYLE")
	if style != "" && style != "auto" {
		s.terminalTheme = "none"
		return
	}

	if termenv.HasDarkBackground() {
		s.terminalTheme = "dark"
		return
	}

	s.terminalTheme = "light"
}

// TerminalTheme returns "light", "dark", or "none" depending on the background color of the terminal.
func (s *IOStreams) TerminalTheme() string {
	if s.terminalTheme == "" {
		s.DetectTerminalTheme()
	}

	return s.terminalTheme
}

func (s *IOStreams) SetColorEnabled(colorEnabled bool) {
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
	s.Out = &pagerWriter{pagedOut}
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

// TerminalWidth returns the width of the terminal that stdout is attached to.
// TODO: investigate whether ProcessTerminalWidth could replace all this.
func (s *IOStreams) TerminalWidth() int {
	if s.termWidthOverride > 0 {
		return s.termWidthOverride
	}

	defaultWidth := DefaultWidth
	out := s.Out
	if s.originalOut != nil {
		out = s.originalOut
	}

	if w, _, err := terminalSize(out); err == nil {
		return w
	}

	if isCygwinTerminal(out) {
		tputExe, err := safeexec.LookPath("tput")
		if err != nil {
			return defaultWidth
		}
		tputCmd := exec.Command(tputExe, "cols")
		tputCmd.Stdin = os.Stdin
		if out, err := tputCmd.Output(); err == nil {
			if w, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
				return w
			}
		}
	}

	return defaultWidth
}

// ProcessTerminalWidth returns the width of the terminal that the process is attached to.
func (s *IOStreams) ProcessTerminalWidth() int {
	w, _, err := s.ttySize()
	if err != nil {
		return DefaultWidth
	}
	return w
}

func (s *IOStreams) ForceTerminal(spec string) {
	s.colorEnabled = !EnvColorDisabled()
	s.SetStdoutTTY(true)

	if w, err := strconv.Atoi(spec); err == nil {
		s.termWidthOverride = w
		return
	}

	ttyWidth, _, err := s.ttySize()
	if err != nil {
		return
	}
	s.termWidthOverride = ttyWidth

	if strings.HasSuffix(spec, "%") {
		if p, err := strconv.Atoi(spec[:len(spec)-1]); err == nil {
			s.termWidthOverride = int(float64(s.termWidthOverride) * (float64(p) / 100))
		}
	}
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
	return ioutil.ReadAll(r)
}

func (s *IOStreams) TempFile(dir, pattern string) (*os.File, error) {
	if s.TempFileOverride != nil {
		return s.TempFileOverride, nil
	}
	return ioutil.TempFile(dir, pattern)
}

func System() *IOStreams {
	stdoutIsTTY := isTerminal(os.Stdout)
	stderrIsTTY := isTerminal(os.Stderr)

	assumeTrueColor := false
	if stdoutIsTTY {
		if err := enableVirtualTerminalProcessing(os.Stdout); err == nil {
			assumeTrueColor = true
		}
	}

	io := &IOStreams{
		In:           os.Stdin,
		originalOut:  os.Stdout,
		Out:          colorable.NewColorable(os.Stdout),
		ErrOut:       colorable.NewColorable(os.Stderr),
		colorEnabled: EnvColorForced() || (!EnvColorDisabled() && stdoutIsTTY),
		is256enabled: assumeTrueColor || Is256ColorSupported(),
		hasTrueColor: assumeTrueColor || IsTrueColorSupported(),
		pagerCommand: os.Getenv("PAGER"),
		ttySize:      ttySize,
	}

	if stdoutIsTTY && stderrIsTTY {
		io.progressIndicatorEnabled = true
	}

	// prevent duplicate isTerminal queries now that we know the answer
	io.SetStdoutTTY(stdoutIsTTY)
	io.SetStderrTTY(stderrIsTTY)
	return io
}

func Test() (*IOStreams, *bytes.Buffer, *bytes.Buffer, *bytes.Buffer) {
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	return &IOStreams{
		In:     ioutil.NopCloser(in),
		Out:    out,
		ErrOut: errOut,
		ttySize: func() (int, int, error) {
			return -1, -1, errors.New("ttySize not implemented in tests")
		},
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

// terminalSize measures the viewport of the terminal that the output stream is connected to
func terminalSize(w io.Writer) (int, int, error) {
	if f, isFile := w.(*os.File); isFile {
		return term.GetSize(int(f.Fd()))
	}
	return 0, 0, fmt.Errorf("%v is not a file", w)
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
