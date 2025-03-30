package cmdutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddJSONFlags(t *testing.T) {
	tests := []struct {
		name        string
		fields      []string
		args        []string
		wantsExport *jsonExporter
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
			wantsExport: &jsonExporter{
				fields:   []string{"number", "title"},
				filter:   "",
				template: "",
			},
		},
		{
			name:   "with jq filter",
			fields: []string{"id", "number", "title"},
			args:   []string{"--json", "number", "-q.number"},
			wantsExport: &jsonExporter{
				fields:   []string{"number"},
				filter:   ".number",
				template: "",
			},
		},
		{
			name:   "with Go template",
			fields: []string{"id", "number", "title"},
			args:   []string{"--json", "number", "-t", "{{.number}}"},
			wantsExport: &jsonExporter{
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
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
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

// TestAddJSONFlagsSetsAnnotations asserts that `AddJSONFlags` function adds the
// appropriate annotation to the command, which could later be used by doc
// generator functions.
func TestAddJSONFlagsSetsAnnotations(t *testing.T) {
	tests := []struct {
		name                string
		cmd                 *cobra.Command
		jsonFields          []string
		expectedAnnotations map[string]string
	}{
		{
			name:                "empty set of fields",
			cmd:                 &cobra.Command{},
			jsonFields:          []string{},
			expectedAnnotations: nil,
		},
		{
			name:                "empty set of fields, with existing annotations",
			cmd:                 &cobra.Command{Annotations: map[string]string{"foo": "bar"}},
			jsonFields:          []string{},
			expectedAnnotations: map[string]string{"foo": "bar"},
		},
		{
			name:       "no other annotations",
			cmd:        &cobra.Command{},
			jsonFields: []string{"few", "json", "fields"},
			expectedAnnotations: map[string]string{
				"help:json-fields": "few,json,fields",
			},
		},
		{
			name:       "with existing annotations (ensure no overwrite)",
			cmd:        &cobra.Command{Annotations: map[string]string{"foo": "bar"}},
			jsonFields: []string{"few", "json", "fields"},
			expectedAnnotations: map[string]string{
				"foo":              "bar",
				"help:json-fields": "few,json,fields",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AddJSONFlags(tt.cmd, nil, tt.jsonFields)
			assert.Equal(t, tt.expectedAnnotations, tt.cmd.Annotations)
		})
	}
}

func TestAddFormatFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantsExport *jsonExporter
		wantsError  string
	}{
		{
			name:        "no format flag",
			args:        []string{},
			wantsExport: nil,
		},
		{
			name:        "empty format flag",
			args:        []string{"--format"},
			wantsExport: nil,
			wantsError:  "flag needs an argument: --format",
		},
		{
			name:        "invalid format field",
			args:        []string{"--format", "idontexist"},
			wantsExport: nil,
			wantsError:  "invalid argument \"idontexist\" for \"--format\" flag: valid values are {json}",
		},
		{
			name:        "cannot combine --format with --web",
			args:        []string{"--format", "json", "--web"},
			wantsExport: nil,
			wantsError:  "cannot use `--web` with `--format`",
		},
		{
			name:        "cannot use --jq without --format",
			args:        []string{"--jq", ".number"},
			wantsExport: nil,
			wantsError:  "cannot use `--jq` without specifying `--format json`",
		},
		{
			name:        "cannot use --template without --format",
			args:        []string{"--template", "{{.number}}"},
			wantsExport: nil,
			wantsError:  "cannot use `--template` without specifying `--format json`",
		},
		{
			name: "with json format",
			args: []string{"--format", "json"},
			wantsExport: &jsonExporter{
				filter:   "",
				template: "",
			},
		},
		{
			name: "with jq filter",
			args: []string{"--format", "json", "-q.number"},
			wantsExport: &jsonExporter{
				filter:   ".number",
				template: "",
			},
		},
		{
			name: "with Go template",
			args: []string{"--format", "json", "-t", "{{.number}}"},
			wantsExport: &jsonExporter{
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
			AddFormatFlags(cmd, &exporter)
			cmd.SetArgs(tt.args)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
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
		exporter jsonExporter
		args     args
		wantW    string
		wantErr  bool
		istty    bool
	}{
		{
			name:     "regular JSON output",
			exporter: jsonExporter{},
			args: args{
				data: map[string]string{"name": "hubot"},
			},
			wantW:   "{\"name\":\"hubot\"}\n",
			wantErr: false,
			istty:   false,
		},
		{
			name:     "call ExportData",
			exporter: jsonExporter{fields: []string{"field1", "field2"}},
			args: args{
				data: &exportableItem{"item1"},
			},
			wantW:   "{\"field1\":\"item1:field1\",\"field2\":\"item1:field2\"}\n",
			wantErr: false,
			istty:   false,
		},
		{
			name:     "recursively call ExportData",
			exporter: jsonExporter{fields: []string{"f1", "f2"}},
			args: args{
				data: map[string]interface{}{
					"s1": []exportableItem{{"i1"}, {"i2"}},
					"s2": []exportableItem{{"i3"}},
				},
			},
			wantW:   "{\"s1\":[{\"f1\":\"i1:f1\",\"f2\":\"i1:f2\"},{\"f1\":\"i2:f1\",\"f2\":\"i2:f2\"}],\"s2\":[{\"f1\":\"i3:f1\",\"f2\":\"i3:f2\"}]}\n",
			wantErr: false,
			istty:   false,
		},
		{
			name:     "with jq filter",
			exporter: jsonExporter{filter: ".name"},
			args: args{
				data: map[string]string{"name": "hubot"},
			},
			wantW:   "hubot\n",
			wantErr: false,
			istty:   false,
		},
		{
			name:     "with jq filter pretty printing",
			exporter: jsonExporter{filter: "."},
			args: args{
				data: map[string]string{"name": "hubot"},
			},
			wantW:   "{\n  \"name\": \"hubot\"\n}\n",
			wantErr: false,
			istty:   true,
		},
		{
			name:     "with Go template",
			exporter: jsonExporter{template: "{{.name}}"},
			args: args{
				data: map[string]string{"name": "hubot"},
			},
			wantW:   "hubot",
			wantErr: false,
			istty:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, w, _ := iostreams.Test()
			io.SetStdoutTTY(tt.istty)
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

func (e *exportableItem) ExportData(fields []string) map[string]interface{} {
	m := map[string]interface{}{}
	for _, f := range fields {
		m[f] = fmt.Sprintf("%s:%s", e.Name, f)
	}
	return m
}

func TestStructExportData(t *testing.T) {
	tf, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	type s struct {
		StringField string
		IntField    int
		BoolField   bool
		TimeField   time.Time
		SliceField  []int
		MapField    map[string]int
		StructField struct {
			A string
			B int
			c bool
		}
		unexportedField int
	}
	export := s{
		StringField: "test",
		IntField:    1,
		BoolField:   true,
		TimeField:   tf,
		SliceField:  []int{1, 2, 3},
		MapField: map[string]int{
			"one":   1,
			"two":   2,
			"three": 3,
		},
		StructField: struct {
			A string
			B int
			c bool
		}{
			A: "a",
			B: 1,
			c: true,
		},
		unexportedField: 4,
	}
	fields := []string{"stringField", "intField", "boolField", "sliceField", "mapField", "structField"}
	tests := []struct {
		name    string
		export  interface{}
		fields  []string
		wantOut string
	}{
		{
			name:    "serializes struct types",
			export:  export,
			fields:  fields,
			wantOut: `{"boolField":true,"intField":1,"mapField":{"one":1,"three":3,"two":2},"sliceField":[1,2,3],"stringField":"test","structField":{"A":"a","B":1}}`,
		},
		{
			name:    "serializes pointer to struct types",
			export:  &export,
			fields:  fields,
			wantOut: `{"boolField":true,"intField":1,"mapField":{"one":1,"three":3,"two":2},"sliceField":[1,2,3],"stringField":"test","structField":{"A":"a","B":1}}`,
		},
		{
			name: "does not serialize non-struct types",
			export: map[string]string{
				"test": "test",
			},
			fields:  nil,
			wantOut: `null`,
		},
		{
			name:    "ignores unknown fields",
			export:  export,
			fields:  []string{"stringField", "unknownField"},
			wantOut: `{"stringField":"test"}`,
		},
		{
			name:    "ignores unexported fields",
			export:  export,
			fields:  []string{"stringField", "unexportedField"},
			wantOut: `{"stringField":"test"}`,
		},
		{
			name:    "field matching is case insensitive but casing impacts JSON output",
			export:  export,
			fields:  []string{"STRINGfield"},
			wantOut: `{"STRINGfield":"test"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.Buffer{}
			encoder := json.NewEncoder(&buf)
			encoder.SetEscapeHTML(false)
			out := StructExportData(tt.export, tt.fields)
			require.NoError(t, encoder.Encode(out))
			require.JSONEq(t, tt.wantOut, buf.String())
		})
	}
}
