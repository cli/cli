package delete

import (
	"fmt"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	Config func() (config.Config, error)
	IO     *iostreams.IOStreams

	Name string
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	var deleteAll bool
	opts := &DeleteOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "delete <alias>",
		Short: "Delete an alias",
		Args:  cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) error {

			if deleteAll {
				return deleteAllAliases(opts)
			} else {

				opts.Name = args[0]
				if runF != nil {
					return runF(opts)
				}
				return deleteRun(opts)
			}

		},
	}
	cmd.Flags().BoolVar(&deleteAll, "all", false, "deletes all aliases")
	return cmd
}

func deleteAllAliases(opts *DeleteOptions) error {
	cfg, err := opts.Config()

	if err != nil {
		return err
	}

	aliasCfg := cfg.Aliases()

	out := aliasCfg.All()

	for alias, expansion := range out {
		err = aliasCfg.Delete(alias)

		if err != nil {
			return fmt.Errorf("failed to delete alias %s: %w", alias, err)
		}

		err = cfg.Write()
		if err != nil {
			return err
		}

		if opts.IO.IsStdoutTTY() {
			cs := opts.IO.ColorScheme()
			fmt.Fprintf(opts.IO.ErrOut, "%s Deleted alias %s; was %s\n", cs.SuccessIconWithColor(cs.Red), alias, expansion)
		}
	}
	return nil
}

func deleteRun(opts *DeleteOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	aliasCfg := cfg.Aliases()

	expansion, err := aliasCfg.Get(opts.Name)
	if err != nil {
		return fmt.Errorf("no such alias %s", opts.Name)

	}

	err = aliasCfg.Delete(opts.Name)
	if err != nil {
		return fmt.Errorf("failed to delete alias %s: %w", opts.Name, err)
	}

	err = cfg.Write()
	if err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.ErrOut, "%s Deleted alias %s; was %s\n", cs.SuccessIconWithColor(cs.Red), opts.Name, expansion)
	}

	return nil
}
