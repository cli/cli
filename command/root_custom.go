package command

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/cli/cli/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.Args = cobra.ArbitraryArgs
	RootCmd.RunE = customCommandFallback
	RootCmd.ValidArgsFunction = customCommandCompletion
}

func customCommandFallback(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		cmd.HelpFunc()(cmd, args)
		return nil
	}

	cmdExe, err := findCustomCommand(args[0])
	if err != nil {
		return err
	}

	ctx := contextForCommand(cmd)
	authToken, err := ctx.AuthToken()
	if err != nil {
		return err
	}

	env := append(os.Environ(), "GITHUB_TOKEN="+authToken)

	if baseRepo, err := ctx.BaseRepo(); err == nil {
		env = append(env, fmt.Sprintf("GH_BASEREPO=%s/%s", baseRepo.RepoOwner(), baseRepo.RepoName()))
	}

	externalCmd := exec.Command(cmdExe, args[1:]...)
	externalCmd.Env = env
	externalCmd.Stdin = os.Stdin
	externalCmd.Stdout = os.Stdout
	externalCmd.Stderr = os.Stderr
	return externalCmd.Run()
}

func customCommandCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	directive := cobra.ShellCompDirectiveDefault

	if len(args) == 0 {
		return findCustomCommands(toComplete), directive
	}

	var results []string
	cmdExe, err := findCustomCommand(args[0])
	if err != nil {
		return results, directive
	}

	cmdArgs := append([]string{"__complete"}, args[1:]...)
	cmdArgs = append(cmdArgs, toComplete)
	buf := bytes.Buffer{}

	// TODO: scan the file to guess whether it supports completions
	externalCmd := exec.Command(cmdExe, cmdArgs...)
	externalCmd.Stdout = &buf
	if err := externalCmd.Run(); err != nil {
		return results, directive
	}

	for _, line := range strings.Split(buf.String(), "\n") {
		if len(strings.TrimSpace(line)) > 0 {
			results = append(results, line)
		}
	}
	return results, directive
}

func findCustomCommand(name string) (string, error) {
	if strings.HasPrefix(name, "@") {
		idx := strings.IndexRune(name, '/')
		cmdName := "gh-" + stripPathComponent(name[idx+1:])
		return path.Join(config.ConfigDir(), "gh-commands", name[1:idx], cmdName), nil
	}

	cmdName := "gh-" + stripPathComponent(name)

	found, _ := filepath.Glob(path.Join(config.ConfigDir(), "gh-commands", "*", cmdName))
	if len(found) == 1 {
		return found[0], nil
	} else if len(found) > 1 {
		namespaces := make([]string, len(found))
		for i, f := range found {
			namespaces[i] = filepath.Base(filepath.Dir(f))
		}
		return "", fmt.Errorf("duplicate command %q was found in the following namespaces: %v", name, namespaces)
	}

	if found, err := exec.LookPath(cmdName); err == nil {
		return found, nil
	}

	return "", fmt.Errorf("unknown command %q", name)
}

func findCustomCommands(prefix string) []string {
	var results []string
	globName := "gh-" + stripPathComponent(prefix) + "*"

	if found, err := filepath.Glob(filepath.Join(config.ConfigDir(), "gh-commands", "*", globName)); err == nil {
		for _, f := range found {
			base := strings.TrimPrefix(filepath.Base(f), "gh-")
			results = append(results, base)
		}
	}

	paths := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(paths) {
		found, err := filepath.Glob(filepath.Join(dir, globName))
		if err != nil {
			continue
		}
		for _, f := range found {
			base := strings.TrimPrefix(filepath.Base(f), "gh-")
			results = append(results, base)
		}
	}

	return results
}

func stripPathComponent(arg string) string {
	if idx := strings.IndexRune(arg, filepath.Separator); idx >= 0 {
		return arg[0:idx]
	}
	return arg
}
