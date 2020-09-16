package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"strings"

	surveyCore "github.com/AlecAivazis/survey/v2/core"
	"github.com/cli/cli/api"
	"github.com/cli/cli/command"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmd/alias/expand"
	"github.com/cli/cli/pkg/cmd/factory"
	"github.com/cli/cli/pkg/cmd/root"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/update"
	"github.com/cli/cli/utils"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
)

var updaterEnabled = ""

func main() {
	currentVersion := command.Version
	updateMessageChan := make(chan *update.ReleaseInfo)
	go func() {
		rel, _ := checkForUpdate(currentVersion)
		updateMessageChan <- rel
	}()

	hasDebug := os.Getenv("DEBUG") != ""

	if hostFromEnv := os.Getenv("GH_HOST"); hostFromEnv != "" {
		ghinstance.OverrideDefault(hostFromEnv)
	}

	cmdFactory := factory.New(command.Version)
	stderr := cmdFactory.IOStreams.ErrOut
	if !cmdFactory.IOStreams.ColorEnabled() {
		surveyCore.DisableColor = true
	} else {
		// override survey's poor choice of color
		surveyCore.TemplateFuncsWithColor["color"] = func(style string) string {
			switch style {
			case "white":
				if cmdFactory.IOStreams.ColorSupport256() {
					return fmt.Sprintf("\x1b[%d;5;%dm", 38, 242)
				}
				return ansi.ColorCode("default")
			default:
				return ansi.ColorCode(style)
			}
		}
	}

	rootCmd := root.NewCmdRoot(cmdFactory, command.Version, command.BuildDate)

	cfg, err := cmdFactory.Config()
	if err != nil {
		fmt.Fprintf(stderr, "failed to read configuration:  %s\n", err)
		os.Exit(2)
	}

	if prompt, _ := cfg.Get("", "prompt"); prompt == config.PromptsDisabled {
		cmdFactory.IOStreams.SetNeverPrompt(true)
	}

	if pager, _ := cfg.Get("", "pager"); pager != "" {
		cmdFactory.IOStreams.SetPager(pager)
	}

	expandedArgs := []string{}
	if len(os.Args) > 0 {
		expandedArgs = os.Args[1:]
	}

	cmd, _, err := rootCmd.Traverse(expandedArgs)
	if err != nil || cmd == rootCmd {
		originalArgs := expandedArgs
		isShell := false

		expandedArgs, isShell, err = expand.ExpandAlias(cfg, os.Args, nil)
		if err != nil {
			fmt.Fprintf(stderr, "failed to process aliases:  %s\n", err)
			os.Exit(2)
		}

		if hasDebug {
			fmt.Fprintf(stderr, "%v -> %v\n", originalArgs, expandedArgs)
		}

		if isShell {
			externalCmd := exec.Command(expandedArgs[0], expandedArgs[1:]...)
			externalCmd.Stderr = os.Stderr
			externalCmd.Stdout = os.Stdout
			externalCmd.Stdin = os.Stdin
			preparedCmd := run.PrepareCmd(externalCmd)

			err = preparedCmd.Run()
			if err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					os.Exit(ee.ExitCode())
				}

				fmt.Fprintf(stderr, "failed to run external command: %s", err)
				os.Exit(3)
			}

			os.Exit(0)
		}
	}

	authCheckEnabled := os.Getenv("GITHUB_TOKEN") == "" &&
		os.Getenv("GITHUB_ENTERPRISE_TOKEN") == "" &&
		cmd != nil && cmdutil.IsAuthCheckEnabled(cmd)
	if authCheckEnabled {
		if !cmdutil.CheckAuth(cfg) {
			fmt.Fprintln(stderr, utils.Bold("Welcome to GitHub CLI!"))
			fmt.Fprintln(stderr)
			fmt.Fprintln(stderr, "To authenticate, please run `gh auth login`.")
			fmt.Fprintln(stderr, "You can also set the GITHUB_TOKEN environment variable, if preferred.")
			os.Exit(4)
		}
	}

	rootCmd.SetArgs(expandedArgs)

	if cmd, err := rootCmd.ExecuteC(); err != nil {
		printError(stderr, err, cmd, hasDebug)

		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 401 {
			fmt.Println("hint: try authenticating with `gh auth login`")
		}

		os.Exit(1)
	}
	if root.HasFailed() {
		os.Exit(1)
	}

	newRelease := <-updateMessageChan
	if newRelease != nil {
		msg := fmt.Sprintf("%s %s → %s\n%s",
			ansi.Color("A new release of gh is available:", "yellow"),
			ansi.Color(currentVersion, "cyan"),
			ansi.Color(newRelease.Version, "cyan"),
			ansi.Color(newRelease.URL, "yellow"))

		fmt.Fprintf(stderr, "\n\n%s\n\n", msg)
	}
}

func printError(out io.Writer, err error, cmd *cobra.Command, debug bool) {
	if err == cmdutil.SilentError {
		return
	}

	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		fmt.Fprintf(out, "error connecting to %s\n", dnsError.Name)
		if debug {
			fmt.Fprintln(out, dnsError)
		}
		fmt.Fprintln(out, "check your internet connection or githubstatus.com")
		return
	}

	fmt.Fprintln(out, err)

	var flagError *cmdutil.FlagError
	if errors.As(err, &flagError) || strings.HasPrefix(err.Error(), "unknown command ") {
		if !strings.HasSuffix(err.Error(), "\n") {
			fmt.Fprintln(out)
		}
		fmt.Fprintln(out, cmd.UsageString())
	}
}

func shouldCheckForUpdate() bool {
	return updaterEnabled != "" && !isCompletionCommand() && utils.IsTerminal(os.Stderr)
}

func isCompletionCommand() bool {
	return len(os.Args) > 1 && os.Args[1] == "completion"
}

func checkForUpdate(currentVersion string) (*update.ReleaseInfo, error) {
	if !shouldCheckForUpdate() {
		return nil, nil
	}

	client, err := command.BasicClient()
	if err != nil {
		return nil, err
	}

	repo := updaterEnabled
	stateFilePath := path.Join(config.ConfigDir(), "state.yml")
	return update.CheckForUpdate(client, stateFilePath, repo, currentVersion)
}
