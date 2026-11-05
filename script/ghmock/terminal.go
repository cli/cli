package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

const (
	terminalCols = 120
	terminalRows = 40
)

// These are variables so tests can shorten them.
var (
	// settleTime is how long gh's output must stay still before ghmock types
	// the next --keys.
	settleTime = 500 * time.Millisecond
	// waitingTime is how long gh's output must stay still, once no --keys are
	// left, before ghmock decides gh is waiting for input and stops it.
	waitingTime = 2 * time.Second
)

// keyNames are the keys --keys can name in braces, such as {enter}, with
// the bytes a terminal sends for each.
var keyNames = map[string]string{
	"enter":     "\r",
	"tab":       "\t",
	"space":     " ",
	"backspace": "\x7f",
	"esc":       "\x1b",
	"up":        "\x1b[A",
	"down":      "\x1b[B",
	"right":     "\x1b[C",
	"left":      "\x1b[D",
	"ctrl-c":    "\x03",
	"ctrl-d":    "\x04",
}

// parseKeys turns a --keys value into the bytes a terminal would send. Keys
// are named in braces, such as "my-repo{enter}", and "{{" types a brace.
func parseKeys(s string) (string, error) {
	var b strings.Builder
	for rest := s; rest != ""; {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:i])
		rest = rest[i:]
		if strings.HasPrefix(rest, "{{") {
			b.WriteByte('{')
			rest = rest[2:]
			continue
		}
		j := strings.IndexByte(rest, '}')
		if j < 0 {
			return "", fmt.Errorf("--keys %q has a { without a }; type a brace as {{", s)
		}
		seq, ok := keyNames[strings.ToLower(rest[1:j])]
		if !ok {
			return "", fmt.Errorf("--keys %q names an unknown key %s; keys are %s", s, rest[:j+1], keyList())
		}
		b.WriteString(seq)
		rest = rest[j+1:]
	}
	return b.String(), nil
}

func keyList() string {
	names := make([]string, 0, len(keyNames))
	for name := range keyNames {
		names = append(names, "{"+name+"}")
	}
	slices.Sort(names)
	return strings.Join(names, " ")
}

// terminalStep is one --keys entry and the screen ghmock saw just before it
// typed them.
type terminalStep struct {
	Keys   string `json:"keys"`
	Screen string `json:"screen"`
}

// terminalRun is what gh did in a terminal.
type terminalRun struct {
	steps []terminalStep
	// screen is the screen when gh exited or ghmock stopped it.
	screen string
	// waiting means gh was still waiting for input when the keys ran out.
	waiting bool
	// unused counts --keys entries gh exited before getting.
	unused int
}

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
	var lastOutput atomic.Int64
	lastOutput.Store(time.Now().UnixNano())
	var sawOutput atomic.Bool
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				_, _ = term.Write(buf[:n])
				lastOutput.Store(time.Now().UnixNano())
				sawOutput.Store(true)
			}
			if err != nil {
				return
			}
		}
	}()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	quietFor := func(d time.Duration) bool {
		return time.Since(time.Unix(0, lastOutput.Load())) >= d
	}
	var run terminalRun
	var waitErr error
	done := false
	for i, entry := range keys {
		seq, err := parseKeys(entry)
		if err != nil {
			return terminalRun{}, err
		}
		if done = waitFor(ctx, exited, &waitErr, func() bool { return sawOutput.Load() && quietFor(settleTime) }); done {
			run.unused = len(keys) - i
			break
		}
		run.steps = append(run.steps, terminalStep{Keys: entry, Screen: screenText(term)})
		lastOutput.Store(time.Now().UnixNano())
		if _, err := keyboard.Write([]byte(seq)); err != nil {
			return terminalRun{}, err
		}
	}
	if !done {
		if done = waitFor(ctx, exited, &waitErr, func() bool { return quietFor(waitingTime) }); !done {
			run.waiting = true
			_ = cmd.Process.Kill()
			waitErr = <-exited
		}
	}
	// Let the last output reach the screen.
	select {
	case <-readDone:
	case <-time.After(time.Second):
	}
	run.screen = screenText(term)
	return run, waitErr
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
