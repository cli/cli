package export

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
func (f *tableFormatter) Table() string {
	f.current = &table{
		// Write directly to the same writer as template execution
		// since writing to a buffer isn't getting flushed properly.
		tabwriter: tabwriter.NewWriter(f.writer, 0, 1, 1, ' ', 0),
	}
	return ""
}

// Write a row of string fields. Formatting must be performed before passing to Row.
func (f *tableFormatter) Row(fields ...string) (string, error) {
	if f.current == nil {
		return "", fmt.Errorf("no current table. use '{{table}}' to start a table")
	}
	row := strings.Join(fields, "\t")
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
