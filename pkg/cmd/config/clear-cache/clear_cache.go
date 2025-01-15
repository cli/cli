package clearcache

import (
	"fmt"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/config"
	"github.com/spf13/cobra"
)

type ClearCacheOptions struct {
	IO       *iostreams.IOStreams
	CacheDir string
}

func NewCmdConfigClearCache(f *cmdutil.Factory, runF func(*ClearCacheOptions) error) *cobra.Command {
	opts := &ClearCacheOptions{
		IO:       f.IOStreams,
		CacheDir: config.CacheDir(),
	}

	cmd := &cobra.Command{
		Use:   "clear-cache",
		Short: "Clear the cli cache",
		Example: heredoc.Doc(`
			# Clear the cli cache
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
	if err := os.RemoveAll(opts.CacheDir); err != nil {
		return err
	}
	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.Out, "%s Cleared the cache\n", cs.SuccessIcon())
	return nil
}
