//go:build !windows
// +build !windows

package prompter

import (
	"bytes"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestSurveyProbeInjectsOnDumbPty(t *testing.T) {
	if testing.Short() {
		t.Skip("needs a pty")
	}
	m, s, err := pty.Open()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	defer m.Close()
	defer s.Close()

	p := newSurveyProbeIO(s, s)

	// Simulate survey writing the probe through the watched writer.
	out := p.Out()
	if _, err := out.Write([]byte("\x1b[999;999f")); err != nil {
		t.Fatalf("write move: %v", err)
	}
	if _, err := out.Write(dsrRequest); err != nil {
		t.Fatalf("write dsr: %v", err)
	}

	// Now read like survey's Location does; expect a synthetic report.
	buf := make([]byte, 64)
	in := p.In()
	start := time.Now()
	n, err := in.Read(buf)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	line := buf[:n]
	t.Logf("elapsed=%v reply=%q", elapsed, line)
	if !bytes.Contains(line, []byte("R")) {
		t.Fatalf("no synthetic cursor report injected: %q", line)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("probe fallback took too long: %v", elapsed)
	}
}

func TestSurveyProbePassthroughRealAnswer(t *testing.T) {
	m, s, err := pty.Open()
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	defer m.Close()
	defer s.Close()

	p := newSurveyProbeIO(s, s)
	// Put the pty in raw mode like survey's SetTermMode does mid-prompt; in
	// canonical mode a partial line (no newline, like a DSR answer) is not
	// readable at all, which is not the environment we are modelling.
	raw, err := unix.IoctlGetTermios(int(s.Fd()), unix.TCGETS)
	if err != nil {
		t.Skipf("termios: %v", err)
	}
	raw.Lflag &^= unix.ICANON | unix.ECHO
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(int(s.Fd()), unix.TCSETS, raw); err != nil {
		t.Skipf("termios set: %v", err)
	}
	// Start a goroutine that supplies a REAL cursor answer after 50 ms.
	go func() {
		time.Sleep(50 * time.Millisecond)
		if _, err := m.Write([]byte("\x1b[24;80R")); err != nil {
			t.Logf("master write: %v", err)
		}
	}()
	// Arm a probe, then supply a REAL answer; it must be passed through and
	// clear the pending state.
	if _, err := p.Out().Write(dsrRequest); err != nil {
		t.Fatalf("write dsr: %v", err)
	}
	buf := make([]byte, 64)
	n, err := p.In().Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Contains(buf[:n], []byte("\x1b[24;80R")) {
		t.Fatalf("real answer not passed through: %q", buf[:n])
	}
	if p.state.hostile {
		t.Fatalf("real answer should clear pending state without hostility")
	}

	// A second probe with no real answer should inject (not hang) and mark
	// the stream hostile so later probes do not wait again.
	if _, err := p.Out().Write(dsrRequest); err != nil {
		t.Fatalf("write dsr 2: %v", err)
	}
	start := time.Now()
	n, err = p.In().Read(buf)
	if err != nil {
		t.Fatalf("read 2: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("second probe waited too long: %v", time.Since(start))
	}
	if !bytes.Contains(buf[:n], []byte("R")) {
		t.Fatalf("second probe got no synthetic reply: %q", buf[:n])
	}
}
