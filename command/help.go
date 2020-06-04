package command

import (
	"fmt"
	"strings"

	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func rootDisplayCommandTypoHelp(command *cobra.Command, args []string) {
	if command != RootCmd {
		// Display helpful error message in case subcommand name was mistyped.
		// This matches Cobra's behavior for root command, which Cobra
		// confusingly doesn't apply to nested commands.
		if command.Parent() == RootCmd && len(args) >= 2 {
			if command.SuggestionsMinimumDistance <= 0 {
				command.SuggestionsMinimumDistance = 2
			}
			candidates := command.SuggestionsFor(args[1])

			errOut := command.OutOrStderr()
			fmt.Fprintf(errOut, "unknown command %q for %q\n", args[1], "gh "+args[0])

			if len(candidates) > 0 {
				fmt.Fprint(errOut, "\nDid you mean this?\n")
				for _, c := range candidates {
					fmt.Fprintf(errOut, "\t%s\n", c)
				}
				fmt.Fprint(errOut, "\n")
			}

			oldOut := command.OutOrStdout()
			command.SetOut(errOut)
			defer command.SetOut(oldOut)
		}
	}
}

func rootHelpFunc(command *cobra.Command, args []string) {
	rootDisplayCommandTypoHelp(command, args)

	coreCommands := []string{}
	additionalCommands := []string{}
	for _, c := range command.Commands() {
		if c.Short == "" {
			continue
		}
		s := rpad(c.Name()+":", c.NamePadding()) + c.Short
		if _, ok := c.Annotations["IsCore"]; ok {
			coreCommands = append(coreCommands, s)
		} else {
			additionalCommands = append(additionalCommands, s)
		}
	}

	// If there are no core commands, assume everything is a core command
	if len(coreCommands) == 0 {
		coreCommands = additionalCommands
		additionalCommands = []string{}
	}

	type helpEntry struct {
		Title string
		Body  string
	}

	helpEntries := []helpEntry{
		{"", command.Long},
		{"USAGE", command.Use},
	}

	if len(coreCommands) > 0 {
		helpEntries = append(helpEntries, helpEntry{"CORE COMMANDS", strings.Join(coreCommands, "\n")})
	}
	if len(additionalCommands) > 0 {
		helpEntries = append(helpEntries, helpEntry{"ADDITIONAL COMMANDS", strings.Join(additionalCommands, "\n")})
	}
	if command.HasLocalFlags() {
		helpEntries = append(helpEntries, helpEntry{"FLAGS", strings.TrimRight(command.LocalFlags().FlagUsages(), "\n")})
	}
	if _, ok := command.Annotations["help:examples"]; ok {
		helpEntries = append(helpEntries, helpEntry{"EXAMPLES", command.Annotations["help:examples"]})
	}
	if _, ok := command.Annotations["help:learnmore"]; ok {
		helpEntries = append(helpEntries, helpEntry{"LEARN MORE", command.Annotations["help:learnmore"]})
	}
	if _, ok := command.Annotations["help:feedback"]; ok {
		helpEntries = append(helpEntries, helpEntry{"FEEDBACK", command.Annotations["help:feedback"]})
	}

	out := colorableOut(command)
	for _, e := range helpEntries {
		if e.Title != "" {
			// If there is a title, add indentation to each line in the body
			fmt.Fprintln(out, utils.Bold(e.Title))

			for _, l := range strings.Split(e.Body, "\n") {
				l = strings.Trim(l, " \n\r")
				if l == "" {
					continue
				}
				fmt.Fprintln(out, "  "+l)
			}
		} else {
			// If there is no title print the body as is
			fmt.Fprintln(out, e.Body)
		}
		fmt.Fprintln(out)
	}
}

// rpad adds padding to the right of a string.
func rpad(s string, padding int) string {
	template := fmt.Sprintf("%%-%ds ", padding)
	return fmt.Sprintf(template, s)
}
