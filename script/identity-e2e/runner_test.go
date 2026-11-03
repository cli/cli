//go:build unix

package identitye2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const runnerJourney = `
exercise_git_journey() {
  prepare_scenario "$current_scenario" "$1"
  printf 'JOURNEY: %s %s\n' "$first_login" "$second_login"
  [[ "$GH_CONFIG_DIR" == "$scenario_root/gh-config" ]]
  [[ "$HOME" == "$scenario_root/home" ]]
  [[ "$GIT_CONFIG_GLOBAL" == "$scenario_root/gitconfig" ]]
  [[ "$(git config --global init.defaultBranch)" == main ]]
}
run_identity_lifecycle() { exercise_git_journey https directory; }
`

type runnerCall struct {
	Args            []string `json:"args"`
	Config          string   `json:"config"`
	Home            string   `json:"home"`
	KeyringDisabled string   `json:"keyringDisabled"`
}

type runnerFixture struct {
	t      *testing.T
	root   string
	normal string
	calls  string
	fake   string
	bash   string
	script string
	env    map[string]string
	stub   string
}

func newRunnerFixture(t *testing.T) *runnerFixture {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("runner tests require Bash 4.4+")
	}
	if err := exec.Command(bash, "-c", "(( BASH_VERSINFO[0] > 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] >= 4) ))").Run(); err != nil {
		t.Skip("runner tests require Bash 4.4+")
	}
	for _, dependency := range []string{"git", "jq"} {
		if _, err := exec.LookPath(dependency); err != nil {
			t.Skipf("runner tests require %s", dependency)
		}
	}
	// Keep all runner-created state in this checkout, including its mktemp root.
	root, err := os.MkdirTemp(".", ".runner-test-")
	require.NoError(t, err)
	root, err = filepath.Abs(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(root)) })
	f := &runnerFixture{
		t: t, root: root, normal: filepath.Join(root, "normal"),
		calls: filepath.Join(root, "calls.jsonl"), bash: bash, stub: runnerJourney,
	}
	f.script, err = filepath.Abs("run.sh")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(f.normal, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "home"), 0o700))
	require.NoError(t, os.WriteFile(f.calls, nil, 0o600))
	f.env = map[string]string{
		"PATH": os.Getenv("PATH"), "HOME": filepath.Join(root, "home"),
		"GH_CONFIG_DIR": f.normal, "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_CONFIG_GLOBAL": filepath.Join(root, "unused-gitconfig"),
		"TMPDIR":            root, "FIXTURE_NORMAL_CONFIG": f.normal,
		"FIXTURE_CALLS": f.calls, "FIXTURE_MODE": "ready",
		"IDENTITY_RUNNER_HELPER": "1",
		"GORACE":                 "atexit_sleep_ms=0",
	}
	f.fake = f.helper("gh")
	return f
}

func runnerQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (f *runnerFixture) helper(kind string) string {
	f.t.Helper()
	binary, err := os.Executable()
	require.NoError(f.t, err)
	path := filepath.Join(f.root, kind)
	script := fmt.Sprintf("#!/bin/sh\nexec %s -test.run='^TestRunnerHelperProcess$' -- %s \"$@\"\n", runnerQuote(binary), runnerQuote(kind))
	require.NoError(f.t, os.WriteFile(path, []byte(script), 0o700))
	return path
}

func (f *runnerFixture) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, f.bash, args...)
	for key, value := range f.env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	return cmd
}

func (f *runnerFixture) captured(args ...string) (int, string, string) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := f.command(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	require.NoError(f.t, ctx.Err(), "runner timed out:\n%s\n%s", stdout.String(), stderr.String())
	return runnerExitCode(f.t, err), stdout.String(), stderr.String()
}

func runnerExitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	require.ErrorAs(t, err, &exit)
	return exit.ExitCode()
}

var runnerANSI = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func (f *runnerFixture) run(responses []string, args ...string) (int, string) {
	f.t.Helper()
	code := "source " + runnerQuote(f.script) + "\n" + f.stub + "\nmain \"$@\""
	argv := append([]string{"-c", code, "runner-test", "--gh", f.fake,
		"--scenario", "identity-lifecycle", "--non-interactive"}, args...)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := f.command(ctx, argv...)
	terminal, err := pty.Start(cmd)
	require.NoError(f.t, err)
	defer terminal.Close()
	// Bash owns a fresh session. Kill its whole group on timeout or a failed
	// prompt assertion so helpers cannot survive the test.
	defer func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }()
	go func() {
		<-ctx.Done()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = terminal.Close()
	}()
	var output strings.Builder
	handled := 0
	buffer := make([]byte, 65536)
	for {
		n, readErr := terminal.Read(buffer)
		output.Write(buffer[:n])
		text := runnerANSI.ReplaceAllString(output.String(), "")
		prompts := 0
		for _, prompt := range []string{
			"Use these two accounts?", "Refresh these accounts or use fresh browser login?",
			"Enter two account numbers to use", "(e to explore, q to quit):", "gh-e2e > ",
		} {
			prompts += strings.Count(text, prompt)
		}
		for handled < prompts {
			if handled >= len(responses) {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				_ = cmd.Wait()
				f.t.Fatalf("unexpected account-selection prompt:\n%s", text)
			}
			_, err := io.WriteString(terminal, responses[handled]+"\n")
			require.NoError(f.t, err)
			handled++
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, syscall.EIO) && ctx.Err() == nil {
				require.NoError(f.t, readErr)
			}
			break
		}
	}
	err = cmd.Wait()
	text := runnerANSI.ReplaceAllString(output.String(), "")
	require.NoError(f.t, ctx.Err(), "runner timed out:\n%s", text)
	return runnerExitCode(f.t, err), text
}

func (f *runnerFixture) callsFor(prefix ...string) []runnerCall {
	f.t.Helper()
	data, err := os.ReadFile(f.calls)
	require.NoError(f.t, err)
	var calls []runnerCall
	for line := range bytes.SplitSeq(bytes.TrimSpace(data), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var call runnerCall
		require.NoError(f.t, json.Unmarshal(line, &call))
		if len(call.Args) >= len(prefix) && slices.Equal(call.Args[:len(prefix)], prefix) {
			calls = append(calls, call)
		}
	}
	return calls
}

func (f *runnerFixture) loginCount(flag string) int {
	count := 0
	for _, call := range f.callsFor("auth", "login") {
		if slices.Contains(call.Args, flag) {
			count++
		}
	}
	return count
}

func TestRunnerOverrideRequiresTerminalBeforeAccountSetup(t *testing.T) {
	t.Parallel()
	// Given captured output cannot support gh's own push-selection prompt
	f := newRunnerFixture(t)
	// When starting an override journey without a terminal
	status, _, stderr := f.captured(f.script, "--gh", f.fake, "--scenario", "https-account-override", "--non-interactive")
	// Then it fails before reading credentials or creating live resources
	assert.NotEqual(t, 0, status)
	assert.Contains(t, stderr, "require a terminal")
	assert.Empty(t, f.callsFor())
}

func TestRunnerGuidedDeletionPreservesTypedConfirmation(t *testing.T) {
	t.Parallel()
	// Given an interactive walkthrough
	f := newRunnerFixture(t)
	code := "source " + runnerQuote(f.script) + "\ngh_bin=\"$1\"\ninteractive=true\ndelete_identities work-identity\n"
	// When it invokes identity deletion
	status, _, stderr := f.captured("-c", code, "test", f.fake)
	// Then gh's own typed-name confirmation is not bypassed
	require.Equal(t, 0, status, stderr)
	calls := f.callsFor("auth", "identity", "delete")
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"auth", "identity", "delete", "work-identity"}, calls[0].Args)
}

func TestRunnerJourneysDiscoverableWithoutAuthentication(t *testing.T) {
	t.Parallel()
	// Given no permission to inspect normal credentials
	f := newRunnerFixture(t)
	// When listing journeys through the ordinary entry point
	status, stdout, stderr := f.captured(f.script, "--gh", f.fake, "--list")
	// Then all added journeys are visible without launching gh
	require.Equal(t, 0, status, stderr)
	for _, journey := range []string{"ssh-cooperation", "logout-recovery", "https-worktrees", "ssh-worktrees"} {
		assert.Contains(t, strings.Split(stdout, "\n"), journey)
	}
	assert.Empty(t, f.callsFor())
}

func TestRunnerRefusesBinaryWithoutKeyringIsolation(t *testing.T) {
	t.Parallel()
	// Given a binary that cannot prevent OS keyring access
	f := newRunnerFixture(t)
	f.env["FIXTURE_MODE"] = "no-keyring-isolation"
	// When starting a journey
	status, output := f.run(nil)
	// Then no account discovery or authentication is attempted
	assert.NotEqual(t, 0, status)
	assert.Contains(t, output, "cannot isolate the OS keyring")
	assert.Empty(t, f.callsFor("auth", "status"))
	assert.Empty(t, f.callsFor("auth", "token"))
	assert.Empty(t, f.callsFor("auth", "login"))
}

func TestRunnerUnattendedDeletionExplicitlyConfirmsAll(t *testing.T) {
	t.Parallel()
	// Given an unattended walkthrough
	f := newRunnerFixture(t)
	code := "source " + runnerQuote(f.script) + "\ngh_bin=\"$1\"\ninteractive=false\ndelete_identities --all\n"
	// When it invokes bulk deletion
	status, _, stderr := f.captured("-c", code, "test", f.fake)
	// Then --all names the target and --confirm supplies explicit confirmation
	require.Equal(t, 0, status, stderr)
	calls := f.callsFor("auth", "identity", "delete")
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"auth", "identity", "delete", "--all", "--confirm"}, calls[0].Args)
}

func TestRunnerReusesSavedAccountsWithoutBrowserLogin(t *testing.T) {
	t.Parallel()
	// Given two healthy saved accounts
	f := newRunnerFixture(t)
	// When reuse is accepted
	status, output := f.run([]string{""})
	// Then scenario-only token login preserves normal state and hides tokens
	require.Equal(t, 0, status, output)
	assert.Contains(t, output, "JOURNEY: alice bob")
	tokens := f.callsFor("auth", "token")
	require.Len(t, tokens, 2)
	for _, call := range tokens {
		assert.Contains(t, call.Args, "--no-refresh")
	}
	logins := f.callsFor("auth", "login")
	require.Len(t, logins, 2)
	for _, call := range logins {
		assert.Contains(t, call.Args, "--with-token")
		assert.NotEqual(t, f.normal, call.Config)
		assert.Equal(t, "true", call.KeyringDisabled)
	}
	assert.NoFileExists(t, filepath.Join(f.normal, "fake-accounts.json"))
	assert.NotContains(t, output, "fixture-alice-token")
	assert.NotContains(t, output, "fixture-bob-token")
}

func TestRunnerSavedAccountSetupHasNoGuidedReview(t *testing.T) {
	t.Parallel()
	// Given healthy saved accounts and an interactive journey
	f := newRunnerFixture(t)
	// When accepting reuse and restoring the accounts into the scenario
	status, output := f.run([]string{"", "", ""}, "--interactive")
	// Then setup completes without scenario-review stops
	require.Equal(t, 0, status, output)
	assert.Contains(t, output, "JOURNEY: alice bob")
	assert.NotContains(t, output, "Given ")
	assert.NotContains(t, output, "Press Enter")
}

func TestRunnerBrowserSetupHasNoGuidedReview(t *testing.T) {
	t.Parallel()
	// Given no reusable accounts and an interactive journey
	f := newRunnerFixture(t)
	f.env["FIXTURE_MODE"] = "missing"
	// When authenticating two accounts and preparing the scenario
	status, output := f.run(make([]string, 10), "--interactive")
	// Then browser authentication runs without extra scenario reviews
	require.Equal(t, 0, status, output)
	assert.Equal(t, 2, f.loginCount("--web"))
	assert.Contains(t, output, "JOURNEY: alice bob")
	assert.NotContains(t, output, "Given ")
	assert.NotContains(t, output, "Press Enter")
}

func TestRunnerSSHSetupHasNoReviewOrHostKeyDump(t *testing.T) {
	t.Parallel()
	// Given an interactive journey using fake SSH and GitHub commands
	f := newRunnerFixture(t)
	f.env["FIXTURE_SSH"] = f.helper("ssh")
	f.stub += `
ssh-keygen() {
  while [[ "$1" != -f ]]; do shift; done
  printf 'fixture private key\n' >"$2"
  printf 'fixture public key\n' >"$2.pub"
}
run_identity_lifecycle() {
  exercise_git_journey ssh directory
  real_ssh="$FIXTURE_SSH"
  configure_ssh
}
`
	// When preparing and checking the isolated SSH routes
	status, output := f.run(make([]string, 16), "--interactive")
	// Then both keys are provisioned and cleaned without reviews or key dumps
	require.Equal(t, 0, status, output)
	assert.Len(t, f.callsFor("ssh-key", "add"), 2)
	assert.Len(t, f.callsFor("api", "--method", "DELETE"), 2)
	assert.Contains(t, output, "Hi alice!")
	assert.Contains(t, output, "Hi bob!")
	assert.NotContains(t, output, "Given ")
	assert.NotContains(t, output, "Press Enter")
	assert.NotContains(t, output, "fixture-public-host-key")
}

func TestRunnerDecliningReuseFallsBackToBrowser(t *testing.T) {
	t.Parallel()
	// Given saved accounts
	f := newRunnerFixture(t)
	// When reuse is declined
	status, output := f.run([]string{"n"})
	// Then two browser accounts are restored and bootstrap accounts cleared
	require.Equal(t, 0, status, output)
	assert.Equal(t, 2, f.loginCount("--web"))
	assert.Equal(t, 2, f.loginCount("--with-token"))
	assert.Len(t, f.callsFor("auth", "logout"), 2)
	assert.NoFileExists(t, filepath.Join(f.normal, "fake-accounts.json"))
	assert.NotContains(t, output, "fixture-alice-token")
}

func TestRunnerMultipleAccountsRequireDistinctSelections(t *testing.T) {
	t.Parallel()
	// Given more than two saved accounts
	f := newRunnerFixture(t)
	f.env["FIXTURE_MODE"] = "multiple"
	// When selecting the same account twice, then correcting the selection
	status, output := f.run([]string{"1 1", "2 1", ""})
	// Then the duplicate is rejected and the selected ordering is respected
	require.Equal(t, 0, status, output)
	assert.Contains(t, output, "Choose two different numbers")
	assert.Contains(t, output, "JOURNEY: bob alice")
	var users []string
	for _, call := range f.callsFor("auth", "token") {
		users = append(users, call.Args[len(call.Args)-1])
	}
	assert.Equal(t, []string{"bob", "alice"}, users)
	assert.Equal(t, 0, f.loginCount("--web"))
}

func TestRunnerGuidedReviewCanExploreBeforeRunningStep(t *testing.T) {
	t.Parallel()
	// Given a guided step and a different gh on PATH
	f := newRunnerFixture(t)
	wrongBin := filepath.Join(f.root, "wrong-bin")
	require.NoError(t, os.Mkdir(wrongBin, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(wrongBin, "gh"), []byte("#!/bin/sh\nprintf 'WRONG GH SELECTED\\n'\n"), 0o700))
	f.env["PATH"] = wrongBin + string(os.PathListSeparator) + f.env["PATH"]
	f.stub += `
run_identity_lifecycle() {
  exercise_git_journey https directory
  interactive=true
  review_step "isolated accounts exist" "we inspect this step" "exploration is available" \
    "$scenario_root/work" "printf 'guided command'"
  printf 'GUIDED COMMAND RAN\n'
  review_complete "$scenario_root/work" "guided command ran"
}
`
	// When exploring the isolated shell before continuing the step
	status, output := f.run([]string{"", "e", "gh version", "printf 'EXPLORATION RAN\\n'", "", "", ""})
	// Then exploration uses the selected gh and the step resumes successfully
	require.Equal(t, 0, status, output)
	assert.Contains(t, output, "Given isolated accounts exist")
	assert.Contains(t, output, "Explore explore before running this step")
	assert.Contains(t, output, "\r\nEXPLORATION RAN\r\n")
	assert.Contains(t, output, "GUIDED COMMAND RAN")
	assert.Contains(t, output, "PASS guided command ran")
	assert.NotContains(t, output, "WRONG GH SELECTED")
	assert.Equal(t, 2, strings.Count(output, "fake gh for account bootstrap tests"))
}

func TestRunnerUnusableAccountsFallBackToBrowser(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"missing", "unhealthy", "unavailable-token"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			// Given accounts that cannot supply reusable healthy tokens
			f := newRunnerFixture(t)
			f.env["FIXTURE_MODE"] = mode
			// When starting the journey, accepting any reuse offer
			status, output := f.run([]string{""})
			// Then the original two-browser-login fallback succeeds
			require.Equal(t, 0, status, output)
			assert.Equal(t, 2, f.loginCount("--web"))
		})
	}
}

func TestRunnerDuplicateBrowserAccountFailsBeforeTokenCapture(t *testing.T) {
	t.Parallel()
	// Given browser login returns the same account twice
	f := newRunnerFixture(t)
	f.env["FIXTURE_MODE"] = "duplicate-login"
	// When declining saved-account reuse
	status, output := f.run([]string{"n"})
	// Then the duplicate is rejected before token capture
	assert.NotEqual(t, 0, status, output)
	assert.Contains(t, output, "expected two distinct accounts")
	assert.Empty(t, f.callsFor("auth", "token"))
}

func TestRunnerScopeRefreshFallsBackWithoutSwitchingOperator(t *testing.T) {
	t.Parallel()
	// Given saved accounts without the scopes required for Git journeys
	f := newRunnerFixture(t)
	f.env["FIXTURE_MODE"] = "missing-scopes"
	// When requesting a scope refresh
	status, output := f.run([]string{"r"}, "--scenario", "https-directory")
	// Then fresh isolated browser login replaces operator switching/refreshing
	require.Equal(t, 0, status, output)
	assert.Contains(t, output, "fresh browser login in isolated configuration")
	assert.Empty(t, f.callsFor("auth", "switch"))
	assert.Empty(t, f.callsFor("auth", "refresh"))
	assert.Equal(t, 2, f.loginCount("--web"))
}

func TestRunnerFailedBootstrapOnlyCleansItsUniqueChild(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		kept int
	}{
		{name: "clean", kept: 0},
		{name: "keep", args: []string{"--keep"}, kept: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given failed login and an unrelated file in the run-root parent
			f := newRunnerFixture(t)
			f.env["FIXTURE_MODE"] = "login-disabled"
			parent := filepath.Join(f.root, "parent")
			require.NoError(t, os.Mkdir(parent, 0o700))
			unrelated := filepath.Join(parent, "unrelated")
			require.NoError(t, os.WriteFile(unrelated, []byte("keep me"), 0o600))
			// When bootstrapping with the selected retention policy
			args := append([]string{"--run-root", parent}, tc.args...)
			status, output := f.run(nil, args...)
			// Then only the runner's unique child is eligible for cleanup
			assert.Equal(t, 1, status, output)
			assert.Contains(t, output, "fixture: browser login disabled")
			content, err := os.ReadFile(unrelated)
			require.NoError(t, err)
			assert.Equal(t, "keep me", string(content))
			children, err := filepath.Glob(filepath.Join(parent, "gh-identity-e2e.*"))
			require.NoError(t, err)
			assert.Len(t, children, tc.kept)
		})
	}
}

func TestRunnerRunRootDoesNotInheritEnclosingRepository(t *testing.T) {
	t.Parallel()
	// Given the run root is inside an existing repository
	f := newRunnerFixture(t)
	parent := filepath.Join(f.root, "outer-repository")
	status, _, stderr := f.captured("-c", "git init -q \"$1\"", "test", parent)
	require.Equal(t, 0, status, stderr)
	f.stub += `
run_identity_lifecycle() {
  exercise_git_journey https directory
  if git -C "$scenario_root/work" rev-parse --show-toplevel >/dev/null 2>&1; then
    fail "scenario unexpectedly inherited an enclosing repository"
  fi
}
`
	// When preparing a scenario beneath that root
	status, output := f.run([]string{""}, "--run-root", parent)
	// Then it does not inherit or remove the enclosing repository
	require.Equal(t, 0, status, output)
	assert.DirExists(t, filepath.Join(parent, ".git"))
}

func TestRunnerAutomaticCleanupDoesNotAddReviewStop(t *testing.T) {
	t.Parallel()
	// Given a completed interactive journey without --keep
	f := newRunnerFixture(t)
	f.stub += `
run_identity_lifecycle() {
  exercise_git_journey https directory
  created_repositories+=("$first_login:$first_login/unique-fixture")
  interactive=true
  finish_scenario
}
`
	// When finishing the journey
	status, output := f.run([]string{"", "", ""})
	// Then its resource is deleted without another scenario review
	require.Equal(t, 0, status, output)
	assert.NotContains(t, output, "Given the journey has completed")
	deletes := f.callsFor("repo", "delete")
	require.Len(t, deletes, 1)
	assert.Equal(t, []string{"repo", "delete", "--yes", "alice/unique-fixture"}, deletes[0].Args)
}

func TestRunnerCleanupFailurePreservesCredentialsAndResources(t *testing.T) {
	t.Parallel()
	// Given a created repository whose deletion fails
	f := newRunnerFixture(t)
	f.env["FIXTURE_CLEANUP_FAIL"] = "1"
	parent := filepath.Join(f.root, "recovery")
	f.stub += `
run_identity_lifecycle() {
  exercise_git_journey https directory
  created_repositories+=("$first_login:$first_login/unique-fixture")
}
`
	// When cleanup attempts the deletion
	status, output := f.run([]string{""}, "--run-root", parent)
	// Then isolated credentials and exact resource names remain for recovery
	assert.Equal(t, 1, status, output)
	runs, err := filepath.Glob(filepath.Join(parent, "gh-identity-e2e.*"))
	require.NoError(t, err)
	require.Len(t, runs, 1)
	content, err := os.ReadFile(filepath.Join(runs[0], "cleanup-needed.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "alice:alice/unique-fixture")
	assert.FileExists(t, filepath.Join(runs[0], "scenarios/identity-lifecycle/gh-config/fake-accounts.json"))
	deletes := f.callsFor("repo", "delete")
	require.Len(t, deletes, 1)
	assert.Equal(t, []string{"repo", "delete", "--yes", "alice/unique-fixture"}, deletes[0].Args)
	assert.NotEqual(t, f.normal, deletes[0].Config)
}

func TestRunnerCleanupRecoversKeyBeforeUploadReturns(t *testing.T) {
	t.Parallel()
	// Given a key registered for cleanup before its upload returned an ID
	f := newRunnerFixture(t)
	f.stub += `
run_identity_lifecycle() {
  exercise_git_journey https directory
  uploaded_ssh_keys+=("$first_login|unique-fixture-key-title|")
}
`
	// When the journey completes
	status, output := f.run([]string{""})
	// Then cleanup looks up the exact title and deletes its ID in isolation
	require.Equal(t, 0, status, output)
	lookup := f.callsFor("api", "user/keys")
	require.Len(t, lookup, 1)
	assert.Contains(t, lookup[0].Args[len(lookup[0].Args)-1], "unique-fixture-key-title")
	deleted := f.callsFor("api", "--method", "DELETE")
	require.Len(t, deleted, 1)
	assert.Equal(t, "user/keys/123", deleted[0].Args[len(deleted[0].Args)-1])
	assert.NotEqual(t, f.normal, deleted[0].Config)
}
