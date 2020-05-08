package command

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/spf13/cobra"
)

func hackyHelp(command *cobra.Command) {
	trimRightSpace := func(s string) string {
		return strings.TrimRightFunc(s, unicode.IsSpace)
	}

	appendIfNotPresent := func(s, stringToAppend string) string {
		if strings.Contains(s, stringToAppend) {
			return s
		}
		return s + " " + stringToAppend
	}

	Gt := func(a interface{}, b interface{}) bool {
		var left, right int64
		av := reflect.ValueOf(a)

		switch av.Kind() {
		case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice:
			left = int64(av.Len())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			left = av.Int()
		case reflect.String:
			left, _ = strconv.ParseInt(av.String(), 10, 64)
		}

		bv := reflect.ValueOf(b)

		switch bv.Kind() {
		case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice:
			right = int64(bv.Len())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			right = bv.Int()
		case reflect.String:
			right, _ = strconv.ParseInt(bv.String(), 10, 64)
		}

		return left > right
	}

	// FIXME Eq is unused by cobra and should be removed in a version 2. It exists only for compatibility with users of cobra.

	// Eq takes two types and checks whether they are equal. Supported types are int and string. Unsupported types will panic.
	Eq := func(a interface{}, b interface{}) bool {
		av := reflect.ValueOf(a)
		bv := reflect.ValueOf(b)

		switch av.Kind() {
		case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice:
			panic("Eq called on unsupported type")
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Int() == bv.Int()
		case reflect.String:
			return av.String() == bv.String()
		}
		return false
	}

	rpad := func(s string, padding int) string {
		template := fmt.Sprintf("%%-%ds", padding)
		return fmt.Sprintf(template, s)
	}

	templateFuncs := template.FuncMap{
		"trim":                    strings.TrimSpace,
		"trimRightSpace":          trimRightSpace,
		"trimTrailingWhitespaces": trimRightSpace,
		"appendIfNotPresent":      appendIfNotPresent,
		"rpad":                    rpad,
		"gt":                      Gt,
		"eq":                      Eq,
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
