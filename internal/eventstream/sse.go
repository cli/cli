// Package eventstream reads server-sent events (SSE) streams, such as the one the GitHub Event Subscriptions API
// delivers events over.
package eventstream

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

// maxLineSize bounds a single line of an SSE stream at 65,536 bytes (64 KiB). Event payloads are small JSON documents
// (under 500 bytes in captured pull request events), so a longer line means a broken or hostile stream. The buffer
// only grows to this size when a line needs it.
const maxLineSize = 64 << 10

// Frame is an event dispatched from a server-sent events stream.
type Frame struct {
	// ID is the stream's last event ID when the frame was dispatched. As the SSE specification requires, a frame
	// without an id field carries over the ID of an earlier frame.
	ID string
	// Event is the event type, or "message" when the frame has no event field.
	Event string
	// Data is the payload. Several data fields in one frame are joined with "\n".
	Data string
}

// FrameReader reads frames from a server-sent events stream. Server-sent events are specified in section 9.2 of the
// [WHATWG HTML Living Standard]; there is no separate SSE standard. FrameReader follows its rules for parsing (9.2.5)
// and interpreting (9.2.6) an event stream, except that the retry field is ignored.
//
// [WHATWG HTML Living Standard]: https://html.spec.whatwg.org/multipage/server-sent-events.html
type FrameReader struct {
	scanner *bufio.Scanner
	started bool
	lastID  string

	// OnComment, when set, is called with the text of each comment line, such as a heartbeat, without the leading
	// colon and the single space after it.
	OnComment func(text string)
}

// NewFrameReader returns a FrameReader that reads from r.
func NewFrameReader(r io.Reader) *FrameReader {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4096), maxLineSize)
	scanner.Split(newLineSplitter())
	return &FrameReader{scanner: scanner}
}

// Next blocks until a complete frame is read and returns it. It returns io.EOF when the stream ends; a frame that
// is not terminated by a blank line before the end is discarded.
//
// Spec: a frame without an event field, or with an empty one, is returned with the event type "message", which is
// the default type of a dispatched event (9.2.6, dispatch step 5).
func (r *FrameReader) Next() (Frame, error) {
	// Spec: the data and event type buffers of 9.2.6. The data buffer holds the values joined with LF rather than
	// each followed by one, so hasData stands in for the buffer being non-empty and no trailing LF has to be removed.
	var data strings.Builder
	var hasData bool
	var event string

	for r.scanner.Scan() {
		line := r.scanner.Text()
		if !r.started {
			r.started = true
			// Spec: the stream may start with a UTF-8 byte order mark (9.2.5), and decoding strips it (9.2.6), so it
			// is not read as part of the first field name. Our API does not send one.
			line = strings.TrimPrefix(line, "\ufeff")
		}

		// Spec: a blank line dispatches the event (9.2.6). With an empty data buffer nothing is dispatched, and the
		// event type buffer is reset so it does not leak into the next frame (dispatch step 2).
		if line == "" {
			if !hasData {
				event = ""
				continue
			}
			// Spec: the type defaults to "message" unless the event type buffer is set (9.2.6, dispatch steps 4 and 5).
			if event == "" {
				event = "message"
			}
			return Frame{ID: r.lastID, Event: event, Data: data.String()}, nil
		}

		// Spec: a line starting with a colon is ignored (9.2.6). Reporting it is our addition, used for heartbeats.
		if comment, ok := strings.CutPrefix(line, ":"); ok {
			if r.OnComment != nil {
				r.OnComment(strings.TrimPrefix(comment, " "))
			}
			continue
		}

		// Spec: the field name is everything before the first colon, or the whole line when there is none, in which
		// case the value is empty. One leading space is removed from the value (9.2.6).
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")

		// Spec: field names are compared literally, and unknown fields are ignored (9.2.6, process the field).
		switch field {
		case "event":
			event = value
		case "data":
			if hasData {
				data.WriteByte('\n')
			}
			data.WriteString(value)
			hasData = true
		case "id":
			// Spec: an ID containing NULL is ignored. The last event ID buffer is never reset between frames, so
			// later frames without an id field keep this value, and an empty id field clears it (9.2.6).
			if !strings.ContainsRune(value, 0) {
				r.lastID = value
			}
		case "retry":
			// Spec: the retry field sets the reconnection time for automatic reconnection (9.2.6). Reconnecting is
			// up to the caller here, so the field is ignored.
		}
	}

	// Spec: at the end of the stream any pending data is discarded (9.2.6).
	if err := r.scanner.Err(); err != nil {
		return Frame{}, err
	}
	return Frame{}, io.EOF
}

// newLineSplitter returns a bufio.SplitFunc for SSE lines. The specification (9.2.5) allows three line endings: CR LF
// ("\r\n"), a lone LF ("\n"), or a lone CR ("\r"), where CR is U+000D CARRIAGE RETURN and LF is U+000A LINE FEED.
//
// A CR is ambiguous until the next byte arrives: it either ends the line on its own or starts a CR LF pair. Rather
// than wait for that byte, which would hold back a complete frame until the server sends more, a CR at the end of
// the available input ends the line immediately, and an LF at the start of the next input is skipped as the second
// half of the pair.
//
// The Event Subscriptions API ends every line with a lone LF, so CR handling is only there for spec compliance.
func newLineSplitter() bufio.SplitFunc {
	var skipLF bool
	return func(data []byte, atEOF bool) (int, []byte, error) {
		// The skipped LF is consumed in the same call rather than returned on its own: a nil token makes the Scanner
		// read again before splitting what it has buffered, which would hold back a complete line until more arrives.
		var skipped int
		if skipLF && len(data) > 0 {
			skipLF = false
			if data[0] == '\n' {
				skipped = 1
				data = data[1:]
			}
		}
		if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
			if data[i] == '\r' {
				if i+1 < len(data) {
					if data[i+1] == '\n' {
						return skipped + i + 2, data[:i], nil
					}
				} else {
					skipLF = true
				}
			}
			return skipped + i + 1, data[:i], nil
		}
		if atEOF && len(data) > 0 {
			return skipped + len(data), data, nil
		}
		return skipped, nil, nil
	}
}
