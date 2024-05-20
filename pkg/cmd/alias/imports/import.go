package imports

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/alias/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type ImportOptions struct {
	Config func() (gh.Config, error)
	IO     *iostreams.IOStreams

	Filename          string
	OverwriteExisting bool

	validAliasName      func(string) bool
	validAliasExpansion func(string) bool
}

func NewCmdImport(f *cmdutil.Factory, runF func(*ImportOptions) error) *cobra.Command {
	opts := &ImportOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "import [<filename> | -]",
		Short: "Import aliases from a YAML file",
		Long: heredoc.Docf(`
			Import aliases from the contents of a YAML file.

			Aliases should be defined as a map in YAML, where the keys represent aliases and
			the values represent the corresponding expansions. An example file should look like
			the following:

			    bugs: issue list --label=bug
			    igrep: '!gh issue list --label="$1" | grep "$2"'
			    features: |-
			        issue list
			        --label=enhancement

			Use %[1]s-%[1]s to read aliases (in YAML format) from standard input.

			The output from %[1]sgh alias list%[1]s can be used to produce a YAML file
			containing your aliases, which you can use to import them from one machine to
			another. Run %[1]sgh help alias list%[1]s to learn more.
		`, "`"),
		Example: heredoc.Doc(`
			# Import aliases from a file
			$ gh alias import aliases.yml

			# Import aliases from standard input
			$ gh alias import -
		`),
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return cmdutil.FlagErrorf("too many arguments")
			}
			if len(args) == 0 && opts.IO.IsStdinTTY() {
				return cmdutil.FlagErrorf("no filename passed and nothing on STDIN")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Filename = "-"
			if len(args) > 0 {
				opts.Filename = args[0]
			}

			opts.validAliasName = shared.ValidAliasNameFunc(cmd)
			opts.validAliasExpansion = shared.ValidAliasExpansionFunc(cmd)

			if runF != nil {
				return runF(opts)
			}

			return importRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.OverwriteExisting, "clobber", false, "Overwrite existing aliases of the same name")

	return cmd
}

func importRun(opts *ImportOptions) error {
	cs := opts.IO.ColorScheme()
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	aliasCfg := cfg.Aliases()

	b, err := cmdutil.ReadFile(opts.Filename, opts.IO.In)
	if err != nil {
		return err
	}

	aliasMap := map[string]string{}
	if err = yaml.Unmarshal(b, &aliasMap); err != nil {
		return err
	}

	isTerminal := opts.IO.IsStdoutTTY()
	if isTerminal {
		if opts.Filename == "-" {
			fmt.Fprintf(opts.IO.ErrOut, "- Importing aliases from standard input\n")
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "- Importing aliases from file %q\n", opts.Filename)
		}
	}

	var msg strings.Builder

	for _, alias := range getSortedKeys(aliasMap) {
		var existingAlias bool
		if _, err := aliasCfg.Get(alias); err == nil {
			existingAlias = true
		}

		if !opts.validAliasName(alias) {
			if !existingAlias {
				msg.WriteString(
					fmt.Sprintf("%s Could not import alias %s: already a gh command or extension\n",
						cs.FailureIcon(),
						cs.Bold(alias),
					),
				)
				continue
			}

			if existingAlias && !opts.OverwriteExisting {
				msg.WriteString(
					fmt.Sprintf("%s Could not import alias %s: name already taken\n",
						cs.FailureIcon(),
						cs.Bold(alias),
					),
				)
				continue
			}
		}

		expansion := aliasMap[alias]

		if !opts.validAliasExpansion(expansion) {
			msg.WriteString(
				fmt.Sprintf("%s Could not import alias %s: expansion does not correspond to a gh command, extension, or alias\n",
					cs.FailureIcon(),
					cs.Bold(alias),
				),
			)
			continue
		}

		aliasCfg.Add(alias, expansion)

		if existingAlias && opts.OverwriteExisting {
			msg.WriteString(
				fmt.Sprintf("%s Changed alias %s\n",
					cs.WarningIcon(),
					cs.Bold(alias),
				),
			)
		} else {
			msg.WriteString(
				fmt.Sprintf("%s Added alias %s\n",
					cs.SuccessIcon(),
					cs.Bold(alias),
				),
			)
		}
	}

	if err := cfg.Write(); err != nil {
		return err
	}

	if isTerminal {
		fmt.Fprintln(opts.IO.ErrOut, msg.String())
	}

	return nil
}

func getSortedKeys(m map[string]string) []string {
	keys := []string{}
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
