package create

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
)

func NewRegexWriter(out io.Writer, regexp *regexp.Regexp, repl string) io.Writer {
	return &RegexWriter{out: out, regexp: *regexp, repl: repl}
}

type RegexWriter struct {
	out    io.Writer
	regexp regexp.Regexp
	repl   string
}

func (s RegexWriter) Write(data []byte) (int, error) {
	filtered := []byte{}
	repl := []byte(s.repl)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Split(scanLines)

	for scanner.Scan() {
		b := scanner.Bytes()
		f := s.regexp.ReplaceAll(b, repl)
		if len(f) > 0 {
			filtered = append(filtered, f...)
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

func scanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[0 : i+1], nil
	}

	if atEOF {
		return len(data), data, nil
	}

	return 0, nil, nil
}
