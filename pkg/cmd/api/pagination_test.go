package api

import (
	"bytes"
	"io"
	"net/http"
	"testing"
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
