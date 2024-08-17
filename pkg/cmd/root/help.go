package root

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func rootUsageFunc(w io.Writer, command *cobra.Command) error {
	fmt.Fprintf(w, "Usage:  %s", command.UseLine())

	var subcommands []*cobra.Command
	for _, c := range command.Commands() {
		if !c.IsAvailableCommand() {
			continue
		}
		subcommands = append(subcommands, c)
	}

	if len(subcommands) > 0 {
		fmt.Fprint(w, "\n\nAvailable commands:\n")
		for _, c := range subcommands {
			fmt.Fprintf(w, "  %s\n", c.Name())
		}
		return nil
	}

	flagUsages := command.LocalFlags().FlagUsages()
	if flagUsages != "" {
		fmt.Fprintln(w, "\n\nFlags:")
		fmt.Fprint(w, text.Indent(dedent(flagUsages), "  "))
	}
	return nil
}

func rootFlagErrorFunc(cmd *cobra.Command, err error) error {
	if err == pflag.ErrHelp {
		return err
	}
	return cmdutil.FlagErrorWrap(err)
}

var hasFailed bool

// HasFailed signals that the main process should exit with non-zero status
func HasFailed() bool {
	return hasFailed
}

// Display helpful error message in case subcommand name was mistyped.
// This matches Cobra's behavior for root command, which Cobra
// confusingly doesn't apply to nested commands.
func nestedSuggestFunc(w io.Writer, command *cobra.Command, arg string) {
	fmt.Fprintf(w, "unknown command %q for %q\n", arg, command.CommandPath())

	var candidates []string
	if arg == "help" {
		candidates = []string{"--help"}
	} else {
		if command.SuggestionsMinimumDistance <= 0 {
			command.SuggestionsMinimumDistance = 2
		}
		candidates = command.SuggestionsFor(arg)
	}

	if len(candidates) > 0 {
		fmt.Fprint(w, "\nDid you mean this?\n")
		for _, c := range candidates {
			fmt.Fprintf(w, "\t%s\n", c)
		}
	}

	fmt.Fprint(w, "\n")
	_ = rootUsageFunc(w, command)
}

func isRootCmd(command *cobra.Command) bool {
	return command != nil && !command.HasParent()
}

func rootHelpFunc(f *cmdutil.Factory, command *cobra.Command, args []string) {
	flags := command.Flags()

	if isRootCmd(command) {
		if versionVal, err := flags.GetBool("version"); err == nil && versionVal {
			fmt.Fprint(f.IOStreams.Out, command.Annotations["versionInfo"])
			return
		} else if err != nil {
			fmt.Fprintln(f.IOStreams.ErrOut, err)
			hasFailed = true
			return
		}
	}

	cs := f.IOStreams.ColorScheme()

	if help, _ := flags.GetBool("help"); !help && !command.Runnable() && len(flags.Args()) > 0 {
		nestedSuggestFunc(f.IOStreams.ErrOut, command, flags.Args()[0])
		hasFailed = true
		return
	}

	namePadding := 12

	type helpEntry struct {
		Title string
		Body  string
	}

	longText := command.Long
	if longText == "" {
		longText = command.Short
	}
	if longText != "" && command.LocalFlags().Lookup("jq") != nil {
		longText = strings.TrimRight(longText, "\n") +
			"\n\nFor more information about output formatting flags, see `gh help formatting`."
	}

	helpEntries := []helpEntry{}
	if longText != "" {
		helpEntries = append(helpEntries, helpEntry{"", longText})
	}
	helpEntries = append(helpEntries, helpEntry{"USAGE", command.UseLine()})

	if len(command.Aliases) > 0 {
		helpEntries = append(helpEntries, helpEntry{"ALIASES", strings.Join(BuildAliasList(command, command.Aliases), ", ") + "\n"})
	}

	for _, g := range GroupedCommands(command) {
		var names []string
		for _, c := range g.Commands {
			names = append(names, rpad(c.Name()+":", namePadding)+c.Short)
		}
		helpEntries = append(helpEntries, helpEntry{
			Title: strings.ToUpper(g.Title),
			Body:  strings.Join(names, "\n"),
		})
	}

	if isRootCmd(command) {
		var helpTopics []string
		if c := findCommand(command, "actions"); c != nil {
			helpTopics = append(helpTopics, rpad(c.Name()+":", namePadding)+c.Short)
		}
		for _, helpTopic := range HelpTopics {
			helpTopics = append(helpTopics, rpad(helpTopic.name+":", namePadding)+helpTopic.short)
		}
		sort.Strings(helpTopics)
		helpEntries = append(helpEntries, helpEntry{"HELP TOPICS", strings.Join(helpTopics, "\n")})
	}

	flagUsages := command.LocalFlags().FlagUsages()
	if flagUsages != "" {
		helpEntries = append(helpEntries, helpEntry{"FLAGS", dedent(flagUsages)})
	}
	inheritedFlagUsages := command.InheritedFlags().FlagUsages()
	if inheritedFlagUsages != "" {
		helpEntries = append(helpEntries, helpEntry{"INHERITED FLAGS", dedent(inheritedFlagUsages)})
	}
	if _, ok := command.Annotations["help:json-fields"]; ok {
		fields := strings.Split(command.Annotations["help:json-fields"], ",")
		helpEntries = append(helpEntries, helpEntry{"JSON FIELDS", text.FormatSlice(fields, 80, 0, "", "", true)})
	}
	if _, ok := command.Annotations["help:arguments"]; ok {
		helpEntries = append(helpEntries, helpEntry{"ARGUMENTS", command.Annotations["help:arguments"]})
	}
	if command.Example != "" {
		helpEntries = append(helpEntries, helpEntry{"EXAMPLES", command.Example})
	}
	if _, ok := command.Annotations["help:environment"]; ok {
		helpEntries = append(helpEntries, helpEntry{"ENVIRONMENT VARIABLES", command.Annotations["help:environment"]})
	}
	helpEntries = append(helpEntries, helpEntry{"LEARN MORE", heredoc.Docf(`
		Use %[1]sgh <command> <subcommand> --help%[1]s for more information about a command.
		Read the manual at https://cli.github.com/manual
		Learn about exit codes using %[1]sgh help exit-codes%[1]s
	`, "`")})

	out := f.IOStreams.Out
	for _, e := range helpEntries {
		if e.Title != "" {
			// If there is a title, add indentation to each line in the body
			fmt.Fprintln(out, cs.Bold(e.Title))
			fmt.Fprintln(out, text.Indent(strings.Trim(e.Body, "\r\n"), "  "))
		} else {
			// If there is no title print the body as is
			fmt.Fprintln(out, e.Body)
		}
		fmt.Fprintln(out)
	}
}

func authHelp() string {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return heredoc.Doc(`
			gh: To use GitHub CLI in a GitHub Actions workflow, set the GH_TOKEN environment variable. Example:
			  env:
			    GH_TOKEN: ${{ github.token }}
		`)
	}

	if os.Getenv("CI") != "" {
		return heredoc.Doc(`
			gh: To use GitHub CLI in automation, set the GH_TOKEN environment variable.
		`)
	}

	return heredoc.Doc(`
		To get started with GitHub CLI, please run:  gh auth login
		Alternatively, populate the GH_TOKEN environment variable with a GitHub API authentication token.
	`)
}

func findCommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, c := range cmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

type CommandGroup struct {
	Title    string
	Commands []*cobra.Command
}

func GroupedCommands(cmd *cobra.Command) []CommandGroup {
	var res []CommandGroup

	for _, g := range cmd.Groups() {
		var cmds []*cobra.Command
		for _, c := range cmd.Commands() {
			if c.GroupID == g.ID && c.IsAvailableCommand() {
				cmds = append(cmds, c)
			}
		}
		if len(cmds) > 0 {
			res = append(res, CommandGroup{
				Title:    g.Title,
				Commands: cmds,
			})
		}
	}

	var cmds []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.GroupID == "" && c.IsAvailableCommand() {
			cmds = append(cmds, c)
		}
	}
	if len(cmds) > 0 {
		defaultGroupTitle := "Additional commands"
		if len(cmd.Groups()) == 0 {
			defaultGroupTitle = "Available commands"
		}
		res = append(res, CommandGroup{
			Title:    defaultGroupTitle,
			Commands: cmds,
		})
	}

	return res
}

// rpad adds padding to the right of a string.
func rpad(s string, padding int) string {
	template := fmt.Sprintf("%%-%ds ", padding)
	return fmt.Sprintf(template, s)
}

func dedent(s string) string {
	lines := strings.Split(s, "\n")
	minIndent := -1

	for _, l := range lines {
		if len(l) == 0 {
			continue
		}

		indent := len(l) - len(strings.TrimLeft(l, " "))
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}

	if minIndent <= 0 {
		return s
	}

	var buf bytes.Buffer
	for _, l := range lines {
		fmt.Fprintln(&buf, strings.TrimPrefix(l, strings.Repeat(" ", minIndent)))
	}
	return strings.TrimSuffix(buf.String(), "\n")
}

func BuildAliasList(cmd *cobra.Command, aliases []string) []string {
	if !cmd.HasParent() {
		return aliases
	}

	parentAliases := append(cmd.Parent().Aliases, cmd.Parent().Name())
	sort.Strings(parentAliases)

	var aliasesWithParentAliases []string
	// e.g aliases = [ls]
	for _, alias := range aliases {
		// e.g parentAliases = [codespaces, cs]
		for _, parentAlias := range parentAliases {
			// e.g. aliasesWithParentAliases = [codespaces list, codespaces ls, cs list, cs ls]
			aliasesWithParentAliases = append(aliasesWithParentAliases, fmt.Sprintf("%s %s", parentAlias, alias))
		}
	}

	return BuildAliasList(cmd.Parent(), aliasesWithParentAliases)
}
