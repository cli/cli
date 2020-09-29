package iostreams

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
)

func NewRegexFilterWriter(out io.Writer, regexp *regexp.Regexp, repl string) io.Writer {
	return &RegexFilterWriter{out: out, regexp: *regexp, repl: repl}
}

type RegexFilterWriter struct {
	out    io.Writer
	regexp regexp.Regexp
	repl   string
}

func (s RegexFilterWriter) Write(data []byte) (int, error) {
	filtered := []byte{}
	repl := []byte(s.repl)
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		b := scanner.Bytes()
		f := s.regexp.ReplaceAll(b, repl)
		if len(f) > 0 {
			filtered = append(filtered, f...)
			filtered = append(filtered, []byte("\n")...)
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	if len(filtered) != 0 {
		_, err := s.out.Write(filtered)
		if err != nil {
			return 0, err
		}
	}

	return len(data), nil
}
