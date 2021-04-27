package export

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
)

func Test_jsonScalarToString(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    string
		wantErr bool
	}{
		{
			name:  "string",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "int",
			input: float64(1234),
			want:  "1234",
		},
		{
			name:  "float",
			input: float64(12.34),
			want:  "12.34",
		},
		{
			name:  "null",
			input: nil,
			want:  "",
		},
		{
			name:  "true",
			input: true,
			want:  "true",
		},
		{
			name:  "false",
			input: false,
			want:  "false",
		},
		{
			name:    "object",
			input:   map[string]interface{}{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jsonScalarToString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("jsonScalarToString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("jsonScalarToString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_executeTemplate(t *testing.T) {
	type args struct {
		json     io.Reader
		template string
		colorize bool
	}
	tests := []struct {
		name    string
		args    args
		wantW   string
		wantErr bool
	}{
		{
			name: "color",
			args: args{
				json:     strings.NewReader(`{}`),
				template: `{{color "blue+h" "songs are like tattoos"}}`,
				colorize: false,
			},
			wantW: "\x1b[0;94msongs are like tattoos\x1b[0m",
		},
		{
			name: "autocolor enabled",
			args: args{
				json:     strings.NewReader(`{}`),
				template: `{{autocolor "red" "stop"}}`,
				colorize: true,
			},
			wantW: "\x1b[0;31mstop\x1b[0m",
		},
		{
			name: "autocolor disabled",
			args: args{
				json:     strings.NewReader(`{}`),
				template: `{{autocolor "red" "go"}}`,
				colorize: false,
			},
			wantW: "go",
		},
		{
			name: "timefmt",
			args: args{
				json:     strings.NewReader(`{"created_at":"2008-02-25T20:18:33Z"}`),
				template: `{{.created_at | timefmt "Mon Jan 2, 2006"}}`,
				colorize: false,
			},
			wantW: "Mon Feb 25, 2008",
		},
		{
			name: "timeago",
			args: args{
				json:     strings.NewReader(fmt.Sprintf(`{"created_at":"%s"}`, time.Now().Add(-5*time.Minute).Format(time.RFC3339))),
				template: `{{.created_at | timeago}}`,
				colorize: false,
			},
			wantW: "5 minutes ago",
		},
		{
			name: "pluck",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"name": "bug"},
					{"name": "feature request"},
					{"name": "chore"}
				]`)),
				template: `{{range(pluck "name" .)}}{{. | printf "%s\n"}}{{end}}`,
				colorize: false,
			},
			wantW: "bug\nfeature request\nchore\n",
		},
		{
			name: "join",
			args: args{
				json:     strings.NewReader(`[ "bug", "feature request", "chore" ]`),
				template: `{{join "\t" .}}`,
				colorize: false,
			},
			wantW: "bug\tfeature request\tchore",
		},
		{
			name: "truncate",
			args: args{
				json:     strings.NewReader(`[ "bug", "feature request", "chore" ]`),
				template: `{{range .}}{{. | truncate 5 | printf "%s\n"}}{{end}}`,
				colorize: false,
			},
			wantW: "bug\nfe...\nchore\n",
		},
		{
			name: "table",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": "One"},
					{"number": 20, "title": "Twenty"},
					{"number": 3000, "title": "Three thousand"}
				]`)),
				template: `{{table}}{{range .}}{{row (.number | printf "#%v") .title}}{{end}}{{endTable}}`,
				colorize: false,
			},
			wantW: "#1    One\n#20   Twenty\n#3000 Three thousand\n",
		},
		{
			name: "table args with int padchar",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": "One"},
					{"number": 20, "title": "Twenty"},
					{"number": 3000, "title": "Three thousand"}
				]`)),
				template: `{{table 8 2 2 '.'}}{{range .}}{{row (.number | printf "#%v") .title}}{{end}}{{endTable}}`,
				colorize: false,
			},
			wantW: "#1......One\n#20.....Twenty\n#3000...Three thousand\n",
		},
		{
			name: "table args with string padchar",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": "One"},
					{"number": 20, "title": "Twenty"},
					{"number": 3000, "title": "Three thousand"}
				]`)),
				template: `{{table 8 2 2 "."}}{{range .}}{{row (.number | printf "#%v") .title}}{{end}}{{endTable}}`,
				colorize: false,
			},
			wantW: "#1......One\n#20.....Twenty\n#3000...Three thousand\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &bytes.Buffer{}
			if err := ExecuteTemplate(w, tt.args.json, tt.args.template, tt.args.colorize); (err != nil) != tt.wantErr {
				t.Errorf("executeTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotW := w.String(); gotW != tt.wantW {
				t.Errorf("executeTemplate() = %q, want %q", gotW, tt.wantW)
			}
		})
	}
}
