package output

import (
	"encoding/json"
	"io"
	"strings"
	"unicode"
)

type jsonwriter struct {
	w      io.Writer
	pretty bool
	cols   []string
	data   []interface{}
}

func (j *jsonwriter) SetHeader(cols []string) {
	j.cols = cols
}

func (j *jsonwriter) Append(values []string) {
	row := make(map[string]string)
	for i, v := range values {
		row[camelize(j.cols[i])] = v
	}
	j.data = append(j.data, row)
}

func (j *jsonwriter) Render() {
	enc := json.NewEncoder(j.w)
	if j.pretty {
		enc.SetIndent("", "  ")
	}
	_ = enc.Encode(j.data)
}

func camelize(s string) string {
	var b strings.Builder
	capitalizeNext := false
	for i, r := range s {
		if r == ' ' {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			b.WriteRune(unicode.ToUpper(r))
			capitalizeNext = false
		} else if i == 0 {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
