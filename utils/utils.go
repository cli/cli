package utils

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func IsDebugEnabled() (bool, string) {
	debugValue := os.LookupEnv("GH_DEBUG")

	switch debugValue {
	case "false", "0", "no", "":
		return false, debugValue
	default:
		return true, debugValue
	}
}

var TerminalSize = func(w interface{}) (int, int, error) {
	if f, isFile := w.(*os.File); isFile {
		return term.GetSize(int(f.Fd()))
	}

	return 0, 0, fmt.Errorf("%v is not a file", w)
}
