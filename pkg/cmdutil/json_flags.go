package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cli/cli/pkg/export"
	"github.com/cli/cli/pkg/jsoncolor"
	"github.com/cli/cli/pkg/set"
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
	f.StringP("template", "t", "", "Format JSON output using a Go template")

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
		if e.Error() == "flag needs an argument: --json" {
			sort.Strings(fields)
			return JSONFlagError{fmt.Errorf("Specify one or more comma-separated fields for `--json`:\n  %s", strings.Join(fields, "\n  "))}
		}
		return c.Parent().FlagErrorFunc()(c, e)
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
	Write(w io.Writer, data interface{}, colorEnabled bool) error
}

type exportFormat struct {
	fields   []string
	filter   string
	template string
}

func (e *exportFormat) Fields() []string {
	return e.fields
}

func (e *exportFormat) Write(w io.Writer, data interface{}, colorEnabled bool) error {
	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		return err
	}

	if e.filter != "" {
		return export.FilterJSON(w, &buf, e.filter)
	} else if e.template != "" {
		return export.ExecuteTemplate(w, &buf, e.template, colorEnabled)
	} else if colorEnabled {
		return jsoncolor.Write(w, &buf, "  ")
	}

	_, err := io.Copy(w, &buf)
	return err
}
