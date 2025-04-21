//go:build !windows

package iostreams

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Netflix/go-expect"
	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"github.com/stretchr/testify/require"
)

func TestStartProgressIndicatorWithLabel(t *testing.T) {
	osOut := os.Stdout
	defer func() { os.Stdout = osOut }()
	// Why do we need a channel in these tests to implement a timeout instead of
	// relying on expect's timeout?
	//
	// Well, expect's timeout is based on the maximum time of a single read
	// from the console. This works in cases like prompting where we block
	// waiting for input because the console is not ready to be read.
	// But in this case, we are not blocking waiting for input and stdout
	// can be constantly read. This means the timeout will never be reached
	// in the event of a expectation failure.
	// To fix this, we need to implement our own timeout that is based
	// specifically on the total time spent reading the console and waiting
	// for the target string instead of the max time for a single read
	// from the console.
	t.Run("progress indicator respects GH_SPINNER_DISABLED is true", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		io := newTestIOStreams(t, console, true)

		done := make(chan error)

		go func() {
			_, err := console.ExpectString("Working...")
			done <- err
		}()

		io.StartProgressIndicatorWithLabel("")
		defer io.StopProgressIndicator()

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out waiting for progress indicator")
		}
	})

	t.Run("progress indicator respects GH_SPINNER_DISABLED is false", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		io := newTestIOStreams(t, console, false)

		done := make(chan error)

		go func() {
			_, err := console.ExpectString("⣾")
			done <- err
		}()

		io.StartProgressIndicatorWithLabel("")
		defer io.StopProgressIndicator()

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out waiting for progress indicator")
		}
	})

	t.Run("progress indicator with GH_SPINNER_DISABLED shows label", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		io := newTestIOStreams(t, console, true)
		progressIndicatorLabel := "downloading happiness"

		done := make(chan error)

		go func() {
			_, err := console.ExpectString(progressIndicatorLabel + "...")
			done <- err
		}()

		io.StartProgressIndicatorWithLabel(progressIndicatorLabel)
		defer io.StopProgressIndicator()

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out waiting for progress indicator")
		}
	})

	t.Run("progress indicator shows label and spinner", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		io := newTestIOStreams(t, console, false)
		progressIndicatorLabel := "downloading happiness"

		done := make(chan error)

		go func() {
			_, err := console.ExpectString(progressIndicatorLabel)
			require.NoError(t, err)
			_, err = console.ExpectString("⣾")
			done <- err
		}()

		io.StartProgressIndicatorWithLabel(progressIndicatorLabel)
		defer io.StopProgressIndicator()

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out waiting for progress indicator")
		}
	})

	t.Run("multiple calls to start progress indicator with GH_SPINNER_DISABLED prints additional labels", func(t *testing.T) {
		console := newTestVirtualTerminal(t)
		io := newTestIOStreams(t, console, true)
		progressIndicatorLabel1 := "downloading happiness"
		progressIndicatorLabel2 := "downloading sadness"
		done := make(chan error)
		go func() {
			_, err := console.ExpectString(progressIndicatorLabel1 + "...")
			require.NoError(t, err)
			_, err = console.ExpectString(progressIndicatorLabel2 + "...")
			done <- err
		}()
		io.StartProgressIndicatorWithLabel(progressIndicatorLabel1)
		defer io.StopProgressIndicator()
		io.StartProgressIndicatorWithLabel(progressIndicatorLabel2)

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out waiting for progress indicator")
		}
	})
}

func newTestVirtualTerminal(t *testing.T) *expect.Console {
	t.Helper()

	// Create a PTY and hook up a virtual terminal emulator
	ptm, pts, err := pty.Open()
	require.NoError(t, err)

	term := vt10x.New(vt10x.WithWriter(pts))

	// Create a console via Expect that allows scripting against the terminal
	consoleOpts := []expect.ConsoleOpt{
		expect.WithStdin(ptm),
		expect.WithStdout(term),
		expect.WithCloser(ptm, pts),
		failOnExpectError(t),
		failOnSendError(t),
		expect.WithDefaultTimeout(time.Second),
	}

	console, err := expect.NewConsole(consoleOpts...)
	require.NoError(t, err)
	t.Cleanup(func() { testCloser(t, console) })

	return console
}

func newTestIOStreams(t *testing.T, console *expect.Console, spinnerDisabled bool) *IOStreams {
	t.Helper()

	in := console.Tty()
	out := console.Tty()
	errOut := console.Tty()

	// Because the briandowns/spinner checks os.Stdout directly,
	// we need this hack to trick it into allowing the spinner to print...
	os.Stdout = out

	io := &IOStreams{
		In:     in,
		Out:    out,
		ErrOut: errOut,
		term:   fakeTerm{},
	}
	io.progressIndicatorEnabled = true
	io.SetSpinnerDisabled(spinnerDisabled)
	return io
}

// failOnExpectError adds an observer that will fail the test in a standardised way
// if any expectation on the command output fails, without requiring an explicit
// assertion.
//
// Use WithRelaxedIO to disable this behaviour.
func failOnExpectError(t *testing.T) expect.ConsoleOpt {
	t.Helper()
	return expect.WithExpectObserver(
		func(matchers []expect.Matcher, buf string, err error) {
			t.Helper()

			if err == nil {
				return
			}

			if len(matchers) == 0 {
				t.Fatalf("Error occurred while matching %q: %s\n", buf, err)
			}

			var criteria []string
			for _, matcher := range matchers {
				criteria = append(criteria, fmt.Sprintf("%q", matcher.Criteria()))
			}
			t.Fatalf("Failed to find [%s] in %q: %s\n", strings.Join(criteria, ", "), buf, err)
		},
	)
}

// failOnSendError adds an observer that will fail the test in a standardised way
// if any sending of input fails, without requiring an explicit assertion.
//
// Use WithRelaxedIO to disable this behaviour.
func failOnSendError(t *testing.T) expect.ConsoleOpt {
	t.Helper()
	return expect.WithSendObserver(
		func(msg string, n int, err error) {
			t.Helper()

			if err != nil {
				t.Fatalf("Failed to send %q: %s\n", msg, err)
			}
			if len(msg) != n {
				t.Fatalf("Only sent %d of %d bytes for %q\n", n, len(msg), msg)
			}
		},
	)
}

// testCloser is a helper to fail the test if a Closer fails to close.
func testCloser(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Errorf("Close failed: %s", err)
	}
}
