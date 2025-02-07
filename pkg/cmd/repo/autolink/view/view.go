package view

import (
	"fmt"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/autolink/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type viewOptions struct {
	BaseRepo       func() (ghrepo.Interface, error)
	Browser        browser.Browser
	AutolinkClient AutolinkViewClient
	IO             *iostreams.IOStreams
	Exporter       cmdutil.Exporter

	ID string
}

type AutolinkViewClient interface {
	View(repo ghrepo.Interface, id string) (*shared.Autolink, error)
}

func NewCmdView(f *cmdutil.Factory, runF func(*viewOptions) error) *cobra.Command {
	opts := &viewOptions{
		Browser: f.Browser,
		IO:      f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "view <id>",
		Short: "View an autolink reference",
		Long:  "View an autolink reference for a repository.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			httpClient, err := f.HttpClient()
			if err != nil {
				return err
			}

			opts.BaseRepo = f.BaseRepo
			opts.ID = args[0]
			opts.AutolinkClient = &AutolinkViewer{HTTPClient: httpClient}

			if runF != nil {
				return runF(opts)
			}

			return viewRun(opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.Exporter, shared.AutolinkFields)

	return cmd
}

func viewRun(opts *viewOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	autolink, err := opts.AutolinkClient.View(repo, opts.ID)

	if err != nil {
		return fmt.Errorf("%s %w", cs.Red("error viewing autolink:"), err)
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, autolink)
	}

	fmt.Fprintf(out, "Autolink in %s\n\n", ghrepo.FullName(repo))

	fmt.Fprint(out, cs.Bold("ID: "))
	fmt.Fprintln(out, cs.Cyanf("%d", autolink.ID))

	fmt.Fprint(out, cs.Bold("Key Prefix: "))
	fmt.Fprintln(out, autolink.KeyPrefix)

	fmt.Fprint(out, cs.Bold("URL Template: "))
	fmt.Fprintln(out, autolink.URLTemplate)

	fmt.Fprint(out, cs.Bold("Alphanumeric: "))
	fmt.Fprintln(out, autolink.IsAlphanumeric)

	return nil
}
