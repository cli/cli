package view

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/markdown"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)

	Selector string
	Filename string
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
	cmd.Flags().StringVarP(&opts.Filename, "filename", "f", "", "display a single file of the gist")

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

	if strings.Contains(gistID, "/") {
		id, err := shared.GistIDFromURL(gistID)
		if err != nil {
			return err
		}
		gistID = id
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
		fmt.Fprintf(opts.IO.Out, "%s\n", cs.Bold(gist.Description))
	}

	if opts.Filename != "" {
		gistFile, ok := gist.Files[opts.Filename]
		if !ok {
			return fmt.Errorf("gist has no such file %q", opts.Filename)
		}

		gist.Files = map[string]*shared.GistFile{
			opts.Filename: gistFile,
		}
	}

	showFilenames := len(gist.Files) > 1

	outs := []string{} // to ensure consistent ordering

	for filename, gistFile := range gist.Files {
		out := ""
		if showFilenames {
			out += fmt.Sprintf("%s\n\n", cs.Gray(filename))
		}
		content := gistFile.Content
		if strings.Contains(gistFile.Type, "markdown") && !opts.Raw {
			style := markdown.GetStyle(opts.IO.DetectTerminalTheme())
			rendered, err := markdown.Render(gistFile.Content, style, "")
			if err == nil {
				content = rendered
			}
		}
		out += fmt.Sprintf("%s\n\n", content)

		outs = append(outs, out)
	}

	sort.Strings(outs)

	for _, out := range outs {
		fmt.Fprint(opts.IO.Out, out)
	}

	return nil
}
