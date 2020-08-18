package delete

import (
	"fmt"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	Config func() (config.Config, error)
	IO     *iostreams.IOStreams

	Name string
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "delete <alias>",
		Short: "Delete an alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Name = args[0]

			if runF != nil {
				return runF(opts)
			}
			return deleteRun(opts)
		},
	}

	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	aliasCfg, err := cfg.Aliases()
	if err != nil {
		return fmt.Errorf("couldn't read aliases config: %w", err)
	}

	expansion, ok := aliasCfg.Get(opts.Name)
	if !ok {
		return fmt.Errorf("no such alias %s", opts.Name)

	}

	err = aliasCfg.Delete(opts.Name)
	if err != nil {
		return fmt.Errorf("failed to delete alias %s: %w", opts.Name, err)
	}

	if opts.IO.IsStdoutTTY() {
		redCheck := utils.Red("✓")
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted alias %s; was %s\n", redCheck, opts.Name, expansion)
	}

	return nil
}
