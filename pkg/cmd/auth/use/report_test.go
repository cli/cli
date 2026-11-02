package use

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExplainJSONSeparatesDefaultsFromObservedConfiguration(t *testing.T) {
	// Given a directory identity outside any repository and a token environment conflict
	s, f := useFixture(t)
	t.Chdir(t.TempDir())
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice",
		SSHHost: "work-ssh", GitUserName: "Alice", GitUserEmail: "alice@example.test"}, false))
	runUse(t, f, "work")
	t.Setenv("GH_TOKEN", "never-print-this")

	// When exporting the explanation through the public command
	out := runUse(t, f, "--explain", "--json", "context,selection,authentication,attribution,configuration,warnings")
	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &report))

	// Then declared defaults, authentication blockers, and observed include edges are explicit
	assert.Equal(t, "persistent", report["context"].(map[string]any)["scope"])
	assert.Equal(t, "declared", report["attribution"].(map[string]any)["status"])
	assert.Equal(t, "blocked", report["authentication"].(map[string]any)["api"].(map[string]any)["status"])
	assert.Equal(t, "blocked", report["authentication"].(map[string]any)["ssh"].(map[string]any)["status"])
	assert.NotEmpty(t, report["configuration"])
	assert.NotContains(t, out, "never-print-this")
	assert.Equal(t, "work\n", runUse(t, f, "--explain", "--json", "selection", "--jq", ".selection.identity.name"))
}

func TestExplainStylesTTYButNotPipedOutput(t *testing.T) {
	// Given a selected identity in a real isolated repository
	s, f := useFixture(t)
	t.Chdir(t.TempDir())
	out, err := exec.Command("git", "init", "-q").CombinedOutput()
	require.NoError(t, err, "%s", out)
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	runUse(t, f, "work")
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetColorEnabled(true)
	f.IOStreams = ios
	cmd := NewCmdUse(f, nil)
	cmd.SetArgs([]string{"--explain"})

	// When rendering the same report to a coloured terminal and a pipe
	require.NoError(t, cmd.Execute(), "%s", stderr.String())
	plain := runUse(t, f, "--explain")

	// Then colour is supplemental and both outputs identify their scope and provenance
	assert.Contains(t, stdout.String(), "\x1b[")
	assert.NotContains(t, plain, "\x1b[")
	assert.Contains(t, plain, `Using identity "work"`)
	assert.Contains(t, plain, "Configuration sources")
	assert.Contains(t, plain, os.Getenv("GIT_CONFIG_GLOBAL"))
}

func TestExplainGraphObservesManualIncludesAndMissingManagedInclude(t *testing.T) {
	// Given a selected identity, a manual relative include, and a removed managed global include
	s, f := useFixture(t)
	repo, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(repo)
	for _, args := range [][]string{{"init", "-q"}, {"config", "--local", "include.path", "../extra.gitconfig"}} {
		out, err := exec.Command("git", args...).CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	extra := filepath.Join(repo, "extra.gitconfig")
	require.NoError(t, os.WriteFile(extra, []byte("[user]\nname = Manual\nemail = manual@example.test\n"), 0o600))
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	runUse(t, f, "work")
	out, err := exec.Command("git", "config", "--global", "--unset-all", "include.path").CombinedOutput()
	require.NoError(t, err, "%s", out)
	require.NoError(t, os.Mkdir(filepath.Join(repo, "src"), 0o700))
	t.Chdir(filepath.Join(repo, "src"))

	// When inspecting from a subdirectory after configuration drift
	output := runUse(t, f, "--explain", "--json", "configuration,selection,attribution,warnings")
	var report struct {
		Configuration []identity.Include
		Warnings      []string
	}
	require.NoError(t, json.Unmarshal([]byte(output), &report))

	// Then the graph reflects actual local include origins rather than reconstructing an installed binding
	require.Len(t, report.Configuration, 1)
	assert.Equal(t, filepath.Join(repo, ".git/config"), report.Configuration[0].Source)
	assert.Equal(t, extra, report.Configuration[0].Target)
	assert.True(t, report.Configuration[0].TargetObserved, "the manually included file was read by Git")
	assert.NotEmpty(t, report.Warnings)
	assert.Contains(t, output, "manual@example.test")
}

func TestExplainGroupsRepeatedIncludeTargetsWithoutLosingJSONConditions(t *testing.T) {
	// Given an owner binding represented by several native Git URL conditions
	s, f := useFixture(t)
	t.Chdir(t.TempDir())
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	runUse(t, f, "work", "--owner", "company")

	// When reading the graph as text and as structured data
	text := runUse(t, f, "--explain", "--owner", "company")
	data := runUse(t, f, "--explain", "--owner", "company", "--json", "configuration", "--jq",
		`[.configuration[] | select(.condition != null)] | length`)

	// Then text displays one target while JSON preserves every actual condition
	assert.Equal(t, 1, strings.Count(text, s.IdentityPath("work")))
	assert.Equal(t, "5\n", data)
}

func TestExplainNamesWhyDirectoryIncludeDoesNotApplyOutsideRepository(t *testing.T) {
	// Given a directory choice inspected outside a Git repository
	s, f := useFixture(t)
	t.Chdir(t.TempDir())
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	runUse(t, f, "work")

	// When explaining the configuration graph
	output := runUse(t, f, "--explain")

	// Then the inactive directory include has a concrete reason, not a file-loading label
	assert.Contains(t, output, "Not applied: this directory is not a Git repository.")
	assert.NotContains(t, output, "(no settings loaded here)")
}

func TestInspectionExplainsLoggedOutIdentityWithoutChangingSelection(t *testing.T) {
	// Given an identity whose referenced account is no longer saved
	s, f := useFixture(t)
	t.Chdir(t.TempDir())
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "work-ssh"}, false))
	runUse(t, f, "work")
	f.Config = func() (gh.Config, error) {
		return config.NewMockConfigFromString("hosts:\n  github.com:\n    users:\n      bob:\n"), nil
	}

	// When inspecting its explanation and current selection
	output := runUse(t, f, "--explain")
	jsonOutput := runUse(t, f, "--explain", "--json", "authentication,warnings")
	view := runUse(t, f, "--view")

	// Then the missing account is explained, while SSH and the persistent reference remain distinct
	assert.Contains(t, output, `account "alice" on github.com is not signed in`)
	assert.Contains(t, output, "gh auth login --hostname github.com")
	assert.Contains(t, output, "gh auth identity edit work --account")
	assert.Contains(t, output, "SSH can still work")
	assert.Contains(t, jsonOutput, `"status":"blocked"`)
	assert.Contains(t, jsonOutput, `"value":"work-ssh"`)
	assert.Contains(t, view, `"name": "work"`)
}
