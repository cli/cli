package cmdutil

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddJSONFlags(t *testing.T) {
	tests := []struct {
		name        string
		fields      []string
		args        []string
		wantsExport *exportFormat
		wantsError  string
	}{
		{
			name:        "no JSON flag",
			fields:      []string{},
			args:        []string{},
			wantsExport: nil,
		},
		{
			name:        "empty JSON flag",
			fields:      []string{"one", "two"},
			args:        []string{"--json"},
			wantsExport: nil,
			wantsError:  "Specify one or more comma-separated fields for `--json`:\n  one\n  two",
		},
		{
			name:        "invalid JSON field",
			fields:      []string{"id", "number"},
			args:        []string{"--json", "idontexist"},
			wantsExport: nil,
			wantsError:  "Unknown JSON field: \"idontexist\"\nAvailable fields:\n  id\n  number",
		},
		{
			name:        "cannot combine --json with --web",
			fields:      []string{"id", "number", "title"},
			args:        []string{"--json", "id", "--web"},
			wantsExport: nil,
			wantsError:  "cannot use `--web` with `--json`",
		},
		{
			name:        "cannot use --jq without --json",
			fields:      []string{},
			args:        []string{"--jq", ".number"},
			wantsExport: nil,
			wantsError:  "cannot use `--jq` without specifying `--json`",
		},
		{
			name:        "cannot use --template without --json",
			fields:      []string{},
			args:        []string{"--template", "{{.number}}"},
			wantsExport: nil,
			wantsError:  "cannot use `--template` without specifying `--json`",
		},
		{
			name:   "with JSON fields",
			fields: []string{"id", "number", "title"},
			args:   []string{"--json", "number,title"},
			wantsExport: &exportFormat{
				fields:   []string{"number", "title"},
				filter:   "",
				template: "",
			},
		},
		{
			name:   "with jq filter",
			fields: []string{"id", "number", "title"},
			args:   []string{"--json", "number", "-q.number"},
			wantsExport: &exportFormat{
				fields:   []string{"number"},
				filter:   ".number",
				template: "",
			},
		},
		{
			name:   "with Go template",
			fields: []string{"id", "number", "title"},
			args:   []string{"--json", "number", "-t", "{{.number}}"},
			wantsExport: &exportFormat{
				fields:   []string{"number"},
				filter:   "",
				template: "{{.number}}",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Run: func(*cobra.Command, []string) {}}
			cmd.Flags().Bool("web", false, "")
			var exporter Exporter
			AddJSONFlags(cmd, &exporter, tt.fields)
			cmd.SetArgs(tt.args)
			cmd.SetOut(ioutil.Discard)
			cmd.SetErr(ioutil.Discard)
			_, err := cmd.ExecuteC()
			if tt.wantsError == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantsError)
				return
			}
			if tt.wantsExport == nil {
				assert.Nil(t, exporter)
			} else {
				assert.Equal(t, tt.wantsExport, exporter)
			}
		})
	}
}

func Test_exportFormat_Write(t *testing.T) {
	type args struct {
		data interface{}
	}
	tests := []struct {
		name     string
		exporter exportFormat
		args     args
		wantW    string
		wantErr  bool
	}{
		{
			name:     "regular JSON output",
			exporter: exportFormat{},
			args: args{
				data: map[string]string{"name": "hubot"},
			},
			wantW:   "{\"name\":\"hubot\"}\n",
			wantErr: false,
		},
		{
			name:     "call ExportData",
			exporter: exportFormat{fields: []string{"field1", "field2"}},
			args: args{
				data: &exportableItem{"item1"},
			},
			wantW:   "{\"field1\":\"item1:field1\",\"field2\":\"item1:field2\"}\n",
			wantErr: false,
		},
		{
			name:     "recursively call ExportData",
			exporter: exportFormat{fields: []string{"f1", "f2"}},
			args: args{
				data: map[string]interface{}{
					"s1": []exportableItem{{"i1"}, {"i2"}},
					"s2": []exportableItem{{"i3"}},
				},
			},
			wantW:   "{\"s1\":[{\"f1\":\"i1:f1\",\"f2\":\"i1:f2\"},{\"f1\":\"i2:f1\",\"f2\":\"i2:f2\"}],\"s2\":[{\"f1\":\"i3:f1\",\"f2\":\"i3:f2\"}]}\n",
			wantErr: false,
		},
		{
			name:     "with jq filter",
			exporter: exportFormat{filter: ".name"},
			args: args{
				data: map[string]string{"name": "hubot"},
			},
			wantW:   "hubot\n",
			wantErr: false,
		},
		{
			name:     "with Go template",
			exporter: exportFormat{template: "{{.name}}"},
			args: args{
				data: map[string]string{"name": "hubot"},
			},
			wantW:   "hubot",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &bytes.Buffer{}
			io := &iostreams.IOStreams{
				Out: w,
			}
			if err := tt.exporter.Write(io, tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("exportFormat.Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotW := w.String(); gotW != tt.wantW {
				t.Errorf("exportFormat.Write() = %q, want %q", gotW, tt.wantW)
			}
		})
	}
}

type exportableItem struct {
	Name string
}

func (e *exportableItem) ExportData(fields []string) *map[string]interface{} {
	m := map[string]interface{}{}
	for _, f := range fields {
		m[f] = fmt.Sprintf("%s:%s", e.Name, f)
	}
	return &m
}
