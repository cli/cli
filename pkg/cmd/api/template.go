package api

import (
	"fmt"
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
			return utils.FuzzyAgoAbbr(now, t), nil
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
