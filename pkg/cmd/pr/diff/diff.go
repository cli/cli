package diff

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"syscall"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DiffOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Finder shared.PRFinder

	SelectorArg string
	UseColor    bool
	Patch       bool
}

func NewCmdDiff(f *cmdutil.Factory, runF func(*DiffOptions) error) *cobra.Command {
	opts := &DiffOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	var colorFlag string

	cmd := &cobra.Command{
		Use:   "diff [<number> | <url> | <branch>]",
		Short: "View changes in a pull request",
		Long: heredoc.Doc(`
			View changes in a pull request. 

			Without an argument, the pull request that belongs to the current branch
			is selected.			
		`),
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
				return cmdutil.FlagErrorf("the value for `--color` must be one of \"auto\", \"always\", or \"never\"")
			}

			if runF != nil {
				return runF(opts)
			}
			return diffRun(opts)
		},
	}

	cmd.Flags().StringVar(&colorFlag, "color", "auto", "Use color in diff output: {always|never|auto}")
	cmd.Flags().BoolVar(&opts.Patch, "patch", false, "Display diff in patch format")

	return cmd
}

func diffRun(opts *DiffOptions) error {
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number"},
	}
	pr, baseRepo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	diff, err := fetchDiff(httpClient, baseRepo, pr.Number, opts.Patch)
	if err != nil {
		return fmt.Errorf("could not find pull request diff: %w", err)
	}
	defer diff.Close()

	err = opts.IO.StartPager()
	if err != nil {
		return err
	}
	defer opts.IO.StopPager()

	if !opts.UseColor {
		_, err = io.Copy(opts.IO.Out, diff)
		if errors.Is(err, syscall.EPIPE) {
			return nil
		}
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
