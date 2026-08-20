package ghcmd

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	surveyCore "github.com/AlecAivazis/survey/v2/core"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/agents"
	"github.com/cli/cli/v2/internal/build"
	"github.com/cli/cli/v2/internal/ci"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/config/migration"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/gherrs"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/internal/update"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	"github.com/cli/cli/v2/pkg/cmd/root"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	xcolor "github.com/cli/go-gh/v2/pkg/x/color"
	"github.com/cli/safeexec"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type exitCode int

const (
	exitOK    exitCode = 0
	exitError exitCode = 1
)

func Main() exitCode {
	buildDate := build.Date
	buildVersion := build.Version
	hasDebug, _ := utils.IsDebugEnabled()

	cfg, cfgErr := config.NewConfig()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load config: %s\n", cfgErr)
	}
	cfgFunc := func() (gh.Config, error) { return cfg, cfgErr }

	var ioStreams *iostreams.IOStreams
	if cfgErr == nil {
		ioStreams = newIOStreams(cfg)
	} else {
		ioStreams = iostreams.System()
	}
	stderr := ioStreams.ErrOut

	ghExecutablePath := executablePath("gh")

	additionalCommonDimensions := ghtelemetry.Dimensions{
		"version":             strings.TrimPrefix(buildVersion, "v"),
		"is_tty":              strconv.FormatBool(ioStreams.IsStdoutTTY()),
		"agent":               string(agents.Detect()),
		"ci":                  strconv.FormatBool(ci.IsCI()),
		"github_actions":      strconv.FormatBool(ci.IsGitHubActions()),
		"accessible_colors":   strconv.FormatBool(ioStreams.AccessibleColorsEnabled()),
		"accessible_prompter": strconv.FormatBool(ioStreams.AccessiblePrompterEnabled()),
		"color_labels":        strconv.FormatBool(ioStreams.ColorLabels()),
		"spinner_disabled":    strconv.FormatBool(ioStreams.GetSpinnerDisabled()),
	}

	var telemetryService ghtelemetry.Service
	switch {
	case cfgErr != nil:
		// Without a valid on-disk config we can't honour user telemetry preferences, so disable it to be safe.
		telemetryService = &telemetry.NoOpService{}
	default:
		telemetryState := telemetry.ParseTelemetryState(cfg.Telemetry().Value)
		telemetryDisabled := mightBeGHESUser(cfg)

		switch telemetryState {
		case telemetry.Disabled:
			telemetryService = &telemetry.NoOpService{}
		case telemetry.Logged:
			// Always construct the real service in log mode so that the log
			// flusher runs and surfaces an explicit "Telemetry payload: none"
			// marker when no events will be sent. This gives the user an
			// observable signal that telemetry is wired up even when their
			// context (e.g. GHES) causes events to be dropped.
			telemetryService = telemetry.NewService(
				telemetry.LogFlusher(ioStreams.ErrOut, ioStreams.ColorEnabled()),
				telemetry.WithAdditionalCommonDimensions(additionalCommonDimensions),
			)
			if telemetryDisabled {
				telemetryService.Disable()
			}
		case telemetry.Enabled:
			if telemetryDisabled {
				telemetryService = &telemetry.NoOpService{}
				break
			}
			sampleRate := 1
			if v, err := strconv.Atoi(os.Getenv("GH_TELEMETRY_SAMPLE_RATE")); err == nil && v >= 0 && v <= 100 {
				sampleRate = v
			}
			additionalCommonDimensions["sample_rate"] = strconv.Itoa(sampleRate)
			telemetryService = telemetry.NewService(
				telemetry.GitHubFlusher(ghExecutablePath),
				telemetry.WithAdditionalCommonDimensions(additionalCommonDimensions),
				telemetry.WithSampleRate(sampleRate),
			)
		default:
			fmt.Fprintf(stderr, "invalid telemetry configuration: %q\n", cfg.Telemetry().Value)
			return exitError
		}
	}
	defer telemetryService.Flush()

	cmdFactory := factory.New(buildVersion, string(agents.Detect()), cfgFunc, ioStreams, ghExecutablePath, telemetryService)

	if cfgErr == nil {
		var m migration.MultiAccount
		if err := cfg.Migrate(m); err != nil {
			fmt.Fprintln(stderr, err)
			return exitError
		}
	}

	ctx := context.Background()
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

	rootCmd, err := root.NewCmdRoot(cmdFactory, telemetryService, buildVersion, buildDate)
	if err != nil {
		fmt.Fprintf(stderr, "failed to create root command: %s\n", err)
		return exitError
	}

	expandedArgs := []string{}
	if len(os.Args) > 0 {
		expandedArgs = os.Args[1:]
	}

	// translate `gh help <command>` to `gh <command> --help` for extensions.
	if len(expandedArgs) >= 2 && expandedArgs[0] == "help" && root.IsExtensionCommand(rootCmd, expandedArgs[1:]) {
		expandedArgs = expandedArgs[1:]
		expandedArgs = append(expandedArgs, "--help")
	}

	rootCmd.SetArgs(expandedArgs)

	var executedCmd *cobra.Command
	var errorDims ghtelemetry.Dimensions
	defer func() {
		if executedCmd == nil {
			telemetryService.Record(ghtelemetry.Event{
				Type: "missing_command",
			})
			return
		}

		// For telemetry-disabled commands, skip telemetry entirely.
		commandPath := executedCmd.CommandPath()
		if cmdutil.IsTelemetryDisabled(executedCmd) {
			return
		}

		var flags []string
		executedCmd.Flags().Visit(func(f *pflag.Flag) {
			flags = append(flags, f.Name)
		})
		slices.Sort(flags)

		var dimensions = ghtelemetry.Dimensions{
			"command": commandPath,
			"flags":   strings.Join(flags, ","),
		}
		maps.Copy(dimensions, errorDims)

		telemetryService.Record(ghtelemetry.Event{
			Type:       "command_invocation",
			Dimensions: dimensions,
		})
	}()

	if executedCmd, err = rootCmd.ExecuteContextC(ctx); err != nil {
		// Preserve the original error before the switch may replace it with a
		// gherrs sentinel. This lets telemetry capture the real error chain.
		originalErr := err

		var pagerPipeErr *iostreams.ErrClosedPagerPipe
		var noResultsErr cmdutil.NoResultsError
		var extErr *root.ExternalCommandExitError
		var authErr *root.AuthError
		var dnsErr *net.DNSError
		var flagErr *cmdutil.FlagError
		var httpErr api.HTTPError

		switch {
		case errors.Is(err, cmdutil.SilentError):
			err = gherrs.SilentError
		case errors.Is(err, cmdutil.PendingError):
			err = gherrs.PendingError
		case cmdutil.IsUserCancellation(err):
			// This should be fixed at the prompting layer.
			if errors.Is(err, terminal.InterruptErr) {
				fmt.Fprint(stderr, "\n")
			}
			err = gherrs.UserCancellationError
		case errors.As(err, &authErr):
			err = gherrs.AuthError
		case errors.As(err, &pagerPipeErr):
			err = nil // user quit the pager, not really an error
		case errors.As(err, &noResultsErr):
			if cmdFactory.IOStreams.IsStdoutTTY() {
				fmt.Fprintln(stderr, noResultsErr.Error())
			}
			err = nil // no data to show, not really an error
		case errors.As(err, &extErr):
			err = &gherrs.ExtensionExecError{Code: extErr.ExitCode()}
		case errors.As(err, &dnsErr):
			var s strings.Builder
			fmt.Fprintf(&s, "error connecting to %s\n", dnsErr.Name)
			if hasDebug {
				fmt.Fprintf(&s, "%v\n", dnsErr)
			}
			fmt.Fprint(&s, "check your internet connection or https://githubstatus.com")
			// Do not set WrappedErr here - the original DNS error is captured
			// separately via originalErr for telemetry, and including it in the
			// displayed Error() would prepend the raw DNS message before the
			// user-friendly guidance.
			err = gherrs.GeneralError{Message: s.String()}
		case strings.Contains(err.Error(), "Incorrect function"):
			var s strings.Builder
			fmt.Fprintln(&s, "You appear to be running in MinTTY without pseudo terminal support.")
			fmt.Fprint(&s, "To learn about workarounds for this error, run:  gh help mintty")
			err = gherrs.GeneralError{WrappedErr: err, Message: s.String()}
		default:
			httpErrMatched := errors.As(err, &httpErr)

			if errors.As(err, &flagErr) || strings.HasPrefix(err.Error(), "unknown command ") {
				var s strings.Builder
				if !strings.HasSuffix(err.Error(), "\n") {
					fmt.Fprintln(&s)
				}
				fmt.Fprint(&s, executedCmd.UsageString())
				err = gherrs.GeneralError{WrappedErr: err, Message: s.String()}
			} else if httpErrMatched && httpErr.StatusCode == 401 {
				authCommand := "gh auth login"
				if cfg, cfgErr := cmdFactory.Config(); cfgErr == nil {
					authCommand = authRecoveryCommand(cfg, httpErr)
				}
				err = gherrs.GeneralError{WrappedErr: err, Message: fmt.Sprintf("Try authenticating with:  %s", authCommand)}
			} else if httpErrMatched {
				if u := factory.SSOURL(); u != "" {
					err = gherrs.GeneralError{WrappedErr: err, Message: fmt.Sprintf("Authorize in your web browser:  %s", u)}
				} else if msg := httpErr.ScopesSuggestion(); msg != "" {
					err = gherrs.GeneralError{WrappedErr: err, Message: msg}
				}
			}
		}

		if err == nil {
			return exitOK
		}

		errorDims = newErrDims(err, originalErr)

		var silenced gherrs.Silenced
		if !errors.As(err, &silenced) {
			fmt.Fprintln(stderr, err)
		}

		if exitCoder, ok := errors.AsType[gherrs.ExitCoder](err); ok {
			return exitCode(exitCoder.ExitCode())
		}

		return exitError
	}

	if root.HasFailed() {
		errorDims = ghtelemetry.Dimensions{"outcome": "error"}
		return exitError
	}

	updateCancel() // if the update checker hasn't completed by now, abort it
	newRelease := <-updateMessageChan
	if newRelease != nil {
		isHomebrew := isUnderHomebrew(cmdFactory.ExecutablePath)
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

func newErrDims(mappedErr, originalErr error) ghtelemetry.Dimensions {
	errTypes := grabAllUnwrappableNestedErrorTypes(originalErr)

	if errors.Is(mappedErr, gherrs.PendingError) || errors.Is(mappedErr, gherrs.UserCancellationError) {
		return ghtelemetry.Dimensions{"outcome": "success", "errTypes": errTypes}
	}

	return ghtelemetry.Dimensions{
		"outcome":  "error",
		"errTypes": errTypes,
	}
}

// noiseErrorTypes are common stdlib wrapper types that provide no analytical
// value in telemetry. We skip them to keep the errTypes dimension focused on
// domain-specific error types that are actually interesting.
var noiseErrorTypes = map[string]bool{
	"errors.errorString": true,
	"errors.joinError":   true,
	"fmt.wrapError":      true,
}

// This is a pretty janky way to get some privacy-respecting visibility into
// what kind of error we're dealing with. It is not at all intended to be comprehensive.
func grabAllUnwrappableNestedErrorTypes(err error) string {
	var types []string
	queue := []error{err}
	for len(queue) > 0 && len(types) < 100 {
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}

		t := strings.TrimPrefix(fmt.Sprintf("%T", current), "*")
		if !noiseErrorTypes[t] {
			types = append(types, t)
		}

		// Traverse single-wrapped errors and multi-errors (e.g. errors.Join).
		if u, ok := current.(interface{ Unwrap() error }); ok {
			if next := u.Unwrap(); next != nil {
				queue = append(queue, next)
			}
		} else if u, ok := current.(interface{ Unwrap() []error }); ok {
			queue = append(queue, u.Unwrap()...)
		}
	}
	return strings.Join(types, ",")
}

func authRecoveryCommand(cfg gh.Config, httpErr api.HTTPError) string {
	if httpErr.RequestURL == nil {
		return "gh auth login"
	}

	hostname := ghauth.NormalizeHostname(httpErr.RequestURL.Hostname())
	token, source := cfg.Authentication().ActiveToken(hostname)
	if shared.AuthTokenRefreshable(token, source) {
		return fmt.Sprintf("gh auth refresh -h %s", hostname)
	}

	return fmt.Sprintf("gh auth login -h %s", hostname)
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

func newIOStreams(cfg gh.Config) *iostreams.IOStreams {
	io := iostreams.System()

	if _, ghPromptDisabled := os.LookupEnv("GH_PROMPT_DISABLED"); ghPromptDisabled {
		io.SetNeverPrompt(true)
	} else if prompt := cfg.Prompt(""); prompt.Value == "disabled" {
		io.SetNeverPrompt(true)
	}

	falseyValues := []string{"false", "0", "no", ""}

	accessiblePrompterValue, accessiblePrompterIsSet := os.LookupEnv("GH_ACCESSIBLE_PROMPTER")
	if accessiblePrompterIsSet {
		if !slices.Contains(falseyValues, accessiblePrompterValue) {
			io.SetAccessiblePrompterEnabled(true)
		}
	} else if prompt := cfg.AccessiblePrompter(""); prompt.Value == "enabled" {
		io.SetAccessiblePrompterEnabled(true)
	}

	experimentalPrompterValue, experimentalPrompterIsSet := os.LookupEnv("GH_EXPERIMENTAL_PROMPTER")
	if experimentalPrompterIsSet {
		if !slices.Contains(falseyValues, experimentalPrompterValue) {
			io.SetExperimentalPrompterEnabled(true)
		}
	}

	ghSpinnerDisabledValue, ghSpinnerDisabledIsSet := os.LookupEnv("GH_SPINNER_DISABLED")
	if ghSpinnerDisabledIsSet {
		if !slices.Contains(falseyValues, ghSpinnerDisabledValue) {
			io.SetSpinnerDisabled(true)
		}
	} else if spinnerDisabled := cfg.Spinner(""); spinnerDisabled.Value == "disabled" {
		io.SetSpinnerDisabled(true)
	}

	// Pager precedence
	// 1. GH_PAGER
	// 2. pager from config
	// 3. PAGER
	if ghPager, ghPagerExists := os.LookupEnv("GH_PAGER"); ghPagerExists {
		io.SetPager(ghPager)
	} else if pager := cfg.Pager(""); pager.Value != "" {
		io.SetPager(pager.Value)
	}

	if ghColorLabels, ghColorLabelsExists := os.LookupEnv("GH_COLOR_LABELS"); ghColorLabelsExists {
		switch ghColorLabels {
		case "", "0", "false", "no":
			io.SetColorLabels(false)
		default:
			io.SetColorLabels(true)
		}
	} else if prompt := cfg.ColorLabels(""); prompt.Value == "enabled" {
		io.SetColorLabels(true)
	}

	io.SetAccessibleColorsEnabled(xcolor.IsAccessibleColorsEnabled())

	return io
}

// Executable is the path to the currently invoked binary
func executablePath(executableName string) string {
	ghPath := os.Getenv("GH_PATH")
	if ghPath != "" {
		return ghPath
	}

	if strings.ContainsRune(executableName, os.PathSeparator) {
		return executableName
	}

	return executable(executableName)
}

// Finds the location of the executable for the current process as it's found in PATH, respecting symlinks.
// If the process couldn't determine its location, return fallbackName. If the executable wasn't found in
// PATH, return the absolute location to the program.
//
// The idea is that the result of this function is callable in the future and refers to the same
// installation of gh, even across upgrades. This is needed primarily for Homebrew, which installs software
// under a location such as `/usr/local/Cellar/gh/1.13.1/bin/gh` and symlinks it from `/usr/local/bin/gh`.
// When the version is upgraded, Homebrew will often delete older versions, but keep the symlink. Because of
// this, we want to refer to the `gh` binary as `/usr/local/bin/gh` and not as its internal Homebrew
// location.
//
// None of this would be needed if we could just refer to GitHub CLI as `gh`, i.e. without using an absolute
// path. However, for some reason Homebrew does not include `/usr/local/bin` in PATH when it invokes git
// commands to update its taps. If `gh` (no path) is being used as git credential helper, as set up by `gh
// auth login`, running `brew update` will print out authentication errors as git is unable to locate
// Homebrew-installed `gh`
func executable(fallback string) string {
	exe, err := os.Executable()
	if err != nil {
		return fallback
	}

	base := filepath.Base(exe)
	path := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(path) {
		p, err := filepath.Abs(filepath.Join(dir, base))
		if err != nil {
			continue
		}
		f, err := os.Lstat(p)
		if err != nil {
			continue
		}

		if p == exe {
			return p
		} else if f.Mode()&os.ModeSymlink != 0 {
			realP, err := filepath.EvalSymlinks(p)
			if err != nil {
				continue
			}
			realExe, err := filepath.EvalSymlinks(exe)
			if err != nil {
				continue
			}
			if realP == realExe {
				return p
			}
		}
	}

	return exe
}

func mightBeGHESUser(cfg gh.Config) bool {
	if os.Getenv("GH_ENTERPRISE_TOKEN") != "" || os.Getenv("GITHUB_ENTERPRISE_TOKEN") != "" {
		return true
	}

	if host := os.Getenv("GH_HOST"); host != "" && ghauth.IsEnterprise(host) {
		return true
	}

	// If any targeted host is Enterprise, then the user is likely a GHES user.
	return slices.ContainsFunc(cfg.Authentication().Hosts(), func(host string) bool {
		return ghauth.IsEnterprise(host)
	})
}
