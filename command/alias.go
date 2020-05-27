package command

import (
	"fmt"
	"strings"

	"github.com/cli/cli/utils"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
)

// TODO
// - [ ] DEBUG support
// - [ ] blank aliases stanza
// - [x] give our commands precedence
// - [x] prevent overriding existing gh command
// - [x] allow overwriting alias
// - [x] forward extra arguments
// - [x] allow duplication of placeholder

func init() {
	RootCmd.AddCommand(aliasCmd)
	aliasCmd.AddCommand(aliasSetCmd)
}

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Create shortcuts for gh commands",
}

var aliasSetCmd = &cobra.Command{
	Use: "set <alias> <expansion>",
	// TODO HACK: even when inside of a single-quoted string, cobra was noticing and parsing any flags
	// used in an alias expansion string argument. Since this command needs no flags, I disabled their
	// parsing. If we ever want to add flags to alias set we'll have to figure this out. I haven't
	// checked if this is fixed in a new cobra.
	DisableFlagParsing: true,
	Short:              "Create a shortcut for a gh command",
	Long: `
This command lets you write your own shortcuts for running gh. They can be simple strings or accept placeholder arguments.`,
	Example: `
	gh alias set pv 'pr view'
	# gh pv -w 123 -> gh pr view -w 123.

	gh alias set bugs 'issue list --label="bugs"'
	# gh bugs -> gh issue list --label="bugs".

	gh alias set epicsBy 'issue list --author="$1" --label="epic"'
	# gh epicsBy vilmibm -> gh issue list --author="$1" --label="epic"
`,
	Args: cobra.MinimumNArgs(2),
	RunE: aliasSet,
}

func aliasSet(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	cfg, err := ctx.Config()
	if err != nil {
		return err
	}

	aliasCfg, err := cfg.Aliases()
	if err != nil {
		return err
	}

	alias := args[0]
	expansion := processArgs(args[1:])

	out := colorableOut(cmd)
	fmt.Fprintf(out, "- Adding alias for %s: %s\n", utils.Bold(alias), utils.Bold(expansion))

	if validCommand(alias) {
		return fmt.Errorf("could not create alias: %q is already a gh command", alias)
	}

	if !validCommand(expansion) {
		return fmt.Errorf("could not create alias: %s does not correspond to a gh command", utils.Bold(expansion))
	}

	successMsg := fmt.Sprintf("%s Added alias.", utils.Green("✓"))

	if aliasCfg.Exists(alias) {
		successMsg = fmt.Sprintf("%s Changed alias %s from %s to %s",
			utils.Green("✓"),
			utils.Bold(alias),
			utils.Bold(aliasCfg.Get(alias)),
			utils.Bold(expansion),
		)
	}

	err = aliasCfg.Add(alias, expansion)
	if err != nil {
		return fmt.Errorf("could not create alias: %s", err)
	}

	fmt.Fprintln(out, successMsg)

	return nil
}

func validCommand(expansion string) bool {
	split, _ := shlex.Split(expansion)
	cmd, _, err := RootCmd.Traverse(split)
	return err == nil && cmd != RootCmd
}

func processArgs(args []string) string {
	if len(args) == 1 {
		return args[0]
	}

	return strings.Join(args, " ")
}
