package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/utils"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(aliasCmd)
	aliasCmd.AddCommand(aliasSetCmd)
	aliasCmd.AddCommand(aliasListCmd)
	aliasCmd.AddCommand(aliasDeleteCmd)

	aliasSetCmd.Flags().BoolP("shell", "s", false, "Declare an alias to be passed through a shell interpreter")
}

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Create command shortcuts",
	Long: heredoc.Doc(`
	Aliases can be used to make shortcuts for gh commands or to compose multiple commands.

	Run "gh help alias set" to learn more.
	`),
}

var aliasSetCmd = &cobra.Command{
	Use:   "set <alias> <expansion>",
	Short: "Create a shortcut for a gh command",
	Long: heredoc.Doc(`
	Declare a word as a command alias that will expand to the specified command(s).

	The expansion may specify additional arguments and flags. If the expansion
	includes positional placeholders such as '$1', '$2', etc., any extra arguments
	that follow the invocation of an alias will be inserted appropriately.

	If '--shell' is specified, the alias will be run through a shell interpreter (sh or pwsh). This allows you
	to compose commands with "|" or redirect output. Note that extra arguments are not passed to
	shell-interpreted aliases; only placeholders ("$1", "$2" on *nix, "$args" in powershell) are
	supported.

	Quotes must always be used when defining a command as in the examples.`),
	Example: heredoc.Doc(`
	$ gh alias set pv 'pr view'
	$ gh pv -w 123
	#=> gh pr view -w 123
	
	$ gh alias set bugs 'issue list --label="bugs"'

	$ gh alias set epicsBy 'issue list --author="$1" --label="epic"'
	$ gh epicsBy vilmibm
	#=> gh issue list --author="vilmibm" --label="epic"

	# On macOS and Linux:
	$ gh alias set --shell igrep 'gh issue list --label="$1" | grep $2'
	$ gh igrep epic foo
	#=> gh issue list --label="epic" | grep "foo"

	# On Windows (Powershell):
	$ gh alias set --shell igrep 'gh issue list --label=$args[0] | Select-String -Pattern $args[1]
	$ gh igrep epic foo
	#=> gh issue list --label=epic | Select-String -Pattern foo`),
	Args: cobra.ExactArgs(2),
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
	expansion := args[1]

	out := colorableOut(cmd)
	fmt.Fprintf(out, "- Adding alias for %s: %s\n", utils.Bold(alias), utils.Bold(expansion))

	shell, err := cmd.Flags().GetBool("shell")
	if err != nil {
		return err
	}
	if shell && !strings.HasPrefix(expansion, "!") {
		expansion = "!" + expansion
	}
	isExternal := strings.HasPrefix(expansion, "!")

	if validCommand(alias) {
		return fmt.Errorf("could not create alias: %q is already a gh command", alias)
	}

	if !isExternal && !validCommand(expansion) {
		return fmt.Errorf("could not create alias: %s does not correspond to a gh command", expansion)
	}

	successMsg := fmt.Sprintf("%s Added alias.", utils.Green("✓"))

	oldExpansion, ok := aliasCfg.Get(alias)
	if ok {
		successMsg = fmt.Sprintf("%s Changed alias %s from %s to %s",
			utils.Green("✓"),
			utils.Bold(alias),
			utils.Bold(oldExpansion),
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
	split, err := shlex.Split(expansion)
	if err != nil {
		return false
	}
	cmd, _, err := RootCmd.Traverse(split)
	return err == nil && cmd != RootCmd
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
	Use:   "delete <alias>",
	Short: "Delete an alias.",
	Args:  cobra.ExactArgs(1),
	RunE:  aliasDelete,
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

	expansion, ok := aliasCfg.Get(alias)
	if !ok {
		return fmt.Errorf("no such alias %s", alias)

	}

	err = aliasCfg.Delete(alias)
	if err != nil {
		return fmt.Errorf("failed to delete alias %s: %w", alias, err)
	}

	out := colorableOut(cmd)
	redCheck := utils.Red("✓")
	fmt.Fprintf(out, "%s Deleted alias %s; was %s\n", redCheck, alias, expansion)

	return nil
}
