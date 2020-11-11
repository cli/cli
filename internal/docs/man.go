package docs

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cpuguy83/go-md2man/v2/md2man"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// GenManTree will generate a man page for this command and all descendants
// in the directory given. The header may be nil. This function may not work
// correctly if your command names have `-` in them. If you have `cmd` with two
// subcmds, `sub` and `sub-third`, and `sub` has a subcommand called `third`
// it is undefined which help output will be in the file `cmd-sub-third.1`.
func GenManTree(cmd *cobra.Command, header *GenManHeader, dir string) error {
	return GenManTreeFromOpts(cmd, GenManTreeOptions{
		Header:           header,
		Path:             dir,
		CommandSeparator: "-",
	})
}

// GenManTreeFromOpts generates a man page for the command and all descendants.
// The pages are written to the opts.Path directory.
func GenManTreeFromOpts(cmd *cobra.Command, opts GenManTreeOptions) error {
	header := opts.Header
	if header == nil {
		header = &GenManHeader{}
	}
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		if err := GenManTreeFromOpts(c, opts); err != nil {
			return err
		}
	}
	section := "1"
	if header.Section != "" {
		section = header.Section
	}

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

	headerCopy := *header
	return GenMan(cmd, &headerCopy, f)
}

// GenManTreeOptions is the options for generating the man pages.
// Used only in GenManTreeFromOpts.
type GenManTreeOptions struct {
	Header           *GenManHeader
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
	date    string
	Source  string
	Manual  string
}

// GenMan will generate a man page for the given command and write it to
// w. The header argument may be nil, however obviously w may not.
func GenMan(cmd *cobra.Command, header *GenManHeader, w io.Writer) error {
	if header == nil {
		header = &GenManHeader{}
	}
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
	header.date = (*header.Date).Format("Jan 2006")
	return nil
}

func manPreamble(buf *bytes.Buffer, header *GenManHeader, cmd *cobra.Command, dashedName string) {
	description := cmd.Long
	if len(description) == 0 {
		description = cmd.Short
	}

	buf.WriteString(fmt.Sprintf(`%% "%s" "%s" "%s" "%s" "%s"
# NAME
`, header.Title, header.Section, header.date, header.Source, header.Manual))
	buf.WriteString(fmt.Sprintf("%s \\- %s\n\n", dashedName, cmd.Short))
	buf.WriteString("# SYNOPSIS\n")
	buf.WriteString(fmt.Sprintf("**%s**\n\n", cmd.UseLine()))
	buf.WriteString("# DESCRIPTION\n")
	buf.WriteString(description + "\n\n")
}

func manPrintFlags(buf *bytes.Buffer, flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		if len(flag.Deprecated) > 0 || flag.Hidden {
			return
		}
		format := ""
		if len(flag.Shorthand) > 0 && len(flag.ShorthandDeprecated) == 0 {
			format = fmt.Sprintf("**-%s**, **--%s**", flag.Shorthand, flag.Name)
		} else {
			format = fmt.Sprintf("**--%s**", flag.Name)
		}
		if len(flag.NoOptDefVal) > 0 {
			format += "["
		}
		if flag.Value.Type() == "string" {
			// put quotes on the value
			format += "=%q"
		} else {
			format += "=%s"
		}
		if len(flag.NoOptDefVal) > 0 {
			format += "]"
		}
		format += "\n\t%s\n\n"
		buf.WriteString(fmt.Sprintf(format, flag.DefValue, flag.Usage))
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
	if flags.HasAvailableFlags() {
		buf.WriteString("# OPTIONS INHERITED FROM PARENT COMMANDS\n")
		manPrintFlags(buf, flags)
		buf.WriteString("\n")
	}
}

func genMan(cmd *cobra.Command, header *GenManHeader) []byte {
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultHelpFlag()

	// something like `rootcmd-subcmd1-subcmd2`
	dashCommandName := strings.Replace(cmd.CommandPath(), " ", "-", -1)

	buf := new(bytes.Buffer)

	manPreamble(buf, header, cmd, dashCommandName)
	manPrintOptions(buf, cmd)
	if len(cmd.Example) > 0 {
		buf.WriteString("# EXAMPLE\n")
		buf.WriteString(fmt.Sprintf("```\n%s\n```\n", cmd.Example))
	}
	if hasSeeAlso(cmd) {
		buf.WriteString("# SEE ALSO\n")
		seealsos := make([]string, 0)
		if cmd.HasParent() {
			parentPath := cmd.Parent().CommandPath()
			dashParentPath := strings.Replace(parentPath, " ", "-", -1)
			seealso := fmt.Sprintf("**%s(%s)**", dashParentPath, header.Section)
			seealsos = append(seealsos, seealso)
		}
		children := cmd.Commands()
		sort.Sort(byName(children))
		for _, c := range children {
			if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
				continue
			}
			seealso := fmt.Sprintf("**%s-%s(%s)**", dashCommandName, c.Name(), header.Section)
			seealsos = append(seealsos, seealso)
		}
		buf.WriteString(strings.Join(seealsos, ", ") + "\n")
	}
	return buf.Bytes()
}

// Test to see if we have a reason to print See Also information in docs
// Basically this is a test for a parent command or a subcommand which is
// both not deprecated and not the autogenerated help command.
func hasSeeAlso(cmd *cobra.Command) bool {
	if cmd.HasParent() {
		return true
	}
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		return true
	}
	return false
}

type byName []*cobra.Command

func (s byName) Len() int           { return len(s) }
func (s byName) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byName) Less(i, j int) bool { return s[i].Name() < s[j].Name() }
