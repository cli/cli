package api

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
)

func Test_filterJSON(t *testing.T) {
	type args struct {
		json  io.Reader
		query string
	}
	tests := []struct {
		name    string
		args    args
		wantW   string
		wantErr bool
	}{
		{
			name: "simple",
			args: args{
				json:  strings.NewReader(`{"name":"Mona", "arms":8}`),
				query: `.name`,
			},
			wantW: "Mona\n",
		},
		{
			name: "multiple queries",
			args: args{
				json:  strings.NewReader(`{"name":"Mona", "arms":8}`),
				query: `.name,.arms`,
			},
			wantW: "Mona\n8\n",
		},
		{
			name: "object as JSON",
			args: args{
				json:  strings.NewReader(`{"user":{"login":"monalisa"}}`),
				query: `.user`,
			},
			wantW: "{\"login\":\"monalisa\"}\n",
		},
		{
			name: "complex",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{
						"title": "First title",
						"labels": [{"name":"bug"}, {"name":"help wanted"}]
					},
					{
						"title": "Second but not last",
						"labels": []
					},
					{
						"title": "Alas, tis' the end",
						"labels": [{}, {"name":"feature"}]
					}
				]`)),
				query: `.[] | [.title,(.labels | map(.name) | join(","))] | @tsv`,
			},
			wantW: heredoc.Doc(`
				First title	bug,help wanted
				Second but not last	
				Alas, tis' the end	,feature
			`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &bytes.Buffer{}
			if err := filterJSON(w, tt.args.json, tt.args.query); (err != nil) != tt.wantErr {
				t.Errorf("filterJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotW := w.String(); gotW != tt.wantW {
				t.Errorf("filterJSON() = %q, want %q", gotW, tt.wantW)
			}
		})
	}
}
