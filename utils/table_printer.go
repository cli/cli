package utils

import (
	"fmt"
	"io"
	"strings"

	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/text"
)

type TablePrinter interface {
	IsTTY() bool
	AddField(string, func(int, string) string, func(string) string)
	EndRow()
	Render() error
}

func NewTablePrinter(io *iostreams.IOStreams) TablePrinter {
	if io.IsStdoutTTY() {
		return &ttyTablePrinter{
			out:      io.Out,
			maxWidth: io.TerminalWidth(),
		}
	}
	return &tsvTablePrinter{
		out: io.Out,
	}
}

type tableField struct {
	Text         string
	TruncateFunc func(int, string) string
	ColorFunc    func(string) string
}

type ttyTablePrinter struct {
	out      io.Writer
	maxWidth int
	rows     [][]tableField
}

func (t ttyTablePrinter) IsTTY() bool {
	return true
}

// Never pass pre-colorized text to AddField; always specify colorFunc. Otherwise, the table printer can't correctly compute the width of its columns.
func (t *ttyTablePrinter) AddField(s string, truncateFunc func(int, string) string, colorFunc func(string) string) {
	if truncateFunc == nil {
		truncateFunc = text.Truncate
	}
	if t.rows == nil {
		t.rows = make([][]tableField, 1)
	}
	rowI := len(t.rows) - 1
	field := tableField{
		Text:         s,
		TruncateFunc: truncateFunc,
		ColorFunc:    colorFunc,
	}
	t.rows[rowI] = append(t.rows[rowI], field)
}

func (t *ttyTablePrinter) EndRow() {
	t.rows = append(t.rows, []tableField{})
}

func (t *ttyTablePrinter) Render() error {
	if len(t.rows) == 0 {
		return nil
	}

	numCols := len(t.rows[0])
	colWidths := make([]int, numCols)
	// measure maximum content width per column
	for _, row := range t.rows {
		for col, field := range row {
			textLen := text.DisplayWidth(field.Text)
			if textLen > colWidths[col] {
				colWidths[col] = textLen
			}
		}
	}

	delim := "  "
	availWidth := t.maxWidth - colWidths[0] - ((numCols - 1) * len(delim))
	// add extra space from columns that are already narrower than threshold
	for col := 1; col < numCols; col++ {
		availColWidth := availWidth / (numCols - 1)
		if extra := availColWidth - colWidths[col]; extra > 0 {
			availWidth += extra
		}
	}
	// cap all but first column to fit available terminal width
	// TODO: support weighted instead of even redistribution
	for col := 1; col < numCols; col++ {
		availColWidth := availWidth / (numCols - 1)
		if colWidths[col] > availColWidth {
			colWidths[col] = availColWidth
		}
	}

	for _, row := range t.rows {
		for col, field := range row {
			if col > 0 {
				_, err := fmt.Fprint(t.out, delim)
				if err != nil {
					return err
				}
			}
			truncVal := field.TruncateFunc(colWidths[col], field.Text)
			if field.ColorFunc != nil {
				truncVal = field.ColorFunc(truncVal)
			}
			if col < numCols-1 {
				// pad value with spaces on the right
				if padWidth := colWidths[col] - text.DisplayWidth(field.Text); padWidth > 0 {
					truncVal += strings.Repeat(" ", padWidth)
				}
			}
			_, err := fmt.Fprint(t.out, truncVal)
			if err != nil {
				return err
			}
		}
		if len(row) > 0 {
			_, err := fmt.Fprint(t.out, "\n")
			if err != nil {
				return err
			}
		}
	}
	return nil
}

type tsvTablePrinter struct {
	out        io.Writer
	currentCol int
}

func (t tsvTablePrinter) IsTTY() bool {
	return false
}

func (t *tsvTablePrinter) AddField(text string, _ func(int, string) string, _ func(string) string) {
	if t.currentCol > 0 {
		fmt.Fprint(t.out, "\t")
	}
	fmt.Fprint(t.out, text)
	t.currentCol++
}

func (t *tsvTablePrinter) EndRow() {
	fmt.Fprint(t.out, "\n")
	t.currentCol = 0
}

func (t *tsvTablePrinter) Render() error {
	return nil
}
