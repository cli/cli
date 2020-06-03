package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cli/cli/utils"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(aliasCmd)
	aliasCmd.AddCommand(aliasSetCmd)
	aliasCmd.AddCommand(aliasListCmd)
	aliasCmd.AddCommand(aliasDeleteCmd)
}

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Create shortcuts for gh commands",
}

var aliasSetCmd = &cobra.Command{
	Use: "set <alias> <expansion>",
	// NB: this allows a user to eschew quotes when specifiying an alias expansion. We'll have to
	// revisit it if we ever want to add flags to alias set but we have no current plans for that.
	DisableFlagParsing: true,
	Short:              "Create a shortcut for a gh command",
	Long:               `This command lets you write your own shortcuts for running gh. They can be simple strings or accept placeholder arguments.`,
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

	expansionStr := strings.Join(expansion, " ")

	out := colorableOut(cmd)
	fmt.Fprintf(out, "- Adding alias for %s: %s\n", utils.Bold(alias), utils.Bold(expansionStr))

	if validCommand([]string{alias}) {
		return fmt.Errorf("could not create alias: %q is already a gh command", alias)
	}

	if !validCommand(expansion) {
		return fmt.Errorf("could not create alias: %s does not correspond to a gh command", utils.Bold(expansionStr))
	}

	successMsg := fmt.Sprintf("%s Added alias.", utils.Green("✓"))

	if aliasCfg.Exists(alias) {
		successMsg = fmt.Sprintf("%s Changed alias %s from %s to %s",
			utils.Green("✓"),
			utils.Bold(alias),
			utils.Bold(aliasCfg.Get(alias)),
			utils.Bold(expansionStr),
		)
	}

	err = aliasCfg.Add(alias, expansionStr)
	if err != nil {
		return fmt.Errorf("could not create alias: %s", err)
	}

	fmt.Fprintln(out, successMsg)

	return nil
}

func validCommand(expansion []string) bool {
	cmd, _, err := RootCmd.Traverse(expansion)
	return err == nil && cmd != RootCmd
}

func processArgs(args []string) []string {
	if len(args) == 1 {
		split, _ := shlex.Split(args[0])
		return split
	}

	newArgs := []string{}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") && strings.Contains(a, " ") {
			a = fmt.Sprintf("%q", a)
		}
		newArgs = append(newArgs, a)
	}

	return newArgs
}

var aliasListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your aliases",
	Long:  `This command prints out all of the aliases gh is configured to use.`,
	Args:  cobra.ExactArgs(0),
	RunE:  aliasList,
}

func aliasList(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	cfg, err := ctx.Config()
	if err != nil {
		return fmt.Errorf("couldn't read config: %w", err)
	}

	aliasCfg, err := cfg.Aliases()
	if err != nil {
		return fmt.Errorf("couldn't read aliases config: %w", err)
	}

	stderr := colorableErr(cmd)

	if aliasCfg.Empty() {
		fmt.Fprintf(stderr, "no aliases configured\n")
		return nil
	}

	stdout := colorableOut(cmd)

	tp := utils.NewTablePrinter(stdout)

	aliasMap := aliasCfg.All()
	keys := []string{}
	for alias := range aliasMap {
		keys = append(keys, alias)
	}
	sort.Strings(keys)

	for _, alias := range keys {
		if tp.IsTTY() {
			// ensure that screen readers pause
			tp.AddField(alias+":", nil, nil)
		} else {
			tp.AddField(alias, nil, nil)
		}
		tp.AddField(aliasMap[alias], nil, nil)
		tp.EndRow()
	}

	return tp.Render()
}

var aliasDeleteCmd = &cobra.Command{
	Use:     "delete <alias>",
	Short:   "Delete an alias.",
	Args:    cobra.ExactArgs(1),
	Example: "gh alias delete co",
	RunE:    aliasDelete,
}

func aliasDelete(cmd *cobra.Command, args []string) error {
	alias := args[0]

	ctx := contextForCommand(cmd)
	cfg, err := ctx.Config()
	if err != nil {
		return fmt.Errorf("couldn't read config: %w", err)
	}

	aliasCfg, err := cfg.Aliases()
	if err != nil {
		return fmt.Errorf("couldn't read aliases config: %w", err)
	}

	if !aliasCfg.Exists(alias) {
		return fmt.Errorf("no such alias %s", alias)
	}

	expansion := aliasCfg.Get(alias)

	err = aliasCfg.Delete(alias)
	if err != nil {
		return fmt.Errorf("failed to delete alias %s: %w", alias, err)
	}

	out := colorableOut(cmd)
	redCheck := utils.Red("✓")
	fmt.Fprintf(out, "%s Deleted alias %s; was %s\n", redCheck, alias, expansion)

	return nil
}
