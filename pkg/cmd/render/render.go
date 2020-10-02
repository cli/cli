package render

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path"
	"syscall"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/markdown"
	"github.com/cli/cli/utils"
	"github.com/enescakir/emoji"
	"github.com/spf13/cobra"
)

type RenderOptions struct {
	IO *iostreams.IOStreams

	FilePath string
}

func NewCmdRender(f *cmdutil.Factory) *cobra.Command {
	opts := RenderOptions{
		IO: f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "render <file-path>",
		Short: "Render Markdown file in the terminal",
		Long: heredoc.Doc(`
			Render the Markdown file pased as argument to have a pretty look before opening a PR for a change. Specially used with README
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.FilePath = args[0]

			return renderRun(&opts)
		},
	}

	return cmd
}

func renderRun(opts *RenderOptions) error {
	filePath := opts.FilePath
	filename := path.Base(filePath)

	if !utils.IsMarkdownFile(filename) {
		_, err := fmt.Fprintf(opts.IO.ErrOut, "Markdown file was expected, but got '%s' instead\n", path.Ext(filename))
		return err
	}

	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		_, err := fmt.Fprintf(opts.IO.ErrOut, "failed to read file '%s'", filePath)
		return err
	}

	contentString := string(content)

	opts.IO.DetectTerminalTheme()

	err = opts.IO.StartPager()
	if err != nil {
		return err
	}
	defer opts.IO.StopPager()

	style := markdown.GetStyle(opts.IO.TerminalTheme())
	renderContent, err := markdown.Render(contentString, style)
	if err != nil {
		return fmt.Errorf("error rendering markdown: %w", err)
	}
	renderContent = emoji.Parse(renderContent)

	_, err = opts.IO.Out.Write([]byte(renderContent))
	if err != nil && !errors.Is(err, syscall.EPIPE) {
		return err
	}

	return nil
}
