package docs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/root"
	"github.com/cpuguy83/go-md2man/v2/md2man"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/pflag"
)

// GenManTree will generate a man page for this command and all descendants
// in the directory given. The header may be nil. This function may not work
// correctly if your command names have `-` in them. If you have `cmd` with two
// subcmds, `sub` and `sub-third`, and `sub` has a subcommand called `third`
// it is undefined which help output will be in the file `cmd-sub-third.1`.
func GenManTree(cmd *cobra.Command, dir string) error {
	if os.Getenv("GH_COBRA") != "" {
		return doc.GenManTreeFromOpts(cmd, doc.GenManTreeOptions{
			Path:             dir,
			CommandSeparator: "-",
		})
	}
	return genManTreeFromOpts(cmd, GenManTreeOptions{
		Path:             dir,
		CommandSeparator: "-",
	})
}

// genManTreeFromOpts generates a man page for the command and all descendants.
// The pages are written to the opts.Path directory.
func genManTreeFromOpts(cmd *cobra.Command, opts GenManTreeOptions) error {
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		if err := genManTreeFromOpts(c, opts); err != nil {
			return err
		}
	}

	section := "1"
	separator := "_"
	if opts.CommandSeparator != "" {
		separator = opts.CommandSeparator
	}
	basename := strings.Replace(cmd.CommandPath(), " ", separator, -1)
	filename := filepath.Join(opts.Path, basename+"."+section)
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	var versionString string
	if v := os.Getenv("GH_VERSION"); v != "" {
		versionString = "GitHub CLI " + v
	}

	return renderMan(cmd, &GenManHeader{
		Section: section,
		Source:  versionString,
		Manual:  "GitHub CLI manual",
	}, f)
}

// GenManTreeOptions is the options for generating the man pages.
// Used only in GenManTreeFromOpts.
type GenManTreeOptions struct {
	Path             string
	CommandSeparator string
}

// GenManHeader is a lot like the .TH header at the start of man pages. These
// include the title, section, date, source, and manual. We will use the
// current time if Date is unset.
type GenManHeader struct {
	Title   string
	Section string
	Date    *time.Time
	Source  string
	Manual  string
}

// renderMan will generate a man page for the given command and write it to
// w. The header argument may be nil, however obviously w may not.
func renderMan(cmd *cobra.Command, header *GenManHeader, w io.Writer) error {
	if err := fillHeader(header, cmd.CommandPath()); err != nil {
		return err
	}

	b := genMan(cmd, header)

	_, err := w.Write(md2man.Render(b))
	return err
}

func fillHeader(header *GenManHeader, name string) error {
	if header.Title == "" {
		header.Title = strings.ToUpper(strings.Replace(name, " ", "\\-", -1))
	}
	if header.Section == "" {
		header.Section = "1"
	}
	if header.Date == nil {
		now := time.Now()
		if epoch := os.Getenv("SOURCE_DATE_EPOCH"); epoch != "" {
			unixEpoch, err := strconv.ParseInt(epoch, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid SOURCE_DATE_EPOCH: %v", err)
			}
			now = time.Unix(unixEpoch, 0)
		}
		header.Date = &now
	}
	return nil
}

func manPreamble(buf *bytes.Buffer, header *GenManHeader, cmd *cobra.Command, dashedName string) {
	buf.WriteString(fmt.Sprintf(`%% "%s" "%s" "%s" "%s" "%s"
# NAME
`, header.Title, header.Section, header.Date.Format("Jan 2006"), header.Source, header.Manual))
	buf.WriteString(fmt.Sprintf("%s \\- %s\n\n", dashedName, cmd.Short))
	buf.WriteString("# SYNOPSIS\n")
	buf.WriteString(fmt.Sprintf("`%s`\n\n", cmd.UseLine()))

	if cmd.Long != "" && cmd.Long != cmd.Short {
		buf.WriteString("# DESCRIPTION\n")
		buf.WriteString(cmd.Long + "\n\n")
	}
}

func manPrintFlags(buf *bytes.Buffer, flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		if len(flag.Deprecated) > 0 || flag.Hidden || flag.Name == "help" {
			return
		}
		varname, usage := pflag.UnquoteUsage(flag)
		if len(flag.Shorthand) > 0 && len(flag.ShorthandDeprecated) == 0 {
			buf.WriteString(fmt.Sprintf("`-%s`, `--%s`", flag.Shorthand, flag.Name))
		} else {
			buf.WriteString(fmt.Sprintf("`--%s`", flag.Name))
		}

		defval := getDefaultValueDisplayString(flag)

		if varname == "" && defval != "" {
			buf.WriteString(fmt.Sprintf(" `%s`\n", strings.TrimSpace(defval)))
		} else if varname == "" {
			buf.WriteString("\n")
		} else {
			buf.WriteString(fmt.Sprintf(" `<%s>%s`\n", varname, defval))
		}
		buf.WriteString(fmt.Sprintf(":   %s\n\n", usage))
	})
}

func manPrintOptions(buf *bytes.Buffer, command *cobra.Command) {
	flags := command.NonInheritedFlags()
	if flags.HasAvailableFlags() {
		buf.WriteString("# OPTIONS\n")
		manPrintFlags(buf, flags)
		buf.WriteString("\n")
	}
	flags = command.InheritedFlags()
	if hasNonHelpFlags(flags) {
		buf.WriteString("# OPTIONS INHERITED FROM PARENT COMMANDS\n")
		manPrintFlags(buf, flags)
		buf.WriteString("\n")
	}
}

func manPrintAliases(buf *bytes.Buffer, command *cobra.Command) {
	if len(command.Aliases) > 0 {
		buf.WriteString("# ALIASES\n")
		buf.WriteString(strings.Join(root.BuildAliasList(command, command.Aliases), ", "))
		buf.WriteString("\n")
	}
}

func manPrintJSONFields(buf *bytes.Buffer, command *cobra.Command) {
	raw, ok := command.Annotations["help:json-fields"]
	if !ok {
		return
	}

	buf.WriteString("# JSON FIELDS\n")
	buf.WriteString(text.FormatSlice(strings.Split(raw, ","), 0, 0, "`", "`", true))
	buf.WriteString("\n")
}

func manPrintExitCodes(buf *bytes.Buffer) {
	buf.WriteString("# EXIT CODES\n")
	buf.WriteString("0: Successful execution\n\n")
	buf.WriteString("1: Error\n\n")
	buf.WriteString("2: Command canceled\n\n")
	buf.WriteString("4: Authentication required\n\n")
	buf.WriteString("NOTE: Specific commands may have additional exit codes. Refer to the command's help for more information.\n\n")
}

func genMan(cmd *cobra.Command, header *GenManHeader) []byte {
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultHelpFlag()

	// something like `rootcmd-subcmd1-subcmd2`
	dashCommandName := strings.Replace(cmd.CommandPath(), " ", "-", -1)

	buf := new(bytes.Buffer)

	manPreamble(buf, header, cmd, dashCommandName)
	for _, g := range root.GroupedCommands(cmd) {
		fmt.Fprintf(buf, "# %s\n", strings.ToUpper(g.Title))
		for _, subcmd := range g.Commands {
			fmt.Fprintf(buf, "`%s`\n:   %s\n\n", manLink(subcmd), subcmd.Short)
		}
	}
	manPrintOptions(buf, cmd)
	manPrintAliases(buf, cmd)
	manPrintJSONFields(buf, cmd)
	manPrintExitCodes(buf)
	if len(cmd.Example) > 0 {
		buf.WriteString("# EXAMPLE\n")
		buf.WriteString(fmt.Sprintf("```\n%s\n```\n", cmd.Example))
	}
	if cmd.HasParent() {
		buf.WriteString("# SEE ALSO\n")
		buf.WriteString(fmt.Sprintf("`%s`\n", manLink(cmd.Parent())))
	}
	return buf.Bytes()
}

func manLink(cmd *cobra.Command) string {
	p := cmd.CommandPath()
	return fmt.Sprintf("%s(%d)", strings.Replace(p, " ", "-", -1), 1)
}
