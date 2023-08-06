package tableprinter

import (
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
)

type TablePrinter struct {
	tableprinter.TablePrinter
	isTTY bool
}

func (t *TablePrinter) HeaderRow(columns ...string) {
	if !t.isTTY {
		return
	}
	for _, col := range columns {
		t.AddField(strings.ToUpper(col))
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

func New(ios *iostreams.IOStreams) *TablePrinter {
	maxWidth := 80
	isTTY := ios.IsStdoutTTY()
	if isTTY {
		maxWidth = ios.TerminalWidth()
	}
	tp := tableprinter.New(ios.Out, isTTY, maxWidth)
	return &TablePrinter{
		TablePrinter: tp,
		isTTY:        isTTY,
	}
}
