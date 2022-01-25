package vt10x

import (
	"strconv"
	"strings"
)

// STR sequences are similar to CSI sequences, but have string arguments (and
// as far as I can tell, don't really have a name; STR is the name I took from
// suckless which I imagine comes from rxvt or xterm).
type strEscape struct {
	typ  rune
	buf  []rune
	args []string
}

func (s *strEscape) reset() {
	s.typ = 0
	s.buf = s.buf[:0]
	s.args = nil
}

func (s *strEscape) put(c rune) {
	// TODO: improve allocs with an array backed slice; bench first
	if len(s.buf) < 256 {
		s.buf = append(s.buf, c)
	}
	// Going by st, it is better to remain silent when the STR sequence is not
	// ended so that it is apparent to users something is wrong. The length sanity
	// check ensures we don't absorb the entire stream into memory.
	// TODO: see what rxvt or xterm does
}

func (s *strEscape) parse() {
	s.args = strings.Split(string(s.buf), ";")
}

func (s *strEscape) arg(i, def int) int {
	if i >= len(s.args) || i < 0 {
		return def
	}
	i, err := strconv.Atoi(s.args[i])
	if err != nil {
		return def
	}
	return i
}

func (s *strEscape) argString(i int, def string) string {
	if i >= len(s.args) || i < 0 {
		return def
	}
	return s.args[i]
}

func (t *State) handleSTR() {
	s := &t.str
	s.parse()

	switch s.typ {
	case ']': // OSC - operating system command
		switch d := s.arg(0, 0); d {
		case 0, 1, 2:
			title := s.argString(1, "")
			if title != "" {
				t.setTitle(title)
			}
		case 4: // color set
			if len(s.args) < 3 {
				break
			}
			// setcolorname(s.arg(1, 0), s.argString(2, ""))
		case 104: // color reset
			// TODO: complain about invalid color, redraw, etc.
			// setcolorname(s.arg(1, 0), nil)
		default:
			t.logf("unknown OSC command %d\n", d)
			// TODO: s.dump()
		}
	case 'k': // old title set compatibility
		title := s.argString(0, "")
		if title != "" {
			t.setTitle(title)
		}
	default:
		// TODO: Ignore these codes instead of complain?
		// 'P': // DSC - device control string
		// '_': // APC - application program command
		// '^': // PM - privacy message

		t.logf("unhandled STR sequence '%c'\n", s.typ)
		// t.str.dump()
	}
}
