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
	UseColor    string
}

func NewCmdDiff(f *cmdutil.Factory, runF func(*DiffOptions) error) *cobra.Command {
	opts := &DiffOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

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
				return &cmdutil.FlagError{Err: errors.New("argument required when using the --repo flag")}
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if !validColorFlag(opts.UseColor) {
				return &cmdutil.FlagError{Err: fmt.Errorf("did not understand color: %q. Expected one of always, never, or auto", opts.UseColor)}
			}

			if opts.UseColor == "auto" && !opts.IO.IsStdoutTTY() {
				opts.UseColor = "never"
			}

			if runF != nil {
				return runF(opts)
			}
			return diffRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.UseColor, "color", "auto", "Use color in diff output: {always|never|auto}")

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
	apiClient := api.NewClientFromHTTP(httpClient)

	diff, err := apiClient.PullRequestDiff(baseRepo, pr.Number)
	if err != nil {
		return fmt.Errorf("could not find pull request diff: %w", err)
	}
	defer diff.Close()

	err = opts.IO.StartPager()
	if err != nil {
		return err
	}
	defer opts.IO.StopPager()

	if opts.UseColor == "never" {
		_, err = io.Copy(opts.IO.Out, diff)
		if errors.Is(err, syscall.EPIPE) {
			return nil
		}
		return err
	}

	diffLines := bufio.NewScanner(diff)
	for diffLines.Scan() {
		diffLine := diffLines.Text()
		switch {
		case isHeaderLine(diffLine):
			fmt.Fprintf(opts.IO.Out, "\x1b[1;38m%s\x1b[m\n", diffLine)
		case isAdditionLine(diffLine):
			fmt.Fprintf(opts.IO.Out, "\x1b[32m%s\x1b[m\n", diffLine)
		case isRemovalLine(diffLine):
			fmt.Fprintf(opts.IO.Out, "\x1b[31m%s\x1b[m\n", diffLine)
		default:
			fmt.Fprintln(opts.IO.Out, diffLine)
		}
	}

	if err := diffLines.Err(); err != nil {
		return fmt.Errorf("error reading pull request diff: %w", err)
	}

	return nil
}

var diffHeaderPrefixes = []string{"+++", "---", "diff", "index"}

func isHeaderLine(dl string) bool {
	for _, p := range diffHeaderPrefixes {
		if strings.HasPrefix(dl, p) {
			return true
		}
	}
	return false
}

func isAdditionLine(dl string) bool {
	return strings.HasPrefix(dl, "+")
}

func isRemovalLine(dl string) bool {
	return strings.HasPrefix(dl, "-")
}

func validColorFlag(c string) bool {
	return c == "auto" || c == "always" || c == "never"
}
