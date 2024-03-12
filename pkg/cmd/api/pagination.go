package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/cli/cli/v2/pkg/jsoncolor"
)

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)

func findNextPage(resp *http.Response) (string, bool) {
	for _, m := range linkRE.FindAllStringSubmatch(resp.Header.Get("Link"), -1) {
		if len(m) > 2 && m[2] == "next" {
			return m[1], true
		}
	}
	return "", false
}

func findEndCursor(r io.Reader) string {
	dec := json.NewDecoder(r)

	var idx int
	var stack []json.Delim
	var lastKey string
	var contextKey string

	var endCursor string
	var hasNextPage bool
	var foundEndCursor bool
	var foundNextPage bool

loop:
	for {
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ""
		}

		switch tt := t.(type) {
		case json.Delim:
			switch tt {
			case '{', '[':
				stack = append(stack, tt)
				contextKey = lastKey
				idx = 0
			case '}', ']':
				stack = stack[:len(stack)-1]
				contextKey = ""
				idx = 0
			}
		default:
			isKey := len(stack) > 0 && stack[len(stack)-1] == '{' && idx%2 == 0
			idx++

			switch tt := t.(type) {
			case string:
				if isKey {
					lastKey = tt
				} else if contextKey == "pageInfo" && lastKey == "endCursor" {
					endCursor = tt
					foundEndCursor = true
					if foundNextPage {
						break loop
					}
				}
			case bool:
				if contextKey == "pageInfo" && lastKey == "hasNextPage" {
					hasNextPage = tt
					foundNextPage = true
					if foundEndCursor {
						break loop
					}
				}
			}
		}
	}

	if hasNextPage {
		return endCursor
	}
	return ""
}

func addPerPage(p string, perPage int, params map[string]interface{}) string {
	if _, hasPerPage := params["per_page"]; hasPerPage {
		return p
	}

	idx := strings.IndexRune(p, '?')
	sep := "?"

	if idx >= 0 {
		if qp, err := url.ParseQuery(p[idx+1:]); err == nil && qp.Get("per_page") != "" {
			return p
		}
		sep = "&"
	}

	return fmt.Sprintf("%s%sper_page=%d", p, sep, perPage)
}

// paginatedArrayReader wraps a Reader to omit the opening and/or the closing square bracket of a
// JSON array in order to apply pagination context between multiple API requests.
type paginatedArrayReader struct {
	io.Reader
	isFirstPage bool
	isLastPage  bool

	isSubsequentRead bool
	cachedByte       byte
}

func (r *paginatedArrayReader) Read(p []byte) (int, error) {
	var n int
	var err error
	if r.cachedByte != 0 && len(p) > 0 {
		p[0] = r.cachedByte
		n, err = r.Reader.Read(p[1:])
		n += 1
		r.cachedByte = 0
	} else {
		n, err = r.Reader.Read(p)
	}
	if !r.isSubsequentRead && !r.isFirstPage && n > 0 && p[0] == '[' {
		if n > 1 && p[1] == ']' {
			// empty array case
			p[0] = ' '
		} else {
			// avoid starting a new array and continue with a comma instead
			p[0] = ','
		}
	}
	if !r.isLastPage && n > 0 && p[n-1] == ']' {
		// avoid closing off an array in case we determine we are at EOF
		r.cachedByte = p[n-1]
		n -= 1
	}
	r.isSubsequentRead = true
	return n, err
}

// jsonArrayWriter wraps a Writer which writes multiple pages of both JSON arrays
// and objects. Call Close to write the end of the array.
type jsonArrayWriter struct {
	io.Writer
	started bool
	color   bool
}

func (w *jsonArrayWriter) Preface() []json.Delim {
	if w.started {
		return []json.Delim{'['}
	}
	return nil
}

// ReadFrom implements io.ReaderFrom to write more data than read,
// which otherwise results in an error from io.Copy().
func (w *jsonArrayWriter) ReadFrom(r io.Reader) (int64, error) {
	var written int64
	buf := make([]byte, 4069)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			n, err := w.Write(buf[:n])
			written += int64(n)

			if err != nil {
				return written, err
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return written, err
		}
	}

	return written, nil
}

func (w *jsonArrayWriter) Close() error {
	var delims string
	if w.started {
		delims = "]"
	} else {
		delims = "[]"
	}

	w.started = false
	if w.color {
		return jsoncolor.WriteDelims(w, delims, ttyIndent)
	}

	_, err := w.Writer.Write([]byte(delims))
	return err
}

func startPage(w io.Writer) error {
	if jaw, ok := w.(*jsonArrayWriter); ok {
		var delims string
		var indent bool

		if !jaw.started {
			delims = "["
			jaw.started = true
		} else {
			delims = ","
			indent = true
		}

		if jaw.color {
			if indent {
				_, err := jaw.Write([]byte(ttyIndent))
				if err != nil {
					return err
				}
			}

			return jsoncolor.WriteDelims(w, delims, ttyIndent)
		}

		_, err := jaw.Write([]byte(delims))
		if err != nil {
			return err
		}
	}

	return nil
}
