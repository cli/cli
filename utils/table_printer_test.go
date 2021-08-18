package utils

import (
	"bytes"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/pkg/iostreams"
)

func Test_ttyTablePrinter_truncate(t *testing.T) {
	buf := bytes.Buffer{}
	tp := &ttyTablePrinter{
		out:      &buf,
		maxWidth: 5,
	}

	tp.AddField("1", nil, nil)
	tp.AddField("hello", nil, nil)
	tp.EndRow()
	tp.AddField("2", nil, nil)
	tp.AddField("world", nil, nil)
	tp.EndRow()

	err := tp.Render()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "1  he\n2  wo\n"
	if buf.String() != expected {
		t.Errorf("expected: %q, got: %q", expected, buf.String())
	}
}

func Test_NewTtyTablePrinter_maxWidth(t *testing.T) {
	tests := []struct {
		name     string
		tty      bool
		maxWidth int
		wantW    string
	}{
		{
			name:     "terminal width less than max",
			tty:      true,
			maxWidth: MaxInt,
			wantW: heredoc.Doc(`1  Caption One
			3  Caption Three
			5  Caption Five
			`),
		},
		{
			name:     "terminal width greater than max",
			tty:      false,
			maxWidth: 15,
			wantW: heredoc.Doc(`1  Caption One
			3  Caption T...
			5  Caption Five
			`),
		},
		{
			name:     "not TTY",
			tty:      false,
			maxWidth: MaxInt,
			wantW: heredoc.Doc(`1  Caption One
			3  Caption Three
			5  Caption Five
			`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, w, _ := iostreams.Test()
			io.SetStdoutTTY(tt.tty)

			printer := NewTtyTablePrinter(io, tt.maxWidth)

			printer.AddField("1", nil, nil)
			printer.AddField("Caption One", nil, nil)
			printer.EndRow()

			printer.AddField("3", nil, nil)
			printer.AddField("Caption Three", nil, nil)
			printer.EndRow()

			printer.AddField("5", nil, nil)
			printer.AddField("Caption Five", nil, nil)
			printer.EndRow()

			if err := printer.Render(); err != nil {
				t.Errorf("failed to render table: %v", err)
				return
			}

			if gotW := w.String(); gotW != tt.wantW {
				t.Errorf("got %q, want %q", gotW, tt.wantW)
			}
		})
	}
}
