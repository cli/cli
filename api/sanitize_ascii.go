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

// GitHub servers return non-printable characters as their unicode code point values.
// The values of \u0000 to \u001F represent C0 ASCII control characters and
// the values of \u0080 to \u009F represent C1 ASCII control characters. These control
// characters will be interpreted by the terminal, this behaviour can be used maliciously
// as an attack vector, especially the control character \u001B. This function wraps
// JSON response bodies in a ReadCloser that transforms C0 and C1 control characters
// to their caret and hex notations respectively so that the terminal will not interpret them.
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

		if bytes.HasPrefix(window, []byte(`\u00`)) {
			repl, _ := mapControlCharacterToCaret(window)
			if s.addEscape {
				repl = append([]byte{'\\'}, repl...)
				s.addEscape = false
			}
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

// mapControlCharacterToCaret maps C0 control sequences to caret notation
// and C1 control sequences to hex notation. C1 control sequences do not
// have caret notation representation.
func mapControlCharacterToCaret(b []byte) ([]byte, bool) {
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
		`\u0080`: `\\200`,
		`\u0081`: `\\201`,
		`\u0082`: `\\202`,
		`\u0083`: `\\203`,
		`\u0084`: `\\204`,
		`\u0085`: `\\205`,
		`\u0086`: `\\206`,
		`\u0087`: `\\207`,
		`\u0088`: `\\210`,
		`\u0089`: `\\211`,
		`\u008a`: `\\212`,
		`\u008b`: `\\213`,
		`\u008c`: `\\214`,
		`\u008d`: `\\215`,
		`\u008e`: `\\216`,
		`\u008f`: `\\217`,
		`\u0090`: `\\220`,
		`\u0091`: `\\221`,
		`\u0092`: `\\222`,
		`\u0093`: `\\223`,
		`\u0094`: `\\224`,
		`\u0095`: `\\225`,
		`\u0096`: `\\226`,
		`\u0097`: `\\227`,
		`\u0098`: `\\230`,
		`\u0099`: `\\231`,
		`\u009a`: `\\232`,
		`\u009b`: `\\233`,
		`\u009c`: `\\234`,
		`\u009d`: `\\235`,
		`\u009e`: `\\236`,
		`\u009f`: `\\237`,
	}
	if c, ok := m[strings.ToLower(string(b))]; ok {
		return []byte(c), true
	}
	return b, false
}
