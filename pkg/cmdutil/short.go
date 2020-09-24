package cmdutil

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func ApplyCuratedShort(cmd *cobra.Command, resource string, commands []string) error {
	commands = filterValidChoices(cmd, commands)

	if len(commands) == 0 {
		return nil
	}

	commandList := commands[0]

	if len(commands) > 1 {
		back, front := commands[len(commands)-1], commands[:len(commands)-1]
		commandList = fmt.Sprintf("%s and %s", strings.Join(front, ", "), back)
	}

	short := commandList + " " + resource

	if additional := len(cmd.Commands()) - len(commands); additional >= 1 {
		short += fmt.Sprintf(" (+%v more)", additional)
	}

	cmd.Short = short

	return nil
}

func filterValidChoices(cmd *cobra.Command, commands []string) []string {
	validChoices := []string{}
	validOptions := []string{}

	for _, sub := range cmd.Commands() {
		validOptions = append(validOptions, sub.Name())
	}

	for _, command := range commands {
		for _, subCommand := range validOptions {
			if strings.EqualFold(command, subCommand) {
				validChoices = append(validChoices, command)
			}
		}
	}

	return validChoices
}
