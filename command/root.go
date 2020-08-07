package command

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmd/factory"
	"github.com/cli/cli/pkg/cmd/root"
	"github.com/cli/cli/utils"
	"github.com/google/shlex"

	"github.com/spf13/cobra"
)

// Version is dynamically set by the toolchain or overridden by the Makefile.
var Version = "DEV"

// BuildDate is dynamically set at build time in the Makefile.
var BuildDate = "" // YYYY-MM-DD

var RootCmd *cobra.Command

func init() {
	if Version == "DEV" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}

	cmdFactory := factory.New(Version)
	RootCmd = root.NewCmdRoot(cmdFactory, Version, BuildDate)
	RootCmd.AddCommand(aliasCmd)
	RootCmd.AddCommand(root.NewCmdCompletion(cmdFactory.IOStreams))
	RootCmd.AddCommand(configCmd)
}

// overridden in tests
var initContext = func() context.Context {
	return context.New()
}

// BasicClient returns an API client for github.com only that borrows from but
// does not depend on user configuration
func BasicClient() (*api.Client, error) {
	var opts []api.ClientOption
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, apiVerboseLog())
	}
	opts = append(opts, api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", Version)))

	token := os.Getenv("GITHUB_TOKEN")
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

func contextForCommand(cmd *cobra.Command) context.Context {
	return initContext()
}

func apiVerboseLog() api.ClientOption {
	logTraffic := strings.Contains(os.Getenv("DEBUG"), "api")
	colorize := utils.IsTerminal(os.Stderr)
	return api.VerboseLog(utils.NewColorable(os.Stderr), logTraffic, colorize)
}

func colorableOut(cmd *cobra.Command) io.Writer {
	out := cmd.OutOrStdout()
	if outFile, isFile := out.(*os.File); isFile {
		return utils.NewColorable(outFile)
	}
	return out
}

func colorableErr(cmd *cobra.Command) io.Writer {
	err := cmd.ErrOrStderr()
	if outFile, isFile := err.(*os.File); isFile {
		return utils.NewColorable(outFile)
	}
	return err
}

func ExecuteShellAlias(args []string) error {
	externalCmd := exec.Command(args[0], args[1:]...)
	externalCmd.Stderr = os.Stderr
	externalCmd.Stdout = os.Stdout
	externalCmd.Stdin = os.Stdin
	preparedCmd := run.PrepareCmd(externalCmd)

	return preparedCmd.Run()
}

var findSh = func() (string, error) {
	shPath, err := exec.LookPath("sh")
	if err == nil {
		return shPath, nil
	}

	if runtime.GOOS == "windows" {
		winNotFoundErr := errors.New("unable to locate sh to execute the shell alias with. The sh.exe interpreter is typically distributed with Git for Windows.")
		// We can try and find a sh executable in a Git for Windows install
		gitPath, err := exec.LookPath("git")
		if err != nil {
			return "", winNotFoundErr
		}

		shPath = filepath.Join(filepath.Dir(gitPath), "..", "bin", "sh.exe")
		_, err = os.Stat(shPath)
		if err != nil {
			return "", winNotFoundErr
		}

		return shPath, nil
	}

	return "", errors.New("unable to locate sh to execute shell alias with")
}

// ExpandAlias processes argv to see if it should be rewritten according to a user's aliases. The
// second return value indicates whether the alias should be executed in a new shell process instead
// of running gh itself.
func ExpandAlias(args []string) (expanded []string, isShell bool, err error) {
	err = nil
	isShell = false
	expanded = []string{}

	if len(args) < 2 {
		// the command is lacking a subcommand
		return
	}

	ctx := initContext()
	cfg, err := ctx.Config()
	if err != nil {
		return
	}
	aliases, err := cfg.Aliases()
	if err != nil {
		return
	}

	expansion, ok := aliases.Get(args[1])
	if ok {
		if strings.HasPrefix(expansion, "!") {
			isShell = true
			shPath, shErr := findSh()
			if shErr != nil {
				err = shErr
				return
			}

			expanded = []string{shPath, "-c", expansion[1:]}

			if len(args[2:]) > 0 {
				expanded = append(expanded, "--")
				expanded = append(expanded, args[2:]...)
			}

			return
		}

		extraArgs := []string{}
		for i, a := range args[2:] {
			if !strings.Contains(expansion, "$") {
				extraArgs = append(extraArgs, a)
			} else {
				expansion = strings.ReplaceAll(expansion, fmt.Sprintf("$%d", i+1), a)
			}
		}
		lingeringRE := regexp.MustCompile(`\$\d`)
		if lingeringRE.MatchString(expansion) {
			err = fmt.Errorf("not enough arguments for alias: %s", expansion)
			return
		}

		var newArgs []string
		newArgs, err = shlex.Split(expansion)
		if err != nil {
			return
		}

		expanded = append(newArgs, extraArgs...)
		return
	}

	expanded = args[1:]
	return
}

func connectedToTerminal(cmd *cobra.Command) bool {
	return utils.IsTerminal(cmd.InOrStdin()) && utils.IsTerminal(cmd.OutOrStdout())
}
