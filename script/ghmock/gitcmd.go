package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type repoOptions struct {
	name   string
	branch string
}

func parseRepoArgs(args []string) (repoOptions, error) {
	var opts repoOptions
	fs := flag.NewFlagSet("repo", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.branch, "branch", "main", "initial branch")
	name, err := parseWithName(fs, args)
	if err != nil {
		return repoOptions{}, err
	}
	if _, _, err := parseGitRepo(name); err != nil {
		return repoOptions{}, usageError{err}
	}
	if opts.branch == "" {
		return repoOptions{}, newUsageError("--branch cannot be empty")
	}
	opts.name = name
	return opts, nil
}

func cmdRepo(args []string, stdout io.Writer) error {
	opts, err := parseRepoArgs(args)
	if err != nil {
		return err
	}
	st, err := openState()
	if err != nil {
		return err
	}
	world, err := st.ensureGitFixture()
	if err != nil {
		return err
	}
	host, repo, err := parseGitRepo(opts.name)
	if err != nil {
		return err
	}
	dir := gitRepoPath(world, host, repo)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("repository %s already exists", opts.name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	scratch, err := newScratch(nil)
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	env, err := gitEnvironment(ghEnv(scratch, "", runOptions{localhost: true}), scratch, world)
	if err != nil {
		return err
	}
	if _, err := localGit(world, env, "check-ref-format", "refs/heads/"+opts.branch); err != nil {
		return fmt.Errorf("invalid branch %q: %w", opts.branch, err)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "repo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if _, err := localGit(world, env, "init", "--bare", "--quiet", "--initial-branch="+opts.branch, tmp); err != nil {
		return err
	}
	if _, err := localGit(tmp, env, "config", "http.receivepack", "true"); err != nil {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Created %s/%s (branch %s)\nHTTPS: https://%s/%s.git\nSSH: git@%s:%s.git\n",
		host, repo, opts.branch, host, repo, host, repo)
	return nil
}

type writeOptions struct {
	path string
	text string
}

func parseWriteArgs(args []string) (writeOptions, error) {
	var opts writeOptions
	fs := flag.NewFlagSet("write", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.text, "text", "", "file contents, including an empty string")
	path, err := parseWithName(fs, args)
	if err != nil {
		return writeOptions{}, err
	}
	if !filepath.IsLocal(path) || filepath.Clean(path) == "." {
		return writeOptions{}, newUsageError("pass a file path relative to the Git workspace")
	}
	hasText := false
	fs.Visit(func(f *flag.Flag) { hasText = hasText || f.Name == "text" })
	if !hasText {
		return writeOptions{}, newUsageError("pass --text with the file contents")
	}
	opts.path = path
	return opts, nil
}

func cmdWrite(args []string, stdout io.Writer) error {
	opts, err := parseWriteArgs(args)
	if err != nil {
		return err
	}
	st, err := openState()
	if err != nil {
		return err
	}
	world, err := st.ensureGitFixture()
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Join(world, "work"))
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(opts.path), 0o755); err != nil {
		return err
	}
	if err := root.WriteFile(opts.path, []byte(opts.text), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Wrote %s (%d bytes)\n", opts.path, len(opts.text))
	return nil
}

type gitOptions struct {
	in      string
	run     string
	before  bool
	build   int
	stdin   string
	timeout time.Duration
	args    []string
}

func parseGitArgs(args []string) (gitOptions, error) {
	var opts gitOptions
	fs := flag.NewFlagSet("git", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.in, "in", "", "directory inside the Git workspace")
	fs.StringVar(&opts.run, "run", "", "inspect a retained run, by ID or latest")
	fs.BoolVar(&opts.before, "before", false, "inspect that run's starting state instead")
	fs.IntVar(&opts.build, "build", 1, "one-based build number in the retained run")
	fs.StringVar(&opts.stdin, "stdin", "", "standard input for git")
	fs.DurationVar(&opts.timeout, "timeout", defaultTimeout, "how long git may take")
	if err := fs.Parse(args); err != nil {
		return gitOptions{}, usageError{err}
	}
	var hasBuild, hasRun bool
	fs.Visit(func(f *flag.Flag) {
		hasBuild = hasBuild || f.Name == "build"
		hasRun = hasRun || f.Name == "run"
	})
	opts.args = fs.Args()
	switch {
	case len(opts.args) == 0:
		return gitOptions{}, newUsageError("missing git arguments; put them after --")
	case opts.in != "" && !filepath.IsLocal(opts.in):
		return gitOptions{}, newUsageError("--in must be a relative path inside the Git workspace")
	case opts.build < 1:
		return gitOptions{}, newUsageError("--build must be positive")
	case hasRun && opts.run == "":
		return gitOptions{}, newUsageError("--run cannot be empty")
	case opts.run == "" && hasBuild:
		return gitOptions{}, newUsageError("--build requires --run")
	case opts.run == "" && opts.before:
		return gitOptions{}, newUsageError("--before requires --run")
	case opts.before && hasBuild:
		return gitOptions{}, newUsageError("--before and --build cannot be used together")
	case opts.timeout <= 0:
		return gitOptions{}, newUsageError("--timeout must be positive")
	}
	return opts, nil
}

func (s *state) gitWorld(opts gitOptions) (string, error) {
	if opts.run == "" {
		return s.ensureGitFixture()
	}
	runs, err := s.readRuns()
	if err != nil {
		return "", err
	}
	for _, rec := range slices.Backward(runs) {

		if opts.run != "latest" && rec.ID != opts.run {
			continue
		}
		if opts.before {
			if rec.GitBaseline == "" || !filepath.IsLocal(rec.GitBaseline) {
				return "", fmt.Errorf("run %s has no Git baseline", rec.ID)
			}
			return filepath.Join(s.dir, filepath.FromSlash(rec.GitBaseline)), nil
		}
		if opts.build > len(rec.Results) {
			return "", fmt.Errorf("run %s has only %d builds", rec.ID, len(rec.Results))
		}
		outcome := rec.Results[opts.build-1].Git
		if outcome == nil {
			return "", fmt.Errorf("run %s build %d has no retained Git workspace", rec.ID, opts.build)
		}
		if !filepath.IsLocal(outcome.Directory) {
			return "", fmt.Errorf("invalid Git workspace in run %s", rec.ID)
		}
		return filepath.Join(s.dir, filepath.FromSlash(outcome.Directory)), nil
	}
	return "", fmt.Errorf("no saved run %q", opts.run)
}

func cmdGit(args []string, stdout, stderr io.Writer) error {
	opts, err := parseGitArgs(args)
	if err != nil {
		return err
	}
	st, err := openState()
	if err != nil {
		return err
	}
	world, err := st.gitWorld(opts)
	if err != nil {
		return err
	}
	dir, err := gitWorkDir(world, opts.in)
	if err != nil {
		return err
	}
	ca, err := newCertAuthority()
	if err != nil {
		return err
	}
	scratch, err := newScratch(ca)
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	backend := &gitBackend{world: world, scratch: scratch}
	srv, err := startServer(st, nil, ca, backend)
	if err != nil {
		return err
	}
	defer srv.close()
	env, err := gitEnvironment(ghEnv(scratch, srv.proxyURL(), runOptions{}), scratch, world)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()
	env = gitDeadlineEnvironment(ctx, env)
	cmd := exec.CommandContext(ctx, "git", opts.args...)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdin = strings.NewReader(opts.stdin)
	var out, diagnostic bytes.Buffer
	cmd.Stdout = io.MultiWriter(stdout, &out)
	cmd.Stderr = io.MultiWriter(stderr, &diagnostic)
	containCommand(cmd, false)
	runErr := cmd.Run()
	requests, err := gitRequests(scratch, srv.close())
	if err != nil {
		return err
	}
	record := gitCommandRecord{
		In: opts.in, Args: opts.args, Stdin: opts.stdin, ExitCode: processExitCode(runErr),
		Stdout: hideDirectory(out.String(), world, "<git>"),
		Stderr: hideDirectory(diagnostic.String(), world, "<git>"), Requests: requests,
	}
	if err := appendJSONL(filepath.Join(world, "commands.jsonl"), []gitCommandRecord{record}); err != nil {
		return err
	}
	if runErr != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("git stopped after %s: %w", opts.timeout, ctx.Err())
		}
		return fmt.Errorf("git: %w", runErr)
	}
	return nil
}

type gitCommandRecord struct {
	In       string          `json:"in,omitempty"`
	Args     []string        `json:"args"`
	Stdin    string          `json:"stdin,omitempty"`
	ExitCode int             `json:"exit_code"`
	Stdout   string          `json:"stdout"`
	Stderr   string          `json:"stderr"`
	Requests []loggedRequest `json:"requests,omitempty"`
}

func gitEnvironment(env []string, scratch, world string) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	world, err = filepath.Abs(world)
	if err != nil {
		return nil, err
	}
	// Relocated Git distributions use this to find their transport helpers.
	// It selects the installed tooling, not the caller's Git configuration.
	if execPath := os.Getenv("GIT_EXEC_PATH"); execPath != "" {
		env = append(env, "GIT_EXEC_PATH="+execPath)
	}
	return append(env,
		"LC_ALL=C",
		"GIT_AUTHOR_NAME=monalisa", "GIT_AUTHOR_EMAIL=monalisa@example.invalid",
		"GIT_COMMITTER_NAME=monalisa", "GIT_COMMITTER_EMAIL=monalisa@example.invalid",
		"GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z",
		"GIT_TEMPLATE_DIR="+filepath.Join(scratch, "templates"),
		"GIT_SSH_COMMAND="+shellQuote(exe)+" _git-ssh", "GIT_SSH_VARIANT=ssh",
		"GHMOCK_GIT_WORLD="+world, "GHMOCK_GIT_LOG="+filepath.Join(scratch, "git-ssh.jsonl"),
	), nil
}

func gitDeadlineEnvironment(ctx context.Context, env []string) []string {
	if deadline, ok := ctx.Deadline(); ok {
		return append(env, "GHMOCK_GIT_DEADLINE="+deadline.UTC().Format(time.RFC3339Nano))
	}
	return env
}

func localGit(dir string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir, cmd.Env = dir, env
	containCommand(cmd, false)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func gitBuildDirectory(runID string, index int) string {
	return filepath.ToSlash(filepath.Join("git", "runs", runID, strconv.Itoa(index+1)))
}
