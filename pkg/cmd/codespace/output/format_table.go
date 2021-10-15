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
	isTTY := isTTY(w)
	if asJSON {
		return &jsonwriter{w: w, pretty: isTTY}
	}
	if isTTY {
		return tablewriter.NewWriter(w)
	}
	return &tabwriter{w: w}
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
