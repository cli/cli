package list

import (
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type ListOptions struct {
	Config func() (config.Config, error)
	IO     *iostreams.IOStreams
}

func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List your aliases",
		Aliases: []string{"ls"},
		Long: heredoc.Doc(`
			This command prints out all of the aliases gh is configured to use.
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	return cmd
}

func listRun(opts *ListOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	aliasCfg := cfg.Aliases()

	aliasMap := aliasCfg.All()

	if len(aliasMap) == 0 {
		return cmdutil.NewNoResultsError("no aliases configured")
	}

	enc := yaml.NewEncoder(opts.IO.Out)
	return enc.Encode(aliasMap)
}
