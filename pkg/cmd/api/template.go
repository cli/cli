package api

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

	"github.com/cli/cli/utils"
	"github.com/mgutz/ansi"
)

func parseTemplate(tpl string, colorEnabled bool) (*template.Template, error) {
	now := time.Now()

	templateFuncs := map[string]interface{}{
		"color":     templateColor,
		"autocolor": templateColor,

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

		"pluck": templatePluck,
		"join":  templateJoin,
	}

	if !colorEnabled {
		templateFuncs["autocolor"] = func(colorName string, input interface{}) (string, error) {
			return jsonScalarToString(input)
		}
	}

	return template.New("").Funcs(templateFuncs).Parse(tpl)
}

func executeTemplate(w io.Writer, input io.Reader, templateStr string, colorEnabled bool) error {
	t, err := parseTemplate(templateStr, colorEnabled)
	if err != nil {
		return err
	}

	jsonData, err := ioutil.ReadAll(input)
	if err != nil {
		return err
	}

	var data interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return err
	}

	return t.Execute(w, data)
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

func templateColor(colorName string, input interface{}) (string, error) {
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
