package cmdutil

import (
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
)

type Factory struct {
	IOStreams  *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
}

func (f Factory) ColorableOut() io.Writer {
	if outFile, isFile := f.IOStreams.Out.(*os.File); isFile {
		return utils.NewColorable(outFile)
	}
	return f.IOStreams.Out
}

func (f Factory) ColorableErr() io.Writer {
	if outFile, isFile := f.IOStreams.ErrOut.(*os.File); isFile {
		return utils.NewColorable(outFile)
	}
	return f.IOStreams.ErrOut
}
