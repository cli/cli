package download

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// DownloadOptions holds the flags and dependencies of gh issue artifact
// download.
type DownloadOptions struct {
	IO       *iostreams.IOStreams
	BaseRepo func() (ghrepo.Interface, error)
	Client   func() (client.ArtifactClient, error)

	IssueNumber int
	// ArtifactNumbers are the artifacts to download, in order. Empty
	// downloads every artifact on the issue.
	ArtifactNumbers []int
	// Dir is the directory files are written to when Output is empty.
	Dir string
	// Output is the path to write the one artifact to, or "-" for standard
	// output.
	Output string
	// Clobber overwrites existing files, and SkipExisting leaves them alone.
	Clobber      bool
	SkipExisting bool
	// Version is the version to download from the edit history. 0 downloads
	// the current version.
	Version int
}

// NewCmdDownload creates the gh issue artifact download command.
func NewCmdDownload(f *cmdutil.Factory, runF func(*DownloadOptions) error) *cobra.Command {
	opts := &DownloadOptions{
		IO: f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "download {<issue-number> | <issue-url>} [<artifact-number>...]",
		Short: "Download issue artifacts (preview)",
		Long: heredoc.Docf(`
			Download issue artifacts to files. Without artifact numbers, every artifact
			on the issue is downloaded.

			Each file is named %[1]s<number>-<name>%[1]s. Documents end in %[1]s.md%[1]s, and links are
			saved as %[1]s.url%[1]s Internet Shortcut files. If a file already exists, that
			artifact fails unless you use %[1]s--clobber%[1]s to overwrite it or %[1]s--skip-existing%[1]s
			to skip it.

			Use %[1]s--output%[1]s to choose the path for a single artifact, or %[1]s--output -%[1]s to
			print its content. Use %[1]s--version%[1]s with a single artifact to download an
			earlier version from its edit history.
		`, "`"),
		Example: heredoc.Doc(`
			# Download every artifact on issue 142
			$ gh issue artifact download 142

			# Download two artifacts into a folder
			$ gh issue artifact download 142 2 5 --dir notes

			# Print one artifact's content
			$ gh issue artifact download 142 2 --output -

			# Download an earlier version of one artifact
			$ gh issue artifact download 142 2 --version 5
		`),
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive("specify only one of `--clobber` or `--skip-existing`", opts.Clobber, opts.SkipExisting); err != nil {
				return err
			}
			outputSet := cmd.Flags().Changed("output")
			if err := cmdutil.MutuallyExclusive("specify only one of `--dir` or `--output`", cmd.Flags().Changed("dir"), outputSet); err != nil {
				return err
			}
			// A blank path would otherwise download every artifact under
			// its usual name.
			if outputSet && opts.Output == "" {
				return cmdutil.FlagErrorf("--output cannot be blank")
			}
			versionSet := cmd.Flags().Changed("version")
			if versionSet && opts.Version < 1 {
				return cmdutil.FlagErrorf("invalid version: %v", opts.Version)
			}

			var err error
			opts.IssueNumber, opts.BaseRepo, err = shared.ParseIssueArg(args[0], f.BaseRepo)
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}
			opts.ArtifactNumbers = make([]int, 0, len(args)-1)
			for _, arg := range args[1:] {
				number, err := shared.ParseArtifactNumber(arg)
				if err != nil {
					return cmdutil.FlagErrorWrap(err)
				}
				opts.ArtifactNumbers = append(opts.ArtifactNumbers, number)
			}

			// One path holds one file, and each artifact numbers its own
			// versions, so both flags need exactly one artifact.
			if outputSet && len(opts.ArtifactNumbers) != 1 {
				return cmdutil.FlagErrorf("--output requires exactly one artifact number")
			}
			if versionSet && len(opts.ArtifactNumbers) != 1 {
				return cmdutil.FlagErrorf("--version requires exactly one artifact number")
			}
			opts.Client = shared.ClientFunc(f)

			if runF != nil {
				return runF(opts)
			}
			return downloadRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Dir, "dir", "D", ".", "The `directory` to download files into")
	cmd.Flags().StringVarP(&opts.Output, "output", "O", "", "The `file` to write a single artifact to (use \"-\" to write to standard output)")
	cmd.Flags().BoolVar(&opts.Clobber, "clobber", false, "Overwrite existing files of the same name")
	cmd.Flags().BoolVar(&opts.SkipExisting, "skip-existing", false, "Skip downloading when files of the same name exist")
	cmd.Flags().IntVar(&opts.Version, "version", 0, "Download the version with this `number` from the edit history")

	return cmd
}

func downloadRun(opts *DownloadOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	if err := shared.CheckHost(repo.RepoHost()); err != nil {
		return err
	}

	c, err := opts.Client()
	if err != nil {
		return err
	}
	if err := shared.CheckIssue(c, repo, opts.IssueNumber); err != nil {
		return err
	}

	if opts.Output == "-" {
		return printBody(opts, c, repo)
	}

	w := &fileWriter{
		dir:          opts.Dir,
		output:       opts.Output,
		clobber:      opts.Clobber,
		skipExisting: opts.SkipExisting,
	}
	defer w.Close()

	failed := false
	if len(opts.ArtifactNumbers) == 0 {
		// The list has every artifact's body, so nothing else is requested.
		artifacts, err := c.List(repo, opts.IssueNumber, "", 0)
		if err != nil {
			return err
		}
		if len(artifacts) == 0 {
			return fmt.Errorf("no artifacts to download from %s#%d", ghrepo.FullName(repo), opts.IssueNumber)
		}
		for _, a := range artifacts {
			if !downloadArtifact(opts, w, &client.ArtifactWithVersions{Artifact: a}) {
				failed = true
			}
		}
	} else {
		for _, number := range opts.ArtifactNumbers {
			a, err := c.Get(repo, opts.IssueNumber, number)
			if err != nil {
				shared.PrintFailure(opts.IO, shared.Result{Number: number}, downloadAction(number), err)
				failed = true
				continue
			}
			if !downloadArtifact(opts, w, a) {
				failed = true
			}
		}
	}

	if failed {
		return cmdutil.SilentError
	}
	return nil
}

// printBody writes one artifact's body to standard output with nothing else,
// so a script gets the body as it's stored. A link's body is its URL. There is
// no result line, so a failure is an ordinary error.
func printBody(opts *DownloadOptions, c client.ArtifactClient, repo ghrepo.Interface) error {
	a, err := c.Get(repo, opts.IssueNumber, opts.ArtifactNumbers[0])
	if err != nil {
		return err
	}
	_, body, err := selectVersion(a, opts.Version)
	if err != nil {
		return err
	}
	_, err = io.WriteString(opts.IO.Out, body)
	return err
}

// downloadArtifact writes one artifact's file and prints its result line. It
// reports false when the artifact failed.
func downloadArtifact(opts *DownloadOptions, w *fileWriter, a *client.ArtifactWithVersions) bool {
	name, body, err := selectVersion(a, opts.Version)
	var fileName string
	if err == nil {
		fileName, err = artifactFileName(a.Number, name, a.Type)
	}
	if err != nil {
		shared.PrintFailure(opts.IO, shared.Result{Number: a.Number}, downloadAction(a.Number), err)
		return false
	}

	path := w.path(fileName)
	result := shared.Result{Number: a.Number, Name: path}
	skipped, err := w.write(path, a.Artifact, body)
	switch {
	case err != nil:
		shared.PrintFailure(opts.IO, result, "write "+path, withoutPath(err, path))
		return false
	case skipped:
		shared.PrintSkip(opts.IO, result, path, "already exists")
	default:
		shared.PrintSuccess(opts.IO, "written", result, opts.IO.ColorScheme().SuccessIcon(), path)
	}
	return true
}

func downloadAction(number int) string {
	return fmt.Sprintf("download artifact %d", number)
}

// selectVersion returns the name and body to download: the current
// version's, or with --version, that version's. A type gh doesn't know is
// refused first, like view does.
func selectVersion(a *client.ArtifactWithVersions, version int) (string, string, error) {
	if err := shared.CheckType(a.Artifact); err != nil {
		return "", "", err
	}
	v, err := shared.FindVersion(a, version)
	if err != nil {
		return "", "", err
	}
	if v == nil {
		return a.Name, a.Body, nil
	}
	return v.Name, v.Body, nil
}

// maxFileNameBytes is the longest file name most file systems allow.
const maxFileNameBytes = 255

// artifactFileName names an artifact's file <number>-<name>, plus .md, or
// .url for a link, unless the name already ends with it in any case. Only
// what would cause a problem changes, the same way on every OS, so an
// artifact gets the same file name everywhere: separators, control
// characters, whitespace and the characters Windows forbids each become "-".
// The number keeps a name such as CON from being a Windows device name. The
// name is shortened, at a character boundary, only when the file name would
// pass maxFileNameBytes.
func artifactFileName(number int, name, artifactType string) (string, error) {
	ext := ".md"
	if artifactType == client.TypeLink {
		ext = ".url"
	}

	stem := strings.Map(func(r rune) rune {
		if unsafeInFileName(r) {
			return '-'
		}
		return r
	}, name)
	if cut := len(stem) - len(ext); cut >= 0 && strings.EqualFold(stem[cut:], ext) {
		stem, ext = stem[:cut], stem[cut:]
	}

	prefix := strconv.Itoa(number) + "-"
	if limit := maxFileNameBytes - len(prefix) - len(ext); len(stem) > limit {
		for limit > 0 && !utf8.RuneStart(stem[limit]) {
			limit--
		}
		stem = stem[:limit]
	}

	// The rule above leaves no separator, so this can't fail today. It keeps a
	// later change to the rule from writing anywhere but directly in the
	// target directory.
	fileName := prefix + stem + ext
	if !filepath.IsLocal(fileName) || filepath.Base(fileName) != fileName {
		return "", fmt.Errorf("unsafe file name %q", fileName)
	}
	return fileName, nil
}

func unsafeInFileName(r rune) bool {
	switch r {
	case '/', '\\', '<', '>', ':', '"', '|', '?', '*':
		return true
	}
	return unicode.IsControl(r) || unicode.IsSpace(r)
}

// fileWriter writes downloaded files, in Dir under their file names or at
// Output. It opens the directory it writes to at the first write, so a
// download that writes nothing creates no directory.
type fileWriter struct {
	dir          string
	output       string
	clobber      bool
	skipExisting bool

	root    *safepaths.Root
	rootErr error
	opened  bool
}

// Close closes the directory the writer opened, if any.
func (w *fileWriter) Close() error {
	if w.root == nil {
		return nil
	}
	return w.root.Close()
}

// path returns where an artifact's file goes: in Dir under its file name, or
// at Output. Like gh repo read-file --output, an existing directory, or a path
// ending in a separator, gets the file under its file name. Stat follows a
// symlink, so a symlink to a directory counts as a directory, like any parent
// directory.
func (w *fileWriter) path(fileName string) string {
	if w.output == "" {
		return filepath.Join(w.dir, fileName)
	}
	if strings.HasSuffix(w.output, "/") || strings.HasSuffix(w.output, string(filepath.Separator)) {
		return filepath.Join(w.output, fileName)
	}
	if info, err := os.Stat(w.output); err == nil && info.IsDir() {
		return filepath.Join(w.output, fileName)
	}
	return w.output
}

var errAlreadyExists = errors.New("already exists (use --clobber to overwrite or --skip-existing to skip)")

// afterCheck runs after write checks a file's path and before it writes the
// file. Tests replace it to change the file system in between, as another
// process could.
var afterCheck = func() {}

// write writes an artifact's file at path, and reports whether it skipped path
// instead. It checks path first, as gh release download does before
// downloading: something already there fails unless skipExisting skips it, or
// clobber overwrites it, which only works on a regular file. gh never writes
// through a symlink at path, even with clobber, but the directory and its
// parents may be symlinks, and missing ones are created.
func (w *fileWriter) write(path string, a client.Artifact, body string) (bool, error) {
	if info, err := os.Lstat(path); err == nil {
		switch {
		case w.skipExisting:
			return true, nil
		case info.Mode()&fs.ModeSymlink != 0:
			return false, errors.New("is a symlink")
		case !info.Mode().IsRegular():
			return false, errors.New("is not a regular file")
		case !w.clobber:
			return false, errAlreadyExists
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}

	content, err := fileContent(a, body)
	if err != nil {
		return false, err
	}

	afterCheck()
	root, err := w.openDir(path)
	if err != nil {
		return false, err
	}
	// safepaths creates a new file exclusively and never follows a symlink at
	// path, so something put there after the check, even a symlink, fails like
	// a file that was there before. A single name makes no directories, so
	// that's the only error saying the name exists. With clobber, safepaths
	// truncates a regular file, and replaces a symlink put there after the
	// check unless it points to a directory or outside the directory, which it
	// refuses.
	f, err := root.Create(filepath.Base(path), 0o644, 0o755, w.clobber)
	if errors.Is(err, fs.ErrExist) && !w.clobber {
		if w.skipExisting {
			return true, nil
		}
		return false, errAlreadyExists
	}
	if err != nil {
		return false, err
	}
	_, err = f.Write(content)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return false, err
}

// openDir opens the directory the file at path goes in, through safepaths,
// creating it and any missing parents: Dir, or with Output, the file's parent
// directory. Every file goes in the same directory, so it's opened once and
// closed by Close. An error opening it is returned as it is: it isn't about
// the file, even when it says a name exists, as it does when the directory is
// a dangling symlink.
func (w *fileWriter) openDir(path string) (*safepaths.Root, error) {
	if !w.opened {
		w.opened = true
		dir := w.dir
		if w.output != "" {
			dir = filepath.Dir(path)
		}
		w.root, w.rootErr = safepaths.OpenRootDir(dir, 0o755)
	}
	return w.root, w.rootErr
}

// fileContent returns what an artifact's file holds: a document's body as it
// is, or an Internet Shortcut for a link. gh never writes a shortcut for
// anything but an http(s) URL.
func fileContent(a client.Artifact, body string) ([]byte, error) {
	if a.Type != client.TypeLink {
		return []byte(body), nil
	}
	u, ok := shared.HTTPURL(body)
	if !ok {
		return nil, fmt.Errorf("artifact %d doesn't link to an http(s) URL", a.Number)
	}
	return []byte("[InternetShortcut]\r\nURL=" + u + "\r\n"), nil
}

// withoutPath drops the path from a file system error about the file itself,
// since the result line already names it. Such an error can name the file by
// the path given, by its name in the directory opened for it, or by the open
// file's full path, so only the last element is compared.
func withoutPath(err error, path string) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok && filepath.Base(pathErr.Path) == filepath.Base(path) {
		return pathErr.Err
	}
	return err
}
