package expand

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/findsh"
	"github.com/google/shlex"
)

// ExpandAlias processes argv to see if it should be rewritten according to a user's aliases. The
// second return value indicates whether the alias should be executed in a new shell process instead
// of running gh itself.
func ExpandAlias(cfg config.Config, args []string, findShFunc func() (string, error)) (expanded []string, isShell bool, err error) {
	if len(args) < 2 {
		// the command is lacking a subcommand
		return
	}
	expanded = args[1:]

	aliases, err := cfg.Aliases()
	if err != nil {
		return
	}

	expansion, ok := aliases.Get(args[1])
	if !ok {
		return
	}

	if strings.HasPrefix(expansion, "!") {
		isShell = true
		if findShFunc == nil {
			findShFunc = findSh
		}
		shPath, shErr := findShFunc()
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

func findSh() (string, error) {
	shPath, err := findsh.Find()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			if runtime.GOOS == "windows" {
				return "", errors.New("unable to locate sh to execute the shell alias with. The sh.exe interpreter is typically distributed with Git for Windows.")
			}
			return "", errors.New("unable to locate sh to execute shell alias with")
		}
		return "", err
	}
	return shPath, nil
}
