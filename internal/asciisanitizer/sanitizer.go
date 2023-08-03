// Package asciisanitizer implements an ASCII control character sanitizer for GitHub API responses so they can be
// safely displayed in the terminal. The GitHub API does not sanitize their responses for terminal display and will
// leave in unescaped ASCII control characters. These ASCII control characters will be interpreted by the terminal,
// this behaviour can be used maliciously as an attack vector, especially the ASCII control characters \u001B and \u009B.
package asciisanitizer

import (
	"bytes"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/transform"
)

// Sanitizer implements transform.Transformer interface.
type Sanitizer struct {
	addEscape bool
}

// Transform uses a sliding window algorithm to detect C0 and C1 control characters as they are read and replaces
// them with equivalent inert characters. Bytes that are not part of a control character are not modified.
// C0 control characters can be either encoded or unencoded. C1 control characters are not encoded.
// Encoded C0 control characters are six byte strings, representing the unicode code point, ranging from \u0000 to \u001F.
func (t *Sanitizer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	transfer := func(write, read []byte) error {
		readLength := len(read)
		writeLength := len(write)
		if writeLength > len(dst) {
			return transform.ErrShortDst
		}
		copy(dst, write)
		nDst += writeLength
		dst = dst[writeLength:]
		nSrc += readLength
		src = src[readLength:]
		return nil
	}

	for len(src) > 0 {
		// Make sure we always have 6 bytes if there are 6 bytes available.
		// This is necessary for encoded C0 control characters.
		if len(src) < 6 && !atEOF {
			err = transform.ErrShortSrc
			return
		}
		// Only support UTF-8 encoded data.
		r, size := utf8.DecodeRune(src)
		if r == utf8.RuneError {
			if !atEOF {
				err = transform.ErrShortSrc
				return
			} else {
				err = errors.New("invalid UTF-8 string")
				return
			}
		}
		// Replace C0 and C1 control characters.
		if unicode.IsControl(r) {
			if repl, found := mapControlToCaret(r); found {
				err = transfer(repl, src[:size])
				if err != nil {
					return
				}
				continue
			}
		}
		// Replace encoded C0 control characters.
		if len(src) >= 6 {
			if repl, found := mapEncodedControlToCaret(src[:6]); found {
				if t.addEscape {
					// Add an escape character when necessary to prevent creating
					// invalid JSON with our replacements.
					repl = append([]byte{'\\'}, repl...)
				}
				err = transfer(repl, src[:6])
				if err != nil {
					return
				}
				continue
			}
		}
		err = transfer(src[:size], src[:size])
		if err != nil {
			return
		}
		if r == '\\' {
			t.addEscape = !t.addEscape
		} else {
			t.addEscape = false
		}
	}
	return
}

// Reset resets the state and allows the Sanitizer to be reused.
func (t *Sanitizer) Reset() {
	t.addEscape = false
}

// mapControlToCaret maps C0 and C1 control characters to their caret notation.
func mapControlToCaret(r rune) ([]byte, bool) {
	//\t (09), \n (10), \v (11), \r (13) are valid C0 characters and are not sanitized.
	m := map[rune]string{
		0:   `^@`,
		1:   `^A`,
		2:   `^B`,
		3:   `^C`,
		4:   `^D`,
		5:   `^E`,
		6:   `^F`,
		7:   `^G`,
		8:   `^H`,
		12:  `^L`,
		14:  `^N`,
		15:  `^O`,
		16:  `^P`,
		17:  `^Q`,
		18:  `^R`,
		19:  `^S`,
		20:  `^T`,
		21:  `^U`,
		22:  `^V`,
		23:  `^W`,
		24:  `^X`,
		25:  `^Y`,
		26:  `^Z`,
		27:  `^[`,
		28:  `^\\`,
		29:  `^]`,
		30:  `^^`,
		31:  `^_`,
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
	if c, ok := m[r]; ok {
		return []byte(c), true
	}
	return nil, false
}

// mapEncodedControlToCaret maps encoded C0 control characters to their caret notation.
// Encoded C0 control characters are six byte strings, representing the unicode code point, ranging from \u0000 to \u001F.
func mapEncodedControlToCaret(b []byte) ([]byte, bool) {
	if len(b) != 6 {
		return nil, false
	}
	if !bytes.HasPrefix(b, []byte(`\u00`)) {
		return nil, false
	}
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
	return nil, false
}
