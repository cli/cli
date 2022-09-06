package iostreams

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestIOStreams_ForceTerminal(t *testing.T) {
	tests := []struct {
		name      string
		iostreams *IOStreams
		arg       string
		wantTTY   bool
		wantWidth int
	}{
		{
			name:      "explicit width",
			iostreams: &IOStreams{},
			arg:       "72",
			wantTTY:   true,
			wantWidth: 72,
		},
		{
			name: "measure width",
			iostreams: &IOStreams{
				ttySize: func() (int, int, error) {
					return 72, 0, nil
				},
			},
			arg:       "true",
			wantTTY:   true,
			wantWidth: 72,
		},
		{
			name: "measure width fails",
			iostreams: &IOStreams{
				Out: &fdWriter{
					Writer: io.Discard,
					fd:     1,
				},
				ttySize: func() (int, int, error) {
					return -1, -1, errors.New("ttySize sabotage!")
				},
			},
			arg:       "true",
			wantTTY:   true,
			wantWidth: 80,
		},
		{
			name: "apply percentage",
			iostreams: &IOStreams{
				ttySize: func() (int, int, error) {
					return 72, 0, nil
				},
			},
			arg:       "50%",
			wantTTY:   true,
			wantWidth: 36,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.iostreams.ForceTerminal(tt.arg)
			if isTTY := tt.iostreams.IsStdoutTTY(); isTTY != tt.wantTTY {
				t.Errorf("IOStreams.IsStdoutTTY() = %v, want %v", isTTY, tt.wantTTY)
			}
			if tw := tt.iostreams.TerminalWidth(); tw != tt.wantWidth {
				t.Errorf("IOStreams.TerminalWidth() = %v, want %v", tw, tt.wantWidth)
			}
		})
	}
}

func TestStopAlternateScreenBuffer(t *testing.T) {
	ios, _, stdout, _ := Test()
	ios.SetAlternateScreenBufferEnabled(true)

	ios.StartAlternateScreenBuffer()
	fmt.Fprint(ios.Out, "test")
	ios.StopAlternateScreenBuffer()

	// Stopping a subsequent time should no-op.
	ios.StopAlternateScreenBuffer()

	const want = "\x1b[?1049htest\x1b[?1049l"
	if got := stdout.String(); got != want {
		t.Errorf("after IOStreams.StopAlternateScreenBuffer() got %q, want %q", got, want)
	}
}

func TestIOStreams_pager(t *testing.T) {
	t.Skip("TODO: fix this test in race detection mode")
	ios, _, stdout, _ := Test()
	ios.SetStdoutTTY(true)
	ios.SetPager(fmt.Sprintf("%s -test.run=TestHelperProcess --", os.Args[0]))
	t.Setenv("GH_WANT_HELPER_PROCESS", "1")
	if err := ios.StartPager(); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(ios.Out, "line1"); err != nil {
		t.Errorf("error writing line 1: %v", err)
	}
	if _, err := fmt.Fprintln(ios.Out, "line2"); err != nil {
		t.Errorf("error writing line 2: %v", err)
	}
	ios.StopPager()
	wants := "pager: line1\npager: line2\n"
	if got := stdout.String(); got != wants {
		t.Errorf("expected %q, got %q", wants, got)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GH_WANT_HELPER_PROCESS") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Printf("pager: %s\n", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error reading stdin: %v", err)
		os.Exit(1)
	}
	os.Exit(0)
}
