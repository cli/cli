package export

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/iostreams"
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
			},
			wantW: "go",
		},
		{
			name: "timefmt",
			args: args{
				json:     strings.NewReader(`{"created_at":"2008-02-25T20:18:33Z"}`),
				template: `{{.created_at | timefmt "Mon Jan 2, 2006"}}`,
			},
			wantW: "Mon Feb 25, 2008",
		},
		{
			name: "timeago",
			args: args{
				json:     strings.NewReader(fmt.Sprintf(`{"created_at":"%s"}`, time.Now().Add(-5*time.Minute).Format(time.RFC3339))),
				template: `{{.created_at | timeago}}`,
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
			},
			wantW: "bug\nfeature request\nchore\n",
		},
		{
			name: "join",
			args: args{
				json:     strings.NewReader(`[ "bug", "feature request", "chore" ]`),
				template: `{{join "\t" .}}`,
			},
			wantW: "bug\tfeature request\tchore",
		},
		{
			name: "table",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": "One"},
					{"number": 20, "title": "Twenty"},
					{"number": 3000, "title": "Three thousand"}
				]`)),
				template: `{{range .}}{{tablerow (.number | printf "#%v") .title}}{{end}}`,
			},
			wantW: heredoc.Doc(`#1     One
			#20    Twenty
			#3000  Three thousand
			`),
		},
		{
			name: "table with multiline text",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": "One\ranother line of text"},
					{"number": 20, "title": "Twenty\nanother line of text"},
					{"number": 3000, "title": "Three thousand\r\nanother line of text"}
				]`)),
				template: `{{range .}}{{tablerow (.number | printf "#%v") .title}}{{end}}`,
			},
			wantW: heredoc.Doc(`#1     One...
			#20    Twenty...
			#3000  Three thousand...
			`),
		},
		{
			name: "table with mixed value types",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": null, "float": false},
					{"number": 20.1, "title": "Twenty-ish", "float": true},
					{"number": 3000, "title": "Three thousand", "float": false}
				]`)),
				template: `{{range .}}{{tablerow .number .title .float}}{{end}}`,
			},
			wantW: heredoc.Doc(`1                      false
			20.10  Twenty-ish      true
			3000   Three thousand  false
			`),
		},
		{
			name: "table with color",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": "One"}
				]`)),
				template: `{{range .}}{{tablerow (.number | color "green") .title}}{{end}}`,
			},
			wantW: "\x1b[0;32m1\x1b[0m  One\n",
		},
		{
			name: "table with header and footer",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": "One"},
					{"number": 2, "title": "Two"}
				]`)),
				template: heredoc.Doc(`HEADER
				{{range .}}{{tablerow .number .title}}{{end}}FOOTER
				`),
			},
			wantW: heredoc.Doc(`HEADER
			FOOTER
			1  One
			2  Two
			`),
		},
		{
			name: "table with header and footer using endtable",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": "One"},
					{"number": 2, "title": "Two"}
				]`)),
				template: heredoc.Doc(`HEADER
				{{range .}}{{tablerow .number .title}}{{end}}{{tablerender}}FOOTER
				`),
			},
			wantW: heredoc.Doc(`HEADER
			1  One
			2  Two
			FOOTER
			`),
		},
		{
			name: "multiple tables with different columns",
			args: args{
				json: strings.NewReader(heredoc.Doc(`{
					"issues": [
						{"number": 1, "title": "One"},
						{"number": 2, "title": "Two"}
					],
					"prs": [
						{"number": 3, "title": "Three", "reviewDecision": "REVIEW_REQUESTED"},
						{"number": 4, "title": "Four", "reviewDecision": "CHANGES_REQUESTED"}
					]
				}`)),
				template: heredoc.Doc(`{{tablerow "ISSUE" "TITLE"}}{{range .issues}}{{tablerow .number .title}}{{end}}{{tablerender}}
				{{tablerow "PR" "TITLE" "DECISION"}}{{range .prs}}{{tablerow .number .title .reviewDecision}}{{end}}`),
			},
			wantW: heredoc.Docf(`ISSUE  TITLE
			1      One
			2      Two

			PR  TITLE  DECISION
			3   Three  REVIEW_REQUESTED
			4   Four   CHANGES_REQUESTED
			`),
		},
		{
			name: "truncate",
			args: args{
				json:     strings.NewReader(`{"title": "This is a long title"}`),
				template: `{{truncate 13 .title}}`,
			},
			wantW: "This is a ...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, w, _ := iostreams.Test()
			io.SetColorEnabled(tt.args.colorize)
			if err := ExecuteTemplate(io, tt.args.json, tt.args.template); (err != nil) != tt.wantErr {
				t.Errorf("executeTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotW := w.String(); gotW != tt.wantW {
				t.Errorf("executeTemplate() = %q, want %q", gotW, tt.wantW)
			}
		})
	}
}
