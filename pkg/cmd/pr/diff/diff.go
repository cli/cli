package diff

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"golang.org/x/text/transform"
)

type DiffOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	Browser    browser.Browser

	Finder shared.PRFinder

	SelectorArg string
	UseColor    bool
	Patch       bool
	NameOnly    bool
	BrowserMode bool
}

func NewCmdDiff(f *cmdutil.Factory, runF func(*DiffOptions) error) *cobra.Command {
	opts := &DiffOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
	}

	var colorFlag string

	cmd := &cobra.Command{
		Use:   "diff [<number> | <url> | <branch>]",
		Short: "View changes in a pull request",
		Long: heredoc.Docf(`
			View changes in a pull request.

			Without an argument, the pull request that belongs to the current branch
			is selected.

			With %[1]s--web%[1]s flag, open the pull request diff in a web browser instead.
		`, "`"),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return cmdutil.FlagErrorf("argument required when using the `--repo` flag")
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			switch colorFlag {
			case "always":
				opts.UseColor = true
			case "auto":
				opts.UseColor = opts.IO.ColorEnabled()
			case "never":
				opts.UseColor = false
			default:
				return fmt.Errorf("unsupported color %q", colorFlag)
			}

			if runF != nil {
				return runF(opts)
			}
			return diffRun(opts)
		},
	}

	cmdutil.StringEnumFlag(cmd, &colorFlag, "color", "", "auto", []string{"always", "never", "auto"}, "Use color in diff output")
	cmd.Flags().BoolVar(&opts.Patch, "patch", false, "Display diff in patch format")
	cmd.Flags().BoolVar(&opts.NameOnly, "name-only", false, "Display only names of changed files")
	cmd.Flags().BoolVarP(&opts.BrowserMode, "web", "w", false, "Open the pull request diff in the browser")

	return cmd
}

func diffRun(opts *DiffOptions) error {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number"},
	}

	if opts.BrowserMode {
		findOptions.Fields = []string{"url"}
	}

	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	if opts.BrowserMode {
		openUrl := fmt.Sprintf("%s/files", pr.URL)
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(openUrl))
		}
		return opts.Browser.Browse(openUrl)
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	if opts.NameOnly {
		opts.Patch = false
	}

	diffReadCloser, err := fetchDiff(httpClient, baseRepo, pr.Number, opts.Patch)
	if err != nil {
		return fmt.Errorf("could not find pull request diff: %w", err)
	}
	defer diffReadCloser.Close()

	var diff io.Reader = diffReadCloser
	if opts.IO.IsStdoutTTY() {
		diff = sanitizedReader(diff)
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	if opts.NameOnly {
		return changedFilesNames(opts.IO.Out, diff)
	}

	if !opts.UseColor {
		_, err = io.Copy(opts.IO.Out, diff)
		return err
	}

	return colorDiffLines(opts.IO.Out, diff)
}

func fetchDiff(httpClient *http.Client, baseRepo ghrepo.Interface, prNumber int, asPatch bool) (io.ReadCloser, error) {
	url := fmt.Sprintf(
		"%srepos/%s/pulls/%d",
		ghinstance.RESTPrefix(baseRepo.RepoHost()),
		ghrepo.FullName(baseRepo),
		prNumber,
	)
	acceptType := "application/vnd.github.v3.diff"
	if asPatch {
		acceptType = "application/vnd.github.v3.patch"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", acceptType)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, api.HandleHTTPError(resp)
	}

	return resp.Body, nil
}

const lineBufferSize = 4096

var (
	colorHeader   = []byte("\x1b[1;38m")
	colorAddition = []byte("\x1b[32m")
	colorRemoval  = []byte("\x1b[31m")
	colorReset    = []byte("\x1b[m")
)

func colorDiffLines(w io.Writer, r io.Reader) error {
	diffLines := bufio.NewReaderSize(r, lineBufferSize)
	wasPrefix := false
	needsReset := false

	for {
		diffLine, isPrefix, err := diffLines.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("error reading pull request diff: %w", err)
		}

		var color []byte
		if !wasPrefix {
			if isHeaderLine(diffLine) {
				color = colorHeader
			} else if isAdditionLine(diffLine) {
				color = colorAddition
			} else if isRemovalLine(diffLine) {
				color = colorRemoval
			}
		}

		if color != nil {
			if _, err := w.Write(color); err != nil {
				return err
			}
			needsReset = true
		}

		if _, err := w.Write(diffLine); err != nil {
			return err
		}

		if !isPrefix {
			if needsReset {
				if _, err := w.Write(colorReset); err != nil {
					return err
				}
				needsReset = false
			}
			if _, err := w.Write([]byte{'\n'}); err != nil {
				return err
			}
		}
		wasPrefix = isPrefix
	}
	return nil
}

var diffHeaderPrefixes = []string{"+++", "---", "diff", "index"}

func isHeaderLine(l []byte) bool {
	dl := string(l)
	for _, p := range diffHeaderPrefixes {
		if strings.HasPrefix(dl, p) {
			return true
		}
	}
	return false
}

func isAdditionLine(l []byte) bool {
	return len(l) > 0 && l[0] == '+'
}

func isRemovalLine(l []byte) bool {
	return len(l) > 0 && l[0] == '-'
}

func changedFilesNames(w io.Writer, r io.Reader) error {
	diff, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	// This is kind of a gnarly regex. We're looking lines of the format:
	// diff --git a/9114-triage b/9114-triage
	// diff --git "a/hello-\360\237\230\200-world" "b/hello-\360\237\230\200-world"
	//
	// From these lines we would look to extract:
	// 9114-triage
	// "hello-\360\237\230\200-world"
	//
	// Note that the b/ is removed but in the second case the preceeding quote remains.
	// This is important for how git handles filenames that would be quoted with core.quotePath.
	// https://git-scm.com/docs/git-config#Documentation/git-config.txt-corequotePath
	//
	// Thus we capture the quote if it exists, and everything that follows the b/
	// We then concatenate those two capture groups together which for the examples above would be:
	// `` + 9114-triage
	// `"`` + hello-\360\237\230\200-world"
	//
	// Where I'm using the `` to indicate a string to avoid confusion with the " character.
	pattern := regexp.MustCompile(`(?:^|\n)diff\s--git.*\s(["]?)b/(.*)`)
	matches := pattern.FindAllStringSubmatch(string(diff), -1)

	for _, val := range matches {
		name := strings.TrimSpace(val[1] + val[2])
		if _, err := w.Write([]byte(name + "\n")); err != nil {
			return err
		}
	}

	return nil
}

func sanitizedReader(r io.Reader) io.Reader {
	return transform.NewReader(r, sanitizer{})
}

// sanitizer replaces non-printable characters with their printable representations
type sanitizer struct{ transform.NopResetter }

// Transform implements transform.Transformer.
func (t sanitizer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	for r, size := rune(0), 0; nSrc < len(src); {
		if r = rune(src[nSrc]); r < utf8.RuneSelf {
			size = 1
		} else if r, size = utf8.DecodeRune(src[nSrc:]); size == 1 && !atEOF && !utf8.FullRune(src[nSrc:]) {
			// Invalid rune.
			err = transform.ErrShortSrc
			break
		}

		if isPrint(r) {
			if nDst+size > len(dst) {
				err = transform.ErrShortDst
				break
			}
			for i := 0; i < size; i++ {
				dst[nDst] = src[nSrc]
				nDst++
				nSrc++
			}
			continue
		} else {
			nSrc += size
		}

		replacement := fmt.Sprintf("\\u{%02x}", r)

		if nDst+len(replacement) > len(dst) {
			err = transform.ErrShortDst
			break
		}

		for _, c := range replacement {
			dst[nDst] = byte(c)
			nDst++
		}
	}
	return
}

// isPrint reports if a rune is safe to be printed to a terminal
func isPrint(r rune) bool {
	return r == '\n' || r == '\r' || r == '\t' || unicode.IsPrint(r)
}
