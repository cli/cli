package template

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	color "github.com/mgutz/ansi"
)

const ellipsis = "..."

type Template struct {
	colorEnabled bool
	output       io.Writer
	tmpl         *template.Template
	tp           *tableprinter.TablePrinter
	width        int
	funcs        template.FuncMap
}

func New(output io.Writer, width int, colorEnabled bool) *Template {
	return &Template{
		colorEnabled: colorEnabled,
		output:       output,
		tp:           tableprinter.NewWithWriter(output, true, width, nil, tableprinter.NoHeader),
		width:        width,
		funcs:        template.FuncMap{},
	}
}

func (t *Template) Funcs(funcMap map[string]interface{}) *Template {
	for name, fn := range funcMap {
		t.funcs[name] = fn
	}
	return t
}

func (t *Template) Parse(source string) error {
	now := time.Now()
	templateFuncs := template.FuncMap{
		"autocolor": colorFunc,
		"color":     colorFunc,
		"hyperlink": hyperlinkFunc,
		"join":      joinFunc,
		"pluck":     pluckFunc,
		"tablerender": func() (string, error) {
			defer func() {
				t.tp = tableprinter.NewWithWriter(t.output, true, t.width, nil, tableprinter.NoHeader)
			}()
			return tableRenderFunc(t.tp)
		},
		"tablerow": func(fields ...interface{}) (string, error) {
			return tableRowFunc(t.tp, fields...)
		},
		"timeago": func(input string) (string, error) {
			timeValue, err := time.Parse(time.RFC3339, input)
			if err != nil {
				return "", err
			}
			return timeAgo(now.Sub(timeValue)), nil
		},
		"timefmt":  timeFormatFunc,
		"truncate": truncateFunc,
	}
	if !t.colorEnabled {
		templateFuncs["autocolor"] = autoColorFunc
	}
	for name, fn := range sprigFuncMap() {
		templateFuncs[name] = fn
	}
	for name, fn := range t.funcs {
		templateFuncs[name] = fn
	}
	parsed, err := template.New("").Funcs(templateFuncs).Parse(source)
	if err != nil {
		return err
	}
	t.tmpl = parsed
	return nil
}

func (t *Template) Execute(input io.Reader) error {
	jsonData, err := io.ReadAll(input)
	if err != nil {
		return err
	}
	var data interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return err
	}
	return t.tmpl.Execute(t.output, data)
}

func (t *Template) Flush() error {
	_, err := tableRenderFunc(t.tp)
	return err
}

func colorFunc(colorName string, input interface{}) (string, error) {
	value, err := jsonScalarToString(input)
	if err != nil {
		return "", err
	}
	return color.Color(value, colorName), nil
}

func pluckFunc(field string, input []interface{}) []interface{} {
	var results []interface{}
	for _, item := range input {
		object := item.(map[string]interface{})
		results = append(results, object[field])
	}
	return results
}

func joinFunc(separator string, input []interface{}) (string, error) {
	var results []string
	for _, item := range input {
		value, err := jsonScalarToString(item)
		if err != nil {
			return "", err
		}
		results = append(results, value)
	}
	return strings.Join(results, separator), nil
}

func timeFormatFunc(format, input string) (string, error) {
	value, err := time.Parse(time.RFC3339, input)
	if err != nil {
		return "", err
	}
	return value.Format(format), nil
}

func truncateFunc(maxWidth int, value interface{}) (string, error) {
	if value == nil {
		return "", nil
	}
	if stringValue, ok := value.(string); ok {
		return text.Truncate(maxWidth, stringValue), nil
	}
	return "", fmt.Errorf("invalid value; expected string, got %T", value)
}

func autoColorFunc(_ string, input interface{}) (string, error) {
	return jsonScalarToString(input)
}

func tableRowFunc(printer *tableprinter.TablePrinter, fields ...interface{}) (string, error) {
	if printer == nil {
		return "", fmt.Errorf("failed to write table row: no table printer")
	}
	for _, field := range fields {
		value, err := jsonScalarToString(field)
		if err != nil {
			return "", fmt.Errorf("failed to write table row: %v", err)
		}
		printer.AddField(value, tableprinter.WithTruncate(truncateMultiline))
	}
	printer.EndRow()
	return "", nil
}

func tableRenderFunc(printer *tableprinter.TablePrinter) (string, error) {
	if printer == nil {
		return "", fmt.Errorf("failed to render table: no table printer")
	}
	if err := printer.Render(); err != nil {
		return "", fmt.Errorf("failed to render table: %v", err)
	}
	return "", nil
}

func jsonScalarToString(input interface{}) (string, error) {
	switch value := input.(type) {
	case string:
		return value, nil
	case float64:
		if math.Trunc(value) == value {
			return strconv.FormatFloat(value, 'f', 0, 64), nil
		}
		return strconv.FormatFloat(value, 'f', 2, 64), nil
	case nil:
		return "", nil
	case bool:
		return fmt.Sprintf("%v", value), nil
	default:
		return "", fmt.Errorf("cannot convert type to string: %v", value)
	}
}

func timeAgo(ago time.Duration) string {
	if ago < time.Minute {
		return "just now"
	}
	if ago < time.Hour {
		return text.Pluralize(int(ago.Minutes()), "minute") + " ago"
	}
	if ago < 24*time.Hour {
		return text.Pluralize(int(ago.Hours()), "hour") + " ago"
	}
	if ago < 30*24*time.Hour {
		return text.Pluralize(int(ago.Hours())/24, "day") + " ago"
	}
	if ago < 365*24*time.Hour {
		return text.Pluralize(int(ago.Hours())/24/30, "month") + " ago"
	}
	return text.Pluralize(int(ago.Hours()/24/365), "year") + " ago"
}

func truncateMultiline(maxWidth int, value string) string {
	if i := strings.IndexAny(value, "\r\n"); i >= 0 {
		value = value[:i] + ellipsis
	}
	return text.Truncate(maxWidth, value)
}

func hyperlinkFunc(link, value string) string {
	if value == "" {
		value = link
	}
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", link, value)
}

func sprigFuncMap() template.FuncMap {
	names := []string{"contains", "hasPrefix", "hasSuffix", "regexMatch", "replace"}
	textFunctions := sprig.TxtFuncMap()
	functions := template.FuncMap{}
	for _, name := range names {
		functions[name] = textFunctions[name]
	}
	return functions
}
