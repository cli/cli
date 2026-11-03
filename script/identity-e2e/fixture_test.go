package identitye2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/require"
)

var (
	offlineBinary = flag.String("gh", "", "Absolute path to the built prerelease for offline journeys")
	oldBinary     = flag.String("old-gh", "", "Absolute path to stable gh for the reset/downgrade journey")
	keepFixtures  = flag.Bool("keep", false, "Keep credential-free fixtures for manual exploration")
)

const (
	fixtureHost  = "identities-fixture.invalid"
	fixtureAlice = "identity-fixture-alice"
	fixtureBob   = "identity-fixture-bob"
	aliceToken   = "fixture-token-alice-not-a-secret"
	bobToken     = "fixture-token-bob-not-a-secret"
)

type offlineFixture struct {
	t                         *testing.T
	binary, root, home, tools string
	log, helper               string
	env                       map[string]string
}

type transportEvent struct {
	Transport   string   `json:"transport"`
	Args        []string `json:"args,omitempty"`
	Cwd         string   `json:"cwd,omitempty"`
	GitDir      string   `json:"gitDir,omitempty"`
	GitWorkTree string   `json:"gitWorkTree,omitempty"`
	Config      string   `json:"config,omitempty"`
	URL         string   `json:"url,omitempty"`
	Username    string   `json:"username,omitempty"`
	Token       string   `json:"token,omitempty"`
}

type selectedIdentity struct {
	Name    string `json:"name"`
	Account string `json:"account"`
}

func requireOfflineBinary(t *testing.T) string {
	t.Helper()
	if *offlineBinary == "" {
		t.Skip("offline journeys require -gh /absolute/path/to/built/gh")
	}
	if runtime.GOOS == "windows" {
		t.Skip("persistent identities are not Windows-qualified")
	}
	return offlineBinaryPath(t, *offlineBinary)
}

func offlineBinaryPath(t *testing.T, path string) string {
	t.Helper()
	require.True(t, filepath.IsAbs(path), "binary path must be absolute")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular(), "binary must be a regular file")
	return path
}

func newOfflineFixture(t *testing.T, binary string) *offlineFixture {
	t.Helper()
	root, err := os.MkdirTemp("", "gh-identity-e2e-")
	require.NoError(t, err)
	helper, err := os.Executable()
	require.NoError(t, err)
	f := &offlineFixture{
		t: t, binary: binary, root: root, home: filepath.Join(root, "home"),
		tools: filepath.Join(root, "tools"), log: filepath.Join(root, "transport.jsonl"),
		helper: helper, env: map[string]string{},
	}
	for _, dir := range []string{f.home, f.tools, filepath.Join(root, "templates"), filepath.Join(f.home, "gh")} {
		require.NoError(t, os.MkdirAll(dir, 0o700))
	}
	if *keepFixtures {
		f.helper = filepath.Join(f.tools, "transport-helper")
		offlineCopy(t, helper, f.helper)
		t.Cleanup(func() {
			f.exportEnvironment()
			t.Logf("Fixture retained: %s; explore in a disposable subshell with ( . %s; exec \"$SHELL\" )",
				root, shellquote.Join(filepath.Join(root, "enter.sh")))
		})
	} else {
		t.Cleanup(func() { require.NoError(t, os.RemoveAll(root)) })
	}
	// Whitelist only non-credential environment; never inherit operator Git/auth settings.
	for _, key := range []string{"PATH", "TMPDIR", "LANG", "LC_ALL", "LC_CTYPE", "TERM", "SHELL", "SystemRoot"} {
		if value, ok := os.LookupEnv(key); ok {
			f.env[key] = value
		}
	}
	maps.Copy(f.env, map[string]string{
		"HOME": f.home, "XDG_CONFIG_HOME": filepath.Join(f.home, ".config"),
		"GH_CONFIG_DIR": filepath.Join(f.home, "gh"), "GH_HOST": fixtureHost,
		"GH_PROMPT_DISABLED": "1", "GH_NO_UPDATE_NOTIFIER": "1", "GH_TELEMETRY": "0",
		"GIT_CONFIG_GLOBAL": filepath.Join(f.home, ".gitconfig"), "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_TERMINAL_PROMPT": "0", "GIT_EXEC_PATH": f.tools,
		"GIT_CEILING_DIRECTORIES": root,
		"GIT_TEMPLATE_DIR":        filepath.Join(root, "templates"),
		"GIT_AUTHOR_DATE":         "2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE": "2026-01-01T00:00:00Z",
		"IDENTITY_FIXTURE_LOG": f.log, "PATH": f.tools + string(os.PathListSeparator) + os.Getenv("PATH"),
	})
	f.write(f.log, "")
	f.write(filepath.Join(f.home, "gh", "config.yml"), "telemetry: disabled\n")
	f.write(filepath.Join(f.home, ".gitconfig"), "[user]\n\tname = Ordinary User\n\temail = ordinary@example.invalid\n[init]\n\tdefaultBranch = main\n")
	f.write(filepath.Join(f.home, "gh", "hosts.yml"), fmt.Sprintf(
		"%s:\n    user: %s\n    oauth_token: %s\n    git_protocol: https\n    users:\n        %s:\n            oauth_token: %s\n        %s:\n            oauth_token: %s\n",
		fixtureHost, fixtureBob, bobToken, fixtureAlice, aliceToken, fixtureBob, bobToken))
	f.transport("ssh", "ssh")
	f.transport("git-remote-https", "https")
	f.executable("security", "exit 44\n")
	f.executable("secret-tool", "exit 1\n")
	f.executable("gh", "exec "+shellquote.Join(binary)+" \"$@\"\n")
	f.git("", "config", "--global", "credential.interactive", "false")
	return f
}

func (f *offlineFixture) write(path, contents string) {
	f.t.Helper()
	require.NoError(f.t, os.WriteFile(path, []byte(contents), 0o600))
}

func (f *offlineFixture) executable(name, source string) {
	f.t.Helper()
	path := filepath.Join(f.tools, name)
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(f.t, os.WriteFile(path, []byte("#!/bin/sh\n"+source), 0o700))
}

func (f *offlineFixture) transport(name, mode string) {
	f.t.Helper()
	f.executable(name, "export IDENTITY_OFFLINE_HELPER="+shellquote.Join(mode)+"\nexec "+
		shellquote.Join(f.helper, "-test.run=^TestOfflineTransportHelper$", "--")+" \"$@\"\n")
}

func (f *offlineFixture) run(directory, input, failure string, args ...string) string {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = directory
	if directory == "" {
		cmd.Dir = f.root
	}
	for key, value := range f.env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	f.t.Logf("$ %s\n%s%s", shellquote.Join(args...), stdout.String(), stderr.String())
	if failure != "" {
		require.Error(f.t, err, "expected command to fail")
		require.Contains(f.t, stdout.String()+stderr.String(), failure)
	} else {
		require.NoError(f.t, err, "%s", stderr.String())
	}
	return stdout.String()
}

func (f *offlineFixture) gh(directory string, args ...string) string {
	f.t.Helper()
	return f.run(directory, "", "", append([]string{f.binary}, args...)...)
}

func (f *offlineFixture) ghFails(directory, failure string, args ...string) string {
	f.t.Helper()
	return f.run(directory, "", failure, append([]string{f.binary}, args...)...)
}

func (f *offlineFixture) git(directory string, args ...string) string {
	f.t.Helper()
	return f.run(directory, "", "", append([]string{"git"}, args...)...)
}

func (f *offlineFixture) gitFails(directory, failure string, args ...string) string {
	f.t.Helper()
	return f.run(directory, "", failure, append([]string{"git"}, args...)...)
}

func (f *offlineFixture) using(env map[string]string) *offlineFixture {
	other := *f
	other.env = maps.Clone(f.env)
	maps.Copy(other.env, env)
	return &other
}

func (f *offlineFixture) mkdir(name string) string {
	f.t.Helper()
	path := filepath.Join(f.root, name)
	require.NoError(f.t, os.MkdirAll(path, 0o700))
	return path
}

func (f *offlineFixture) repo(name, url string) string {
	f.t.Helper()
	path := f.mkdir(name)
	f.git("", "init", "-q", path)
	if url != "" {
		f.git(path, "remote", "add", "origin", url)
	}
	return path
}

func (f *offlineFixture) create(name, account, ssh, email string) {
	f.t.Helper()
	args := []string{"auth", "identity", "create", name, "--hostname", fixtureHost, "--account", account}
	if ssh != "" {
		args = append(args, "--ssh-host", ssh)
	}
	if email != "" {
		args = append(args, "--git-user-name", "Alice Work", "--git-user-email", email)
	}
	f.gh("", args...)
}

func (f *offlineFixture) selected(directory string) *selectedIdentity {
	f.t.Helper()
	var id *selectedIdentity
	require.NoError(f.t, json.Unmarshal([]byte(f.gh(directory, "auth", "use", "--view", "--hostname", fixtureHost)), &id))
	return id
}

func (f *offlineFixture) selectedName(directory string) string {
	f.t.Helper()
	id := f.selected(directory)
	require.NotNil(f.t, id, "expected an identity in %s", directory)
	return id.Name
}

func (f *offlineFixture) gitValue(directory, key string) string {
	f.t.Helper()
	return strings.TrimSpace(f.git(directory, "config", "--get", key))
}

func (f *offlineFixture) credential(directory, owner, username, failure string) string {
	f.t.Helper()
	request := fmt.Sprintf("protocol=https\nhost=%s\npath=%s/project.git\n", fixtureHost, owner)
	if username != "" {
		request += "username=" + username + "\n"
	}
	return f.run(directory, request+"\n", failure, "git", "credential", "fill")
}

func (f *offlineFixture) events() []transportEvent {
	f.t.Helper()
	var events []transportEvent
	scanner := bufio.NewScanner(bytes.NewReader(offlineRead(f.t, f.log)))
	for scanner.Scan() {
		var event transportEvent
		require.NoError(f.t, json.Unmarshal(scanner.Bytes(), &event))
		events = append(events, event)
	}
	require.NoError(f.t, scanner.Err())
	return events
}

func (f *offlineFixture) lastEvent() transportEvent {
	f.t.Helper()
	events := f.events()
	require.NotEmpty(f.t, events)
	return events[len(events)-1]
}

func (f *offlineFixture) exportEnvironment() {
	var text strings.Builder
	text.WriteString("# Source only inside a disposable subshell. All credentials are fake.\nunset SSH_AUTH_SOCK DBUS_SESSION_BUS_ADDRESS\n")
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GH_") || strings.HasPrefix(key, "GITHUB_") || strings.HasPrefix(key, "GIT_") {
			fmt.Fprintf(&text, "unset %s\n", shellquote.Join(key))
		}
	}
	for key, value := range f.env {
		fmt.Fprintf(&text, "export %s=%s\n", key, shellquote.Join(value))
	}
	fmt.Fprintf(&text, "cd %s\n", shellquote.Join(f.root))
	f.write(filepath.Join(f.root, "enter.sh"), text.String())
}

func offlineRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func offlineCopy(t *testing.T, from, to string) {
	t.Helper()
	require.NoError(t, os.WriteFile(to, offlineRead(t, from), 0o700))
}

// TestOfflineTransportHelper runs only in subprocesses selected by the fixture's wrappers.
func TestOfflineTransportHelper(t *testing.T) {
	mode := os.Getenv("IDENTITY_OFFLINE_HELPER")
	if mode == "" {
		return
	}
	var args []string
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if err := offlineTransport(mode, args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func offlineTransport(mode string, args []string) error {
	event := transportEvent{Transport: mode}
	switch mode {
	case "ssh":
		event.Args = args
		var err error
		event.Cwd, err = os.Getwd()
		if err != nil {
			return err
		}
		event.GitDir, event.GitWorkTree = os.Getenv("GIT_DIR"), os.Getenv("GIT_WORK_TREE")
		config, err := exec.Command("git", "config", "--get-regexp", `^gh-identity\.`).Output()
		if err != nil {
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				return err
			}
		}
		event.Config = string(config)
	case "https":
		if len(args) != 2 {
			return fmt.Errorf("expected remote name and URL, got %q", args)
		}
		event.URL = args[1]
		cmd := exec.Command("git", "credential", "fill")
		cmd.Stdin = strings.NewReader("url=" + event.URL + "\n\n")
		cmd.Stderr = os.Stderr
		credential, err := cmd.Output()
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(string(credential), "\n") {
			key, value, _ := strings.Cut(line, "=")
			switch key {
			case "username":
				event.Username = value
			case "password":
				event.Token = value
			}
		}
	default:
		return fmt.Errorf("unknown fixture transport %q", mode)
	}
	log, err := os.OpenFile(os.Getenv("IDENTITY_FIXTURE_LOG"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(log).Encode(event)
	closeErr := log.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if mode == "ssh" {
		// An empty Git protocol v0 repository advertisement.
		if _, err := io.WriteString(os.Stdout, "0000"); err != nil {
			return err
		}
		_, err := io.CopyN(io.Discard, os.Stdin, 4)
		if err != nil && err != io.EOF {
			return err
		}
		return nil
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		var response string
		switch {
		case strings.HasPrefix(line, "capabilities"):
			response = "option\n\n"
		case strings.HasPrefix(line, "option "):
			response = "ok\n"
		case strings.HasPrefix(line, "list"):
			response = "\n"
		case strings.TrimSpace(line) != "":
			return fmt.Errorf("unsupported fixture remote-helper operation: %s", line)
		default:
			continue
		}
		if _, err := io.WriteString(os.Stdout, response); err != nil {
			return err
		}
	}
	return scanner.Err()
}
