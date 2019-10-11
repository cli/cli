package command

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/logrusorgru/aurora"
)

func style(data interface{}, s string) string {
	tmpl, err := template.New("").Funcs(styles()).Parse(s)

	if err != nil {
		panic(err)
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, data)
	if err != nil {
		panic(err)

	}
	return strings.ReplaceAll(buf.String(), "\t", "  ")
}

func styles() template.FuncMap {
	return template.FuncMap{
		"black": func(s ...string) string {
			return aurora.Black(strings.Join(s, "")).String()
		},
		"red": func(s ...string) string {
			return aurora.Red(strings.Join(s, "")).String()
		},
		"green": func(s ...string) string {
			return aurora.Green(strings.Join(s, "")).String()
		},
		"yellow": func(s ...string) string {
			return aurora.Yellow(strings.Join(s, "")).String()
		},
		"blue": func(s ...string) string {
			return aurora.Blue(strings.Join(s, "")).String()
		},
		"magenta": func(s ...string) string {
			return aurora.Magenta(strings.Join(s, "")).String()
		},
		"cyan": func(s ...string) string {
			return aurora.Cyan(strings.Join(s, "")).String()
		},
		"white": func(s ...string) string {
			return aurora.White(strings.Join(s, "")).String()
		},
		"gray": func(s ...string) string {
			return aurora.Gray(8, strings.Join(s, "")).String()
		},

		"bold": func(s ...string) string {
			return aurora.Bold(strings.Join(s, "")).String()
		},
		"italic": func(s ...string) string {
			return aurora.Italic(strings.Join(s, "")).String()
		},
		"underline": func(s ...string) string {
			return aurora.Underline(strings.Join(s, "")).String()
		},

		"truncate": func(width int, s string) string {
			if len(s) > width {
				return s[0:width-3] + "..."
			}
			return s
		},
	}
}
