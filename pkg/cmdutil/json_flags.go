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
}

func checkJSONFlags(cmd *cobra.Command) (*exportFormat, error) {
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
		return &exportFormat{
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

type Exporter interface {
	Fields() []string
	Write(io *iostreams.IOStreams, data interface{}) error
}

type exportFormat struct {
	fields   []string
	filter   string
	template string
}

func (e *exportFormat) Fields() []string {
	return e.fields
}

// Write serializes data into JSON output written to w. If the object passed as data implements exportable,
// or if data is a map or slice of exportable object, ExportData() will be called on each object to obtain
// raw data for serialization.
func (e *exportFormat) Write(ios *iostreams.IOStreams, data interface{}) error {
	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(e.exportData(reflect.ValueOf(data))); err != nil {
		return err
	}

	w := ios.Out
	if e.filter != "" {
		return jq.Evaluate(&buf, w, e.filter)
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

func (e *exportFormat) exportData(v reflect.Value) interface{} {
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
