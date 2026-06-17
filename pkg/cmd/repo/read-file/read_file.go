package readfile

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// fileFields are the JSON fields selectable via the --json flag.
var fileFields = []string{
	"name",
	"path",
	"gitSHA",
	"size",
	"url",
	"htmlUrl",
	"gitUrl",
	"downloadUrl",
	"type",
	"encoding",
	"content",
}

// ReadFileOptions holds the configuration for the read-file command.
type ReadFileOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Exporter   cmdutil.Exporter

	Path    string
	Ref     string
	Output  string
	Clobber bool

	AllowEscapeSequences bool
}

// NewCmdReadFile creates the `gh repo read-file` command.
func NewCmdReadFile(f *cmdutil.Factory, runF func(*ReadFileOptions) error) *cobra.Command {
	opts := &ReadFileOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "read-file <path> [flags]",
		Short: "Read a file from a repository (preview)",
		Long: heredoc.Docf(`
			Read the contents of a file in a GitHub repository without cloning it.

			This command is in preview and subject to change without notice.

			By default, the file is read from the default branch. Use the %[1]s--ref%[1]s flag to
			read from a specific branch, tag, or commit.

			When run in TTY mode, the content is shown through your pager. When stdout is piped or
			redirected, the raw content is written directly. To save the file to disk instead, use
			the %[1]s--output%[1]s flag.

			By default, the command refuses to output a file that contains terminal escape sequences,
			since they could manipulate your terminal. Pass %[1]s--allow-escape-sequences%[1]s to read the file anyway.
			This check applies only to terminal and piped output; writing to disk with %[1]s--output%[1]s always
			includes the raw bytes, as if %[1]s--allow-escape-sequences%[1]s were given.
		`, "`"),
		Example: heredoc.Doc(`
			# Read a file from the default branch
			$ gh repo read-file README.md --repo cli/cli

			# Read a file at a specific ref
			$ gh repo read-file README.md --repo cli/cli --ref v2.50.0

			# Save a file to disk
			$ gh repo read-file README.md --repo cli/cli --output download/README.md

			# Print selected fields as JSON
			$ gh repo read-file README.md --repo cli/cli --json name,path,size,type

			# Read a file that contains terminal escape sequences
			$ gh repo read-file path/to/file --repo OWNER/REPO --allow-escape-sequences
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.BaseRepo = f.BaseRepo

			opts.Path = args[0]

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--json` or `--output`",
				opts.Exporter != nil,
				opts.Output != "",
			); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			return readFileRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Ref, "ref", "", "The branch, tag, or commit to read from")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "Write the file to a `path` instead of stdout")
	cmd.Flags().BoolVar(&opts.Clobber, "clobber", false, "Overwrite the output path if it already exists")
	cmd.Flags().BoolVar(&opts.AllowEscapeSequences, "allow-escape-sequences", false, "Allow printing terminal escape sequences")

	cmdutil.AddJSONFlags(cmd, &opts.Exporter, fileFields)

	cmdutil.EnableRepoOverride(cmd, f)

	return cmd
}

func readFileRun(opts *ReadFileOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("%w. Run this command from within a git repository, or use the `--repo` flag to specify one", err)
	}

	file, err := fetchFile(httpClient, repo, opts.Path, opts.Ref)
	if err != nil {
		return err
	}

	// If the API didn't return the file content inline, it'll set the encoding to "none" (e.g. for large files).
	contentAvailable := file.Encoding != "none"

	if opts.Exporter != nil {
		// Only pay for the file content when the caller actually selected the content field.
		if !contentAvailable && slices.Contains(opts.Exporter.Fields(), "content") {
			if err := loadContent(httpClient, repo, file, opts.Ref); err != nil {
				return err
			}
		}
		return opts.Exporter.Write(opts.IO, file)
	}

	if !contentAvailable {
		if err := loadContent(httpClient, repo, file, opts.Ref); err != nil {
			return err
		}
	}

	if opts.Output != "" {
		dest, err := writeToOutput(file, opts.Output, opts.Clobber)
		if err != nil {
			return err
		}
		if opts.IO.IsStdoutTTY() {
			cs := opts.IO.ColorScheme()
			fmt.Fprintf(opts.IO.ErrOut, "%s Wrote %s to %s\n", cs.SuccessIcon(), file.Path, dest)
		}
		return nil
	}

	if len(file.Content) == 0 {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.ErrOut, "%s file is empty\n", cs.WarningIcon())
		return nil
	}

	if mime, ok := binaryContentType(file.Content); ok {
		if opts.IO.IsStdoutTTY() {
			return fmt.Errorf("binary file (%s, %s); use --output to save to a file or pipe stdout",
				mime, text.FormatSize(int64(file.Size)))
		}
		_, err = opts.IO.Out.Write(file.Content)
		return err
	}

	// Refuse terminal escape sequences unless --allow-escape-sequences, in both TTY and non-TTY modes,
	// so a malicious file cannot manipulate a downstream terminal.
	if !opts.AllowEscapeSequences && containsEscapeSequence(file.Content) {
		return errors.New("file contains terminal escape sequences; use --allow-escape-sequences to read anyway")
	}

	if opts.IO.IsStdoutTTY() {
		if err := opts.IO.StartPager(); err != nil {
			fmt.Fprintf(opts.IO.ErrOut, "error starting pager: %v\n", err)
		}
		defer opts.IO.StopPager()
	}

	_, err = opts.IO.Out.Write(file.Content)
	return err
}

// loadContent fetches the raw file bytes when the Contents API did not return them inline.
// The API only omits inline content for large files, which it marks with a "none" encoding;
// everything else (including empty files) comes back base64-encoded, so there is nothing to fetch.
func loadContent(httpClient *http.Client, repo ghrepo.Interface, file *repoFile, ref string) error {
	if file.Encoding != "none" {
		return nil
	}

	raw, err := fetchRawFile(httpClient, repo, file.Path, ref)
	if err != nil {
		return err
	}
	file.Content = raw
	return nil
}

// lstatResult captures the subset of output-path information the command needs,
// determined without following a symlink at the path.
type lstatResult struct {
	isDir     bool
	isSymlink bool
}

// lstat is a simplified abstraction around [os.Lstat] that reports whether the
// path is a directory or a symlink, using Lstat so that a symlink at the path
// is not followed.
func lstat(path string) (*lstatResult, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	return &lstatResult{
		isDir:     info.IsDir(),
		isSymlink: info.Mode()&os.ModeSymlink != 0,
	}, nil
}

// These indirect the file system operations used when writing output, so tests can
// substitute them without touching the real file system.
var (
	lstatF     = lstat
	mkdirAllF  = os.MkdirAll
	writeFileF = os.WriteFile
)

// writeToOutput writes file content to a local path and returns the final destination.
// A symlink target is refused, a directory target receives the file under its remote basename,
// and missing parent directories are created. Existing files are only overwritten when clobber is true.
func writeToOutput(file *repoFile, output string, clobber bool) (string, error) {
	dest := output

	// A trailing separator signals the user intends dest to be a directory even if it does not exist yet.
	asDir := strings.HasSuffix(dest, "/") || strings.HasSuffix(dest, string(os.PathSeparator))

	if lr, err := lstatF(dest); err == nil {
		if lr.isSymlink {
			return "", fmt.Errorf("output path is a symlink")
		}
		if lr.isDir {
			asDir = true
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if asDir {
		dest = filepath.Join(dest, file.Name)
	}

	if lr, err := lstatF(dest); err == nil {
		if lr.isSymlink {
			return "", fmt.Errorf("output path is a symlink")
		}
		if !clobber {
			return "", fmt.Errorf("output path already exists: %q (use --clobber to overwrite)", dest)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if dir := filepath.Dir(dest); dir != "" && dir != "." {
		if err := mkdirAllF(dir, 0755); err != nil {
			return "", err
		}
	}

	if err := writeFileF(dest, file.Content, 0644); err != nil {
		return "", err
	}

	return dest, nil
}

// binaryContentType reports whether content appears to be binary and, if so, returns
// its detected MIME type. Textual content returns ("", false).
func binaryContentType(content []byte) (string, bool) {
	if len(content) == 0 {
		return "", false
	}

	ct := http.DetectContentType(content)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}

	if strings.HasPrefix(ct, "text/") {
		return "", false
	}
	return ct, true
}

// containsEscapeSequence reports whether content contains an ANSI escape byte (0x1B),
// which could be used to manipulate the terminal when printed.
func containsEscapeSequence(content []byte) bool {
	return bytes.IndexByte(content, 0x1B) >= 0
}
