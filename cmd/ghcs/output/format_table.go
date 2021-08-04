package output

import (
	"io"
	"os"

	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"
)

type Table interface {
	SetHeader([]string)
	Append([]string)
	Render()
}

func NewTable(w io.Writer, asJSON bool) Table {
	f, ok := w.(*os.File)
	isTTY := ok && term.IsTerminal(int(f.Fd()))

	if asJSON {
		return &jsonwriter{w: w, pretty: isTTY}
	}
	if isTTY {
		return tablewriter.NewWriter(w)
	}
	return &tabwriter{w: w}
}
