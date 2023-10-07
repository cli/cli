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

	// Assert that we tried to add a header.
	addedHeader bool
}

func (t *TablePrinter) IsTTY() bool {
	return t.isTTY
}

func (t *TablePrinter) HeaderRow(columns ...string) {
	if t.addedHeader {
		return
	}
	t.addedHeader = true

	if !t.isTTY {
		return
	}
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

type option func(*TablePrinter)

func New(ios *iostreams.IOStreams, options ...option) *TablePrinter {
	maxWidth := 80
	isTTY := ios.IsStdoutTTY()
	if isTTY {
		maxWidth = ios.TerminalWidth()
	}

	return NewWithOptions(ios.Out, isTTY, maxWidth, ios.ColorScheme(), options...)
}

func NewWithOptions(w io.Writer, isTTY bool, maxWidth int, cs *iostreams.ColorScheme, options ...option) *TablePrinter {
	tp := &TablePrinter{
		TablePrinter: tableprinter.New(w, isTTY, maxWidth),
		isTTY:        isTTY,
		cs:           cs,
	}

	for _, opt := range options {
		opt(tp)
	}
	return tp
}

// NoHeader disable printing or checking for a table header.
func NoHeader(tp *TablePrinter) {
	tp.addedHeader = true
}

func (t *TablePrinter) Render() error {
	if !t.addedHeader {
		panic("must call HeaderRow")
	}
	return t.TablePrinter.Render()
}
