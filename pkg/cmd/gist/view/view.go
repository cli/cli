package view

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/spf13/cobra"
)

type browser interface {
	Browse(string) error
}

type ViewOptions struct {
	IO         *iostreams.IOStreams
	Config     func() (gh.Config, error)
	HttpClient func() (*http.Client, error)
	Browser    browser
	Prompter   prompter.Prompter

	Selector  string
	Filename  string
	Raw       bool
	Web       bool
	ListFiles bool
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
		Prompter:   f.Prompter,
	}

	cmd := &cobra.Command{
		Use:   "view [<id> | <url>]",
		Short: "View a gist",
		Long:  `View the given gist or select from recent gists.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.Selector = args[0]
			}

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
	cmd.Flags().BoolVar(&opts.ListFiles, "files", false, "List file names from the gist")
	cmd.Flags().StringVarP(&opts.Filename, "filename", "f", "", "Display a single file from the gist")

	return cmd
}

func viewRun(opts *ViewOptions) error {
	gistID := opts.Selector
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	hostname, _ := cfg.Authentication().DefaultHost()

	cs := opts.IO.ColorScheme()
	if gistID == "" {
		if !opts.IO.CanPrompt() {
			return cmdutil.FlagErrorf("gist ID or URL required when not running interactively")
		}

		gist, err := shared.PromptGists(opts.Prompter, client, hostname, cs)
		if err != nil {
			return err
		}

		if gist.ID == "" {
			fmt.Fprintln(opts.IO.Out, "No gists found.")
			return nil
		}
		gistID = gist.ID
	}

	if opts.Web {
		gistURL := gistID
		if !strings.Contains(gistURL, "/") {
			gistURL = ghinstance.GistPrefix(hostname) + gistID
		}
		if opts.IO.IsStderrTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(gistURL))
		}
		return opts.Browser.Browse(gistURL)
	}

	if strings.Contains(gistID, "/") {
		id, err := shared.GistIDFromURL(gistID)
		if err != nil {
			return err
		}
		gistID = id
	}

	gist, err := shared.GetGist(client, hostname, gistID)
	if err != nil {
		return err
	}

	opts.IO.DetectTerminalTheme()
	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "starting pager failed: %v\n", err)
	}
	defer opts.IO.StopPager()

	render := func(gf *shared.GistFile) error {
		if shared.IsBinaryContents([]byte(gf.Content)) {
			if len(gist.Files) == 1 || opts.Filename != "" {
				return fmt.Errorf("error: file is binary")
			}
			_, err = fmt.Fprintln(opts.IO.Out, cs.Gray("(skipping rendering binary content)"))
			return nil
		}

		if strings.Contains(gf.Type, "markdown") && !opts.Raw {
			rendered, err := markdown.Render(gf.Content,
				markdown.WithTheme(opts.IO.TerminalTheme()),
				markdown.WithWrap(opts.IO.TerminalWidth()))
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

	if gist.Description != "" && !opts.ListFiles {
		fmt.Fprintf(opts.IO.Out, "%s\n\n", cs.Bold(gist.Description))
	}

	showFilenames := len(gist.Files) > 1
	filenames := make([]string, 0, len(gist.Files))
	for fn := range gist.Files {
		filenames = append(filenames, fn)
	}

	sort.Slice(filenames, func(i, j int) bool {
		return strings.ToLower(filenames[i]) < strings.ToLower(filenames[j])
	})

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
