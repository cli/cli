package utils

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/text"
)

type TablePrinter interface {
	IsTTY() bool
	AddField(string, func(int, string) string, func(string) string)
	EndRow()
	Render() error
}

type TablePrinterOptions struct {
	IsTTY bool
}

func NewTablePrinter(io *iostreams.IOStreams) TablePrinter {
	return NewTablePrinterWithOptions(io, TablePrinterOptions{
		IsTTY: io.IsStdoutTTY(),
	})
}

func NewTablePrinterWithOptions(io *iostreams.IOStreams, opts TablePrinterOptions) TablePrinter {
	if opts.IsTTY {
		var maxWidth int
		if io.IsStdoutTTY() {
			maxWidth = io.TerminalWidth()
		} else {
			maxWidth = io.ProcessTerminalWidth()
		}
		return &ttyTablePrinter{
			out:      io.Out,
			maxWidth: maxWidth,
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

func (f *tableField) DisplayWidth() int {
	return text.DisplayWidth(f.Text)
}

type ttyTablePrinter struct {
	out      io.Writer
	maxWidth int
	rows     [][]tableField
}

func (t ttyTablePrinter) IsTTY() bool {
	return true
}

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

	delim := "  "
	numCols := len(t.rows[0])
	colWidths := t.calculateColumnWidths(len(delim))

	for _, row := range t.rows {
		for col, field := range row {
			if col > 0 {
				_, err := fmt.Fprint(t.out, delim)
				if err != nil {
					return err
				}
			}
			truncVal := field.TruncateFunc(colWidths[col], field.Text)
			if col < numCols-1 {
				// pad value with spaces on the right
				if padWidth := colWidths[col] - field.DisplayWidth(); padWidth > 0 {
					truncVal += strings.Repeat(" ", padWidth)
				}
			}
			if field.ColorFunc != nil {
				truncVal = field.ColorFunc(truncVal)
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

func (t *ttyTablePrinter) calculateColumnWidths(delimSize int) []int {
	numCols := len(t.rows[0])
	allColWidths := make([][]int, numCols)
	for _, row := range t.rows {
		for col, field := range row {
			allColWidths[col] = append(allColWidths[col], field.DisplayWidth())
		}
	}

	// calculate max & median content width per column
	maxColWidths := make([]int, numCols)
	// medianColWidth := make([]int, numCols)
	for col := 0; col < numCols; col++ {
		widths := allColWidths[col]
		sort.Ints(widths)
		maxColWidths[col] = widths[len(widths)-1]
		// medianColWidth[col] = widths[(len(widths)+1)/2]
	}

	colWidths := make([]int, numCols)
	// never truncate the first column
	colWidths[0] = maxColWidths[0]
	// never truncate the last column if it contains URLs
	if strings.HasPrefix(t.rows[0][numCols-1].Text, "https://") {
		colWidths[numCols-1] = maxColWidths[numCols-1]
	}

	availWidth := func() int {
		setWidths := 0
		for col := 0; col < numCols; col++ {
			setWidths += colWidths[col]
		}
		return t.maxWidth - delimSize*(numCols-1) - setWidths
	}
	numFixedCols := func() int {
		fixedCols := 0
		for col := 0; col < numCols; col++ {
			if colWidths[col] > 0 {
				fixedCols++
			}
		}
		return fixedCols
	}

	// set the widths of short columns
	if w := availWidth(); w > 0 {
		if numFlexColumns := numCols - numFixedCols(); numFlexColumns > 0 {
			perColumn := w / numFlexColumns
			for col := 0; col < numCols; col++ {
				if max := maxColWidths[col]; max < perColumn {
					colWidths[col] = max
				}
			}
		}
	}

	firstFlexCol := -1
	// truncate long columns to the remaining available width
	if numFlexColumns := numCols - numFixedCols(); numFlexColumns > 0 {
		perColumn := availWidth() / numFlexColumns
		for col := 0; col < numCols; col++ {
			if colWidths[col] == 0 {
				if firstFlexCol == -1 {
					firstFlexCol = col
				}
				if max := maxColWidths[col]; max < perColumn {
					colWidths[col] = max
				} else {
					colWidths[col] = perColumn
				}
			}
		}
	}

	// add remainder to the first flex column
	if w := availWidth(); w > 0 && firstFlexCol > -1 {
		colWidths[firstFlexCol] += w
		if max := maxColWidths[firstFlexCol]; max < colWidths[firstFlexCol] {
			colWidths[firstFlexCol] = max
		}
	}

	return colWidths
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
