package docs

import (
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func printOptions(w io.Writer, cmd *cobra.Command) error {
	flags := cmd.NonInheritedFlags()
	flags.SetOutput(w)
	if flags.HasAvailableFlags() {
		fmt.Fprint(w, "### Options\n\n")
		if err := printFlagsHTML(w, flags); err != nil {
			return err
		}
		fmt.Fprint(w, "\n\n")
	}

	parentFlags := cmd.InheritedFlags()
	parentFlags.SetOutput(w)
	if hasNonHelpFlags(parentFlags) {
		fmt.Fprint(w, "### Options inherited from parent commands\n\n")
		if err := printFlagsHTML(w, parentFlags); err != nil {
			return err
		}
		fmt.Fprint(w, "\n\n")
	}
	return nil
}

func hasNonHelpFlags(fs *pflag.FlagSet) (found bool) {
	fs.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && f.Name != "help" {
			found = true
		}
	})
	return
}

type flagView struct {
	Name      string
	Varname   string
	Shorthand string
	Usage     string
}

var flagsTemplate = `
<dl class="flags">{{ range . }}
	<dt>{{ if .Shorthand }}<code>-{{.Shorthand}}</code>, {{ end -}}
		<code>--{{.Name}}{{ if .Varname }} &lt;{{.Varname}}&gt;{{ end }}</code></dt>
	<dd>{{.Usage}}</dd>
{{ end }}</dl>
`

var tpl = template.Must(template.New("flags").Parse(flagsTemplate))

func printFlagsHTML(w io.Writer, fs *pflag.FlagSet) error {
	var flags []flagView
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		varname, usage := pflag.UnquoteUsage(f)
		flags = append(flags, flagView{
			Name:      f.Name,
			Varname:   varname,
			Shorthand: f.Shorthand,
			Usage:     usage,
		})
	})
	return tpl.Execute(w, flags)
}

// GenMarkdown creates markdown output.
func GenMarkdown(cmd *cobra.Command, w io.Writer) error {
	return GenMarkdownCustom(cmd, w, func(s string) string { return s })
}

// GenMarkdownCustom creates custom markdown output.
func GenMarkdownCustom(cmd *cobra.Command, w io.Writer, linkHandler func(string) string) error {
	fmt.Fprintf(w, "## %s\n\n", cmd.CommandPath())

	hasLong := cmd.Long != ""
	if !hasLong {
		fmt.Fprintf(w, "%s\n\n", cmd.Short)
	}
	if cmd.Runnable() {
		fmt.Fprintf(w, "```\n%s\n```\n\n", cmd.UseLine())
	}
	if hasLong {
		fmt.Fprintf(w, "%s\n\n", cmd.Long)
	}

	for _, g := range subcommandGroups(cmd) {
		if len(g.Commands) == 0 {
			continue
		}
		fmt.Fprintf(w, "### %s\n\n", g.Name)
		for _, subcmd := range g.Commands {
			fmt.Fprintf(w, "* [%s](%s)\n", subcmd.CommandPath(), linkHandler(cmdManualPath(subcmd)))
		}
		fmt.Fprint(w, "\n\n")
	}

	if err := printOptions(w, cmd); err != nil {
		return err
	}

	if len(cmd.Example) > 0 {
		fmt.Fprint(w, "### Examples\n\n{% highlight bash %}{% raw %}\n")
		fmt.Fprint(w, cmd.Example)
		fmt.Fprint(w, "{% endraw %}{% endhighlight %}\n\n")
	}

	if cmd.HasParent() {
		p := cmd.Parent()
		fmt.Fprint(w, "### See also\n\n")
		fmt.Fprintf(w, "* [%s](%s)\n", p.CommandPath(), linkHandler(cmdManualPath(p)))
	}

	return nil
}

type commandGroup struct {
	Name     string
	Commands []*cobra.Command
}

// subcommandGroups lists child commands of a Cobra command split into groups.
// TODO: have rootHelpFunc use this instead of repeating the same logic.
func subcommandGroups(c *cobra.Command) []commandGroup {
	var rest []*cobra.Command
	var core []*cobra.Command
	var actions []*cobra.Command

	for _, subcmd := range c.Commands() {
		if !subcmd.IsAvailableCommand() {
			continue
		}
		if _, ok := subcmd.Annotations["IsCore"]; ok {
			core = append(core, subcmd)
		} else if _, ok := subcmd.Annotations["IsActions"]; ok {
			actions = append(actions, subcmd)
		} else {
			rest = append(rest, subcmd)
		}
	}

	if len(core) > 0 {
		return []commandGroup{
			{
				Name:     "Core commands",
				Commands: core,
			},
			{
				Name:     "Actions commands",
				Commands: actions,
			},
			{
				Name:     "Additional commands",
				Commands: rest,
			},
		}
	}

	return []commandGroup{
		{
			Name:     "Commands",
			Commands: rest,
		},
	}
}

// GenMarkdownTree will generate a markdown page for this command and all
// descendants in the directory given. The header may be nil.
// This function may not work correctly if your command names have `-` in them.
// If you have `cmd` with two subcmds, `sub` and `sub-third`,
// and `sub` has a subcommand called `third`, it is undefined which
// help output will be in the file `cmd-sub-third.1`.
func GenMarkdownTree(cmd *cobra.Command, dir string) error {
	identity := func(s string) string { return s }
	emptyStr := func(s string) string { return "" }
	return GenMarkdownTreeCustom(cmd, dir, emptyStr, identity)
}

// GenMarkdownTreeCustom is the the same as GenMarkdownTree, but
// with custom filePrepender and linkHandler.
func GenMarkdownTreeCustom(cmd *cobra.Command, dir string, filePrepender, linkHandler func(string) string) error {
	for _, c := range cmd.Commands() {
		_, forceGeneration := c.Annotations["markdown:generate"]
		if c.Hidden && !forceGeneration {
			continue
		}

		if err := GenMarkdownTreeCustom(c, dir, filePrepender, linkHandler); err != nil {
			return err
		}
	}

	filename := filepath.Join(dir, cmdManualPath(cmd))
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.WriteString(f, filePrepender(filename)); err != nil {
		return err
	}
	if err := GenMarkdownCustom(cmd, f, linkHandler); err != nil {
		return err
	}
	return nil
}

func cmdManualPath(c *cobra.Command) string {
	if basenameOverride, found := c.Annotations["markdown:basename"]; found {
		return basenameOverride + ".md"
	}
	return strings.ReplaceAll(c.CommandPath(), " ", "_") + ".md"
}
