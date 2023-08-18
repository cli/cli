package clearcache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ClearCacheOptions struct {
	IO       *iostreams.IOStreams
	CacheDir string
}

func NewCmdConfigClearCache(f *cmdutil.Factory, runF func(*ClearCacheOptions) error) *cobra.Command {
	opts := &ClearCacheOptions{
		IO:       f.IOStreams,
		CacheDir: filepath.Join(os.TempDir(), "gh-cli-cache"),
	}

	cmd := &cobra.Command{
		Use:   "clear-cache",
		Short: "Clear the gh cli cache",
		Example: heredoc.Doc(`
			$ gh config clear-cache
		`),
		Args: cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			if runF != nil {
				return runF(opts)
			}

			return clearCacheRun(opts)
		},
	}

	return cmd
}

func clearCacheRun(opts *ClearCacheOptions) error {
	err := os.RemoveAll(opts.CacheDir)

	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.Out, "Cleared cache dir: %s\n", opts.CacheDir)

	return nil
}
