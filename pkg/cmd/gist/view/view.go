package view

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/markdown"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/pkg/text"
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
	cmd.Flags().BoolVarP(&opts.ListFiles, "files", "", false, "List file names from the gist")
	cmd.Flags().StringVarP(&opts.Filename, "filename", "f", "", "Display a single file from the gist")

	return cmd
}

func viewRun(opts *ViewOptions) error {
	gistID := opts.Selector
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cs := opts.IO.ColorScheme()
	if gistID == "" {
		gistID, err = promptGists(client, cs)
		if err != nil {
			return err
		}

		if gistID == "" {
			fmt.Fprintln(opts.IO.Out, "No gists found.")
			return nil
		}
	}

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
		if shared.IsBinaryContents([]byte(gf.Content)) {
			if len(gist.Files) == 1 || opts.Filename != "" {
				return fmt.Errorf("error: file is binary")
			}
			_, err = fmt.Fprintln(opts.IO.Out, cs.Gray("(skipping rendering binary content)"))
			return nil
		}

		if strings.Contains(gf.Type, "markdown") && !opts.Raw {
			rendered, err := markdown.Render(gf.Content, markdownStyle)
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

func promptGists(client *http.Client, cs *iostreams.ColorScheme) (gistID string, err error) {
	gists, err := shared.ListGists(client, ghinstance.OverridableDefault(), 10, "all")
	if err != nil {
		return "", err
	}

	if len(gists) == 0 {
		return "", nil
	}

	var opts []string
	var result int
	var gistIDs = make([]string, len(gists))

	for i, gist := range gists {
		gistIDs[i] = gist.ID
		description := ""
		gistName := ""

		if gist.Description != "" {
			description = gist.Description
		}

		filenames := make([]string, 0, len(gist.Files))
		for fn := range gist.Files {
			filenames = append(filenames, fn)
		}
		sort.Strings(filenames)
		gistName = filenames[0]

		gistTime := utils.FuzzyAgo(time.Since(gist.UpdatedAt))
		// TODO: support dynamic maxWidth
		description = text.Truncate(100, text.ReplaceExcessiveWhitespace(description))
		opt := fmt.Sprintf("%s %s %s", cs.Bold(gistName), description, cs.Gray(gistTime))
		opts = append(opts, opt)
	}

	questions := &survey.Select{
		Message: "Select a gist",
		Options: opts,
	}

	err = prompt.SurveyAskOne(questions, &result)

	if err != nil {
		return "", err
	}

	return gistIDs[result], nil
}
