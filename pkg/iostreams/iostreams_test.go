package iostreams

import (
	"errors"
	"fmt"
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
