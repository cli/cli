package utils

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

func NewTablePrinter(w io.Writer) *TTYTablePrinter {
	tty := false
	ttyWidth := 80
	if outFile, isFile := w.(*os.File); isFile {
		fd := int(outFile.Fd())
		tty = terminal.IsTerminal(fd)
		if w, _, err := terminal.GetSize(fd); err == nil {
			ttyWidth = w
		}
	}
	return &TTYTablePrinter{
		out:       w,
		IsTTY:     tty,
		maxWidth:  ttyWidth,
		colWidths: []int{},
		colFuncs:  make(map[int]func(string) string),
	}
}

type TTYTablePrinter struct {
	out       io.Writer
	IsTTY     bool
	maxWidth  int
	colWidths []int
	colFuncs  map[int]func(string) string
}

func (t *TTYTablePrinter) SetContentWidth(col, width int) {
	if col == len(t.colWidths) {
		t.colWidths = append(t.colWidths, 0)
	}
	if width > t.colWidths[col] {
		t.colWidths[col] = width
	}
}

func (t *TTYTablePrinter) SetColorFunc(col int, colorize func(string) string) {
	t.colFuncs[col] = colorize
}

// FitColumns caps all but first column to fit available terminal width.
func (t *TTYTablePrinter) FitColumns() {
	numCols := len(t.colWidths)
	delimWidth := 2
	availWidth := t.maxWidth - t.colWidths[0] - ((numCols - 1) * delimWidth)
	// TODO: avoid widening columns that already fit
	// TODO: support weighted instead of even redistribution
	for col := 1; col < len(t.colWidths); col++ {
		t.colWidths[col] = availWidth / (numCols - 1)
	}
}

func (t *TTYTablePrinter) WriteRow(fields ...string) error {
	lastCol := len(fields) - 1
	delim := "\t"
	if t.IsTTY {
		delim = "  "
	}

	for col, val := range fields {
		if col > 0 {
			_, err := fmt.Fprint(t.out, delim)
			if err != nil {
				return err
			}
		}
		if t.IsTTY {
			truncVal := truncate(t.colWidths[col], val)
			if col != lastCol {
				truncVal = fmt.Sprintf("%-*s", t.colWidths[col], truncVal)
			}
			if t.colFuncs[col] != nil {
				truncVal = t.colFuncs[col](truncVal)
			}
			_, err := fmt.Fprint(t.out, truncVal)
			if err != nil {
				return err
			}
		} else {
			_, err := fmt.Fprint(t.out, val)
			if err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprint(t.out, "\n")
	if err != nil {
		return err
	}
	return nil
}

func truncate(maxLength int, title string) string {
	if len(title) > maxLength {
		return title[0:maxLength-3] + "..."
	}
	return title
}
