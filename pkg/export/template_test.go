package export

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/pkg/iostreams"
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
				template: `{{range .}}{{row (.number | printf "#%v") .title}}{{end}}`,
			},
			wantW: "#1\tOne\n#20\tTwenty\n#3000\tThree thousand\n",
		},
		{
			name: "table with mixed value types",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": null},
					{"number": 20.1, "title": "Twenty-ish", "float": true},
					{"number": 3000, "title": "Three thousand", "float": false}
				]`)),
				template: `{{range .}}{{row .number .title .float}}{{end}}`,
			},
			wantW: "1\t\t\n20.10\tTwenty-ish\ttrue\n3000\tThree thousand\tfalse\n",
		},
		{
			name: "table with color",
			args: args{
				json: strings.NewReader(heredoc.Doc(`[
					{"number": 1, "title": "One"}
				]`)),
				template: `{{range .}}{{row (.number | color "green") .title}}{{end}}`,
			},
			wantW: "\x1b[0;32m1\x1b[0m\tOne\n",
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
