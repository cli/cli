package utils

import (
	"io"
	"strings"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
)

type TablePrinter interface {
	IsTTY() bool
	AddField(string, func(int, string) string, func(string) string)
	EndRow()
	Render() error
}

type TablePrinterOptions struct {
	IsTTY    bool
	MaxWidth int
	Out      io.Writer
}

// Deprecated: use internal/tableprinter
func NewTablePrinter(io *iostreams.IOStreams) TablePrinter {
	return NewTablePrinterWithOptions(io, TablePrinterOptions{
		IsTTY: io.IsStdoutTTY(),
	})
}

// Deprecated: use internal/tableprinter
func NewTablePrinterWithOptions(ios *iostreams.IOStreams, opts TablePrinterOptions) TablePrinter {
	var out io.Writer
	if opts.Out != nil {
		out = opts.Out
	} else {
		out = ios.Out
	}
	var maxWidth int
	if opts.IsTTY {
		if opts.MaxWidth > 0 {
			maxWidth = opts.MaxWidth
		} else {
			maxWidth = ios.TerminalWidth()
		}
	}
	tp := tableprinter.New(out, opts.IsTTY, maxWidth)
	return &printer{
		tp:    tp,
		isTTY: opts.IsTTY,
	}
}

type printer struct {
	tp       tableprinter.TablePrinter
	colIndex int
	isTTY    bool
}

func (p printer) IsTTY() bool {
	return p.isTTY
}

func (p *printer) AddField(s string, truncateFunc func(int, string) string, colorFunc func(string) string) {
	if truncateFunc == nil {
		// Disallow ever truncating the 1st colum or any URL value
		if p.colIndex == 0 || isURL(s) {
			p.tp.AddField(s, tableprinter.WithTruncate(nil), tableprinter.WithColor(colorFunc))
		} else {
			p.tp.AddField(s, tableprinter.WithColor(colorFunc))
		}
	} else {
		p.tp.AddField(s, tableprinter.WithTruncate(truncateFunc), tableprinter.WithColor(colorFunc))
	}
	p.colIndex++
}

func (p *printer) EndRow() {
	p.tp.EndRow()
	p.colIndex = 0
}

func (p *printer) Render() error {
	return p.tp.Render()
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}
