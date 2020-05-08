package command

import (
	"fmt"
	"io"
	"strings"
	"text/template"
	"unicode"

	"github.com/spf13/cobra"
)

func hackyHelp(command *cobra.Command) {
	trimRightSpace := func(s string) string {
		return strings.TrimRightFunc(s, unicode.IsSpace)
	}

	rpad := func(s string, padding int) string {
		template := fmt.Sprintf("%%-%ds", padding)
		return fmt.Sprintf(template, s)
	}

	templateFuncs := template.FuncMap{
		"trim":                    strings.TrimSpace,
		"trimTrailingWhitespaces": trimRightSpace,
		"rpad":                    rpad,
	}

	tmpl := func(w io.Writer, text string, data interface{}) error {
		t := template.New("top")
		t.Funcs(templateFuncs)
		template.Must(t.Parse(text))
		return t.Execute(w, data)
	}

	err := tmpl(command.OutOrStdout(), command.HelpTemplate(), command)
	if err != nil {
		command.Println(err)
	}
}
