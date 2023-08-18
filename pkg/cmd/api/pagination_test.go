package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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
		json string
		want string
	}{
		{
			name: "blank",
			json: `{}`,
			want: "",
		},
		{
			name: "unrelated fields",
			json: `{
				"hasNextPage": true,
				"endCursor": "THE_END"
			}`,
			want: "",
		},
		{
			name: "has next page",
			json: `{
				"pageInfo": {
					"hasNextPage": true,
					"endCursor": "THE_END"
				}
			}`,
			want: "THE_END",
		},
		{
			name: "more pageInfo blocks",
			json: `{
				"pageInfo": {
					"hasNextPage": true,
					"endCursor": "THE_END"
				},
				"pageInfo": {
					"hasNextPage": true,
					"endCursor": "NOT_THIS"
				}
			}`,
			want: "THE_END",
		},
		{
			name: "no next page",
			json: `{
				"pageInfo": {
					"hasNextPage": false,
					"endCursor": "THE_END"
				}
			}`,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			json := bytes.NewReader([]byte(tt.json))
			if got := findEndCursor(json); got != tt.want {
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

func Test_mergeJSON_object(t *testing.T) {
	page1 := `{
		"data": {
			"repository": {
				"labels": {
					"nodes": [
						{
							"name": "bug",
							"description": "Something isn't working"
						},
						{
							"name": "tracking issue",
							"description": ""
						}
					],
					"pageInfo": {
						"hasNextPage": true,
						"endCursor": "Y3Vyc29yOnYyOpK5MjAxOS0xMC0xMFQxMTozODowMy0wNjowMM5f3HZq"
					}
				}
			}
		}
	}`

	page2 := `{
		"data": {
			"repository": {
				"labels": {
					"nodes": [
						{
							"name": "blocked",
							"description": ""
						},
						{
							"name": "needs-design",
							"description": "An engineering task needs design to proceed"
						}
					],
					"pageInfo": {
						"hasNextPage": false,
						"endCursor": "Y3Vyc29yOnYyOpK5MjAxOS0xMS0xOFQxMDowMzoxNi0wNzowMM5kXbLp"
					}
				}
			}
		}
	}`

	var data interface{}
	for _, page := range []string{page1, page2} {
		err := mergeJSON(&data, bytes.NewReader([]byte(page)))
		if !assert.NoError(t, err) {
			return
		}
	}

	actual, err := json.Marshal(data)
	if !assert.NoError(t, err) {
		return
	}

	expected := `{
		"data": {
			"repository": {
				"labels": {
					"nodes": [
						{
							"name": "bug",
							"description": "Something isn't working"
						},
						{
							"name": "tracking issue",
							"description": ""
						},
						{
							"name": "blocked",
							"description": ""
						},
						{
							"name": "needs-design",
							"description": "An engineering task needs design to proceed"
						}
					],
					"pageInfo": {
						"hasNextPage": false,
						"endCursor": "Y3Vyc29yOnYyOpK5MjAxOS0xMS0xOFQxMDowMzoxNi0wNzowMM5kXbLp"
					}
				}
			}
		}
	}`

	assert.JSONEq(t, expected, string(actual))
}

func Test_mergeJSON_array(t *testing.T) {
	page1 := `[
		{
			"name": "bug",
			"description": "Something isn't working"
		},
		{
			"name": "tracking issue",
			"description": ""
		}
	]`

	page2 := `[
		{
			"name": "blocked",
			"description": ""
		},
		{
			"name": "needs-design",
			"description": "An engineering task needs design to proceed"
		}
	]`

	var data interface{}
	for _, page := range []string{page1, page2} {
		err := mergeJSON(&data, bytes.NewReader([]byte(page)))
		if !assert.NoError(t, err) {
			return
		}
	}

	actual, err := json.Marshal(data)
	if !assert.NoError(t, err) {
		return
	}

	expected := `[
		{
			"name": "bug",
			"description": "Something isn't working"
		},
		{
			"name": "tracking issue",
			"description": ""
		},
		{
			"name": "blocked",
			"description": ""
		},
		{
			"name": "needs-design",
			"description": "An engineering task needs design to proceed"
		}
	]`

	assert.JSONEq(t, expected, string(actual))
}
