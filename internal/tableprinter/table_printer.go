package tableprinter

import (
	"io"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
)

type TablePrinter struct {
	tableprinter.TablePrinter
	isTTY bool
	cs    *iostreams.ColorScheme
}

func (t *TablePrinter) headerRow(columns ...string) {
	for _, col := range columns {
		// TODO: Consider truncating longer headers e.g., NUMBER, or removing unnecessary headers e.g., DESCRIPTION with no descriptions.
		t.AddField(strings.ToUpper(col), WithColor(t.cs.Bold))
	}
	t.EndRow()
}

// In tty mode display the fuzzy time difference between now and t.
// In nontty mode just display t with the time.RFC3339 format.
func (tp *TablePrinter) AddTimeField(now, t time.Time, c func(string) string) {
	tf := t.Format(time.RFC3339)
	if tp.isTTY {
		tf = text.FuzzyAgo(now, t)
	}
	tp.AddField(tf, tableprinter.WithColor(c))
}

var (
	WithTruncate = tableprinter.WithTruncate
	WithColor    = tableprinter.WithColor
)

type headerProviderOptFn func() []string

func WithHeaders(headers ...string) headerProviderOptFn {
	return func() []string {
		return headers
	}
}

var NoHeaders = func() []string {
	return nil
}

func New(ios *iostreams.IOStreams, headerFn headerProviderOptFn) *TablePrinter {
	maxWidth := 80
	isTTY := ios.IsStdoutTTY()
	if isTTY {
		maxWidth = ios.TerminalWidth()
	}

	return NewWithOptions(ios.Out, isTTY, maxWidth, ios.ColorScheme(), headerFn)
}

func NewWithOptions(w io.Writer, isTTY bool, maxWidth int, cs *iostreams.ColorScheme, headerFn headerProviderOptFn) *TablePrinter {
	tp := &TablePrinter{
		TablePrinter: tableprinter.New(w, isTTY, maxWidth),
		isTTY:        isTTY,
		cs:           cs,
	}

	if isTTY && headerFn != nil {
		if headers := headerFn(); len(headers) > 0 {
			tp.headerRow(headers...)
		}
	}

	return tp
}
