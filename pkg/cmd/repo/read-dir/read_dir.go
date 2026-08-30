package readdir

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// dirEntryFields are the JSON fields selectable via the --json flag.
var dirEntryFields = []string{
	"name",
	"path",
	"nameRaw",
	"pathRaw",
	"type",
	"gitType",
	"mode",
	"modeOctal",
	"gitSHA",
	"size",
	"submodule",
}

// ReadDirOptions holds the configuration for the read-dir command.
type ReadDirOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Exporter   cmdutil.Exporter

	Path string
	Ref  string
}

// NewCmdReadDir creates the `gh repo read-dir` command.
func NewCmdReadDir(f *cmdutil.Factory, runF func(*ReadDirOptions) error) *cobra.Command {
	opts := &ReadDirOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "read-dir [<path>] [flags]",
		Short: "List a directory in a repository (preview)",
		Long: heredoc.Docf(`
			List the contents of a directory in a GitHub repository without cloning it.

			This command is in preview and subject to change without notice.

			By default, the directory is listed from the default branch. Use the %[1]s--ref%[1]s flag to
			list from a specific branch, tag, or commit. When no path is given, the repository root
			is listed.
		`, "`"),
		Example: heredoc.Doc(`
			# List the root of the default branch
			$ gh repo read-dir --repo cli/cli

			# List a subdirectory
			$ gh repo read-dir docs --repo cli/cli

			# List a directory at a specific ref
			$ gh repo read-dir docs --repo cli/cli --ref v2.50.0

			# Print selected fields as JSON
			$ gh repo read-dir docs --repo cli/cli --json name,path,type,size
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			if len(args) > 0 {
				opts.Path = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return readDirRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Ref, "ref", "", "The branch, tag, or commit to list from")

	cmdutil.AddJSONFlags(cmd, &opts.Exporter, dirEntryFields)

	cmdutil.EnableRepoOverride(cmd, f)

	return cmd
}

func readDirRun(opts *ReadDirOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("%w. Run this command from within a git repository, or use the `--repo` flag to specify one", err)
	}

	dir, err := fetchTree(httpClient, repo, opts.Path, opts.Ref)
	if err != nil {
		return err
	}

	if opts.Exporter != nil {
		return opts.Exporter.Write(opts.IO, dir)
	}

	if len(dir.Entries) == 0 {
		location := ghrepo.FullName(repo)
		if opts.Path != "" {
			location = fmt.Sprintf("%s/%s", location, strings.TrimPrefix(opts.Path, "/"))
		}
		fmt.Fprintf(opts.IO.ErrOut, "No entries found in %s\n", location)
		return nil
	}

	if !opts.IO.IsStdoutTTY() {
		return writeTSV(opts.IO, dir)
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
	}
	defer opts.IO.StopPager()

	location := ghrepo.FullName(repo)
	if opts.Path != "" {
		location = fmt.Sprintf("%s/%s", location, strings.TrimPrefix(opts.Path, "/"))
	}
	noun := "entries"
	if len(dir.Entries) == 1 {
		noun = "entry"
	}
	fmt.Fprintf(opts.IO.Out, "Showing %d %s in %s\n\n", len(dir.Entries), noun, location)

	return writeTable(opts.IO, dir)
}

// writeTSV writes a tab-separated listing for non-TTY output: type, name,
// octal mode, and raw byte size, with no header.
func writeTSV(io *iostreams.IOStreams, dir *repoDir) error {
	var sb strings.Builder
	for _, e := range dir.Entries {
		fmt.Fprintf(&sb, "%s\t%s\t%s\t%d\n", e.Type, e.Name, e.modeOctal(), e.Size)
	}
	_, err := io.Out.Write([]byte(sb.String()))
	return err
}

// writeTable writes the TTY listing as a TYPE/NAME/SIZE table. Names are colored
// by entry type and directories and submodules show "-" since git reports no size
// for them.
func writeTable(io *iostreams.IOStreams, dir *repoDir) error {
	cs := io.ColorScheme()
	tp := tableprinter.New(io, tableprinter.WithHeader("TYPE", "NAME", "SIZE"))
	for _, e := range dir.Entries {
		entryType := e.Type
		if e.Type == "file" && e.isExecutable() {
			entryType = "file*"
		}

		var color string
		switch e.Type {
		case "dir":
			color = "blue"
		case "symlink":
			color = "cyan"
		case "submodule":
			color = "yellow"
		case "file":
			if e.isExecutable() {
				color = "green"
			}
		}

		size := "-"
		if e.Type == "file" || e.Type == "symlink" {
			size = text.FormatSize(int64(e.Size))
		}

		tp.AddField(entryType)
		tp.AddField(e.Name, tableprinter.WithColor(cs.ColorFromString(color)))
		tp.AddField(size)
		tp.EndRow()
	}
	return tp.Render()
}
