package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type gitBackend struct {
	world   string
	scratch string
}

type gitRequest struct {
	Service  string `json:"service"`
	ExitCode int    `json:"exit_code"`
	Error    string `json:"error,omitempty"`
}

// serveGitHTTP delegates smart HTTP to Git before the JSON fixture handler
// reads the body. Pack streams must not be truncated or schema-checked.
func (m *mockServer) serveGitHTTP(w http.ResponseWriter, r *http.Request, entry loggedRequest) bool {
	if m.git == nil || isAPIHost(entry.Host) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		return false
	}
	endpoint := strings.Join(parts[2:], "/")
	if endpoint != "info/refs" && endpoint != "git-upload-pack" && endpoint != "git-receive-pack" {
		return false
	}
	entry.Name = r.Method + " " + entry.Host + r.URL.Path
	service := endpoint
	if endpoint == "info/refs" {
		service = r.URL.Query().Get("service")
	}
	entry.Git = &gitRequest{Service: service}
	defer func() { m.record(entry) }()
	fail := func(status int, err error) {
		entry.Status = status
		entry.Git.Error = err.Error()
		http.Error(w, "ghmock: "+err.Error(), status)
	}
	host, repo, err := parseGitRepo(entry.Host + "/" + strings.Join(parts[:2], "/"))
	if err != nil {
		fail(http.StatusBadRequest, err)
		return true
	}
	if service != "git-upload-pack" && service != "git-receive-pack" {
		fail(http.StatusBadRequest, fmt.Errorf("unsupported Git service %q", service))
		return true
	}
	if (endpoint == "info/refs" && r.Method != http.MethodGet) || (endpoint != "info/refs" && r.Method != http.MethodPost) {
		fail(http.StatusMethodNotAllowed, fmt.Errorf("method %s is not supported for %s", r.Method, endpoint))
		return true
	}
	repoDir := gitRepoPath(m.git.world, host, repo)
	if _, err := os.Stat(filepath.Join(repoDir, "HEAD")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(http.StatusNotFound, fmt.Errorf("no Git repository %s/%s; create it with ghmock repo", host, repo))
		} else {
			fail(http.StatusInternalServerError, err)
		}
		return true
	}
	env, err := gitEnvironment(ghEnv(m.git.scratch, m.proxyURL(), runOptions{}), m.git.scratch, m.git.world)
	if err != nil {
		fail(http.StatusInternalServerError, err)
		return true
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "http-backend")
	cmd.Dir = m.git.world
	cmd.Env = append(env,
		"GIT_PROJECT_ROOT="+filepath.Dir(filepath.Dir(repoDir)),
		"GIT_HTTP_EXPORT_ALL=1",
		"PATH_INFO=/"+repo+".git/"+endpoint,
		"REQUEST_METHOD="+r.Method,
		"QUERY_STRING="+r.URL.RawQuery,
		"CONTENT_TYPE="+r.Header.Get("Content-Type"),
		"HTTP_CONTENT_ENCODING="+r.Header.Get("Content-Encoding"),
		"GIT_PROTOCOL="+r.Header.Get("Git-Protocol"),
		"REMOTE_ADDR=127.0.0.1",
	)
	if r.ContentLength >= 0 {
		cmd.Env = append(cmd.Env, "CONTENT_LENGTH="+strconv.FormatInt(r.ContentLength, 10))
	}
	cmd.Stdin = r.Body
	containCommand(cmd, false)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.StdoutPipe()
	if err != nil {
		fail(http.StatusInternalServerError, err)
		return true
	}
	if err := cmd.Start(); err != nil {
		fail(http.StatusInternalServerError, err)
		return true
	}
	reader := bufio.NewReader(output)
	headers, err := textproto.NewReader(reader).ReadMIMEHeader()
	if err != nil {
		cancel()
		waitErr := cmd.Wait()
		fail(http.StatusInternalServerError, fmt.Errorf("git http-backend headers: %w", errors.Join(err, waitErr)))
		return true
	}
	status := http.StatusOK
	if value := headers.Get("Status"); value != "" {
		code, _, _ := strings.Cut(value, " ")
		status, err = strconv.Atoi(code)
		if err != nil || status < 100 || status > 599 {
			cancel()
			_ = cmd.Wait()
			fail(http.StatusInternalServerError, fmt.Errorf("invalid git http-backend status %q", value))
			return true
		}
	}
	for name, values := range headers {
		if name != "Status" {
			w.Header()[name] = values
		}
	}
	entry.Matched, entry.Status = true, status
	w.WriteHeader(status)
	_, copyErr := io.Copy(w, reader)
	if copyErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	entry.Git.ExitCode = processExitCode(waitErr)
	if err := errors.Join(copyErr, waitErr); err != nil {
		entry.Git.Error = strings.TrimSpace(stderr.String())
		if entry.Git.Error == "" {
			entry.Git.Error = err.Error()
		}
	}
	return true
}

// cmdGitSSH replaces the SSH transport, not the Git protocol. Git's requested
// upload/receive-pack runs against the corresponding local bare repository.
// The command string is parsed as data; it is never passed to a shell.
func cmdGitSSH(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-p", "-o", "-i", "-F":
			if len(args) < 2 {
				return fmt.Errorf("missing value for SSH option %s", args[0])
			}
			args = args[2:]
		case "-4", "-6", "-T":
			args = args[1:]
		default:
			return fmt.Errorf("unsupported SSH option %q in ghmock transport", args[0])
		}
	}
	if len(args) != 2 {
		return fmt.Errorf("ghmock SSH transport expects a host and a Git service")
	}
	host := args[0]
	if _, after, ok := strings.Cut(host, "@"); ok {
		host = after
	}
	service, quoted, ok := strings.Cut(args[1], " ")
	if !ok || (service != "git-upload-pack" && service != "git-receive-pack") ||
		len(quoted) < 2 || quoted[0] != '\'' || quoted[len(quoted)-1] != '\'' {
		return fmt.Errorf("unsupported SSH command %q in ghmock transport", args[1])
	}
	path := strings.TrimPrefix(quoted[1:len(quoted)-1], "/")
	host, repo, err := parseGitRepo(host + "/" + path)
	if err != nil {
		return err
	}
	world, logPath := os.Getenv("GHMOCK_GIT_WORLD"), os.Getenv("GHMOCK_GIT_LOG")
	if world == "" || logPath == "" {
		return fmt.Errorf("ghmock SSH transport requires a Git run environment")
	}
	entry := loggedRequest{
		Time: time.Now().UTC(), Method: "SSH", Host: host, Path: "/" + repo + ".git",
		Name: "SSH " + host + "/" + repo + ".git", Git: &gitRequest{Service: service},
	}
	dir := gitRepoPath(world, host, repo)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", strings.TrimPrefix(service, "git-"), dir)
	cmd.Env = os.Environ()
	cmd.Stdin, cmd.Stdout = stdin, stdout
	var diagnostic bytes.Buffer
	cmd.Stderr = io.MultiWriter(stderr, &diagnostic)
	cmd.WaitDelay = 5 * time.Second
	err = cmd.Run()
	entry.Matched = err == nil
	entry.Git.ExitCode = processExitCode(err)
	entry.Git.Error = strings.TrimSpace(diagnostic.String())
	if logErr := appendJSONL(logPath, []loggedRequest{entry}); logErr != nil {
		return errors.Join(err, fmt.Errorf("recording Git SSH request: %w", logErr))
	}
	return err
}

func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func gitRequests(scratch string, requests []loggedRequest) ([]loggedRequest, error) {
	ssh, err := readJSONL[loggedRequest](filepath.Join(scratch, "git-ssh.jsonl"))
	if err != nil {
		return nil, err
	}
	requests = append(requests, ssh...)
	slices.SortStableFunc(requests, func(a, b loggedRequest) int { return a.Time.Compare(b.Time) })
	for i := range requests {
		requests[i].Seq = i + 1
	}
	return requests, nil
}
