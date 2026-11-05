//go:build !windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// runInTerminal starts cmd in a pseudo-terminal, so gh can prompt as it does
// for a user, and types each keys entry once the screen settles. It returns
// cmd.Wait's error. The terminal answers gh's queries, such as where the
// cursor is, which survey's prompts need.
func runInTerminal(ctx context.Context, cmd *exec.Cmd, keys []string) (terminalRun, error) {
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: terminalCols, Rows: terminalRows})
	if err != nil {
		return terminalRun{}, fmt.Errorf("starting a terminal: %w", err)
	}
	defer ptmx.Close()

	keyboard := &lockedWriter{w: ptmx}
	term := vt10x.New(vt10x.WithSize(terminalCols, terminalRows), vt10x.WithWriter(keyboard))
	var output terminalOutput
	output.lastOutput.Store(time.Now().UnixNano())
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		reader := bufio.NewReader(io.TeeReader(ptmx, &output))
		for {
			if err := term.Parse(reader); err != nil {
				return
			}
		}
	}()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	quietFor := func(d time.Duration) bool {
		return time.Since(time.Unix(0, output.lastOutput.Load())) >= d
	}
	var run terminalRun
	var waitErr error
	done := false
	for i, entry := range keys {
		seq, err := parseKeys(entry)
		if err != nil {
			return terminalRun{}, err
		}
		if done = waitFor(ctx, exited, &waitErr, func() bool { return output.sawOutput.Load() && quietFor(settleTime) }); done {
			run.unused = len(keys) - i
			break
		}
		run.steps = append(run.steps, terminalStep{Keys: entry, Screen: screenText(term)})
		output.lastOutput.Store(time.Now().UnixNano())
		if _, err := keyboard.Write([]byte(seq)); err != nil {
			return terminalRun{}, err
		}
	}
	if !done {
		waitFor(ctx, exited, &waitErr, func() bool { return false })
	}
	// Let the last output reach the screen.
	select {
	case <-readDone:
	case <-time.After(time.Second):
	}
	run.screen = screenText(term)
	run.output = output.String()
	return run, waitErr
}

type terminalOutput struct {
	mu         sync.Mutex
	buffer     bytes.Buffer
	lastOutput atomic.Int64
	sawOutput  atomic.Bool
}

func (o *terminalOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n, err := o.buffer.Write(p)
	o.lastOutput.Store(time.Now().UnixNano())
	o.sawOutput.Store(true)
	return n, err
}

func (o *terminalOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}

// waitFor waits until gh exits, the context ends or ready returns true. It
// reports whether gh exited, storing cmd.Wait's error in waitErr.
func waitFor(ctx context.Context, exited <-chan error, waitErr *error, ready func() bool) bool {
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-exited:
			*waitErr = err
			return true
		case <-ctx.Done():
			*waitErr = <-exited
			return true
		case <-tick.C:
			if ready() {
				return false
			}
		}
	}
}

// screenText is the terminal's screen as text, without trailing spaces or
// blank lines at the bottom.
func screenText(term vt10x.Terminal) string {
	lines := strings.Split(term.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// lockedWriter lets ghmock's typing and the terminal's answers to gh share
// the pseudo-terminal without interleaving.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
