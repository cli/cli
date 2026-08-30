package iostreams

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBinaryContentType(t *testing.T) {
	tests := []struct {
		name       string
		content    []byte
		wantMIME   string
		wantBinary bool
	}{
		{
			name:       "empty content is not binary",
			content:    []byte{},
			wantBinary: false,
		},
		{
			name:       "plain text is not binary",
			content:    []byte("hello world\n"),
			wantBinary: false,
		},
		{
			name:       "png is binary",
			content:    append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 16)...),
			wantMIME:   "image/png",
			wantBinary: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, ok := BinaryContentType(tt.content)
			assert.Equal(t, tt.wantBinary, ok)
			assert.Equal(t, tt.wantMIME, mime)
		})
	}
}

func TestContainsEscapeSequence(t *testing.T) {
	assert.False(t, ContainsEscapeSequence([]byte("plain text")))
	assert.True(t, ContainsEscapeSequence([]byte("danger\x1b[31m")))
}

func TestCopyGuardedContent(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 16)...)
	readErr := errors.New("boom")

	tests := []struct {
		name    string
		content []byte
		// reader overrides content for cases a byte slice cannot express, such
		// as a mid-stream read failure.
		reader  io.Reader
		isTTY   bool
		wantOut []byte
		// wantErrIs matches a sentinel error with errors.Is; wantErrAs matches a
		// typed error (e.g. BinaryTerminalError, which carries a MIME field) with
		// errors.As, so its value must be a pointer to that error type.
		wantErrIs error
		wantErrAs any
	}{
		{
			name:    "clean text is written",
			content: []byte("hello world\n"),
			isTTY:   true,
			wantOut: []byte("hello world\n"),
		},
		{
			name:      "text with escape is refused",
			content:   []byte("danger\x1b[31mtext"),
			isTTY:     true,
			wantErrIs: ErrEscapeSequence,
		},
		{
			name:      "text with escape is refused when piped",
			content:   []byte("danger\x1b[31mtext"),
			isTTY:     false,
			wantErrIs: ErrEscapeSequence,
		},
		{
			name:      "binary to terminal is refused",
			content:   png,
			isTTY:     true,
			wantErrAs: &BinaryTerminalError{},
		},
		{
			name:    "binary when piped is written raw",
			content: png,
			isTTY:   false,
			wantOut: png,
		},
		{
			name:    "empty content writes nothing",
			content: []byte{},
			isTTY:   true,
			wantOut: nil,
		},
		{
			// Content past the sniff window is still inspected, so an escape
			// hiding beyond the first chunk is caught.
			name:      "text with escape past the sniff window is refused",
			content:   append(bytes.Repeat([]byte("a"), contentSniffLen*2), []byte("\x1b[31m")...),
			isTTY:     false,
			wantErrIs: ErrEscapeSequence,
		},
		{
			name:      "read failure unrelated to EOF is surfaced",
			reader:    io.MultiReader(strings.NewReader("hi"), errReader{readErr}),
			isTTY:     false,
			wantErrIs: readErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.reader
			if r == nil {
				r = bytes.NewReader(tt.content)
			}

			var buf bytes.Buffer
			err := CopyGuardedContent(&buf, r, tt.isTTY)

			if tt.wantErrAs != nil {
				require.ErrorAs(t, err, tt.wantErrAs)
				assert.Empty(t, buf.Bytes())
				return
			}
			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
				assert.Empty(t, buf.Bytes())
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, buf.Bytes())
		})
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }
