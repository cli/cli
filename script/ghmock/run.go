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
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	defaultTimeout = time.Minute

	// fakeToken is the token gh gets. It authenticates nothing.
	fakeToken = "ghmock-fake-token"
	// fakeUser is the account gh is logged in as.
	fakeUser = "monalisa"
)

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
	fs.DurationVar(&opts.timeout, "timeout", defaultTimeout, "how long each gh run may take")
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

func cmdRun(args []string, stdout, stderr io.Writer) error {
	opts, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	st, err := openState()
	if err != nil {
		return err
	}
	chk := newChecker(stderr)
	chk.preload()
	rec, _, err := runAll(st, chk, opts)
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, rec.Report)
	return nil
}

// runAll runs each binary in turn and appends its requests to the log. It
// then saves the run and its report for ghmock export. A nil checker turns
// schema checks off.
func runAll(st *state, chk *checker, opts runOptions) (runRecord, []runResult, error) {
	start := time.Now().UTC()
	rec := runRecord{ID: newRunID(start), Time: start.Truncate(time.Second), Command: opts.commandLine()}
	results := make([]runResult, 0, len(opts.binaries))
	for _, binary := range opts.binaries {
		res, err := runBinary(st, chk, binary, opts)
		if err != nil {
			return runRecord{}, nil, fmt.Errorf("%s: %w", binary, err)
		}
		for i := range res.requests {
			res.requests[i].Run = rec.ID
			res.requests[i].Binary = binary
		}
		if err := st.appendLog(res.requests); err != nil {
			return runRecord{}, nil, err
		}
		results = append(results, res)
		rec.Results = append(rec.Results, runOutcome{
			Binary: binary, Path: res.path, Version: res.version, ExitCode: res.exitCode,
			TimedOut: res.timedOut, Stdout: res.stdout, Stderr: res.stderr,
		})
	}

	var notes []string
	if chk != nil {
		notes = chk.unavailable()
		rec.Schemas = chk.schemaFiles()
	}
	var report strings.Builder
	printReport(&report, opts, results, notes)
	rec.Report = report.String()
	if err := st.appendRun(rec); err != nil {
		return runRecord{}, nil, err
	}
	return rec, results, nil
}

// commandLine is the ghmock run command for these options, quoted for a
// POSIX shell.
func (o runOptions) commandLine() string {
	parts := []string{"ghmock", "run"}
	for _, b := range o.binaries {
		parts = append(parts, "--gh", shellQuote(b))
	}
	if o.tty {
		parts = append(parts, "--tty")
	}
	if o.stdin != "" {
		parts = append(parts, "--stdin", shellQuote(o.stdin))
	}
	if o.loggedOut {
		parts = append(parts, "--logged-out")
	}
	if o.timeout != defaultTimeout {
		parts = append(parts, "--timeout", o.timeout.String())
	}
	parts = append(parts, "--")
	for _, a := range o.args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func runBinary(st *state, chk *checker, binary string, opts runOptions) (runResult, error) {
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
	if err := writeGHConfig(filepath.Join(scratch, "config"), !opts.loggedOut); err != nil {
		return runResult{}, err
	}

	srv, err := startServer(st, chk)
	if err != nil {
		return runResult{}, err
	}
	env := ghEnv(scratch, srv.proxyURL(), opts)
	res := runResult{path: path, version: ghVersion(path, env, workDir)}
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
	res.stdout = hideScratch(stdout.String(), scratch)
	res.stderr = hideScratch(stderr.String(), scratch)
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
	var abs string
	var err error
	if !strings.ContainsRune(p, filepath.Separator) && !strings.Contains(p, "/") {
		abs, err = exec.LookPath(p)
	} else {
		abs, err = filepath.Abs(p)
	}
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
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%s is not executable", abs)
	}
	if err := checkBuildsDir(p, abs); err != nil {
		return "", err
	}
	return abs, nil
}

// buildsDir, when set at build time with -ldflags "-X main.buildsDir=DIR",
// limits --gh to binaries inside DIR. The issue-triage workflow sets it: its
// agent's only command is ghmock, and without this, ghmock run --gh /bin/sh
// would run any program.
var buildsDir string

// checkBuildsDir checks that a binary is inside buildsDir, after following
// symlinks, so a link can't point outside it.
func checkBuildsDir(name, abs string) error {
	if buildsDir == "" {
		return nil
	}
	root, err := filepath.EvalSymlinks(buildsDir)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s isn't a gh build in %s, and this ghmock only runs those", name, buildsDir)
	}
	return nil
}

// writeGHConfig writes gh's config the way gh auth login --insecure-storage
// leaves it, logged in as fakeUser with fakeToken. A token in GH_TOKEN would
// also apply to github.com, so gh auth status would check github.com, which
// ghmock blocks. The config version marks the multi-account migration as
// done, as it is for anyone who has logged in with a recent gh.
func writeGHConfig(dir string, loggedIn bool) error {
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("version: \"1\"\n"), 0o600); err != nil {
		return err
	}
	if !loggedIn {
		return nil
	}
	hosts := fmt.Sprintf(`%[1]s:
    users:
        %[2]s:
            oauth_token: %[3]s
    git_protocol: https
    oauth_token: %[3]s
    user: %[2]s
`, mockHost, fakeUser, fakeToken)
	return os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte(hosts), 0o600)
}

// hideScratch replaces the run's scratch folder with <scratch> in gh's
// output. Each binary gets its own folder, so paths such as the hosts.yml
// that gh auth status names would otherwise differ between binaries.
func hideScratch(text, scratch string) string {
	paths := []string{scratch}
	// macOS gives gh /private/var/... for a temporary folder under /var/....
	if resolved, err := filepath.EvalSymlinks(scratch); err == nil && resolved != scratch {
		paths = append(paths, resolved)
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, p := range paths {
		text = strings.ReplaceAll(text, p, "<scratch>")
	}
	return text
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

func newRunID(start time.Time) string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return start.Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}
