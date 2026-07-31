package iostreams

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// contentSniffLen is how many leading bytes are inspected to classify content as
// binary or textual, matching the sample size used by [http.DetectContentType].
const contentSniffLen = 512

// ContainsEscapeSequence reports whether b contains an ANSI escape byte (0x1B),
// which can manipulate a terminal when printed.
func ContainsEscapeSequence(b []byte) bool {
	return bytes.IndexByte(b, 0x1B) >= 0
}

// BinaryContentType reports whether content appears to be binary and, if so,
// returns its detected MIME type. Textual content returns ("", false).
func BinaryContentType(content []byte) (string, bool) {
	if len(content) == 0 {
		return "", false
	}
	ct := http.DetectContentType(content)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if strings.HasPrefix(ct, "text/") {
		return "", false
	}
	return ct, true
}

// BinaryTerminalError reports that binary content was about to be written to a
// terminal, where it is unreadable and may carry control bytes.
type BinaryTerminalError struct {
	MIME string
}

func (e BinaryTerminalError) Error() string {
	return fmt.Sprintf("refusing to output binary content (%s) to the terminal", e.MIME)
}

// ErrEscapeSequence reports that textual content carried terminal escape
// sequences and was refused.
var ErrEscapeSequence = errors.New("content contains terminal escape sequences")

// CopyGuardedContent writes external content from r to w under the safety model
// used by byte-moving commands: binary content is refused when w targets a
// terminal and streamed verbatim otherwise, while textual content is refused when
// it carries terminal escape sequences. Binary content streams without buffering;
// only textual content is buffered, so its escapes are caught before any byte is
// written. On refusal the output stream is left untouched; otherwise it receives
// the full content. isTTY reports whether w targets the user's terminal.
//
// It returns [BinaryTerminalError] or [ErrEscapeSequence] so callers can add
// command-specific guidance. Callers that must stream verbatim (an explicit
// opt-out, or output bound for a file) should copy directly instead.
func CopyGuardedContent(w io.Writer, r io.Reader, isTTY bool) error {
	head := make([]byte, contentSniffLen)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	head = head[:n]

	if mime, ok := BinaryContentType(head); ok {
		if isTTY {
			return BinaryTerminalError{MIME: mime}
		}
		if _, err := w.Write(head); err != nil {
			return err
		}
		_, err := io.Copy(w, r)
		return err
	}

	rest, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	content := append(head, rest...)
	if ContainsEscapeSequence(content) {
		return ErrEscapeSequence
	}
	_, err = w.Write(content)
	return err
}
