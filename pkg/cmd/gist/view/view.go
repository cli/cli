package view

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)

	Selector string
	Raw      bool
	Web      bool
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "view {<gist id> | <gist url>}",
		Short: "View a gist",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Selector = args[0]

			if !opts.IO.IsStdoutTTY() {
				opts.Raw = true
			}

			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Raw, "raw", "r", false, "do not try and render markdown")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "open gist in browser")

	return cmd
}

func viewRun(opts *ViewOptions) error {
	gistID := opts.Selector

	if opts.Web {
		gistURL := gistID
		if !strings.Contains(gistURL, "/") {
			hostname := ghinstance.OverridableDefault()
			gistURL = ghinstance.GistPrefix(hostname) + gistID
		}
		if opts.IO.IsStderrTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", utils.DisplayURL(gistURL))
		}
		return utils.OpenInBrowser(gistURL)
	}

	u, err := url.Parse(opts.Selector)
	if err == nil {
		if strings.HasPrefix(u.Path, "/") {
			gistID = u.Path[1:]
		}
	}

	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	gist, err := shared.GetGist(client, ghinstance.OverridableDefault(), gistID)
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	if gist.Description != "" {
		fmt.Fprintf(opts.IO.ErrOut, "%s\n", cs.Bold(gist.Description))
	}

	for filename, gistFile := range gist.Files {
		fmt.Fprintf(opts.IO.ErrOut, "%s\n", cs.Gray(filename))
		fmt.Fprintln(opts.IO.ErrOut)
		content := gistFile.Content
		if strings.Contains(gistFile.Type, "markdown") && !opts.Raw {
			rendered, err := utils.RenderMarkdown(gistFile.Content)
			if err == nil {
				content = rendered
			}
		}
		fmt.Fprintf(opts.IO.Out, "%s\n", content)
		fmt.Fprintln(opts.IO.Out)
	}

	// TODO print gist files, possibly with rendered markdown
	return nil
}
