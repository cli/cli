package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/jsoncolor"
	"github.com/cli/cli/v2/pkg/set"
	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type JSONFlagError struct {
	error
}

func AddJSONFlags(cmd *cobra.Command, exportTarget *Exporter, fields []string) {
	f := cmd.Flags()
	f.StringSlice("json", nil, "Output JSON with the specified `fields`")
	f.StringP("jq", "q", "", "Filter JSON output using a jq `expression`")
	f.StringP("template", "t", "", "Format JSON output using a Go template; see \"gh help formatting\"")

	_ = cmd.RegisterFlagCompletionFunc("json", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var results []string
		var prefix string
		if idx := strings.LastIndexByte(toComplete, ','); idx >= 0 {
			prefix = toComplete[:idx+1]
			toComplete = toComplete[idx+1:]
		}
		toComplete = strings.ToLower(toComplete)
		for _, f := range fields {
			if strings.HasPrefix(strings.ToLower(f), toComplete) {
				results = append(results, prefix+f)
			}
		}
		sort.Strings(results)
		return results, cobra.ShellCompDirectiveNoSpace
	})

	oldPreRun := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if oldPreRun != nil {
			if err := oldPreRun(c, args); err != nil {
				return err
			}
		}
		if export, err := checkJSONFlags(c); err == nil {
			if export == nil {
				*exportTarget = nil
			} else {
				allowedFields := set.NewStringSet()
				allowedFields.AddValues(fields)
				for _, f := range export.fields {
					if !allowedFields.Contains(f) {
						sort.Strings(fields)
						return JSONFlagError{fmt.Errorf("Unknown JSON field: %q\nAvailable fields:\n  %s", f, strings.Join(fields, "\n  "))}
					}
				}
				*exportTarget = export
			}
		} else {
			return err
		}
		return nil
	}

	cmd.SetFlagErrorFunc(func(c *cobra.Command, e error) error {
		if c == cmd && e.Error() == "flag needs an argument: --json" {
			sort.Strings(fields)
			return JSONFlagError{fmt.Errorf("Specify one or more comma-separated fields for `--json`:\n  %s", strings.Join(fields, "\n  "))}
		}
		if cmd.HasParent() {
			return cmd.Parent().FlagErrorFunc()(c, e)
		}
		return e
	})

	if len(fields) == 0 {
		return
	}

	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["help:json-fields"] = strings.Join(fields, ",")
}

func checkJSONFlags(cmd *cobra.Command) (*jsonExporter, error) {
	f := cmd.Flags()
	jsonFlag := f.Lookup("json")
	jqFlag := f.Lookup("jq")
	tplFlag := f.Lookup("template")
	webFlag := f.Lookup("web")

	if jsonFlag.Changed {
		if webFlag != nil && webFlag.Changed {
			return nil, errors.New("cannot use `--web` with `--json`")
		}
		jv := jsonFlag.Value.(pflag.SliceValue)
		return &jsonExporter{
			fields:   jv.GetSlice(),
			filter:   jqFlag.Value.String(),
			template: tplFlag.Value.String(),
		}, nil
	} else if jqFlag.Changed {
		return nil, errors.New("cannot use `--jq` without specifying `--json`")
	} else if tplFlag.Changed {
		return nil, errors.New("cannot use `--template` without specifying `--json`")
	}
	return nil, nil
}

func AddFormatFlags(cmd *cobra.Command, exportTarget *Exporter) {
	var format string
	StringEnumFlag(cmd, &format, "format", "", "", []string{"json"}, "Output format")
	f := cmd.Flags()
	f.StringP("jq", "q", "", "Filter JSON output using a jq `expression`")
	f.StringP("template", "t", "", "Format JSON output using a Go template; see \"gh help formatting\"")

	oldPreRun := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if oldPreRun != nil {
			if err := oldPreRun(c, args); err != nil {
				return err
			}
		}

		if export, err := checkFormatFlags(c); err == nil {
			if export == nil {
				*exportTarget = nil
			} else {
				*exportTarget = export
			}
		} else {
			return err
		}
		return nil
	}
}

func checkFormatFlags(cmd *cobra.Command) (*jsonExporter, error) {
	f := cmd.Flags()
	formatFlag := f.Lookup("format")
	formatValue := formatFlag.Value.String()
	jqFlag := f.Lookup("jq")
	tplFlag := f.Lookup("template")
	webFlag := f.Lookup("web")

	if formatFlag.Changed {
		if webFlag != nil && webFlag.Changed {
			return nil, errors.New("cannot use `--web` with `--format`")
		}
		return &jsonExporter{
			filter:   jqFlag.Value.String(),
			template: tplFlag.Value.String(),
		}, nil
	} else if jqFlag.Changed && formatValue != "json" {
		return nil, errors.New("cannot use `--jq` without specifying `--format json`")
	} else if tplFlag.Changed && formatValue != "json" {
		return nil, errors.New("cannot use `--template` without specifying `--format json`")
	}
	return nil, nil
}

type Exporter interface {
	Fields() []string
	Write(io *iostreams.IOStreams, data interface{}) error
}

type jsonExporter struct {
	fields   []string
	filter   string
	template string
}

// NewJSONExporter returns an Exporter to emit JSON.
func NewJSONExporter() *jsonExporter {
	return &jsonExporter{}
}

func (e *jsonExporter) Fields() []string {
	return e.fields
}

func (e *jsonExporter) SetFields(fields []string) {
	e.fields = fields
}

// Write serializes data into JSON output written to w. If the object passed as data implements exportable,
// or if data is a map or slice of exportable object, ExportData() will be called on each object to obtain
// raw data for serialization.
func (e *jsonExporter) Write(ios *iostreams.IOStreams, data interface{}) error {
	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(e.exportData(reflect.ValueOf(data))); err != nil {
		return err
	}

	w := ios.Out
	if e.filter != "" {
		indent := ""
		if ios.IsStdoutTTY() {
			indent = "  "
		}
		if err := jq.EvaluateFormatted(&buf, w, e.filter, indent, ios.ColorEnabled()); err != nil {
			return err
		}
	} else if e.template != "" {
		t := template.New(w, ios.TerminalWidth(), ios.ColorEnabled())
		if err := t.Parse(e.template); err != nil {
			return err
		}
		if err := t.Execute(&buf); err != nil {
			return err
		}
		return t.Flush()
	} else if ios.ColorEnabled() {
		return jsoncolor.Write(w, &buf, "  ")
	}

	_, err := io.Copy(w, &buf)
	return err
}

func (e *jsonExporter) exportData(v reflect.Value) interface{} {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if !v.IsNil() {
			return e.exportData(v.Elem())
		}
	case reflect.Slice:
		a := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			a[i] = e.exportData(v.Index(i))
		}
		return a
	case reflect.Map:
		t := reflect.MapOf(v.Type().Key(), emptyInterfaceType)
		m := reflect.MakeMapWithSize(t, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			ve := reflect.ValueOf(e.exportData(iter.Value()))
			m.SetMapIndex(iter.Key(), ve)
		}
		return m.Interface()
	case reflect.Struct:
		if v.CanAddr() && reflect.PtrTo(v.Type()).Implements(exportableType) {
			ve := v.Addr().Interface().(exportable)
			return ve.ExportData(e.fields)
		} else if v.Type().Implements(exportableType) {
			ve := v.Interface().(exportable)
			return ve.ExportData(e.fields)
		}
	}
	return v.Interface()
}

type exportable interface {
	ExportData([]string) map[string]interface{}
}

var exportableType = reflect.TypeOf((*exportable)(nil)).Elem()
var sliceOfEmptyInterface []interface{}
var emptyInterfaceType = reflect.TypeOf(sliceOfEmptyInterface).Elem()

// Basic function that can be used with structs that need to implement
// the exportable interface. It has numerous limitations so verify
// that it works as expected with the struct and fields you want to export.
// If it does not, then implementing a custom ExportData method is necessary.
// Perhaps this should be moved up into exportData for the case when
// a struct does not implement the exportable interface, but for now it will
// need to be explicitly used.
func StructExportData(s interface{}, fields []string) map[string]interface{} {
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		// If s is not a struct or pointer to a struct return nil.
		return nil
	}
	data := make(map[string]interface{}, len(fields))
	for _, f := range fields {
		sf := fieldByName(v, f)
		if sf.IsValid() && sf.CanInterface() {
			data[f] = sf.Interface()
		}
	}
	return data
}

func fieldByName(v reflect.Value, field string) reflect.Value {
	return v.FieldByNameFunc(func(s string) bool {
		return strings.EqualFold(field, s)
	})
}
