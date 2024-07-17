package listcmd

import (
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type ConsoleListRenderer struct {
	IO *iostreams.IOStreams
}

func (r ConsoleListRenderer) Render(sponsors []Sponsor) error {
	tp := tableprinter.New(r.IO, tableprinter.WithHeader("SPONSOR"))
	for _, sponsor := range sponsors {
		tp.AddField(string(sponsor))
		tp.EndRow()
	}
	return tp.Render()
}
