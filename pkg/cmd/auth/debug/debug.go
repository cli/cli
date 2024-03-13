package status

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type StatusOptions struct {
	IO     *iostreams.IOStreams
	Config func() (config.Config, error)
}

func NewCmdDebug(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "debug",
		Args:  cobra.ExactArgs(0),
		Short: "Debugs 8802",
		Long:  heredoc.Doc(`Debugs 8802`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return debugRun(opts)
		},
	}

	return cmd
}

func debugRun(opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	authCfg := cfg.Authentication()

	if _, err := authCfg.TokenFromKeyring("github.com"); err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, "everything seems good :)")
	return nil
}
