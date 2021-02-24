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

	Selector  string
	Filename  string
	Raw       bool
	Web       bool
	ListFiles bool
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "view {<id> | <url>}",
		Short: "View a gist",
		Args:  cmdutil.ExactArgs(1, "cannot view: gist argument required"),
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

	cmd.Flags().BoolVarP(&opts.Raw, "raw", "r", false, "Print raw instead of rendered gist contents")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open gist in the browser")
	cmd.Flags().BoolVarP(&opts.ListFiles, "files", "", false, "List file names from the gist")
	cmd.Flags().StringVarP(&opts.Filename, "filename", "f", "", "Display a single file from the gist")

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

	theme := opts.IO.DetectTerminalTheme()
	markdownStyle := markdown.GetStyle(theme)
	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "starting pager failed: %v\n", err)
	}
	defer opts.IO.StopPager()

	render := func(gf *shared.GistFile) error {
		if strings.Contains(gf.Type, "markdown") && !opts.Raw {
			rendered, err := markdown.Render(gf.Content, markdownStyle, "")
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(opts.IO.Out, rendered)
			return err
		}

		if _, err := fmt.Fprint(opts.IO.Out, gf.Content); err != nil {
			return err
		}
		if !strings.HasSuffix(gf.Content, "\n") {
			_, err := fmt.Fprint(opts.IO.Out, "\n")
			return err
		}
		return nil
	}

	if opts.Filename != "" {
		gistFile, ok := gist.Files[opts.Filename]
		if !ok {
			return fmt.Errorf("gist has no such file: %q", opts.Filename)
		}
		return render(gistFile)
	}

	cs := opts.IO.ColorScheme()

	if gist.Description != "" && !opts.ListFiles {
		fmt.Fprintf(opts.IO.Out, "%s\n\n", cs.Bold(gist.Description))
	}

	showFilenames := len(gist.Files) > 1
	filenames := make([]string, 0, len(gist.Files))
	for fn := range gist.Files {
		filenames = append(filenames, fn)
	}
	sort.Strings(filenames)

	if opts.ListFiles {
		for _, fn := range filenames {
			fmt.Fprintln(opts.IO.Out, fn)
		}
		return nil
	}

	for i, fn := range filenames {
		if showFilenames {
			fmt.Fprintf(opts.IO.Out, "%s\n\n", cs.Gray(fn))
		}
		if err := render(gist.Files[fn]); err != nil {
			return err
		}
		if i < len(filenames)-1 {
			fmt.Fprint(opts.IO.Out, "\n")
		}
	}

	return nil
}
