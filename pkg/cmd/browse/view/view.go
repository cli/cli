package view

import (
	"net/http"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type ViewOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser

	SelectorArg string
	WebMode     bool
	Comments    bool
	Exporter    cmdutil.Exporter

	Now func() time.Time
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {

	cmd := &cobra.Command{
		Use:   "view",
		Short: "View in browser",
		Long: heredoc.Doc(`
			Open the repository page in the browser
		`),
		Args: cobra.ExactArgs(1),
		// RunE: func(cmd *cobra.Command, args []string) error {
		// 	// support `-R, --repo` override
		// 	opts.BaseRepo = f.BaseRepo

		// 	if len(args) > 0 {
		// 		opts.SelectorArg = args[0]
		// 	}

		// 	if runF != nil {
		// 		return runF(opts)
		// 	}
		// 	return viewRun(opts)
		// },
	}

	return cmd
}
