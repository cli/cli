package api

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_findNextPage(t *testing.T) {
	tests := []struct {
		name  string
		resp  *http.Response
		want  string
		want1 bool
	}{
		{
			name:  "no Link header",
			resp:  &http.Response{},
			want:  "",
			want1: false,
		},
		{
			name: "no next page in Link",
			resp: &http.Response{
				Header: http.Header{
					"Link": []string{`<https://api.github.com/issues?page=3>; rel="last"`},
				},
			},
			want:  "",
			want1: false,
		},
		{
			name: "has next page",
			resp: &http.Response{
				Header: http.Header{
					"Link": []string{`<https://api.github.com/issues?page=2>; rel="next", <https://api.github.com/issues?page=3>; rel="last"`},
				},
			},
			want:  "https://api.github.com/issues?page=2",
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := findNextPage(tt.resp)
			if got != tt.want {
				t.Errorf("findNextPage() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("findNextPage() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_findEndCursor(t *testing.T) {
	tests := []struct {
		name string
		json io.Reader
		want string
	}{
		{
			name: "blank",
			json: bytes.NewBufferString(`{}`),
			want: "",
		},
		{
			name: "unrelated fields",
			json: bytes.NewBufferString(`{
				"hasNextPage": true,
				"endCursor": "THE_END"
			}`),
			want: "",
		},
		{
			name: "has next page",
			json: bytes.NewBufferString(`{
				"pageInfo": {
					"hasNextPage": true,
					"endCursor": "THE_END"
				}
			}`),
			want: "THE_END",
		},
		{
			name: "more pageInfo blocks",
			json: bytes.NewBufferString(`{
				"pageInfo": {
					"hasNextPage": true,
					"endCursor": "THE_END"
				},
				"pageInfo": {
					"hasNextPage": true,
					"endCursor": "NOT_THIS"
				}
			}`),
			want: "THE_END",
		},
		{
			name: "no next page",
			json: bytes.NewBufferString(`{
				"pageInfo": {
					"hasNextPage": false,
					"endCursor": "THE_END"
				}
			}`),
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findEndCursor(tt.json); got != tt.want {
				t.Errorf("findEndCursor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_addPerPage(t *testing.T) {
	type args struct {
		p       string
		perPage int
		params  map[string]interface{}
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "adds per_page",
			args: args{
				p:       "items",
				perPage: 13,
				params:  nil,
			},
			want: "items?per_page=13",
		},
		{
			name: "avoids adding per_page if already in params",
			args: args{
				p:       "items",
				perPage: 13,
				params: map[string]interface{}{
					"state":    "open",
					"per_page": 99,
				},
			},
			want: "items",
		},
		{
			name: "avoids adding per_page if already in query",
			args: args{
				p:       "items?per_page=6&state=open",
				perPage: 13,
				params:  nil,
			},
			want: "items?per_page=6&state=open",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := addPerPage(tt.args.p, tt.args.perPage, tt.args.params); got != tt.want {
				t.Errorf("addPerPage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJsonArrayWriter(t *testing.T) {
	tests := []struct {
		name  string
		pages []string
		want  string
	}{
		{
			name:  "empty",
			pages: nil,
			want:  "[]",
		},
		{
			name:  "single array",
			pages: []string{`[1,2]`},
			want:  `[[1,2]]`,
		},
		{
			name:  "multiple arrays",
			pages: []string{`[1,2]`, `[3]`},
			want:  `[[1,2],[3]]`,
		},
		{
			name:  "single object",
			pages: []string{`{"foo":1,"bar":"a"}`},
			want:  `[{"foo":1,"bar":"a"}]`,
		},
		{
			name:  "multiple pages",
			pages: []string{`{"foo":1,"bar":"a"}`, `{"foo":2,"bar":"b"}`},
			want:  `[{"foo":1,"bar":"a"},{"foo":2,"bar":"b"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			w := &jsonArrayWriter{
				Writer: buf,
			}

			for _, page := range tt.pages {
				require.NoError(t, startPage(w))

				n, err := w.Write([]byte(page))
				require.NoError(t, err)
				assert.Equal(t, len(page), n)
			}

			require.NoError(t, w.Close())
			assert.Equal(t, tt.want, buf.String())
		})
	}
}

func TestJsonArrayWriter_Copy(t *testing.T) {
	tests := []struct {
		name  string
		limit int
	}{
		{
			name: "unlimited",
		},
		{
			name:  "limited",
			limit: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			w := &jsonArrayWriter{
				Writer: buf,
			}

			r := &noWriteToReader{
				Reader: bytes.NewBufferString(`[1,2]`),
				limit:  tt.limit,
			}

			require.NoError(t, startPage(w))

			n, err := io.Copy(w, r)
			require.NoError(t, err)
			assert.Equal(t, int64(5), n)

			require.NoError(t, w.Close())
			assert.Equal(t, `[[1,2]]`, buf.String())
		})
	}
}

type noWriteToReader struct {
	io.Reader
	limit int
}

func (r *noWriteToReader) Read(p []byte) (int, error) {
	if r.limit > 0 {
		p = p[:r.limit]
	}
	return r.Reader.Read(p)
}
