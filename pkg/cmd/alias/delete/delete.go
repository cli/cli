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
	All  bool
}

func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "delete {<alias> | --all}",
		Short: "Delete set aliases",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !opts.All {
				return cmdutil.FlagErrorf("specify an alias to delete or `--all`")
			}
			if len(args) > 0 && opts.All {
				return cmdutil.FlagErrorf("cannot use `--all` with alias name")
			}
			if len(args) > 1 {
				return cmdutil.FlagErrorf("too many arguments")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.All, "all", "a", false, "Delete all registered aliases")

	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	aliasCfg := cfg.Aliases()

	aliases := make(map[string]string)
	if opts.All {
		aliases = aliasCfg.All()
	} else {
		expansion, err := aliasCfg.Get(opts.Name)
		if err != nil {
			return fmt.Errorf("no such alias %s", opts.Name)
		}
		aliases[opts.Name] = expansion
	}

	for name, expansion := range aliases {
		err = aliasCfg.Delete(name)
		if err != nil {
			return fmt.Errorf("failed to delete alias %s: %w", name, err)
		}

		err = cfg.Write()
		if err != nil {
			return err
		}

		if opts.IO.IsStdoutTTY() {
			cs := opts.IO.ColorScheme()
			fmt.Fprintf(opts.IO.ErrOut, "%s Deleted alias %s; was %s\n", cs.SuccessIconWithColor(cs.Red), name, expansion)
		}
	}

	return nil
}
