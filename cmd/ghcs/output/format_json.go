package output

import (
	"encoding/json"
	"io"
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
		row[j.cols[i]] = v
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
