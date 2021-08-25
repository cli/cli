package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	surveyCore "github.com/AlecAivazis/survey/v2/core"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/build"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/internal/update"
	"github.com/cli/cli/v2/pkg/cmd/alias/expand"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	"github.com/cli/cli/v2/pkg/cmd/root"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/utils"
	"github.com/cli/safeexec"
	"github.com/mattn/go-colorable"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
)

var updaterEnabled = ""

type exitCode int

const (
	exitOK     exitCode = 0
	exitError  exitCode = 1
	exitCancel exitCode = 2
	exitAuth   exitCode = 4
)

func main() {
	code := mainRun()
	os.Exit(int(code))
}

func mainRun() exitCode {
	buildDate := build.Date
	buildVersion := build.Version

	updateMessageChan := make(chan *update.ReleaseInfo)
	go func() {
		rel, _ := checkForUpdate(buildVersion)
		updateMessageChan <- rel
	}()

	hasDebug := os.Getenv("DEBUG") != ""

	cmdFactory := factory.New(buildVersion)
	stderr := cmdFactory.IOStreams.ErrOut

	if spec := os.Getenv("GH_FORCE_TTY"); spec != "" {
		cmdFactory.IOStreams.ForceTerminal(spec)
	}
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

	// Enable running gh from Windows File Explorer's address bar. Without this, the user is told to stop and run from a
	// terminal. With this, a user can clone a repo (or take other actions) directly from explorer.
	if len(os.Args) > 1 && os.Args[1] != "" {
		cobra.MousetrapHelpText = ""
	}

	rootCmd := root.NewCmdRoot(cmdFactory, buildVersion, buildDate)

	cfg, err := cmdFactory.Config()
	if err != nil {
		fmt.Fprintf(stderr, "failed to read configuration:  %s\n", err)
		return exitError
	}

	// TODO: remove after FromFullName has been revisited
	if host, err := cfg.DefaultHost(); err == nil {
		ghrepo.SetDefaultHost(host)
	}

	expandedArgs := []string{}
	if len(os.Args) > 0 {
		expandedArgs = os.Args[1:]
	}

	// translate `gh help <command>` to `gh <command> --help` for extensions
	if len(expandedArgs) == 2 && expandedArgs[0] == "help" && !hasCommand(rootCmd, expandedArgs[1:]) {
		expandedArgs = []string{expandedArgs[1], "--help"}
	}

	if !hasCommand(rootCmd, expandedArgs) {
		originalArgs := expandedArgs
		isShell := false

		argsForExpansion := append([]string{"gh"}, expandedArgs...)
		expandedArgs, isShell, err = expand.ExpandAlias(cfg, argsForExpansion, nil)
		if err != nil {
			fmt.Fprintf(stderr, "failed to process aliases:  %s\n", err)
			return exitError
		}

		if hasDebug {
			fmt.Fprintf(stderr, "%v -> %v\n", originalArgs, expandedArgs)
		}

		if isShell {
			exe, err := safeexec.LookPath(expandedArgs[0])
			if err != nil {
				fmt.Fprintf(stderr, "failed to run external command: %s", err)
				return exitError
			}

			externalCmd := exec.Command(exe, expandedArgs[1:]...)
			externalCmd.Stderr = os.Stderr
			externalCmd.Stdout = os.Stdout
			externalCmd.Stdin = os.Stdin
			preparedCmd := run.PrepareCmd(externalCmd)

			err = preparedCmd.Run()
			if err != nil {
				var execError *exec.ExitError
				if errors.As(err, &execError) {
					return exitCode(execError.ExitCode())
				}
				fmt.Fprintf(stderr, "failed to run external command: %s", err)
				return exitError
			}

			return exitOK
		} else if len(expandedArgs) > 0 && !hasCommand(rootCmd, expandedArgs) {
			extensionManager := cmdFactory.ExtensionManager
			if found, err := extensionManager.Dispatch(expandedArgs, os.Stdin, os.Stdout, os.Stderr); err != nil {
				var execError *exec.ExitError
				if errors.As(err, &execError) {
					return exitCode(execError.ExitCode())
				}
				fmt.Fprintf(stderr, "failed to run extension: %s", err)
				return exitError
			} else if found {
				return exitOK
			}
		}
	}

	// provide completions for aliases and extensions
	rootCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var results []string
		if aliases, err := cfg.Aliases(); err == nil {
			for aliasName := range aliases.All() {
				if strings.HasPrefix(aliasName, toComplete) {
					results = append(results, aliasName)
				}
			}
		}
		for _, ext := range cmdFactory.ExtensionManager.List(false) {
			if strings.HasPrefix(ext.Name(), toComplete) {
				results = append(results, ext.Name())
			}
		}
		return results, cobra.ShellCompDirectiveNoFileComp
	}

	cs := cmdFactory.IOStreams.ColorScheme()

	authError := errors.New("authError")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// require that the user is authenticated before running most commands
		if cmdutil.IsAuthCheckEnabled(cmd) && !cmdutil.CheckAuth(cfg) {
			fmt.Fprintln(stderr, cs.Bold("Welcome to GitHub CLI!"))
			fmt.Fprintln(stderr)
			fmt.Fprintln(stderr, "To authenticate, please run `gh auth login`.")
			return authError
		}

		return nil
	}

	rootCmd.SetArgs(expandedArgs)

	if cmd, err := rootCmd.ExecuteC(); err != nil {
		if err == cmdutil.SilentError {
			return exitError
		} else if cmdutil.IsUserCancellation(err) {
			if errors.Is(err, terminal.InterruptErr) {
				// ensure the next shell prompt will start on its own line
				fmt.Fprint(stderr, "\n")
			}
			return exitCancel
		} else if errors.Is(err, authError) {
			return exitAuth
		}

		printError(stderr, err, cmd, hasDebug)

		if strings.Contains(err.Error(), "Incorrect function") {
			fmt.Fprintln(stderr, "You appear to be running in MinTTY without pseudo terminal support.")
			fmt.Fprintln(stderr, "To learn about workarounds for this error, run: gh help mintty")
			return exitError
		}

		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 401 {
			fmt.Fprintln(stderr, "hint: try authenticating with `gh auth login`")
		}

		return exitError
	}
	if root.HasFailed() {
		return exitError
	}

	newRelease := <-updateMessageChan
	if newRelease != nil {
		isHomebrew := isUnderHomebrew(cmdFactory.Executable)
		if isHomebrew && isRecentRelease(newRelease.PublishedAt) {
			// do not notify Homebrew users before the version bump had a chance to get merged into homebrew-core
			return exitOK
		}
		fmt.Fprintf(stderr, "\n\n%s %s → %s\n",
			ansi.Color("A new release of gh is available:", "yellow"),
			ansi.Color(buildVersion, "cyan"),
			ansi.Color(newRelease.Version, "cyan"))
		if isHomebrew {
			fmt.Fprintf(stderr, "To upgrade, run: %s\n", "brew update && brew upgrade gh")
		}
		fmt.Fprintf(stderr, "%s\n\n",
			ansi.Color(newRelease.URL, "yellow"))
	}

	return exitOK
}

// hasCommand returns true if args resolve to a built-in command
func hasCommand(rootCmd *cobra.Command, args []string) bool {
	c, _, err := rootCmd.Traverse(args)
	return err == nil && c != rootCmd
}

func printError(out io.Writer, err error, cmd *cobra.Command, debug bool) {
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		fmt.Fprintf(out, "error connecting to %s\n", dnsError.Name)
		if debug {
			fmt.Fprintln(out, dnsError)
		}
		fmt.Fprintln(out, "check your internet connection or https://githubstatus.com")
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
	if os.Getenv("GH_NO_UPDATE_NOTIFIER") != "" {
		return false
	}
	if os.Getenv("CODESPACES") != "" {
		return false
	}
	return updaterEnabled != "" && !isCI() && utils.IsTerminal(os.Stdout) && utils.IsTerminal(os.Stderr)
}

// based on https://github.com/watson/ci-info/blob/HEAD/index.js
func isCI() bool {
	return os.Getenv("CI") != "" || // GitHub Actions, Travis CI, CircleCI, Cirrus CI, GitLab CI, AppVeyor, CodeShip, dsari
		os.Getenv("BUILD_NUMBER") != "" || // Jenkins, TeamCity
		os.Getenv("RUN_ID") != "" // TaskCluster, dsari
}

func checkForUpdate(currentVersion string) (*update.ReleaseInfo, error) {
	if !shouldCheckForUpdate() {
		return nil, nil
	}

	client, err := basicClient(currentVersion)
	if err != nil {
		return nil, err
	}

	repo := updaterEnabled
	stateFilePath := filepath.Join(config.StateDir(), "state.yml")
	return update.CheckForUpdate(client, stateFilePath, repo, currentVersion)
}

// BasicClient returns an API client for github.com only that borrows from but
// does not depend on user configuration
func basicClient(currentVersion string) (*api.Client, error) {
	var opts []api.ClientOption
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, apiVerboseLog())
	}
	opts = append(opts, api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", currentVersion)))

	token, _ := config.AuthTokenFromEnv(ghinstance.Default())
	if token == "" {
		if c, err := config.ParseDefaultConfig(); err == nil {
			token, _ = c.Get(ghinstance.Default(), "oauth_token")
		}
	}
	if token != "" {
		opts = append(opts, api.AddHeader("Authorization", fmt.Sprintf("token %s", token)))
	}
	return api.NewClient(opts...), nil
}

func apiVerboseLog() api.ClientOption {
	logTraffic := strings.Contains(os.Getenv("DEBUG"), "api")
	colorize := utils.IsTerminal(os.Stderr)
	return api.VerboseLog(colorable.NewColorable(os.Stderr), logTraffic, colorize)
}

func isRecentRelease(publishedAt time.Time) bool {
	return !publishedAt.IsZero() && time.Since(publishedAt) < time.Hour*24
}

// Check whether the gh binary was found under the Homebrew prefix
func isUnderHomebrew(ghBinary string) bool {
	brewExe, err := safeexec.LookPath("brew")
	if err != nil {
		return false
	}

	brewPrefixBytes, err := exec.Command(brewExe, "--prefix").Output()
	if err != nil {
		return false
	}

	brewBinPrefix := filepath.Join(strings.TrimSpace(string(brewPrefixBytes)), "bin") + string(filepath.Separator)
	return strings.HasPrefix(ghBinary, brewBinPrefix)
}
