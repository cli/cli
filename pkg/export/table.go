package export

// This formatter differs from github.com/cli/cli/utils.TablePrinter
// because text/tabwriter.Writer ignores escape sequences like color
// terminal sequences. This allows users to more easily use --template
// and relies on text/tabwriter to handle elastic tabstops.

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

type tableFormatter struct {
	current *table
	writer  io.Writer
}

type table struct {
	tabwriter *tabwriter.Writer
}

// Initialize a new tabwriter.
func (f *tableFormatter) Table(args ...interface{}) (string, error) {
	minwidth, err := intArg(args, 0, 0)
	if err != nil {
		return "", fmt.Errorf("failed to initialize table: %v", err)
	}

	tabwidth, err := intArg(args, 1, 1)
	if err != nil {
		return "", fmt.Errorf("failed to initialize table: %v", err)
	}

	padding, err := intArg(args, 2, 1)
	if err != nil {
		return "", fmt.Errorf("failed to initialize table: %v", err)
	}

	padchar, err := byteArg(args, 3, ' ')
	if err != nil {
		return "", fmt.Errorf("failed to initialize table: %v", err)
	}

	flags, err := uintArg(args, 4, 0)
	if err != nil {
		return "", fmt.Errorf("failed to initialize table: %v", err)
	}

	f.current = &table{
		// Write directly to the same writer as template execution
		// since writing to a simple buffer doesn't flush properly.
		tabwriter: tabwriter.NewWriter(f.writer, minwidth, tabwidth, padding, padchar, flags),
	}
	return "", nil
}

// Write a row of string fields.
func (f *tableFormatter) Row(fields ...interface{}) (string, error) {
	if f.current == nil {
		return "", fmt.Errorf("no current table. use '{{table}}' to start a table")
	}
	stringFields := make([]string, len(fields))
	for i, e := range fields {
		s, err := jsonScalarToString(e)
		if err != nil {
			return "", fmt.Errorf("failed to write row: %v", err)
		}
		stringFields[i] = s
	}
	row := strings.Join(stringFields, "\t")
	if _, err := f.current.tabwriter.Write([]byte(row + "\n")); err != nil {
		return "", fmt.Errorf("failed to write row: %v", err)
	}
	return "", nil
}

// End the table and flush rows, adjusting for columns' widths using elastic tabstops.
func (f *tableFormatter) EndTable() (string, error) {
	if f.current == nil {
		return "", fmt.Errorf("no current table. use '{{table}}' to start a table")
	}
	f.current.tabwriter.Flush()
	f.current = nil
	return "", nil
}

// Need to account for limited type inference within templates.
func byteArg(args []interface{}, index int, defaultValue byte) (byte, error) {
	if len(args) > index {
		if t, ok := args[index].(byte); ok {
			return t, nil
		}
		if t, ok := args[index].(int); ok {
			if t < 0 || t > 255 {
				return 0, fmt.Errorf("argument %d value %d out of range", index, t)
			}
			return byte(t), nil
		}
		if t, ok := args[index].(string); ok {
			if len(t) != 1 {
				return 0, fmt.Errorf("argument %d must contain only 1 character", index)
			}
			return byte(t[0]), nil
		}
		return 0, fmt.Errorf("argument %d not a byte", index)
	}
	return defaultValue, nil
}

func intArg(args []interface{}, index int, defaultValue int) (int, error) {
	if len(args) > index {
		if t, ok := args[index].(int); ok {
			return t, nil
		}
		return 0, fmt.Errorf("argument %d not an int", index)
	}
	return defaultValue, nil
}

func uintArg(args []interface{}, index int, defaultValue uint) (uint, error) {
	if len(args) > index {
		if t, ok := args[index].(uint); ok {
			return t, nil
		}
		if t, ok := args[index].(int); ok {
			if t < 0 {
				return 0, fmt.Errorf("argument %d value %d out of range", index, t)
			}
			return uint(t), nil
		}
		return 0, fmt.Errorf("argument %d not a uint", index)
	}
	return defaultValue, nil
}
