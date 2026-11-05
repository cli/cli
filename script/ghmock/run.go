package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// fakeToken is the token gh gets. It authenticates nothing.
const fakeToken = "ghmock-fake-token"

// runOptions are the parsed flags of ghmock run.
type runOptions struct {
	binaries  []string
	tty       bool
	stdin     string
	loggedOut bool
	timeout   time.Duration
	args      []string
}

// runResult is what one gh binary did during a run.
type runResult struct {
	label    string
	path     string
	version  string
	exitCode int
	timedOut bool
	stdout   string
	stderr   string
	requests []loggedRequest
}

// stringList collects a repeatable string flag.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func parseRunArgs(args []string) (runOptions, error) {
	opts := runOptions{}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Var((*stringList)(&opts.binaries), "gh", "gh binary to run; repeat to compare binaries")
	fs.BoolVar(&opts.tty, "tty", false, "make gh behave as if writing to a 120-column terminal")
	fs.StringVar(&opts.stdin, "stdin", "", "text to pass to gh on standard input")
	fs.BoolVar(&opts.loggedOut, "logged-out", false, "run gh without a token")
	fs.DurationVar(&opts.timeout, "timeout", time.Minute, "how long each gh run may take")
	if err := fs.Parse(args); err != nil {
		return runOptions{}, usageError{err}
	}
	opts.args = fs.Args()

	switch {
	case len(opts.binaries) == 0:
		return runOptions{}, newUsageError("at least one --gh is required")
	case len(opts.args) == 0:
		return runOptions{}, newUsageError("missing gh arguments; put them after --")
	case opts.timeout <= 0:
		return runOptions{}, newUsageError("--timeout must be positive")
	}
	return opts, nil
}

func cmdRun(args []string, stdout io.Writer) error {
	opts, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	st, err := openState()
	if err != nil {
		return err
	}
	results, err := runAll(st, opts)
	if err != nil {
		return err
	}
	printReport(stdout, opts, results)
	return nil
}

// runAll runs each binary in turn and appends its requests to the log.
func runAll(st *state, opts runOptions) ([]runResult, error) {
	runID := newRunID()
	results := make([]runResult, 0, len(opts.binaries))
	for _, binary := range opts.binaries {
		res, err := runBinary(st, binary, opts)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", binary, err)
		}
		for i := range res.requests {
			res.requests[i].Run = runID
			res.requests[i].Binary = binary
		}
		if err := st.appendLog(res.requests); err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

func runBinary(st *state, binary string, opts runOptions) (runResult, error) {
	path, err := resolveBinary(binary)
	if err != nil {
		return runResult{}, err
	}

	scratch, err := os.MkdirTemp("", "ghmock-run-")
	if err != nil {
		return runResult{}, err
	}
	defer os.RemoveAll(scratch)
	workDir := filepath.Join(scratch, "work")
	for _, d := range []string{"home", "tmp", "config", "work"} {
		if err := os.MkdirAll(filepath.Join(scratch, d), 0o755); err != nil {
			return runResult{}, err
		}
	}

	srv, err := startServer(st)
	if err != nil {
		return runResult{}, err
	}
	env := ghEnv(scratch, srv.proxyURL(), opts)
	res := runResult{label: binary, path: path, version: ghVersion(path, env, workDir)}
	srv.discard()

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, opts.args...)
	cmd.Env = env
	cmd.Dir = workDir
	cmd.Stdin = strings.NewReader(opts.stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 5 * time.Second
	runErr := cmd.Run()

	res.requests = srv.close()
	res.stdout = stdout.String()
	res.stderr = stderr.String()
	var exitErr *exec.ExitError
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.timedOut = true
		res.exitCode = -1
	case errors.As(runErr, &exitErr):
		res.exitCode = exitErr.ExitCode()
	case runErr != nil:
		return runResult{}, runErr
	}
	return res, nil
}

// resolveBinary turns a --gh value into an absolute path. A bare name is
// looked up on PATH.
func resolveBinary(p string) (string, error) {
	if !strings.ContainsRune(p, filepath.Separator) && !strings.Contains(p, "/") {
		return exec.LookPath(p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", abs)
	}
	if info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%s is not executable", abs)
	}
	return abs, nil
}

// ghEnv builds gh's whole environment from scratch, so the caller's tokens,
// config and git settings never reach it. Every run gets fresh directories,
// so an alias or extension added in one run is gone in the next.
func ghEnv(scratch, proxyURL string, opts runOptions) []string {
	dir := func(name string) string { return filepath.Join(scratch, name) }
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + dir("home"),
		"TMPDIR=" + dir("tmp"),
		"GH_CONFIG_DIR=" + dir("config"),
		"XDG_CONFIG_HOME=" + dir("home/.config"),
		"XDG_DATA_HOME=" + dir("home/.local/share"),
		"XDG_STATE_HOME=" + dir("home/.local/state"),
		"XDG_CACHE_HOME=" + dir("home/.cache"),
		"GH_HOST=" + mockHost,
		"HTTP_PROXY=" + proxyURL,
		"HTTPS_PROXY=" + proxyURL,
		"http_proxy=" + proxyURL,
		"https_proxy=" + proxyURL,
		"NO_PROXY=",
		"no_proxy=",
		"GH_NO_UPDATE_NOTIFIER=1",
		"GH_NO_EXTENSION_UPDATE_NOTIFIER=1",
		"GH_TELEMETRY=disabled",
		"GH_PROMPT_DISABLED=1",
		"GH_SPINNER_DISABLED=1",
		"GH_PAGER=cat",
		"PAGER=cat",
		"GH_BROWSER=echo",
		"BROWSER=echo",
		"GH_EDITOR=true",
		"EDITOR=true",
		"VISUAL=true",
		"NO_COLOR=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + dir("home/.gitconfig"),
	}
	if !opts.loggedOut {
		env = append(env, "GH_TOKEN="+fakeToken)
	}
	if opts.tty {
		env = append(env, "GH_FORCE_TTY=120")
	}
	return env
}

func ghVersion(path string, env []string, dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = env
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "unknown version"
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(line)
}

func newRunID() string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}
