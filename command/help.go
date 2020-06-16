package command

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func rootUsageFunc(command *cobra.Command) error {
	command.Printf("Usage:  %s", command.UseLine())

	flagUsages := command.LocalFlags().FlagUsages()
	if flagUsages != "" {
		command.Printf("\n\nFlags:\n%s", flagUsages)
	}
	return nil
}

func rootHelpFunc(command *cobra.Command, args []string) {
	// Display helpful error message in case subcommand name was mistyped.
	// This matches Cobra's behavior for root command, which Cobra
	// confusingly doesn't apply to nested commands.
	if command != RootCmd {
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

	coreCommands := []string{}
	additionalCommands := []string{}
	for _, c := range command.Commands() {
		if c.Short == "" {
			continue
		}
		if c.Hidden {
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

	helpEntries := []helpEntry{}
	if command.Long != "" {
		helpEntries = append(helpEntries, helpEntry{"", command.Long})
	} else if command.Short != "" {
		helpEntries = append(helpEntries, helpEntry{"", command.Short})
	}
	helpEntries = append(helpEntries, helpEntry{"USAGE", command.UseLine()})
	if len(coreCommands) > 0 {
		helpEntries = append(helpEntries, helpEntry{"CORE COMMANDS", strings.Join(coreCommands, "\n")})
	}
	if len(additionalCommands) > 0 {
		helpEntries = append(helpEntries, helpEntry{"ADDITIONAL COMMANDS", strings.Join(additionalCommands, "\n")})
	}

	flagUsages := command.LocalFlags().FlagUsages()
	if flagUsages != "" {
		dedent := regexp.MustCompile(`(?m)^  `)
		helpEntries = append(helpEntries, helpEntry{"FLAGS", dedent.ReplaceAllString(flagUsages, "")})
	}
	if _, ok := command.Annotations["help:arguments"]; ok {
		helpEntries = append(helpEntries, helpEntry{"ARGUMENTS", command.Annotations["help:arguments"]})
	}
	if command.Example != "" {
		helpEntries = append(helpEntries, helpEntry{"EXAMPLES", command.Example})
	}
	helpEntries = append(helpEntries, helpEntry{"LEARN MORE", `
Use "gh <command> <subcommand> --help" for more information about a command.
Read the manual at https://cli.github.com/manual`})
	if _, ok := command.Annotations["help:feedback"]; ok {
		helpEntries = append(helpEntries, helpEntry{"FEEDBACK", command.Annotations["help:feedback"]})
	}

	out := colorableOut(command)
	for _, e := range helpEntries {
		if e.Title != "" {
			// If there is a title, add indentation to each line in the body
			fmt.Fprintln(out, utils.Bold(e.Title))

			for _, l := range strings.Split(strings.Trim(e.Body, "\n\r"), "\n") {
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
