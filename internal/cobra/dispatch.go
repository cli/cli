package cobra

import (
	"fmt"
	"os"
)

func (c *Command) Traverse(args []string) (*Command, []string, error) {
	foundCmd := c
	for {
		if len(args) == 0 {
			return foundCmd, args, nil
		}

		cmdName := args[0]
		if cmdName != "" && cmdName[0] == '-' {
			return foundCmd, args, nil
		}

		isFound := false
		for _, cmd := range foundCmd.childCommands {
			if isNameMatch(cmd, cmdName) {
				foundCmd = cmd
				isFound = true
				break
			}
		}
		if !isFound {
			if !foundCmd.Runnable() {
				return foundCmd, args, fmt.Errorf("%s: could not find command %q", foundCmd.CommandPath(), cmdName)
			}
			return foundCmd, args, nil
		}

		args = args[1:]
	}
}

func isNameMatch(c *Command, name string) bool {
	if c.Name() == name {
		return true
	}
	for _, alias := range c.Aliases {
		if alias == name {
			return true
		}
	}
	return false
}

func (c *Command) ExecuteC() (*Command, error) {
	cmd, args, err := c.Traverse(c.args)
	if err != nil {
		return cmd, err
	}

	flags := cmd.Flags()
	if err := flags.Parse(args); err != nil {
		return cmd, err
	}
	args = flags.Args()

	if !cmd.Runnable() {
		fmt.Fprint(os.Stdout, cmd.UsageString())
		return cmd, nil
	}

	if cmd.Args != nil {
		if err := cmd.Args(cmd, args); err != nil {
			return cmd, err
		}
	}

	if cmd.PreRunE != nil {
		if err := cmd.PreRunE(cmd, args); err != nil {
			return cmd, err
		}
	} else if cmd.PreRun != nil {
		cmd.PreRun(cmd, args)
	}

	if cmd.RunE != nil {
		return cmd, cmd.RunE(cmd, args)
	}
	cmd.Run(cmd, args)
	return cmd, nil
}

func (c *Command) CommandPath() string {
	if c.HasParent() {
		return c.Parent().CommandPath() + " " + c.Name()
	}
	return c.Name()
}

func (c *Command) Runnable() bool {
	return c.RunE != nil || c.Run != nil
}

func (c *Command) Commands() []*Command {
	return c.childCommands
}

func (c *Command) AddCommand(cmds ...*Command) {
	for _, child := range cmds {
		child.parentCommand = c
	}
	c.childCommands = append(c.childCommands, cmds...)
}

func (c *Command) HasParent() bool {
	return c.Parent() != nil
}

func (c *Command) Parent() *Command {
	return nil
}

func (c *Command) Root() *Command {
	if c.HasParent() {
		return c.Parent().Root()
	}
	return c
}
