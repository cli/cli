package delete

import (
	"fmt"
	"sort"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type DeleteOptions struct {
	Config func() (gh.Config, error)
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
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && !opts.All {
				return cmdutil.FlagErrorf("specify an alias to delete or `--all`")
			}
			if len(args) > 0 && opts.All {
				return cmdutil.FlagErrorf("cannot use `--all` with alias name")
			}
			if len(args) > 0 {
				opts.Name = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.All, "all", false, "Delete all aliases")

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
		if len(aliases) == 0 {
			return cmdutil.NewNoResultsError("no aliases configured")
		}
	} else {
		expansion, err := aliasCfg.Get(opts.Name)
		if err != nil {
			return fmt.Errorf("no such alias %s", opts.Name)
		}
		aliases[opts.Name] = expansion
	}

	for name := range aliases {
		if err := aliasCfg.Delete(name); err != nil {
			return fmt.Errorf("failed to delete alias %s: %w", name, err)
		}
	}

	if err := cfg.Write(); err != nil {
		return err
	}

	if opts.IO.IsStdoutTTY() {
		cs := opts.IO.ColorScheme()
		keys := make([]string, 0, len(aliases))
		for k := range aliases {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(opts.IO.ErrOut, "%s Deleted alias %s; was %s\n", cs.SuccessIconWithColor(cs.Red), k, aliases[k])
		}
	}

	return nil
}
