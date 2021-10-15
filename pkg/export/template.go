package export

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/text"
	"github.com/cli/cli/v2/utils"
	"github.com/mgutz/ansi"
)

type Template struct {
	io           *iostreams.IOStreams
	tablePrinter utils.TablePrinter
	template     *template.Template
	templateStr  string
}

func NewTemplate(io *iostreams.IOStreams, template string) Template {
	return Template{
		io:          io,
		templateStr: template,
	}
}

func (t *Template) parseTemplate(tpl string) (*template.Template, error) {
	now := time.Now()

	templateFuncs := map[string]interface{}{
		"color":     t.color,
		"autocolor": t.color,

		"timefmt": func(format, input string) (string, error) {
			t, err := time.Parse(time.RFC3339, input)
			if err != nil {
				return "", err
			}
			return t.Format(format), nil
		},
		"timeago": func(input string) (string, error) {
			t, err := time.Parse(time.RFC3339, input)
			if err != nil {
				return "", err
			}
			return timeAgo(now.Sub(t)), nil
		},

		"pluck":       templatePluck,
		"join":        templateJoin,
		"tablerow":    t.tableRow,
		"tablerender": t.tableRender,
		"truncate":    text.Truncate,
	}

	if !t.io.ColorEnabled() {
		templateFuncs["autocolor"] = func(colorName string, input interface{}) (string, error) {
			return jsonScalarToString(input)
		}
	}

	return template.New("").Funcs(templateFuncs).Parse(tpl)
}

func (t *Template) Execute(input io.Reader) error {
	w := t.io.Out

	if t.template == nil {
		template, err := t.parseTemplate(t.templateStr)
		if err != nil {
			return err
		}

		t.template = template
	}

	jsonData, err := ioutil.ReadAll(input)
	if err != nil {
		return err
	}

	var data interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return err
	}

	return t.template.Execute(w, data)
}

func ExecuteTemplate(io *iostreams.IOStreams, input io.Reader, template string) error {
	t := NewTemplate(io, template)
	if err := t.Execute(input); err != nil {
		return err
	}
	return t.End()
}

func jsonScalarToString(input interface{}) (string, error) {
	switch tt := input.(type) {
	case string:
		return tt, nil
	case float64:
		if math.Trunc(tt) == tt {
			return strconv.FormatFloat(tt, 'f', 0, 64), nil
		} else {
			return strconv.FormatFloat(tt, 'f', 2, 64), nil
		}
	case nil:
		return "", nil
	case bool:
		return fmt.Sprintf("%v", tt), nil
	default:
		return "", fmt.Errorf("cannot convert type to string: %v", tt)
	}
}

func (t *Template) color(colorName string, input interface{}) (string, error) {
	text, err := jsonScalarToString(input)
	if err != nil {
		return "", err
	}
	return ansi.Color(text, colorName), nil
}

func templatePluck(field string, input []interface{}) []interface{} {
	var results []interface{}
	for _, item := range input {
		obj := item.(map[string]interface{})
		results = append(results, obj[field])
	}
	return results
}

func templateJoin(sep string, input []interface{}) (string, error) {
	var results []string
	for _, item := range input {
		text, err := jsonScalarToString(item)
		if err != nil {
			return "", err
		}
		results = append(results, text)
	}
	return strings.Join(results, sep), nil
}

func (t *Template) tableRow(fields ...interface{}) (string, error) {
	if t.tablePrinter == nil {
		t.tablePrinter = utils.NewTablePrinterWithOptions(t.io, utils.TablePrinterOptions{IsTTY: true})
	}
	for _, e := range fields {
		s, err := jsonScalarToString(e)
		if err != nil {
			return "", fmt.Errorf("failed to write table row: %v", err)
		}
		t.tablePrinter.AddField(s, text.TruncateColumn, nil)
	}
	t.tablePrinter.EndRow()
	return "", nil
}

func (t *Template) tableRender() (string, error) {
	if t.tablePrinter != nil {
		err := t.tablePrinter.Render()
		t.tablePrinter = nil
		if err != nil {
			return "", fmt.Errorf("failed to render table: %v", err)
		}
	}
	return "", nil
}

func (t *Template) End() error {
	// Finalize any template actions.
	if _, err := t.tableRender(); err != nil {
		return err
	}
	return nil
}

func timeAgo(ago time.Duration) string {
	if ago < time.Minute {
		return "just now"
	}
	if ago < time.Hour {
		return utils.Pluralize(int(ago.Minutes()), "minute") + " ago"
	}
	if ago < 24*time.Hour {
		return utils.Pluralize(int(ago.Hours()), "hour") + " ago"
	}
	if ago < 30*24*time.Hour {
		return utils.Pluralize(int(ago.Hours())/24, "day") + " ago"
	}
	if ago < 365*24*time.Hour {
		return utils.Pluralize(int(ago.Hours())/24/30, "month") + " ago"
	}
	return utils.Pluralize(int(ago.Hours()/24/365), "year") + " ago"
}
