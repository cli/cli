package eventstream

import (
	"bufio"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrameReader(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantFrames   []Frame
		wantComments []string
	}{
		{
			name: "stream with heartbeats, events, and an end frame",
			input: heredoc.Doc(`
				: keep-alive

				id: e1
				event: test.event
				data: {"id":"e1"}

				: keep-alive

				event: test.end
				data: {"outcome":"reconnect","reason":"shutdown"}

			`),
			wantFrames: []Frame{
				{ID: "e1", Event: "test.event", Data: `{"id":"e1"}`},
				{ID: "e1", Event: "test.end", Data: `{"outcome":"reconnect","reason":"shutdown"}`},
			},
			wantComments: []string{"keep-alive", "keep-alive"},
		},
		{
			name: "joins several data fields with a newline",
			input: heredoc.Doc(`
				data: {"a":
				data: 1}

			`),
			wantFrames: []Frame{{Event: "message", Data: "{\"a\":\n1}"}},
		},
		{
			name: "removes only one leading space from a value",
			input: heredoc.Doc(`
				event:x
				data:  two spaces

			`),
			wantFrames: []Frame{{Event: "x", Data: " two spaces"}},
		},
		{
			// Raw string literals cannot hold carriage returns, so this input stays escaped.
			name:  "accepts CRLF and CR line endings",
			input: "data: a\r\ndata: b\r\n\r\ndata: c\r\rdata: d\n\n",
			wantFrames: []Frame{
				{Event: "message", Data: "a\nb"},
				{Event: "message", Data: "c"},
				{Event: "message", Data: "d"},
			},
		},
		{
			name: "strips a leading byte order mark",
			input: "\ufeff" + heredoc.Doc(`
				data: a

			`),
			wantFrames: []Frame{{Event: "message", Data: "a"}},
		},
		{
			name: "does not dispatch a frame without data or let its event type leak",
			input: heredoc.Doc(`
				event: ignored

				data: a

			`),
			wantFrames: []Frame{{Event: "message", Data: "a"}},
		},
		{
			name: "dispatches an empty data field and a field without a colon",
			input: heredoc.Doc(`
				data

				data:

			`),
			wantFrames: []Frame{{Event: "message"}, {Event: "message"}},
		},
		{
			name: "keeps the last event ID, ignores an ID with NUL, and resets on an empty ID",
			input: heredoc.Docf(`
				id: 1
				data: a

				data: b

				id: 2%s
				data: c

				id
				data: d

			`, "\x00"),
			wantFrames: []Frame{
				{ID: "1", Event: "message", Data: "a"},
				{ID: "1", Event: "message", Data: "b"},
				{ID: "1", Event: "message", Data: "c"},
				{ID: "", Event: "message", Data: "d"},
			},
		},
		{
			name: "ignores unknown fields and retry",
			input: heredoc.Doc(`
				retry: 1000
				foo: bar
				data: a

			`),
			wantFrames: []Frame{{Event: "message", Data: "a"}},
		},
		{
			name: "discards a frame cut off by the end of the stream",
			input: heredoc.Doc(`
				data: a

				data: b
			`),
			wantFrames: []Frame{{Event: "message", Data: "a"}},
		},
		{
			name: "reports comments without their leading colon and space",
			input: heredoc.Doc(`
				:no space
				:  two spaces
				:

			`),
			wantComments: []string{"no space", " two spaces", ""},
		},
	}

	readers := map[string]func(string) io.Reader{
		"whole":    func(s string) io.Reader { return strings.NewReader(s) },
		"one byte": func(s string) io.Reader { return iotest.OneByteReader(strings.NewReader(s)) },
	}

	for _, tt := range tests {
		for readerName, newReader := range readers {
			t.Run(tt.name+"/"+readerName, func(t *testing.T) {
				var comments []string
				r := NewFrameReader(newReader(tt.input))
				r.OnComment = func(text string) { comments = append(comments, text) }

				var frames []Frame
				for {
					frame, err := r.Next()
					if err == io.EOF {
						break
					}
					require.NoError(t, err)
					frames = append(frames, frame)
				}

				assert.Equal(t, tt.wantFrames, frames)
				assert.Equal(t, tt.wantComments, comments)
			})
		}
	}
}

func TestFrameReaderSplitCRLFWithBufferedFrameEnd(t *testing.T) {
	// The CR arrives alone, and the LF that completes it arrives together with the blank line that ends the frame.
	// The stream then stays open, as it does between heartbeats.
	body, w := io.Pipe()
	t.Cleanup(func() { w.Close() })
	go func() {
		_, _ = w.Write([]byte("data: a\r"))
		_, _ = w.Write([]byte("\n\r\n"))
	}()

	type result struct {
		frame Frame
		err   error
	}
	done := make(chan result, 1)
	go func() {
		frame, err := NewFrameReader(body).Next()
		done <- result{frame, err}
	}()

	select {
	case res := <-done:
		require.NoError(t, res.err)
		assert.Equal(t, Frame{Event: "message", Data: "a"}, res.frame)
	case <-time.After(5 * time.Second):
		w.Close()
		<-done
		t.Fatal("the frame was not returned until the stream sent more bytes")
	}
}

func TestFrameReaderLineTooLong(t *testing.T) {
	r := NewFrameReader(strings.NewReader("data: " + strings.Repeat("x", maxLineSize) + "\n\n"))

	_, err := r.Next()

	require.ErrorIs(t, err, bufio.ErrTooLong)
}

func TestFrameReaderReadError(t *testing.T) {
	readErr := io.ErrClosedPipe
	r := NewFrameReader(io.MultiReader(strings.NewReader("data: a\n"), iotest.ErrReader(readErr)))

	_, err := r.Next()

	require.ErrorIs(t, err, readErr)
}
