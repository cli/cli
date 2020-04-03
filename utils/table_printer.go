package utils

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/cli/cli/pkg/text"
	"github.com/mattn/go-isatty"
	"golang.org/x/crypto/ssh/terminal"
)

type TablePrinter interface {
	IsTTY() bool
	AddField(string, func(int, string) string, func(string) string)
	EndRow()
	Render() error
}

func NewTablePrinter(w io.Writer) TablePrinter {
	if outFile, isFile := w.(*os.File); isFile {
		// TODO: use utils.IsTerminal()
		isCygwin := isatty.IsCygwinTerminal(outFile.Fd())
		if isatty.IsTerminal(outFile.Fd()) || isCygwin {
			ttyWidth := 80
			if w, _, err := terminal.GetSize(int(outFile.Fd())); err == nil {
				ttyWidth = w
			} else if isCygwin {
				tputCmd := exec.Command("tput", "cols")
				tputCmd.Stdin = os.Stdin
				if out, err := tputCmd.Output(); err == nil {
					if w, err := strconv.Atoi(strings.TrimSpace(string(out))); err == nil {
						ttyWidth = w
					}
				}
			}
			return &ttyTablePrinter{
				out:      NewColorable(outFile),
				maxWidth: ttyWidth,
			}
		}
	}
	return &tsvTablePrinter{
		out: w,
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
			if col < numCols-1 {
				// pad value with spaces on the right
				if padWidth := colWidths[col] - text.DisplayWidth(field.Text); padWidth > 0 {
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
