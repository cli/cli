package cmdutil

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func ApplyCuratedShort(cmd *cobra.Command, resource string, commands []string) error {
	commands = validCommandList(cmd, commands)

	if len(commands) == 0 {
		return nil
	}

	commandList := commands[0]

	if len(commands) > 1 {
		back, front := commands[len(commands)-1], commands[:len(commands)-1]
		commandList = fmt.Sprintf("%s and %s", strings.Join(front, ", "), back)
	}

	short := strings.Join([]string{commandList, resource}, " ")

	if additional := len(cmd.Commands()) - len(commands); additional >= 1 {
		short += fmt.Sprintf(" (+%v more)", additional)
	}

	cmd.Short = short

	return nil
}

func validCommandList(cmd *cobra.Command, commands []string) []string {
	valid := commands[:0]
	subCommands := []string{}

	for _, sub := range cmd.Commands() {
		subCommands = append(subCommands, sub.Name())
	}

	for _, command := range commands {
		for _, subCommand := range subCommands {
			if strings.EqualFold(command, subCommand) {
				valid = append(valid, command)
			}
		}
	}

	return valid
}
