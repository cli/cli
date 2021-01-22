// Build tasks for the GitHub CLI project.
//
// Usage:  go run script/build.go [<task>]
//
// Known tasks are:
//
//   bin/gh:
//     Builds the main executable.
//     Supported environment variables:
//     - GH_VERSION: determined from source by default
//     - GH_OAUTH_CLIENT_ID
//     - GH_OAUTH_CLIENT_SECRET
//     - SOURCE_DATE_EPOCH: enables reproducible builds
//     - GO_LDFLAGS
//
//   manpages:
//     Builds the man pages under `share/man/man1/`.
//
//   clean:
//     Deletes all built files.
//

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cli/safeexec"
)

var tasks = map[string]func(string) error{
	"bin/gh": func(exe string) error {
		info, err := os.Stat(exe)
		if err == nil && !sourceFilesLaterThan(info.ModTime()) {
			fmt.Printf("%s: `%s` is up to date.\n", self, exe)
			return nil
		}

		ldflags := os.Getenv("GO_LDFLAGS")
		ldflags = fmt.Sprintf("-X github.com/cli/cli/internal/build.Version=%s %s", version(), ldflags)
		ldflags = fmt.Sprintf("-X github.com/cli/cli/internal/build.Date=%s %s", date(), ldflags)
		if oauthSecret := os.Getenv("GH_OAUTH_CLIENT_SECRET"); oauthSecret != "" {
			ldflags = fmt.Sprintf("-X github.com/cli/cli/internal/authflow.oauthClientSecret=%s %s", oauthSecret, ldflags)
			ldflags = fmt.Sprintf("-X github.com/cli/cli/internal/authflow.oauthClientID=%s %s", os.Getenv("GH_OAUTH_CLIENT_ID"), ldflags)
		}

		return run("go", "build", "-trimpath", "-ldflags", ldflags, "-o", exe, "./cmd/gh")
	},
	"manpages": func(_ string) error {
		return run("go", "run", "./cmd/gen-docs", "--man-page", "--doc-path", "./share/man/man1/")
	},
	"clean": func(_ string) error {
		return rmrf("bin", "share")
	},
}

var self string

func main() {
	task := "bin/gh"
	if runtime.GOOS == "windows" {
		task = "bin\\gh.exe"
	}

	if len(os.Args) > 1 {
		task = os.Args[1]
	}

	self = filepath.Base(os.Args[0])
	if self == "build" {
		self = "build.go"
	}

	t := tasks[normalizeTask(task)]
	if t == nil {
		fmt.Fprintf(os.Stderr, "Don't know how to build task `%s`.\n", task)
		os.Exit(1)
	}

	err := t(task)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintf(os.Stderr, "%s: building task `%s` failed.\n", self, task)
		os.Exit(1)
	}
}

func version() string {
	if versionEnv := os.Getenv("GH_VERSION"); versionEnv != "" {
		return versionEnv
	}
	if desc, err := cmdOutput("git", "describe", "--tags"); err == nil {
		return desc
	}
	rev, _ := cmdOutput("git", "rev-parse", "--short", "HEAD")
	return rev
}

func date() string {
	t := time.Now()
	if sourceDate := os.Getenv("SOURCE_DATE_EPOCH"); sourceDate != "" {
		if sec, err := strconv.ParseInt(sourceDate, 10, 64); err == nil {
			t = time.Unix(sec, 0)
		}
	}
	return t.Format("2006-01-02")
}

func sourceFilesLaterThan(t time.Time) bool {
	foundLater := false
	_ = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if foundLater {
			return filepath.SkipDir
		}
		if len(path) > 1 && (path[0] == '.' || path[0] == '_') {
			if info.IsDir() {
				return filepath.SkipDir
			} else {
				return nil
			}
		}
		if info.IsDir() {
			return nil
		}
		if path == "go.mod" || path == "go.sum" || (strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")) {
			if info.ModTime().After(t) {
				foundLater = true
			}
		}
		return nil
	})
	return foundLater
}

func rmrf(targets ...string) error {
	args := append([]string{"rm", "-rf"}, targets...)
	announce(args...)
	for _, target := range targets {
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	}
	return nil
}

func announce(args ...string) {
	fmt.Println(shellInspect(args))
}

func run(args ...string) error {
	exe, err := safeexec.LookPath(args[0])
	if err != nil {
		return err
	}
	announce(args...)
	cmd := exec.Command(exe, args[1:]...)
	return cmd.Run()
}

func cmdOutput(args ...string) (string, error) {
	exe, err := safeexec.LookPath(args[0])
	if err != nil {
		return "", err
	}
	cmd := exec.Command(exe, args[1:]...)
	cmd.Stderr = ioutil.Discard
	out, err := cmd.Output()
	return strings.TrimSuffix(string(out), "\n"), err
}

func shellInspect(args []string) string {
	fmtArgs := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsAny(arg, " \t'\"") {
			fmtArgs[i] = fmt.Sprintf("%q", arg)
		} else {
			fmtArgs[i] = arg
		}
	}
	return strings.Join(fmtArgs, " ")
}

func normalizeTask(t string) string {
	return filepath.ToSlash(strings.TrimSuffix(t, ".exe"))
}
