package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
)

var jsonTypeRE = regexp.MustCompile(`[/+]json($|;)`)

// GitHub servers do not sanitize their API output for terminal display
// and leave in unescaped ASCII control characters.
// C0 control characters are represented in their unicode code point form ranging from \u0000 to \u001F.
// C1 control characters are represented in two bytes, the first being 0xC2 and the second ranging from 0x80 to 0x9F.
// These control characters will be interpreted by the terminal, this behaviour can be
// used maliciously as an attack vector, especially the control characters \u001B and \u009B.
// This function wraps JSON response bodies in a ReadCloser that transforms C0 and C1
// control characters to their caret notations respectively so that the terminal will not
// interpret them.
func AddASCIISanitizer(rt http.RoundTripper) http.RoundTripper {
	return &funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		res, err := rt.RoundTrip(req)
		if err != nil || !jsonTypeRE.MatchString(res.Header.Get("Content-Type")) {
			return res, err
		}
		res.Body = &sanitizeASCIIReadCloser{ReadCloser: res.Body}
		return res, err
	}}
}

// sanitizeASCIIReadCloser implements the ReadCloser interface.
type sanitizeASCIIReadCloser struct {
	io.ReadCloser
	addEscape bool
	remainder []byte
}

// Read uses a sliding window alogorithm to detect C0 and C1
// ASCII control sequences as they are read and replaces them
// with equivelent inert characters. Characters that are not part
// of a control sequence not modified.
func (s *sanitizeASCIIReadCloser) Read(out []byte) (int, error) {
	var bufIndex, outIndex int
	outLen := len(out)
	buf := make([]byte, outLen)

	bufLen, readErr := s.ReadCloser.Read(buf)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		if bufLen > 0 {
			// Do not sanitize if there was a read error that is not EOF.
			bufLen = copy(out, buf)
		}
		return bufLen, readErr
	}
	buf = buf[:bufLen]

	if s.remainder != nil {
		buf = append(s.remainder, buf...)
		bufLen += len(s.remainder)
		s.remainder = s.remainder[:0]
	}

	for bufIndex < bufLen-6 && outIndex < outLen {
		window := buf[bufIndex : bufIndex+6]

		// Replace C1 Control Characters
		if window[0] == 0xC2 {
			repl, _ := mapC1ToCaret(window[:2])
			for j := 0; j < len(repl); j++ {
				if outIndex < outLen {
					out[outIndex] = repl[j]
					outIndex++
				} else {
					s.remainder = append(s.remainder, repl[j])
				}
			}
			bufIndex += 2
			continue
		}

		// Replace C0 Control Characters
		if bytes.HasPrefix(window, []byte(`\u00`)) {
			repl, found := mapC0ToCaret(window)
			if s.addEscape && found {
				repl = append([]byte{'\\'}, repl...)
			}
			s.addEscape = false
			for j := 0; j < len(repl); j++ {
				if outIndex < outLen {
					out[outIndex] = repl[j]
					outIndex++
				} else {
					s.remainder = append(s.remainder, repl[j])
				}
			}
			bufIndex += 6
			continue
		}

		if window[0] == '\\' {
			s.addEscape = !s.addEscape
		} else {
			s.addEscape = false
		}

		out[outIndex] = buf[bufIndex]
		outIndex++
		bufIndex++
	}

	if readErr != nil && errors.Is(readErr, io.EOF) {
		remaining := bufLen - bufIndex
		for j := 0; j < remaining; j++ {
			if outIndex < outLen {
				out[outIndex] = buf[bufIndex]
				outIndex++
				bufIndex++
			} else {
				s.remainder = append(s.remainder, buf[bufIndex])
				bufIndex++
			}
		}
	} else {
		if bufIndex < bufLen {
			s.remainder = append(s.remainder, buf[bufIndex:]...)
		}
	}

	if len(s.remainder) != 0 {
		readErr = nil
	}

	return outIndex, readErr
}

// mapC0ToCaret maps C0 control sequences to caret notation.
func mapC0ToCaret(b []byte) ([]byte, bool) {
	m := map[string]string{
		`\u0000`: `^@`,
		`\u0001`: `^A`,
		`\u0002`: `^B`,
		`\u0003`: `^C`,
		`\u0004`: `^D`,
		`\u0005`: `^E`,
		`\u0006`: `^F`,
		`\u0007`: `^G`,
		`\u0008`: `^H`,
		`\u0009`: `^I`,
		`\u000a`: `^J`,
		`\u000b`: `^K`,
		`\u000c`: `^L`,
		`\u000d`: `^M`,
		`\u000e`: `^N`,
		`\u000f`: `^O`,
		`\u0010`: `^P`,
		`\u0011`: `^Q`,
		`\u0012`: `^R`,
		`\u0013`: `^S`,
		`\u0014`: `^T`,
		`\u0015`: `^U`,
		`\u0016`: `^V`,
		`\u0017`: `^W`,
		`\u0018`: `^X`,
		`\u0019`: `^Y`,
		`\u001a`: `^Z`,
		`\u001b`: `^[`,
		`\u001c`: `^\\`,
		`\u001d`: `^]`,
		`\u001e`: `^^`,
		`\u001f`: `^_`,
	}
	if c, ok := m[strings.ToLower(string(b))]; ok {
		return []byte(c), true
	}
	return b, false
}

// mapC1ToCaret maps C1 control sequences to caret notation.
// C1 control sequences are two bytes and start with 0xC2.
func mapC1ToCaret(b []byte) ([]byte, bool) {
	if len(b) != 2 {
		return b, false
	}
	if b[0] != 0xC2 {
		return b, false
	}
	m := map[byte]string{
		128: `^@`,
		129: `^A`,
		130: `^B`,
		131: `^C`,
		132: `^D`,
		133: `^E`,
		134: `^F`,
		135: `^G`,
		136: `^H`,
		137: `^I`,
		138: `^J`,
		139: `^K`,
		140: `^L`,
		141: `^M`,
		142: `^N`,
		143: `^O`,
		144: `^P`,
		145: `^Q`,
		146: `^R`,
		147: `^S`,
		148: `^T`,
		149: `^U`,
		150: `^V`,
		151: `^W`,
		152: `^X`,
		153: `^Y`,
		154: `^Z`,
		155: `^[`,
		156: `^\\`,
		157: `^]`,
		158: `^^`,
		159: `^_`,
	}
	if c, ok := m[b[1]]; ok {
		return []byte(c), true
	}
	return b, false
}
