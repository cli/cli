package main

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	terminalCols = 120
	terminalRows = 40
)

var settleTime = 500 * time.Millisecond

var keyNames = map[string]string{
	"enter": "\r", "tab": "\t", "space": " ", "backspace": "\x7f",
	"esc": "\x1b", "up": "\x1b[A", "down": "\x1b[B",
	"right": "\x1b[C", "left": "\x1b[D", "ctrl-c": "\x03", "ctrl-d": "\x04",
}

func parseKeys(s string) (string, error) {
	var b strings.Builder
	for rest := s; rest != ""; {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:i])
		rest = rest[i:]
		if strings.HasPrefix(rest, "{{") {
			b.WriteByte('{')
			rest = rest[2:]
			continue
		}
		j := strings.IndexByte(rest, '}')
		if j < 0 {
			return "", fmt.Errorf("--keys %q has a { without a }; type a brace as {{", s)
		}
		seq, ok := keyNames[strings.ToLower(rest[1:j])]
		if !ok {
			return "", fmt.Errorf("--keys %q names an unknown key %s; keys are %s", s, rest[:j+1], keyList())
		}
		b.WriteString(seq)
		rest = rest[j+1:]
	}
	return b.String(), nil
}

func keyList() string {
	names := make([]string, 0, len(keyNames))
	for name := range keyNames {
		names = append(names, "{"+name+"}")
	}
	slices.Sort(names)
	return strings.Join(names, " ")
}

type terminalStep struct {
	Keys   string `json:"keys"`
	Screen string `json:"screen"`
}

type terminalRun struct {
	steps   []terminalStep
	screen  string
	output  string
	waiting bool
	unused  int
}
