package ghcmd

import (
	"context"
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
	"github.com/cli/cli/v2/internal/config/migration"
	"github.com/cli/cli/v2/internal/update"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	"github.com/cli/cli/v2/pkg/cmd/root"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/cli/safeexec"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
)

// updaterEnabled is a statically linked build property set in gh formula within homebrew/homebrew-core
// used to control whether users are notified of newer GitHub CLI releases.
// This needs to be set to 'cli/cli' as it affects where update.CheckForUpdate() checks for releases.
// It is unclear whether this means that only homebrew builds will check for updates or not.
// Development builds leave this empty as impossible to determine if newer or not.
// For more information, <https://github.com/Homebrew/homebrew-core/blob/master/Formula/g/gh.rb>.
var updaterEnabled = ""

type exitCode int

const (
	exitOK      exitCode = 0
	exitError   exitCode = 1
	exitCancel  exitCode = 2
	exitAuth    exitCode = 4
	exitPending exitCode = 8
)

func Main() exitCode {
	buildDate := build.Date
	buildVersion := build.Version
	hasDebug, _ := utils.IsDebugEnabled()

	cmdFactory := factory.New(buildVersion)
	stderr := cmdFactory.IOStreams.ErrOut

	ctx := context.Background()

	if cfg, err := cmdFactory.Config(); err == nil {
		var m migration.MultiAccount
		if err := cfg.Migrate(m); err != nil {
			fmt.Fprintln(stderr, err)
			return exitError
		}
	}

	updateCtx, updateCancel := context.WithCancel(ctx)
	defer updateCancel()
	updateMessageChan := make(chan *update.ReleaseInfo)
	go func() {
		rel, err := checkForUpdate(updateCtx, cmdFactory, buildVersion)
		if err != nil && hasDebug {
			fmt.Fprintf(stderr, "warning: checking for update failed: %v", err)
		}
		updateMessageChan <- rel
	}()

	if !cmdFactory.IOStreams.ColorEnabled() {
		surveyCore.DisableColor = true
		ansi.DisableColors(true)
	} else {
		// override survey's poor choice of color
		surveyCore.TemplateFuncsWithColor["color"] = func(style string) string {
			switch style {
			case "white":
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

	rootCmd, err := root.NewCmdRoot(cmdFactory, buildVersion, buildDate)
	if err != nil {
		fmt.Fprintf(stderr, "failed to create root command: %s\n", err)
		return exitError
	}

	expandedArgs := []string{}
	if len(os.Args) > 0 {
		expandedArgs = os.Args[1:]
	}

	// translate `gh help <command>` to `gh <command> --help` for extensions.
	if len(expandedArgs) >= 2 && expandedArgs[0] == "help" && isExtensionCommand(rootCmd, expandedArgs[1:]) {
		expandedArgs = expandedArgs[1:]
		expandedArgs = append(expandedArgs, "--help")
	}

	rootCmd.SetArgs(expandedArgs)

	if cmd, err := rootCmd.ExecuteContextC(ctx); err != nil {
		var pagerPipeError *iostreams.ErrClosedPagerPipe
		var noResultsError cmdutil.NoResultsError
		var extError *root.ExternalCommandExitError
		var authError *root.AuthError
		if err == cmdutil.SilentError {
			return exitError
		} else if err == cmdutil.PendingError {
			return exitPending
		} else if cmdutil.IsUserCancellation(err) {
			if errors.Is(err, terminal.InterruptErr) {
				// ensure the next shell prompt will start on its own line
				fmt.Fprint(stderr, "\n")
			}
			return exitCancel
		} else if errors.As(err, &authError) {
			return exitAuth
		} else if errors.As(err, &pagerPipeError) {
			// ignore the error raised when piping to a closed pager
			return exitOK
		} else if errors.As(err, &noResultsError) {
			if cmdFactory.IOStreams.IsStdoutTTY() {
				fmt.Fprintln(stderr, noResultsError.Error())
			}
			// no results is not a command failure
			return exitOK
		} else if errors.As(err, &extError) {
			// pass on exit codes from extensions and shell aliases
			return exitCode(extError.ExitCode())
		}

		printError(stderr, err, cmd, hasDebug)

		if strings.Contains(err.Error(), "Incorrect function") {
			fmt.Fprintln(stderr, "You appear to be running in MinTTY without pseudo terminal support.")
			fmt.Fprintln(stderr, "To learn about workarounds for this error, run:  gh help mintty")
			return exitError
		}

		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == 401 {
			fmt.Fprintln(stderr, "Try authenticating with:  gh auth login")
		} else if u := factory.SSOURL(); u != "" {
			// handles organization SAML enforcement error
			fmt.Fprintf(stderr, "Authorize in your web browser:  %s\n", u)
		} else if msg := httpErr.ScopesSuggestion(); msg != "" {
			fmt.Fprintln(stderr, msg)
		}

		return exitError
	}
	if root.HasFailed() {
		return exitError
	}

	updateCancel() // if the update checker hasn't completed by now, abort it
	newRelease := <-updateMessageChan
	if newRelease != nil {
		isHomebrew := isUnderHomebrew(cmdFactory.Executable())
		if isHomebrew && isRecentRelease(newRelease.PublishedAt) {
			// do not notify Homebrew users before the version bump had a chance to get merged into homebrew-core
			return exitOK
		}
		fmt.Fprintf(stderr, "\n\n%s %s → %s\n",
			ansi.Color("A new release of gh is available:", "yellow"),
			ansi.Color(strings.TrimPrefix(buildVersion, "v"), "cyan"),
			ansi.Color(strings.TrimPrefix(newRelease.Version, "v"), "cyan"))
		if isHomebrew {
			fmt.Fprintf(stderr, "To upgrade, run: %s\n", "brew upgrade gh")
		}
		fmt.Fprintf(stderr, "%s\n\n",
			ansi.Color(newRelease.URL, "yellow"))
	}

	return exitOK
}

// isExtensionCommand returns true if args resolve to an extension command.
func isExtensionCommand(rootCmd *cobra.Command, args []string) bool {
	c, _, err := rootCmd.Find(args)
	return err == nil && c != nil && c.GroupID == "extension"
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

func checkForUpdate(ctx context.Context, f *cmdutil.Factory, currentVersion string) (*update.ReleaseInfo, error) {
	if updaterEnabled == "" || !update.ShouldCheckForUpdate() {
		return nil, nil
	}
	httpClient, err := f.HttpClient()
	if err != nil {
		return nil, err
	}
	stateFilePath := filepath.Join(config.StateDir(), "state.yml")
	return update.CheckForUpdate(ctx, httpClient, stateFilePath, updaterEnabled, currentVersion)
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
