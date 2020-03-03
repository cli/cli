// imported from https://github.com/spf13/cobra/pull/754
// author: Tim Reddehase

package cobrafish

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func GenCompletion(c *cobra.Command, w io.Writer) error {
	buf := new(bytes.Buffer)

	writeFishPreamble(c, buf)
	writeFishCommandCompletion(c, c, buf)

	_, err := buf.WriteTo(w)
	return err
}

func writeFishPreamble(cmd *cobra.Command, buf *bytes.Buffer) {
	subCommandNames := []string{}
	rangeCommands(cmd, func(subCmd *cobra.Command) {
		subCommandNames = append(subCommandNames, subCmd.Name())
	})
	buf.WriteString(fmt.Sprintf(`
function __fish_%s_no_subcommand --description 'Test if %s has yet to be given the subcommand'
	for i in (commandline -opc)
		if contains -- $i %s
			return 1
		end
	end
	return 0
end
function __fish_%s_seen_subcommand_path --description 'Test whether the full path of subcommands is the current path'
	  set -l cmd (commandline -opc)
	  set -e cmd[1]
    set -l pattern (string replace -a " " ".+" "$argv")
    string match -r "$pattern" (string trim -- "$cmd")
end
# borrowed from current fish-shell master, since it is not in current 2.7.1 release
function __fish_seen_argument
	argparse 's/short=+' 'l/long=+' -- $argv
	set cmd (commandline -co)
	set -e cmd[1]
	for t in $cmd
		for s in $_flag_s
			if string match -qr "^-[A-z0-9]*"$s"[A-z0-9]*\$" -- $t
				return 0
			end
		end
		for l in $_flag_l
			if string match -q -- "--$l" $t
				return 0
			end
		end
	end
	return 1
end
`, cmd.Name(), cmd.Name(), strings.Join(subCommandNames, " "), cmd.Name()))
}

func writeFishCommandCompletion(rootCmd, cmd *cobra.Command, buf *bytes.Buffer) {
	rangeCommands(cmd, func(subCmd *cobra.Command) {
		condition := commandCompletionCondition(rootCmd, cmd)
		escapedDescription := strings.Replace(subCmd.Short, "'", "\\'", -1)
		buf.WriteString(fmt.Sprintf("complete -c %s -f %s -a %s -d '%s'\n", rootCmd.Name(), condition, subCmd.Name(), escapedDescription))
	})
	for _, validArg := range append(cmd.ValidArgs, cmd.ArgAliases...) {
		condition := commandCompletionCondition(rootCmd, cmd)
		buf.WriteString(
			fmt.Sprintf("complete -c %s -f %s -a %s -d '%s'\n",
				rootCmd.Name(), condition, validArg, fmt.Sprintf("Positional Argument to %s", cmd.Name())))
	}
	writeCommandFlagsCompletion(rootCmd, cmd, buf)
	rangeCommands(cmd, func(subCmd *cobra.Command) {
		writeFishCommandCompletion(rootCmd, subCmd, buf)
	})
}

func writeCommandFlagsCompletion(rootCmd, cmd *cobra.Command, buf *bytes.Buffer) {
	cmd.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if nonCompletableFlag(flag) {
			return
		}
		writeCommandFlagCompletion(rootCmd, cmd, buf, flag)
	})
	cmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if nonCompletableFlag(flag) {
			return
		}
		writeCommandFlagCompletion(rootCmd, cmd, buf, flag)
	})
}

func writeCommandFlagCompletion(rootCmd, cmd *cobra.Command, buf *bytes.Buffer, flag *pflag.Flag) {
	shortHandPortion := ""
	if len(flag.Shorthand) > 0 {
		shortHandPortion = fmt.Sprintf("-s %s", flag.Shorthand)
	}
	condition := completionCondition(rootCmd, cmd)
	escapedUsage := strings.Replace(flag.Usage, "'", "\\'", -1)
	buf.WriteString(fmt.Sprintf("complete -c %s -f %s %s %s -l %s -d '%s'\n",
		rootCmd.Name(), condition, flagRequiresArgumentCompletion(flag), shortHandPortion, flag.Name, escapedUsage))
}

func flagRequiresArgumentCompletion(flag *pflag.Flag) string {
	if flag.Value.Type() != "bool" {
		return "-r"
	}
	return ""
}

func subCommandPath(rootCmd *cobra.Command, cmd *cobra.Command) string {
	path := make([]string, 0, 1)
	currentCmd := cmd
	if rootCmd == cmd {
		return ""
	}
	for {
		path = append([]string{currentCmd.Name()}, path...)
		if currentCmd.Parent() == rootCmd {
			return strings.Join(path, " ")
		}
		currentCmd = currentCmd.Parent()
	}
}

func rangeCommands(cmd *cobra.Command, callback func(subCmd *cobra.Command)) {
	for _, subCmd := range cmd.Commands() {
		if !subCmd.IsAvailableCommand() || strings.HasPrefix(subCmd.Use, "help") {
			continue
		}
		callback(subCmd)
	}
}

func commandCompletionCondition(rootCmd, cmd *cobra.Command) string {
	localNonPersistentFlags := cmd.LocalNonPersistentFlags()
	bareConditions := make([]string, 0, 1)
	if rootCmd != cmd {
		bareConditions = append(bareConditions, fmt.Sprintf("__fish_%s_seen_subcommand_path %s", rootCmd.Name(), subCommandPath(rootCmd, cmd)))
	} else {
		bareConditions = append(bareConditions, fmt.Sprintf("__fish_%s_no_subcommand", rootCmd.Name()))
	}
	localNonPersistentFlags.VisitAll(func(flag *pflag.Flag) {
		flagSelector := fmt.Sprintf("-l %s", flag.Name)
		if len(flag.Shorthand) > 0 {
			flagSelector = fmt.Sprintf("-s %s %s", flag.Shorthand, flagSelector)
		}
		bareConditions = append(bareConditions, fmt.Sprintf("not __fish_seen_argument %s", flagSelector))
	})
	return fmt.Sprintf("-n '%s'", strings.Join(bareConditions, "; and "))
}

func completionCondition(rootCmd, cmd *cobra.Command) string {
	condition := fmt.Sprintf("-n '__fish_%s_no_subcommand'", rootCmd.Name())
	if rootCmd != cmd {
		condition = fmt.Sprintf("-n '__fish_%s_seen_subcommand_path %s'", rootCmd.Name(), subCommandPath(rootCmd, cmd))
	}
	return condition
}

func nonCompletableFlag(flag *pflag.Flag) bool {
	return flag.Hidden || len(flag.Deprecated) > 0
}
