package identitye2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetDowngrade(t *testing.T) {
	prototype := requireOfflineBinary(t)
	if *oldBinary == "" {
		t.Skip("downgrade journey also requires -old-gh /absolute/path/to/stable/gh")
	}
	old := offlineBinaryPath(t, *oldBinary)

	// Given a stable installation and ordinary HTTPS, SSH, alias and attribution settings
	f := newOfflineFixture(t, prototype)
	installed := filepath.Join(f.root, "installed-gh")
	offlineCopy(t, old, installed)
	f.binary = installed
	f.executable("gh", "exec "+shellquote.Join(installed)+" \"$@\"\n")
	oldVersion := f.gh("", "version")
	repo := f.repo("project", "git@"+fixtureHost+":company/project.git")
	helper := "!" + shellquote.Join(installed, "auth", "git-credential")
	f.git("", "config", "--global", "credential.https://"+fixtureHost+".helper", helper)
	f.git("", "config", "--global", "core.sshCommand", "ssh -F fixture-config")
	f.git("", "config", "--global", "ssh.variant", "ssh")
	f.gh("", "alias", "set", "fixture-version", "version")
	hostsPath, configPath := filepath.Join(f.home, "gh", "hosts.yml"), filepath.Join(f.home, "gh", "config.yml")
	accountsBefore, configBefore := offlineRead(t, hostsPath), offlineRead(t, configPath)
	gitBefore := f.git("", "config", "--global", "--list")
	credentialBefore := f.credential(repo, "company", "", "")
	require.Contains(t, credentialBefore, "username="+fixtureBob+"\n")
	require.Contains(t, credentialBefore, bobToken)
	f.git(repo, "ls-remote", "https://"+fixtureHost+"/company/project.git")
	assert.Equal(t, fixtureBob, f.lastEvent().Username)
	f.git(repo, "ls-remote", "origin")
	assert.Contains(t, f.lastEvent().Args, "git@"+fixtureHost)
	assert.Equal(t, oldVersion, f.gh("", "fixture-version"))

	// When upgrading and opting in at the same executable path
	offlineCopy(t, prototype, installed)
	f.create("experiment", fixtureAlice, "fixture-alice", "alice@example.invalid")
	f.gh(repo, "auth", "use", "experiment")

	// Then reset is required before downgrading, and restores the complete original behavior
	assert.Contains(t, f.credential(repo, "company", "", ""), "username="+fixtureAlice+"\n")
	f.git(repo, "ls-remote", "https://"+fixtureHost+"/company/project.git")
	assert.Equal(t, fixtureAlice, f.lastEvent().Username)
	assert.Equal(t, "alice@example.invalid", f.gitValue(repo, "user.email"))
	f.git(repo, "ls-remote", "origin")
	assert.Contains(t, f.lastEvent().Args, "git@fixture-alice")
	offlineCopy(t, old, installed)
	f.gitFails(repo, "Could not read from remote repository", "ls-remote", "origin")
	offlineCopy(t, prototype, installed)
	f.ghFails("", "--confirm required", "auth", "reset-multi-account-experiment")
	assert.Equal(t, "experiment", f.selectedName(repo))
	f.gh("", "auth", "reset-multi-account-experiment", "--confirm")
	f.gh("", "auth", "reset-multi-account-experiment", "--confirm")
	assert.NoFileExists(t, filepath.Join(f.home, "gh", "auth.gitconfig"))
	assert.NoDirExists(t, filepath.Join(f.home, "gh", "identities"))
	offlineCopy(t, old, installed)
	assert.Equal(t, oldVersion, f.gh("", "version"))
	assert.Equal(t, oldVersion, f.gh("", "fixture-version"))
	assert.Equal(t, credentialBefore, f.credential(repo, "company", "", ""))
	f.git(repo, "ls-remote", "https://"+fixtureHost+"/company/project.git")
	assert.Equal(t, fixtureBob, f.lastEvent().Username)
	assert.Equal(t, accountsBefore, offlineRead(t, hostsPath))
	assert.Equal(t, configBefore, offlineRead(t, configPath))
	assert.Equal(t, gitBefore, f.git("", "config", "--global", "--list"))
	assert.Equal(t, "ordinary@example.invalid", f.gitValue(repo, "user.email"))
	f.git(repo, "ls-remote", "origin")
	require.GreaterOrEqual(t, len(f.lastEvent().Args), 2)
	assert.Equal(t, []string{"-F", "fixture-config"}, f.lastEvent().Args[:2])
	assert.Contains(t, f.lastEvent().Args, "git@"+fixtureHost)
	f.git(repo, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "After reset")
	assert.Equal(t, "ordinary@example.invalid|ordinary@example.invalid",
		strings.TrimSpace(f.git(repo, "log", "-1", "--format=%ae|%ce")))
}
