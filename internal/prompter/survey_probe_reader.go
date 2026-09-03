//go:build !windows
// +build !windows

package prompter

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// This file works around cli/cli#14323: the survey-based prompts (used through
// go-gh's prompter) determine the terminal size by writing a cursor-position
// request (ESC[6n, a Device Status Report) to stdout and then blocking on
// stdin until the terminal answers ESC[<row>;<col>R. A terminal emulator
// answers instantly, but a bare PTY without one behind it — CI wrappers,
// automation harnesses, pty-based RPC supervisors — never does, so the prompt
// hangs forever.
//
// surveyProbeIO sits between go-gh and survey (the same seam as
// survey_escape_reader.go). Its output side watches for DSR requests and
// records that an answer is due; its input side waits for stdin bytes with a
// bounded poll, and if the deadline expires without a plausible answer, it
// injects a synthetic ESC[<row>;<col>R so the probe completes and the prompt
// proceeds.

const surveyProbeTimeoutDefault = 500 * time.Millisecond

var dsrRequest = []byte("\x1b[6n")

// surveyProbeTimeout is how long the terminal has to answer the DSR request.
// Real terminals answer in microseconds, and the go-expect virtual terminals
// used in the prompter test suite answer synchronously; even a slow remote
// link has orders of magnitude of headroom. GH_CURSOR_QUERY_TIMEOUT_MS
// overrides it (in milliseconds) for tests.
func surveyProbeTimeout() time.Duration {
	if v := os.Getenv("GH_CURSOR_QUERY_TIMEOUT_MS"); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return surveyProbeTimeoutDefault
}

// probeState tracks cursor-position requests and whether they have been
// answered by the terminal.
type probeState struct {
	mu  sync.Mutex
	out *os.File
	// unanswered counts DSR requests written to the terminal that have not
	// seen any plausible reply yet.
	unanswered int
	// deadline is when the oldest unanswered request expires.
	deadline time.Time
	// hostile records that the terminal already missed a deadline once; on
	// such a stream later probes are answered synthetically without waiting
	// again, so multi-probe prompts do not pay the timeout twice.
	hostile bool
}

func (s *probeState) requestStarted(now time.Time, timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unanswered++
	if s.hostile || s.deadline.IsZero() {
		s.deadline = now
		if s.hostile {
			// Answer immediately; the terminal has already shown it will
			// not.
			s.deadline = now.Add(-time.Millisecond)
		} else {
			s.deadline = now.Add(timeout)
		}
	}
}

var dsrReplyPattern = regexp.MustCompile(`\x1b\[\d+;\d+R`)

// sawInput records that arbitrary bytes arrived. Only bytes that look like a
// cursor-position report clear the pending probe; user keystrokes typed
// while the request is in flight are passed through to survey's probe loop
// but do not cancel it.
func (s *probeState) sawInput(b []byte) {
	if len(b) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if dsrReplyPattern.Match(b) {
		s.unanswered = 0
		s.deadline = time.Time{}
	}
}

// replyToInject returns a synthetic cursor report if every outstanding
// request's deadline has expired, and marks the stream hostile so subsequent
// probes are answered instantly.
func (s *probeState) replyToInject() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unanswered == 0 || s.deadline.IsZero() || time.Now().Before(s.deadline) {
		return ""
	}
	s.hostile = true
	w, h := s.fallbackSize()
	return "\x1b[" + strconv.Itoa(h) + ";" + strconv.Itoa(w) + "R"
}

// surveyProbeIO wraps the streams handed to the survey prompter.
type surveyProbeIO struct {
	in    *os.File
	out   *os.File
	state *probeState
}

func newSurveyProbeIO(in, out *os.File) *surveyProbeIO {
	state := &probeState{out: out}
	return &surveyProbeIO{
		in:    in,
		out:   out,
		state: state,
	}
}

// In returns the bounded reader to hand to survey as Stdio.In.
func (p *surveyProbeIO) In() io.Reader {
	if p.in == nil {
		return nil
	}
	return &surveyProbeReader{probe: p}
}

// Out returns the watching writer to hand to survey as Stdio.Out.
func (p *surveyProbeIO) Out() io.Writer {
	if p.out == nil {
		return nil
	}
	return &surveyProbeWriter{probe: p}
}

// surveyProbeWriter watches survey's output for DSR requests. It never
// modifies what is written.
type surveyProbeWriter struct {
	probe *surveyProbeIO
}

// Fd satisfies ghPrompter.FileWriter (survey's renderer needs the real
// descriptor for terminal ioctls).
func (w *surveyProbeWriter) Fd() uintptr { return w.probe.out.Fd() }

func (w *surveyProbeWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, dsrRequest) {
		w.probe.state.requestStarted(time.Now(), surveyProbeTimeout())
	}
	return w.probe.out.Write(p)
}

// surveyProbeReader serves stdin reads, injecting a synthetic cursor report
// when the terminal fails to answer a probe in time.
type surveyProbeReader struct {
	probe    *surveyProbeIO
	injected []byte
}

// waitReadable polls the terminal fd, returning true when data is available
// before the deadline.
func (r *surveyProbeReader) waitReadable(remaining time.Duration) bool {
	fds := []unix.PollFd{{Fd: int32(r.probe.in.Fd()), Events: unix.POLLIN}}
	ms := int(remaining / time.Millisecond)
	if ms < 1 {
		ms = 1
	}
	n, err := unix.Poll(fds, ms)
	return err == nil && n > 0
}

func (r *surveyProbeReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if n := copy(p, r.injected); n > 0 {
		r.injected = r.injected[n:]
		return n, nil
	}

	// Without an outstanding probe, read the way survey always has.
	if !r.probe.state.awaitingReply() {
		return r.probe.in.Read(p)
	}

	remaining := r.probe.state.remaining()
	if remaining > 0 && r.waitReadable(remaining) {
		n, err := r.probe.in.Read(p)
		r.probe.state.sawInput(p[:n])
		return n, err
	}

	// Deadline expired without any input: answer the probe synthetically.
	reply := r.probe.state.replyToInject()
	if reply == "" {
		// Should not happen when awaitingReply() was true; last-resort
		// re-check under the state lock to avoid a blocking read here.
		if r.probe.state.awaitingReply() {
			// Force-expire and inject.
			r.probe.state.forceExpire()
			reply = r.probe.state.replyToInject()
		}
		if reply == "" {
			return r.probe.in.Read(p)
		}
	}
	r.injected = append(r.injected, reply...)
	n := copy(p, reply)
	r.injected = r.injected[n:]
	return n, nil
}

func (s *probeState) awaitingReply() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unanswered > 0
}

func (s *probeState) forceExpire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unanswered > 0 {
		s.deadline = time.Now().Add(-time.Millisecond)
	}
}

func (s *probeState) remaining() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deadline.IsZero() {
		return 0
	}
	return time.Until(s.deadline)
}

// fallbackSize computes the terminal size used when the DSR probe fails:
// TIOCGWINSZ first (a bare pty still reports the harness's winsize), then
func (s *probeState) fallbackSize() (int, int) {
	if s.out != nil {
		if ws, err := unix.IoctlGetWinsize(int(s.out.Fd()), unix.TIOCGWINSZ); err == nil && ws != nil {
			if ws.Col > 0 && ws.Row > 0 {
				return int(ws.Col), int(ws.Row)
			}
		}
	}
	w, h := 80, 24
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			w = n
		}
	}
	if v := os.Getenv("LINES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			h = n
		}
	}
	return w, h
}
