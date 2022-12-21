package cmdutil

import "github.com/spf13/cobra"

func AddGroup(parent *cobra.Command, title string, cmds ...*cobra.Command) {
	g := &cobra.Group{
		Title: title,
		ID:    title,
	}
	parent.AddGroup(g)
	for _, c := range cmds {
		c.GroupID = g.ID
		parent.AddCommand(c)
	}
}
