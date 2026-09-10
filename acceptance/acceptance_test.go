//go:build acceptance

package acceptance_test

import (
	"bytes"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"math/rand"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghcmd"
	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/cli/go-internal/testscript"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func ghMain() int {
	if repositoryCreationIsManaged(os.Args[1:], os.Getenv("GH_ACCEPTANCE_FIXTURE_MODE")) {
		fmt.Fprintln(os.Stderr, "gh repo create requires 'fixture-repo none'")
		return 1
	}
	return int(ghcmd.Main())
}

func repositoryCreationIsManaged(args []string, fixtureMode string) bool {
	return len(args) > 1 && args[0] == "repo" && args[1] == "create" && fixtureMode != "none"
}

const (
	workflowPollAttempts          = 13
	workflowPollInterval          = 5 * time.Second
	workflowRunStatusPollAttempts = 61
	workflowRunStatusPollInterval = time.Second
	repositoryReadyPollAttempts   = 31
	repositoryReadyPollInterval   = time.Second
)

var errWorkflowRunRegistrationTimeout = errors.New("workflow run did not register within 60 seconds")

func waitForWorkflowRun(list func() (string, error), sleep func(time.Duration)) (string, error) {
	for attempt := 0; attempt < workflowPollAttempts; attempt++ {
		runID, err := list()
		if err != nil {
			return "", err
		}
		if runID != "" {
			return runID, nil
		}
		if attempt < workflowPollAttempts-1 {
			sleep(workflowPollInterval)
		}
	}
	return "", errWorkflowRunRegistrationTimeout
}

func workflowRunIDFromOutput(output string) string {
	runURL, err := url.ParseRequestURI(strings.TrimSpace(output))
	if err != nil || runURL.Scheme == "" || runURL.Host == "" ||
		path.Base(path.Dir(runURL.Path)) != "runs" ||
		path.Base(path.Dir(path.Dir(runURL.Path))) != "actions" {
		return ""
	}
	runID := path.Base(runURL.Path)
	if _, err := strconv.ParseInt(runID, 10, 64); err != nil {
		return ""
	}
	return runID
}

func resolveWorkflowRunID(output string, list func() (string, error), sleep func(time.Duration)) (string, error) {
	if runID := workflowRunIDFromOutput(output); runID != "" {
		return runID, nil
	}
	return waitForWorkflowRun(list, sleep)
}

type workflowRunDiagnosticExecutor func(name string, args ...string) (stdout string, stderr string, err error)

func collectWorkflowRunDiagnostics(exec workflowRunDiagnosticExecutor, runListArgs []string) string {
	var diagnostics strings.Builder
	fmt.Fprintf(&diagnostics, "run filters: %s\n", strings.Join(runListArgs, " "))
	run := func(label string, name string, args ...string) string {
		stdout, stderr, err := exec(name, args...)
		fmt.Fprintf(&diagnostics, "%s:\n", label)
		if output := strings.TrimSpace(stdout); output != "" {
			fmt.Fprintln(&diagnostics, output)
		}
		if output := strings.TrimSpace(stderr); output != "" {
			fmt.Fprintf(&diagnostics, "stderr:\n%s\n", output)
		}
		if err != nil {
			fmt.Fprintf(&diagnostics, "error: %v\n", err)
		}
		return strings.TrimSpace(stdout)
	}

	run("repository", "git", "remote", "get-url", "origin")
	commit := run("local commit", "git", "rev-parse", "HEAD")
	branch := commandFlagValue(runListArgs, "--branch", "-b")
	event := commandFlagValue(runListArgs, "--event", "-e")

	if branch != "" {
		run("remote branch", "git", "ls-remote", "origin", "refs/heads/"+branch)
	}
	run("workflow files", "git", "ls-tree", "-r", "--name-only", "HEAD", ".github/workflows")
	run("recent workflow runs", "gh", "run", "list", "--limit", "20", "--json", "databaseId,workflowName,event,headBranch,headSha,status,createdAt")

	if commit != "" {
		run(
			"commit check suites",
			"gh", "api", "repos/{owner}/{repo}/commits/"+commit+"/check-suites",
			"--jq", `.check_suites[] | {id, status, conclusion, app: .app.slug, head_sha}`,
		)
	}

	apiArgs := []string{
		"api", "repos/{owner}/{repo}/actions/runs",
		"--method", "GET",
		"-f", "per_page=1",
		"--include",
		"--silent",
	}
	if branch != "" {
		apiArgs = append(apiArgs, "-f", "branch="+branch)
	}
	if event != "" {
		apiArgs = append(apiArgs, "-f", "event="+event)
	}
	if commit != "" {
		apiArgs = append(apiArgs, "-f", "head_sha="+commit)
	}
	run("Actions API response headers", "gh", apiArgs...)

	return strings.TrimSpace(diagnostics.String())
}

func commandFlagValue(args []string, names ...string) string {
	for i, arg := range args {
		for _, name := range names {
			if arg == name && i+1 < len(args) {
				return args[i+1]
			}
			if value, ok := strings.CutPrefix(arg, name+"="); ok {
				return value
			}
		}
	}
	return ""
}

func waitForWorkflow(list func() ([]string, error), expected string, sleep func(time.Duration)) error {
	for attempt := 0; attempt < workflowPollAttempts; attempt++ {
		workflows, err := list()
		if err != nil {
			return err
		}
		for _, workflow := range workflows {
			if workflow == expected {
				return nil
			}
		}
		if attempt < workflowPollAttempts-1 {
			sleep(workflowPollInterval)
		}
	}
	return fmt.Errorf("workflow %q did not register within 60 seconds", expected)
}

func waitForRepositoryReady(check func() (bool, error), sleep func(time.Duration)) error {
	for attempt := 0; attempt < repositoryReadyPollAttempts; attempt++ {
		ready, err := check()
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if attempt < repositoryReadyPollAttempts-1 {
			sleep(repositoryReadyPollInterval)
		}
	}
	return errors.New("repository did not finish initializing within 30 seconds")
}

func waitForWorkflowRunStatus(view func() (string, error), expected string, sleep func(time.Duration)) error {
	for attempt := 0; attempt < workflowRunStatusPollAttempts; attempt++ {
		status, err := view()
		if err != nil {
			return err
		}
		if status == expected {
			return nil
		}
		if status == "completed" {
			return fmt.Errorf("workflow run completed before reaching status %q", expected)
		}
		if attempt < workflowRunStatusPollAttempts-1 {
			sleep(workflowRunStatusPollInterval)
		}
	}
	return fmt.Errorf("workflow run did not reach status %q within 60 seconds", expected)
}

func outputForEnvironment(output string) (string, error) {
	if strings.TrimSpace(output) == "" {
		return "", errors.New("command output is empty")
	}
	return strings.TrimRight(output, "\n"), nil
}

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"gh": ghMain,
	}))
}

func TestGenerateSSHPublicKey(t *testing.T) {
	first, err := generateSSHPublicKey("myTitle")
	require.NoError(t, err)
	second, err := generateSSHPublicKey("myTitle")
	require.NoError(t, err)

	publicKey, comment, options, rest, err := ssh.ParseAuthorizedKey(first)
	require.NoError(t, err)
	assert.Equal(t, ssh.KeyAlgoED25519, publicKey.Type())
	assert.Equal(t, "myTitle", comment)
	assert.Empty(t, options)
	assert.Empty(t, rest)
	assert.NotEqual(t, first, second)
}

func TestSandboxFilePath(t *testing.T) {
	root := t.TempDir()

	path, err := sandboxFilePath(root, root, "keys/deploy.pub")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "keys/deploy.pub"), path)

	_, err = sandboxFilePath(root, root, filepath.Join(root, "deploy.pub"))
	assert.EqualError(t, err, "path must be relative to the testscript sandbox")

	_, err = sandboxFilePath(root, root, "../deploy.pub")
	assert.EqualError(t, err, "path must stay within the testscript sandbox")
}

func TestRepositoryCreationIsManaged(t *testing.T) {
	assert.True(t, repositoryCreationIsManaged([]string{"repo", "create", "example"}, "shared"))
	assert.True(t, repositoryCreationIsManaged([]string{"repo", "create", "example"}, "isolated"))
	assert.True(t, repositoryCreationIsManaged([]string{"repo", "create", "example"}, "undeclared"))
	assert.False(t, repositoryCreationIsManaged([]string{"repo", "create", "example"}, "none"))
	assert.False(t, repositoryCreationIsManaged([]string{"repo", "view", "example"}, "shared"))
}

func TestWaitForWorkflowRun(t *testing.T) {
	t.Run("returns registered run", func(t *testing.T) {
		var attempts int
		var sleeps []time.Duration
		runID, err := waitForWorkflowRun(func() (string, error) {
			attempts++
			if attempts == 3 {
				return "1234", nil
			}
			return "", nil
		}, func(duration time.Duration) {
			sleeps = append(sleeps, duration)
		})

		require.NoError(t, err)
		assert.Equal(t, "1234", runID)
		assert.Equal(t, []time.Duration{workflowPollInterval, workflowPollInterval}, sleeps)
	})

	t.Run("returns list error", func(t *testing.T) {
		_, err := waitForWorkflowRun(func() (string, error) {
			return "", errors.New("listing runs")
		}, func(time.Duration) {
			t.Fatal("unexpected sleep")
		})

		require.EqualError(t, err, "listing runs")
	})

	t.Run("times out", func(t *testing.T) {
		var attempts int
		var sleeps int
		_, err := waitForWorkflowRun(func() (string, error) {
			attempts++
			return "", nil
		}, func(time.Duration) {
			sleeps++
		})

		require.EqualError(t, err, "workflow run did not register within 60 seconds")
		assert.Equal(t, 13, attempts)
		assert.Equal(t, 12, sleeps)
	})
}

func TestWorkflowRunIDFromOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "GitHub.com workflow run URL",
			output: "https://github.com/OWNER/REPO/actions/runs/1234\n",
			want:   "1234",
		},
		{
			name:   "GHEC workflow run URL",
			output: "https://example.ghe.com/OWNER/REPO/actions/runs/5678\n",
			want:   "5678",
		},
		{
			name:   "empty legacy dispatch output",
			output: "",
		},
		{
			name:   "unrelated URL",
			output: "https://github.com/OWNER/REPO/actions/workflows/main.yml",
		},
		{
			name:   "non-numeric run identifier",
			output: "https://github.com/OWNER/REPO/actions/runs/latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, workflowRunIDFromOutput(tt.output))
		})
	}
}

func TestResolveWorkflowRunIDUsesDispatchOutput(t *testing.T) {
	runID, err := resolveWorkflowRunID(
		"https://github.com/OWNER/REPO/actions/runs/1234\n",
		func() (string, error) {
			t.Fatal("unexpected workflow run list")
			return "", nil
		},
		func(time.Duration) {
			t.Fatal("unexpected sleep")
		},
	)

	require.NoError(t, err)
	assert.Equal(t, "1234", runID)
}

func TestResolveWorkflowRunIDFallsBackToPolling(t *testing.T) {
	var attempts int
	runID, err := resolveWorkflowRunID("", func() (string, error) {
		attempts++
		return "5678", nil
	}, func(time.Duration) {
		t.Fatal("unexpected sleep")
	})

	require.NoError(t, err)
	assert.Equal(t, "5678", runID)
	assert.Equal(t, 1, attempts)
}

func TestCollectWorkflowRunDiagnostics(t *testing.T) {
	// Given diagnostic commands that expose each Actions registration boundary
	var actionsAPIArgs []string
	exec := func(name string, args ...string) (string, string, error) {
		switch {
		case name == "git" && args[0] == "rev-parse":
			return "abc123\n", "", nil
		case name == "git" && args[0] == "remote":
			return "https://github.com/example/repo.git\n", "", nil
		case name == "git" && args[0] == "ls-remote":
			return "abc123\trefs/heads/feature\n", "", nil
		case name == "git" && args[0] == "ls-tree":
			return ".github/workflows/workflow.yml\n", "", nil
		case name == "gh" && args[0] == "run":
			return `[{"databaseId":42,"headBranch":"other"}]`, "", nil
		case name == "gh" && args[1] == "repos/{owner}/{repo}/commits/abc123/check-suites":
			return `{"id":7,"app":"actions"}`, "", nil
		case name == "gh" && args[1] == "repos/{owner}/{repo}/actions/runs":
			actionsAPIArgs = append([]string(nil), args...)
			return "x-github-request-id: REQUEST-ID\n", "", nil
		default:
			return "", "", fmt.Errorf("unexpected command: %s %s", name, strings.Join(args, " "))
		}
	}

	// When collecting diagnostics for a run that did not register
	diagnostics := collectWorkflowRunDiagnostics(exec, []string{"--branch", "feature", "--event", "push"})

	// Then the snapshot includes evidence from every boundary and the request ID
	assert.Contains(t, diagnostics, "run filters: --branch feature --event push")
	assert.Contains(t, diagnostics, "repository:\nhttps://github.com/example/repo.git")
	assert.Contains(t, diagnostics, "local commit:\nabc123")
	assert.Contains(t, diagnostics, "remote branch:\nabc123\trefs/heads/feature")
	assert.Contains(t, diagnostics, "workflow files:\n.github/workflows/workflow.yml")
	assert.Contains(t, diagnostics, "recent workflow runs:\n"+`[{"databaseId":42,"headBranch":"other"}]`)
	assert.Contains(t, diagnostics, "commit check suites:\n"+`{"id":7,"app":"actions"}`)
	assert.Contains(t, diagnostics, "Actions API response headers:\nx-github-request-id: REQUEST-ID")
	assert.Equal(t, []string{
		"api", "repos/{owner}/{repo}/actions/runs",
		"--method", "GET",
		"-f", "per_page=1",
		"--include",
		"--silent",
		"-f", "branch=feature",
		"-f", "event=push",
		"-f", "head_sha=abc123",
	}, actionsAPIArgs)
}

func TestCollectWorkflowRunDiagnosticsIncludesCommandFailures(t *testing.T) {
	// Given diagnostic commands that fail while gathering supplementary evidence
	exec := func(string, ...string) (string, string, error) {
		return "", "service unavailable", errors.New("exit status 1")
	}

	// When collecting diagnostics for a run that did not register
	diagnostics := collectWorkflowRunDiagnostics(exec, nil)

	// Then each failure is reported without replacing the original timeout
	assert.Contains(t, diagnostics, "stderr:\nservice unavailable")
	assert.Contains(t, diagnostics, "error: exit status 1")
}

func TestWaitForWorkflow(t *testing.T) {
	// Given a workflow definition that appears after GitHub processes the pushed file
	var attempts int
	var sleeps []time.Duration

	// When waiting for the workflow by name
	err := waitForWorkflow(func() ([]string, error) {
		attempts++
		if attempts == 3 {
			return []string{"Other Workflow", "Test Workflow Name"}, nil
		}
		return []string{"Other Workflow"}, nil
	}, "Test Workflow Name", func(duration time.Duration) {
		sleeps = append(sleeps, duration)
	})

	// Then polling continues until that exact workflow is registered
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{workflowPollInterval, workflowPollInterval}, sleeps)
}

func TestWaitForWorkflowTimesOut(t *testing.T) {
	// Given a pushed workflow definition that never appears
	var attempts int
	var sleeps int

	// When waiting for the workflow by name
	err := waitForWorkflow(func() ([]string, error) {
		attempts++
		return []string{"Other Workflow"}, nil
	}, "Test Workflow Name", func(time.Duration) {
		sleeps++
	})

	// Then the wait stops after one minute
	require.EqualError(t, err, `workflow "Test Workflow Name" did not register within 60 seconds`)
	assert.Equal(t, 13, attempts)
	assert.Equal(t, 12, sleeps)
}

func TestWaitForRepositoryReady(t *testing.T) {
	// Given a repository whose initialized default branch appears after creation
	checks := []bool{false, false, true}
	var sleeps []time.Duration

	// When waiting for repository initialization to finish
	err := waitForRepositoryReady(func() (bool, error) {
		ready := checks[0]
		checks = checks[1:]
		return ready, nil
	}, func(duration time.Duration) {
		sleeps = append(sleeps, duration)
	})

	// Then polling continues until the default branch commit is available
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{repositoryReadyPollInterval, repositoryReadyPollInterval}, sleeps)
}

func TestWaitForRepositoryReadyTimesOut(t *testing.T) {
	// Given a repository whose default branch remains unavailable
	var attempts int
	var sleeps int

	// When waiting for repository initialization to finish
	err := waitForRepositoryReady(func() (bool, error) {
		attempts++
		return false, nil
	}, func(time.Duration) {
		sleeps++
	})

	// Then the wait stops after thirty seconds
	require.EqualError(t, err, "repository did not finish initializing within 30 seconds")
	assert.Equal(t, 31, attempts)
	assert.Equal(t, 30, sleeps)
}

func TestWaitForRepositoryReadyReturnsUnexpectedError(t *testing.T) {
	// Given a repository readiness check that fails unexpectedly
	check := func() (bool, error) {
		return false, errors.New("checking repository")
	}

	// When waiting for repository initialization to finish
	err := waitForRepositoryReady(check, func(time.Duration) {
		t.Fatal("unexpected sleep")
	})

	// Then the unexpected failure is returned instead of retried
	require.EqualError(t, err, "checking repository")
}

func TestWaitForWorkflowRunStatus(t *testing.T) {
	// Given a workflow run that is registered but still queued
	statuses := []string{"queued", "in_progress"}
	var sleeps []time.Duration

	// When waiting for the run to start
	err := waitForWorkflowRunStatus(func() (string, error) {
		status := statuses[0]
		statuses = statuses[1:]
		return status, nil
	}, "in_progress", func(duration time.Duration) {
		sleeps = append(sleeps, duration)
	})

	// Then polling continues until cancellation can target the active run
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{workflowRunStatusPollInterval}, sleeps)
}

func TestWaitForWorkflowRunStatusStopsWhenRunCompletes(t *testing.T) {
	// Given a workflow run that completed before reaching the expected status
	view := func() (string, error) {
		return "completed", nil
	}

	// When waiting for a status the run can no longer reach
	err := waitForWorkflowRunStatus(view, "in_progress", func(time.Duration) {
		t.Fatal("unexpected sleep")
	})

	// Then the failed precondition is reported immediately
	require.EqualError(t, err, `workflow run completed before reaching status "in_progress"`)
}

func TestWaitForWorkflowRunStatusTimesOut(t *testing.T) {
	// Given a workflow run that remains queued
	var attempts int
	var sleeps int

	// When waiting for it to start
	err := waitForWorkflowRunStatus(func() (string, error) {
		attempts++
		return "queued", nil
	}, "in_progress", func(time.Duration) {
		sleeps++
	})

	// Then the wait stops after one minute
	require.EqualError(t, err, `workflow run did not reach status "in_progress" within 60 seconds`)
	assert.Equal(t, 61, attempts)
	assert.Equal(t, 60, sleeps)
}

func TestOutputForEnvironment(t *testing.T) {
	value, err := outputForEnvironment("1234\n")
	require.NoError(t, err)
	assert.Equal(t, "1234", value)

	_, err = outputForEnvironment("\n")
	require.EqualError(t, err, "command output is empty")
}

func TestAcceptance(t *testing.T) {
	var tsEnv testScriptEnv
	if err := tsEnv.fromEnv(); err != nil {
		t.Fatal(err)
	}

	fixtureRepositories, err := newFixtureRepositoryManager(tsEnv)
	if err != nil {
		t.Fatal(err)
	}
	registerFixtureRepositoryCleanup(t, tsEnv.skipDefer, fixtureRepositories)

	testGroups, err := selectAcceptanceTestGroups(
		acceptanceTestGroups(t),
		os.Getenv("GH_ACCEPTANCE_GROUP"),
	)
	if err != nil {
		t.Fatal(err)
	}

	validateAcceptanceScripts(t, tsEnv, testGroups)

	for _, group := range testGroups {
		t.Run(group, func(t *testing.T) {
			testscript.Run(t, testScriptParamsFor(t, tsEnv, fixtureRepositories, group))
		})
	}
}

func registerFixtureRepositoryCleanup(t *testing.T, skip bool, fixtureRepositories *fixtureRepositoryManager) {
	t.Helper()
	if skip {
		return
	}
	t.Cleanup(func() {
		if err := fixtureRepositories.cleanup(); err != nil {
			t.Errorf("cleaning up fixture repositories: %v", err)
		}
	})
}

func validateAcceptanceScripts(t *testing.T, tsEnv testScriptEnv, groups []string) {
	t.Helper()

	for _, group := range groups {
		candidates, _, err := acceptanceScriptCandidates(tsEnv, group)
		require.NoError(t, err)
		for _, file := range candidates {
			require.NoError(t, validateFixtureRepositoryDeclaration(file))
			_, err := requiresUserCapabilityForScript(file)
			require.NoError(t, err)
		}
	}
}

func acceptanceTestGroups(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir("testdata")
	require.NoError(t, err)

	var groups []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		files, err := filepath.Glob(filepath.Join("testdata", entry.Name(), "*.txtar"))
		require.NoError(t, err)
		if len(files) > 0 {
			groups = append(groups, entry.Name())
		}
	}
	require.NotEmpty(t, groups)
	return groups
}

func selectAcceptanceTestGroups(available []string, requested string) ([]string, error) {
	if requested == "" || requested == "all" {
		return available, nil
	}

	for _, group := range available {
		if group == requested {
			return []string{requested}, nil
		}
	}

	return nil, fmt.Errorf("unknown acceptance test group %q; available groups: %s", requested, strings.Join(available, ", "))
}

func TestSelectAcceptanceTestGroups(t *testing.T) {
	available := []string{"api", "pr", "repo"}

	tests := []struct {
		name      string
		requested string
		want      []string
		wantErr   string
	}{
		{name: "empty selects all", want: available},
		{name: "all selects all", requested: "all", want: available},
		{name: "group selects one", requested: "pr", want: []string{"pr"}},
		{name: "unknown group errors", requested: "pull-request", wantErr: `unknown acceptance test group "pull-request"; available groups: api, pr, repo`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectAcceptanceTestGroups(available, tt.requested)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

type acceptanceScript struct {
	file                   string
	requiresUserCapability bool
}

func filterAcceptanceScripts(candidates []acceptanceScript, filtered, hasUserCapability bool) ([]string, string, error) {
	if filtered && len(candidates) == 0 {
		return nil, "no selected script belongs to this command directory", nil
	}

	files := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.requiresUserCapability && !hasUserCapability {
			if filtered {
				return nil, "", fmt.Errorf("%s requires a token that authenticates a user", candidate.file)
			}
			continue
		}
		files = append(files, candidate.file)
	}
	if len(files) == 0 {
		return nil, "all scripts require a token that authenticates a user", nil
	}

	return files, "", nil
}

func TestFilterAcceptanceScripts(t *testing.T) {
	userOnly := acceptanceScript{file: "user.txtar", requiresUserCapability: true}
	compatible := acceptanceScript{file: "installation.txtar"}

	tests := []struct {
		name              string
		candidates        []acceptanceScript
		filtered          bool
		hasUserCapability bool
		wantFiles         []string
		wantSkip          string
		wantErr           string
	}{
		{
			name:              "user token keeps all scripts",
			candidates:        []acceptanceScript{userOnly, compatible},
			hasUserCapability: true,
			wantFiles:         []string{"user.txtar", "installation.txtar"},
		},
		{
			name:       "installation token omits user-only scripts",
			candidates: []acceptanceScript{userOnly, compatible},
			wantFiles:  []string{"installation.txtar"},
		},
		{
			name:       "all incompatible scripts skip",
			candidates: []acceptanceScript{userOnly},
			wantSkip:   "all scripts require a token that authenticates a user",
		},
		{
			name:     "empty explicit selection skips",
			filtered: true,
			wantSkip: "no selected script belongs to this command directory",
		},
		{
			name:       "explicit compatible selection remains",
			candidates: []acceptanceScript{compatible},
			filtered:   true,
			wantFiles:  []string{"installation.txtar"},
		},
		{
			name:       "explicit incompatible selection errors",
			candidates: []acceptanceScript{userOnly},
			filtered:   true,
			wantErr:    "user.txtar requires a token that authenticates a user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, skipReason, err := filterAcceptanceScripts(tt.candidates, tt.filtered, tt.hasUserCapability)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFiles, files)
			assert.Equal(t, tt.wantSkip, skipReason)
		})
	}
}

func testScriptParamsFor(t *testing.T, tsEnv testScriptEnv, fixtureRepositories *fixtureRepositoryManager, command string) testscript.Params {
	t.Helper()

	scriptFiles, filtered, err := acceptanceScriptCandidates(tsEnv, command)
	if err != nil {
		t.Fatal(err)
	}

	candidates := make([]acceptanceScript, 0, len(scriptFiles))
	for _, file := range scriptFiles {
		requiresUserCapability, err := requiresUserCapabilityForScript(file)
		if err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, acceptanceScript{
			file:                   file,
			requiresUserCapability: requiresUserCapability,
		})
	}

	files, skipReason, err := filterAcceptanceScripts(candidates, filtered, tsEnv.hasUserCapability)
	if err != nil {
		t.Fatal(err)
	}
	if skipReason != "" {
		if filtered {
			t.Skipf("testdata/%s: %s", command, skipReason)
		}
		t.Skip(skipReason)
	}

	return testscript.Params{
		Files:               files,
		Setup:               sharedSetup(tsEnv),
		Cmds:                sharedCmds(tsEnv, fixtureRepositories),
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		TestWork:            tsEnv.preserveWorkDir,
	}
}

func acceptanceScriptCandidates(tsEnv testScriptEnv, command string) ([]string, bool, error) {
	files, filtered := selectScripts(command, tsEnv.scripts)
	if filtered {
		return files, true, nil
	}
	files, err := filepath.Glob(filepath.Join("testdata", command, "*.txtar"))
	return files, false, err
}

var keyT struct{}

func sharedSetup(tsEnv testScriptEnv) func(ts *testscript.Env) error {
	return func(ts *testscript.Env) error {
		scriptName, ok := extractScriptName(ts.Vars)
		if !ok {
			ts.T().Fatal("script name not found")
		}

		// When using script name to uniquely identify where test data comes from,
		// some places like GitHub Actions secret names don't accept hyphens.
		// Replace them with underscores until such a time this becomes a problem.
		ts.Setenv("SCRIPT_NAME", strings.ReplaceAll(scriptName, "-", "_"))
		ts.Setenv("HOME", ts.Cd)
		ts.Setenv("GH_CONFIG_DIR", ts.Cd)

		ts.Setenv("GH_HOST", tsEnv.host)
		ts.Setenv("ORG", tsEnv.org)

		if tsEnv.apiHost == "" {
			ts.Setenv("GH_TOKEN", tsEnv.token)
		} else {
			// api_host is only readable from hosts.yml, and a GH_TOKEN in the
			// environment resolves auth without ever consulting that file, so
			// the token has to move into the same place as the override.
			hostsFile := filepath.Join(ts.Cd, "hosts.yml")
			hostsContent := fmt.Sprintf(""+
				"%[1]s:\n"+
				"    user: %[2]s\n"+
				"    oauth_token: %[3]s\n"+
				"    git_protocol: https\n"+
				"    api_host: %[4]s\n"+
				"    users:\n"+
				"        %[2]s:\n"+
				"            oauth_token: %[3]s\n",
				tsEnv.host, tsEnv.user, tsEnv.token, tsEnv.apiHost)
			if err := os.WriteFile(hostsFile, []byte(hostsContent), 0o600); err != nil {
				return fmt.Errorf("writing sandbox hosts.yml: %w", err)
			}
		}
		ts.Setenv("GH_ACCEPTANCE_FIXTURE_MODE", "undeclared")

		ts.Setenv("RANDOM_STRING", randomString(10))

		ts.Setenv("GH_TELEMETRY", "false")

		// testscript constructs a fresh environment from a fixed allowlist and
		// does not propagate SSL_CERT_FILE. When the operator has set it - for
		// instance because all API traffic routes through a gateway whose CA is
		// not in the system bundle - honour that intent explicitly, or every
		// request inside the sandbox will fail certificate verification.
		if certFile := os.Getenv("SSL_CERT_FILE"); certFile != "" {
			ts.Setenv("SSL_CERT_FILE", certFile)
		}

		// The sandbox overrides HOME, so git cannot find the user's global
		// config. Write a minimal identity so commits inside the sandbox
		// don't fail with "Author identity unknown".
		gitCfg := filepath.Join(ts.Cd, ".gitconfig")
		gitCfgContent := heredoc.Doc(`
			[user]
				name = GitHub CLI Acceptance Test Runner
				email = cli-acceptance-test-runner@github.com
		`)
		if err := os.WriteFile(gitCfg, []byte(gitCfgContent), 0o644); err != nil {
			return fmt.Errorf("writing sandbox .gitconfig: %w", err)
		}

		ts.Values[keyT] = ts.T()
		return nil
	}
}

// sharedCmds defines a collection of custom testscript commands for our use.
func sharedCmds(tsEnv testScriptEnv, fixtureRepositories *fixtureRepositoryManager) map[string]func(ts *testscript.TestScript, neg bool, args []string) {
	return map[string]func(ts *testscript.TestScript, neg bool, args []string){
		"defer": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! defer")
			}

			if tsEnv.skipDefer {
				return
			}

			tt, ok := ts.Value(keyT).(testscript.T)
			if !ok {
				ts.Fatalf("%v is not a testscript.T", ts.Value(keyT))
			}

			ts.Defer(func() {
				if args[0] == "cleanup-repo" {
					if len(args) != 2 {
						tt.Fatal("usage: defer cleanup-repo REPO")
						return
					}
					if err := fixtureRepositories.delete(args[1]); err != nil {
						tt.Fatal(err)
					}
					return
				}

				// If you're wondering why we're not using ts.Check here, it's because it raises a panic, and testscript
				// only catches the panics directly from commands, not from the deferred functions. So what we do
				// instead is grab the `t` in the setup function and store it as a value. It's important that we use
				// `t` from the setup function because it represents the subtest created for each individual script,
				// rather than each top-level test.
				// See: https://github.com/rogpeppe/go-internal/issues/276
				if err := ts.Exec(args[0], args[1:]...); err != nil {
					tt.FailNow()
				}
			})
		},
		"env2upper": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! env2upper")
			}
			if len(args) == 0 {
				ts.Fatalf("usage: env2upper name=value ...")
			}
			for _, env := range args {
				i := strings.Index(env, "=")

				if i < 0 {
					ts.Fatalf("env2upper: argument does not match name=value")
				}

				ts.Setenv(env[:i], strings.ToUpper(env[i+1:]))
			}
		},
		"fixture-repo": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! fixture-repo")
			}
			if len(args) == 1 && args[0] == "none" {
				ts.Setenv("GH_ACCEPTANCE_FIXTURE_MODE", "none")
				return
			}
			if len(args) != 2 || (args[0] != "shared" && args[0] != "isolated") {
				ts.Fatalf("usage: fixture-repo (shared|isolated) ENV_VAR, or fixture-repo none")
			}

			repository, err := fixtureRepositories.repository(args[0])
			ts.Check(err)
			ts.Setenv("GH_ACCEPTANCE_FIXTURE_MODE", args[0])
			ts.Setenv(args[1], repository)
		},
		"generate-ssh-key": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! generate-ssh-key")
			}
			if len(args) < 1 || len(args) > 2 {
				ts.Fatalf("usage: generate-ssh-key file [comment]")
			}

			comment := ""
			if len(args) == 2 {
				comment = args[1]
			}
			publicKey, err := generateSSHPublicKey(comment)
			ts.Check(err)
			outputPath, err := sandboxFilePath(ts.Getenv("WORK"), ts.MkAbs("."), args[0])
			ts.Check(err)
			ts.Check(os.WriteFile(outputPath, publicKey, 0o644))
		},
		"replace": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! replace")
			}
			if len(args) < 2 {
				ts.Fatalf("usage: replace file env...")
			}

			src := ts.MkAbs(args[0])
			ts.Logf("replace src: %s", src)

			// Preserve the existing file mode while replacing the contents similar to native cp behavior
			info, err := os.Stat(src)
			ts.Check(err)
			mode := info.Mode() & 0o777
			data, err := os.ReadFile(src)
			ts.Check(err)

			for _, arg := range args[1:] {
				i := strings.Index(arg, "=")
				if i < 0 {
					ts.Fatalf("replace: %s argument does not match name=value", arg)
				}

				name := fmt.Sprintf("$%s", arg[:i])
				value := arg[i+1:]
				ts.Logf("replace %s: %s", name, value)

				// `replace` was originally built similar to `cp` and `cmpenv`, expanding environment variables within a file.
				// However files with content that looks like environments variable such as GitHub Actions workflows
				// were being modified unexpectedly. Thus `replace` has been designed to using string replacement
				// looking for `$KEY` specifically.
				data = []byte(strings.ReplaceAll(string(data), name, value))
			}

			ts.Check(os.WriteFile(src, data, mode))
		},
		"stdout2env": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! stdout2env")
			}
			if len(args) != 1 {
				ts.Fatalf("usage: stdout2env name")
			}

			value, err := outputForEnvironment(ts.ReadFile("stdout"))
			ts.Check(err)
			ts.Setenv(args[0], value)
		},
		"wait-for-run": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! wait-for-run")
			}
			if len(args) < 1 {
				ts.Fatalf("usage: wait-for-run ENV_VAR [run-list-flags...]")
			}

			runID, err := resolveWorkflowRunID(ts.ReadFile("stdout"), func() (string, error) {
				listArgs := append([]string{"run", "list"}, args[1:]...)
				listArgs = append(listArgs, "--limit", "1", "--json", "databaseId", "--jq", ".[].databaseId")
				if err := ts.Exec("gh", listArgs...); err != nil {
					return "", err
				}
				return strings.TrimSpace(ts.ReadFile("stdout")), nil
			}, time.Sleep)
			if errors.Is(err, errWorkflowRunRegistrationTimeout) {
				diagnostics := collectWorkflowRunDiagnostics(func(name string, args ...string) (string, string, error) {
					err := ts.Exec(name, args...)
					return ts.ReadFile("stdout"), ts.ReadFile("stderr"), err
				}, args[1:])
				ts.Logf("workflow run registration diagnostics:\n%s", diagnostics)
			}
			ts.Check(err)
			ts.Setenv(args[0], runID)
		},
		"wait-for-workflow": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! wait-for-workflow")
			}
			if len(args) != 1 {
				ts.Fatalf("usage: wait-for-workflow NAME")
			}

			err := waitForWorkflow(func() ([]string, error) {
				if err := ts.Exec("gh", "workflow", "list", "--all", "--limit", "1000", "--json", "name", "--jq", ".[].name"); err != nil {
					return nil, err
				}
				output := strings.TrimSpace(ts.ReadFile("stdout"))
				if output == "" {
					return nil, nil
				}
				return strings.Split(output, "\n"), nil
			}, args[0], time.Sleep)
			ts.Check(err)
		},
		"wait-for-repository-ready": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! wait-for-repository-ready")
			}
			if len(args) != 1 {
				ts.Fatalf("usage: wait-for-repository-ready OWNER/REPO")
			}

			err := waitForRepositoryReady(func() (bool, error) {
				if err := ts.Exec("gh", "api", "repos/"+args[0]+"/commits/HEAD", "--silent"); err != nil {
					stderr := ts.ReadFile("stderr")
					if strings.Contains(stderr, "HTTP 404") || strings.Contains(stderr, "HTTP 409") {
						return false, nil
					}
					return false, err
				}
				return true, nil
			}, time.Sleep)
			ts.Check(err)
		},
		"wait-for-run-status": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! wait-for-run-status")
			}
			if len(args) != 2 {
				ts.Fatalf("usage: wait-for-run-status RUN_ID STATUS")
			}

			err := waitForWorkflowRunStatus(func() (string, error) {
				if err := ts.Exec("gh", "run", "view", args[0], "--json", "status", "--jq", ".status"); err != nil {
					return "", err
				}
				return strings.TrimSpace(ts.ReadFile("stdout")), nil
			}, args[1], time.Sleep)
			ts.Check(err)
		},
		"sleep": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! sleep")
			}
			if len(args) != 1 {
				ts.Fatalf("usage: sleep seconds")
			}

			// sleep for the given number of seconds
			seconds, err := strconv.Atoi(args[0])
			if err != nil {
				ts.Fatalf("invalid number of seconds: %v", err)
			}

			d := time.Duration(seconds) * time.Second
			time.Sleep(d)
		},
		"jq-assert": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! jq-assert")
			}
			if len(args) != 3 {
				ts.Fatalf("usage: jq-assert ENV_VAR expression regexp")
			}

			input := ts.Getenv(args[0])
			if input == "" {
				ts.Fatalf("jq-assert: environment variable %s is empty or unset", args[0])
			}

			var buf bytes.Buffer
			if err := jq.Evaluate(strings.NewReader(input), &buf, args[1]); err != nil {
				ts.Fatalf("jq-assert: %v", err)
			}

			result := strings.TrimRight(buf.String(), "\n") // jq.Evaluate adds a newline at the end
			ts.Logf("jq-assert %s %q => %s", args[0], args[1], result)

			re, err := regexp.Compile(args[2])
			if err != nil {
				ts.Fatalf("jq-assert: invalid regexp %q: %v", args[2], err)
			}
			if !re.MatchString(result) {
				ts.Fatalf("jq-assert: result %q does not match %q", result, args[2])
			}
		},
		"jq2env": func(ts *testscript.TestScript, neg bool, args []string) {
			if neg {
				ts.Fatalf("unsupported: ! jq2env")
			}
			if len(args) != 3 {
				ts.Fatalf("usage: jq2env SRC_ENV expression DST_ENV")
			}

			input := ts.Getenv(args[0])
			if input == "" {
				ts.Fatalf("jq2env: environment variable %s is empty or unset", args[0])
			}

			var buf bytes.Buffer
			if err := jq.Evaluate(strings.NewReader(input), &buf, args[1]); err != nil {
				ts.Fatalf("jq2env: %v", err)
			}

			result := strings.TrimRight(buf.String(), "\n") // jq.Evaluate adds a newline at the end
			ts.Logf("jq2env %s %q => %s => %s", args[0], args[1], result, args[2])
			ts.Setenv(args[2], result)
		},
	}
}

func generateSSHPublicKey(comment string) ([]byte, error) {
	publicKey, _, err := ed25519.GenerateKey(cryptorand.Reader)
	if err != nil {
		return nil, err
	}

	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return nil, err
	}

	authorizedKey := bytes.TrimSpace(ssh.MarshalAuthorizedKey(sshPublicKey))
	if comment != "" {
		authorizedKey = append(authorizedKey, ' ')
		authorizedKey = append(authorizedKey, comment...)
	}
	return append(authorizedKey, '\n'), nil
}

func sandboxFilePath(root, currentDir, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", errors.New("path must be relative to the testscript sandbox")
	}

	outputPath := filepath.Clean(filepath.Join(currentDir, name))
	relativePath, err := filepath.Rel(root, outputPath)
	if err != nil {
		return "", err
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", errors.New("path must stay within the testscript sandbox")
	}
	return outputPath, nil
}

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randomString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func extractScriptName(vars []string) (string, bool) {
	for _, kv := range vars {
		if strings.HasPrefix(kv, "WORK=") {
			v := strings.Split(kv, "=")[1]
			return strings.CutPrefix(filepath.Base(v), "script-")
		}
	}
	return "", false
}

type missingEnvError struct {
	missingEnvs []string
}

func (e missingEnvError) Error() string {
	return fmt.Sprintf("environment variable(s) %s must be set and non-empty", strings.Join(e.missingEnvs, ", "))
}

type testScriptEnv struct {
	host              string
	org               string
	token             string
	user              string
	hasUserCapability bool

	// scripts optionally narrows a run to named scripts within the command
	// directory being run. Empty means run every script in the directory.
	scripts []string

	// apiHost, when set, routes API traffic through that hostname by writing a
	// hosts.yml instead of authenticating from GH_TOKEN. Used by the gateway
	// harness in script/api-host-gateway.
	apiHost string

	skipDefer       bool
	preserveWorkDir bool
}

func (e *testScriptEnv) fromEnv() error {
	envMap := map[string]string{}

	requiredEnvVars := []string{
		"GH_ACCEPTANCE_HOST",
		"GH_ACCEPTANCE_ORG",
		"GH_ACCEPTANCE_TOKEN",
	}

	var missingEnvs []string
	for _, key := range requiredEnvVars {
		val, ok := os.LookupEnv(key)
		if val == "" || !ok {
			missingEnvs = append(missingEnvs, key)
			continue
		}

		envMap[key] = val
	}

	if len(missingEnvs) > 0 {
		return missingEnvError{missingEnvs: missingEnvs}
	}

	if envMap["GH_ACCEPTANCE_ORG"] == "github" || envMap["GH_ACCEPTANCE_ORG"] == "cli" {
		return fmt.Errorf("GH_ACCEPTANCE_ORG cannot be 'github' or 'cli'")
	}

	e.host = envMap["GH_ACCEPTANCE_HOST"]
	e.org = envMap["GH_ACCEPTANCE_ORG"]
	e.token = envMap["GH_ACCEPTANCE_TOKEN"]
	var err error
	e.hasUserCapability, err = tokenHasUserCapability(e.token)
	if err != nil {
		return err
	}

	e.scripts = parseScriptFilter(os.Getenv("GH_ACCEPTANCE_SCRIPT"))
	e.preserveWorkDir = os.Getenv("GH_ACCEPTANCE_PRESERVE_WORK_DIR") == "true"
	e.skipDefer = os.Getenv("GH_ACCEPTANCE_SKIP_DEFER") == "true"
	e.apiHost = os.Getenv("GH_ACCEPTANCE_API_HOST")
	e.user = os.Getenv("GH_ACCEPTANCE_USER")
	if e.apiHost != "" && e.user == "" {
		return fmt.Errorf("GH_ACCEPTANCE_USER is required when GH_ACCEPTANCE_API_HOST is set")
	}

	return nil
}
