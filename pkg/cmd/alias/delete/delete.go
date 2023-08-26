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
		Use:   "delete <alias>",
		Short: "Delete an alias",
		Args:  cobra.MaximumNArgs(1),
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

	var names, expansions []string
	if opts.All {
		aliases := aliasCfg.All()
		for name, expansion := range aliases {
			names = append(names, name)
			expansions = append(expansions, expansion)
		}
	} else {
		expansion, err := aliasCfg.Get(opts.Name)
		if err != nil {
			return fmt.Errorf("no such alias %s", opts.Name)
		}
		names = append(names, opts.Name)
		expansions = append(expansions, expansion)
	}

	for i := 0; i < len(names); i++ {
		err = aliasCfg.Delete(names[i])
		if err != nil {
			return fmt.Errorf("failed to delete alias %s: %w", names[i], err)
		}

		err = cfg.Write()
		if err != nil {
			return err
		}

		if opts.IO.IsStdoutTTY() {
			cs := opts.IO.ColorScheme()
			fmt.Fprintf(opts.IO.ErrOut, "%s Deleted alias %s; was %s\n", cs.SuccessIconWithColor(cs.Red), names[i], expansions[i])
		}
	}

	return nil
}
