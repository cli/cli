package unwrap

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

var indentRE = regexp.MustCompile(`^[\t ]`)
var listItemRE = regexp.MustCompile(`^\s*([-*]|\d+\.?)\s`)
var headerRE = regexp.MustCompile(`^\s*[a-zA-Z-]+:\s`)

const shortLineLen = 50

// type Writer struct {
// 	out io.Writer
// 	buf strings.Builder
// }

// func (w *Writer) Write(data []byte) (int, error) {
// 	if len(data) == 0 {
// 		return 0, nil
// 	}

// 	startAt := 0
// 	for pos, c := range data {
// 		if c == '\n' {
// 			w.buf.Write(data[startAt:pos])
// 			startAt = pos+1
// 		}
// 	}

// 	return w.out.Write(data)
// }

func Unwrap(text string) string {
	s := bufio.NewScanner(bytes.NewBufferString(text))
	var out strings.Builder
	inListItem := false
	needSpace := false
	var lastLineLength int
	for s.Scan() {
		line := s.Text()
		if headerRE.MatchString(line) {
			if needSpace {
				out.WriteRune('\n')
			}
			out.WriteString(line)
			out.WriteRune('\n')
			needSpace = false
			inListItem = false
			continue
		}
		if listItemRE.MatchString(line) {
			if needSpace {
				out.WriteRune('\n')
			}
			out.WriteString(line)
			needSpace = true
			inListItem = true
			continue
		}
		if indentRE.MatchString(line) {
			if inListItem {
				if needSpace {
					out.WriteRune(' ')
				}
				out.WriteString(strings.TrimSpace(line))
				needSpace = true
				continue
			}
			out.WriteString(line)
			out.WriteRune('\n')
			needSpace = false
			continue
		}
		if strings.TrimSpace(line) == "" {
			if needSpace {
				out.WriteRune('\n')
			}
			out.WriteRune('\n')
			inListItem = false
			needSpace = false
			continue
		}
		if needSpace {
			if len(line) < shortLineLen && lastLineLength < shortLineLen {
				out.WriteRune('\n')
			} else {
				out.WriteRune(' ')
			}
		}
		out.WriteString(line)
		lastLineLength = len(line)
		needSpace = true
	}
	if needSpace {
		out.WriteRune('\n')
	}
	if s.Err() != nil {
		return text
	}
	return out.String()
}
