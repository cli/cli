package identitye2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfflineRepository(t *testing.T) {
	// Given two identities and a repository with an unrelated nested repository
	f := newOfflineFixture(t, requireOfflineBinary(t))
	f.create("work", fixtureAlice, "github-work", "alice@work.invalid")
	f.create("public", fixtureAlice, "github-public", "alice@public.invalid")
	repo := f.repo("repository", "git@"+fixtureHost+":company/project.git")
	child := f.mkdir("repository/subdirectory")

	// When applying from a subdirectory and using independent Git
	f.gh(child, "auth", "use", "work")

	// Then attribution and both transports agree without rewriting the remote
	assert.Equal(t, "work", f.selectedName(child))
	assert.Equal(t, "alice@work.invalid", f.gitValue(repo, "user.email"))
	f.git(repo, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "Fixture commit")
	assert.Equal(t, "Alice Work <alice@work.invalid> | Alice Work <alice@work.invalid>",
		strings.TrimSpace(f.git(repo, "log", "-1", "--format=%an <%ae> | %cn <%ce>")))
	assert.Contains(t, f.credential(repo, "company", "", ""), aliceToken)
	f.git(repo, "ls-remote", "origin")
	assert.Contains(t, f.lastEvent().Args, "git@github-work")
	assert.Equal(t, "git@"+fixtureHost+":company/project.git", f.gitValue(repo, "remote.origin.url"))
	nested := f.repo("repository/nested", "git@"+fixtureHost+":elsewhere/project.git")
	assert.Nil(t, f.selected(nested))
	f.gh(repo, "auth", "use", "public")
	assert.Equal(t, "alice@public.invalid", f.gitValue(repo, "user.email"))
	assert.Equal(t, "ordinary@example.invalid", f.gitValue(nested, "user.email"))
}

func TestOfflineScopes(t *testing.T) {
	// Given owner, directory and repository choices
	f := newOfflineFixture(t, requireOfflineBinary(t))
	for _, name := range []string{"owner-default", "tree-default", "repository-override"} {
		f.create(name, fixtureAlice, "ssh-"+name, name+"@example.invalid")
	}
	f.gh("", "auth", "use", "owner-default", "--owner", "company")
	tree := f.mkdir("workspace")
	f.gh(tree, "auth", "use", "tree-default")
	var choices []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(f.gh("", "auth", "use", "--list")), &choices))
	assert.Len(t, choices, 2)
	for _, choice := range choices {
		assert.NotContains(t, choice, "recursive")
		assert.NotContains(t, choice, "gitDir")
	}
	assert.Equal(t, "tree-default", f.selectedName(tree))
	repo := f.repo("workspace/project", "https://"+fixtureHost+"/company/project.git")
	assert.Equal(t, "tree-default", f.selectedName(repo))
	assert.Equal(t, "tree-default@example.invalid", f.gitValue(repo, "user.email"))

	// When overriding the repository and removing choices in specificity order
	f.gh(repo, "auth", "use", "repository-override")

	// Then each removal reveals the next applicable choice
	assert.Equal(t, "repository-override", f.selectedName(repo))
	f.git(repo, "ls-remote", "git@"+fixtureHost+":company/project.git")
	assert.Contains(t, f.lastEvent().Args, "git@ssh-repository-override")
	f.gh(repo, "auth", "use", "--unset", "--hostname", fixtureHost)
	assert.Equal(t, "tree-default", f.selectedName(repo))
	f.gh(tree, "auth", "use", "--unset", "--hostname", fixtureHost)
	assert.Equal(t, "owner-default", f.selectedName(repo))
	assert.Equal(t, "owner-default@example.invalid", f.gitValue(repo, "user.email"))
}

func TestOfflineOwners(t *testing.T) {
	// Given one owner choice and the supported URL forms
	f := newOfflineFixture(t, requireOfflineBinary(t))
	f.create("work", fixtureAlice, "github-work", "alice@work.invalid")
	f.gh("", "auth", "use", "work", "--owner", "company")
	urls := map[string]string{
		"https":           "https://" + fixtureHost + "/company/project.git",
		"ssh":             "git@" + fixtureHost + ":company/project.git",
		"ssh-url":         "ssh://git@" + fixtureHost + "/company/project.git",
		"ssh-certificate": "org-1234@" + fixtureHost + ":company/project.git",
	}
	for name, url := range urls {
		// When Git accesses a repository using that URL
		repo := f.repo(name, url)
		f.git(repo, "ls-remote", "origin")

		// Then account and attribution selection agree for every URL form
		assert.Equal(t, "work", f.selectedName(repo))
		assert.Equal(t, "alice@work.invalid", f.gitValue(repo, "user.email"))
	}
	var httpsEvents []transportEvent
	for _, event := range f.events() {
		if event.Transport == "https" {
			httpsEvents = append(httpsEvents, event)
		}
	}
	require.Len(t, httpsEvents, 1)
	assert.Equal(t, fixtureAlice, httpsEvents[0].Username)
	assert.Equal(t, aliceToken, httpsEvents[0].Token)
	assert.Nil(t, f.selected(f.root))
}

func TestOfflineLiveEdits(t *testing.T) {
	// Given two repositories sharing an identity
	f := newOfflineFixture(t, requireOfflineBinary(t))
	f.create("work", fixtureAlice, "github-work", "alice@work.invalid")
	repos := []string{f.repo("first", "git@"+fixtureHost+":company/project.git"), f.repo("second", "git@"+fixtureHost+":company/project.git")}
	for _, repo := range repos {
		f.gh(repo, "auth", "use", "work")
	}

	// When changing the shared account, SSH alias and attribution
	f.gh("", "auth", "identity", "edit", "work", "--account", fixtureBob,
		"--ssh-host", "github-new-work", "--git-user-name", "Bob Work", "--git-user-email", "new@work.invalid")

	// Then both repositories immediately use the edit and recover after confirmed deletion
	for _, repo := range repos {
		id := f.selected(repo)
		require.NotNil(t, id)
		assert.Equal(t, fixtureBob, id.Account)
		assert.Equal(t, "Bob Work", f.gitValue(repo, "user.name"))
		assert.Equal(t, "new@work.invalid", f.gitValue(repo, "user.email"))
		assert.Contains(t, f.credential(repo, "company", "", ""), bobToken)
		f.git(repo, "ls-remote", "origin")
		assert.Contains(t, f.lastEvent().Args, "git@github-new-work")
	}
	f.ghFails("", "--confirm required", "auth", "identity", "delete", "work")
	f.gh("", "auth", "identity", "delete", "work", "--confirm")
	for _, repo := range repos {
		assert.Nil(t, f.selected(repo))
		assert.Equal(t, "ordinary@example.invalid", f.gitValue(repo, "user.email"))
	}
	assert.JSONEq(t, "[]", f.gh("", "auth", "identity", "list"))
}

func TestOfflineAccountOnly(t *testing.T) {
	// Given an attributed identity and an account-only alternative
	f := newOfflineFixture(t, requireOfflineBinary(t))
	f.create("work", fixtureAlice, "github-work", "alice@work.invalid")
	f.create("account-only", fixtureBob, "github-personal", "")
	repo := f.repo("project", "https://"+fixtureHost+"/company/project.git")
	f.gh(repo, "auth", "use", "work")

	// When replacing the choice with an account-only identity
	f.gh(repo, "auth", "use", "account-only")

	// Then ordinary attribution is restored while authentication changes
	assert.Equal(t, "ordinary@example.invalid", f.gitValue(repo, "user.email"))
	assert.Contains(t, f.credential(repo, "company", "", ""), bobToken)
}

func TestOfflineNestedTrees(t *testing.T) {
	// Given nested directory choices installed in reverse order
	f := newOfflineFixture(t, requireOfflineBinary(t))
	f.create("parent", fixtureAlice, "github-parent", "alice@work.invalid")
	f.create("child", fixtureBob, "github-child", "child@example.invalid")
	tree := f.mkdir("tree")
	child := f.mkdir("tree/child")
	f.gh(child, "auth", "use", "child")
	f.gh(tree, "auth", "use", "parent")

	// When a repository is created under the child
	repo := f.repo("tree/child/project", "git@"+fixtureHost+":company/project.git")

	// Then the child wins and explanations distinguish current and descendant choices
	assert.Equal(t, "child", f.selectedName(repo))
	assert.Equal(t, "child@example.invalid", f.gitValue(repo, "user.email"))
	assert.Contains(t, f.credential(repo, "company", "", ""), bobToken)
	f.git(repo, "ls-remote", "origin")
	assert.Contains(t, f.lastEvent().Args, "git@github-child")
	explanation := f.gh(repo, "auth", "use", "--explain", "--hostname", fixtureHost)
	assert.Contains(t, explanation, `Using identity "child"`)
	assert.Contains(t, explanation, "Overrides directory "+offlineCanonical(t, tree)+" -> parent: more specific directory")
	parentExplanation := f.gh(tree, "auth", "use", "--explain", "--hostname", fixtureHost)
	assert.Contains(t, parentExplanation, "Descendant overrides (do not affect this directory)")
	assert.Contains(t, parentExplanation, "directory "+offlineCanonical(t, child)+" -> child")
	f.gh(child, "auth", "use", "--unset", "--hostname", fixtureHost)
	assert.Equal(t, "parent", f.selectedName(repo))
	assert.Equal(t, "alice@work.invalid", f.gitValue(repo, "user.email"))
}

func TestOfflineAmbiguousOwners(t *testing.T) {
	// Given two remotes with competing owner choices
	f := newOfflineFixture(t, requireOfflineBinary(t))
	f.create("work", fixtureAlice, "github-work", "alice@work.invalid")
	f.create("personal", fixtureBob, "github-personal", "personal@example.invalid")
	f.gh("", "auth", "use", "work", "--owner", "company")
	f.gh("", "auth", "use", "personal", "--owner", "personal")
	repo := f.repo("project", "git@"+fixtureHost+":company/project.git")
	f.git(repo, "remote", "add", "fork", "git@"+fixtureHost+":personal/project.git")
	f.ghFails(repo, "multiple owners", "auth", "use", "--hostname", fixtureHost)

	// When a repository choice resolves the ambiguity
	f.gh(repo, "auth", "use", "work")

	// Then even the other owner's remote uses the explicit repository account
	assert.Equal(t, "work", f.selectedName(repo))
	assert.Contains(t, f.credential(repo, "personal", "", ""), aliceToken)
	assert.Equal(t, "alice@work.invalid", f.gitValue(repo, "user.email"))
}

func TestOfflineFailures(t *testing.T) {
	// Given saved accounts and a valid identity
	f := newOfflineFixture(t, requireOfflineBinary(t))
	f.create("work", fixtureAlice, "github-work", "alice@work.invalid")

	// When incomplete or conflicting configuration is supplied
	// Then commands fail with actionable errors rather than silently falling back
	f.ghFails("", "supply both --git-user-name and --git-user-email", "auth", "identity", "create",
		"missing-email", "--hostname", fixtureHost, "--account", fixtureAlice, "--git-user-name", "Alice")
	f.ghFails("", "--clear-git cannot be combined with --git-user-name or --git-user-email",
		"auth", "identity", "edit", "work", "--clear-git", "--git-user-email", "alice@work.invalid")
	f.gh("", "auth", "use", "work")
	assert.Equal(t, "work", f.selectedName(f.root))
	f.gh("", "auth", "use", "--unset", "--hostname", fixtureHost)
	f.ghFails("", "is not saved", "auth", "identity", "create", "unknown", "--hostname", fixtureHost, "--account", "not-saved")
	f.create("https-only", fixtureAlice, "", "")
	repo := f.repo("project", "git@"+fixtureHost+":company/project.git")
	f.ghFails(repo, "has no SSH route", "auth", "use", "https-only")
	f.gh("", "auth", "identity", "edit", "https-only", "--ssh-host", "github-temporary")
	f.gh(repo, "auth", "use", "https-only")
	f.gh("", "auth", "identity", "edit", "https-only", "--ssh-host", "")
	f.gitFails(repo, "has no SSH route", "ls-remote", "origin")
	f.gh(repo, "auth", "use", "work")
	f.credential(repo, "company", fixtureBob, "conflicts with identity")
	f.using(map[string]string{"GH_ENTERPRISE_TOKEN": "fixture-token-unknown-account"}).
		ghFails(repo, "overrides saved accounts", "auth", "use", "work")
	f.ghFails("", "cannot be combined", "auth", "use", "work", "--owner", "company", "--directory", f.root)
	f.ghFails("", "identity name", "auth", "identity", "create", "../escape", "--hostname", fixtureHost, "--account", fixtureAlice)
	conflict := f.repo("ssh-conflict", "")
	f.git(conflict, "config", "core.sshCommand", "ssh -i another-key")
	f.ghFails(conflict, "core.sshCommand", "auth", "use", "work")
}

func TestOfflineGitClone(t *testing.T) {
	// Given an owner choice and SSH/HTTPS clone URLs
	f := newOfflineFixture(t, requireOfflineBinary(t))
	f.create("work", fixtureAlice, "github-work", "alice@work.invalid")
	f.gh("", "auth", "use", "work", "--owner", "company")
	for protocol, url := range map[string]string{"ssh": "git@" + fixtureHost + ":company/project.git", "https": "https://" + fixtureHost + "/company/project.git"} {
		// When independent Git clones into a new repository
		repo := filepath.Join(f.root, "clone-"+protocol)
		f.git("", "clone", url, repo)

		// Then the owner choice applies without rewriting the URL
		assert.Equal(t, "work", f.selectedName(repo))
		assert.Equal(t, url, f.gitValue(repo, "remote.origin.url"))
	}
	f.create("tree", fixtureAlice, "github-tree", "tree@example.invalid")
	tree := f.mkdir("tree")
	f.gh(tree, "auth", "use", "tree")
	destination := filepath.Join(tree, "destination")
	f.git("", "clone", "git@"+fixtureHost+":company/project.git", destination)
	assert.Contains(t, f.lastEvent().Args, "git@github-work")
	assert.Equal(t, "tree", f.selectedName(destination))
	f.git(destination, "ls-remote", "origin")
	assert.Contains(t, f.lastEvent().Args, "git@github-tree")
}

func TestOfflineInheritedMetadata(t *testing.T) {
	// Given a broader attribution choice and an account-only identity
	f := newOfflineFixture(t, requireOfflineBinary(t))
	f.create("parent", fixtureAlice, "github-work", "alice@work.invalid")
	f.create("account-only", fixtureBob, "github-personal", "")
	tree := f.mkdir("tree")
	f.gh(tree, "auth", "use", "parent")
	repo := f.repo("tree/project", "https://"+fixtureHost+"/company/project.git")

	// When the repository selects only a different account
	f.gh(repo, "auth", "use", "account-only")

	// Then ordinary Git precedence retains the broader attribution
	assert.Equal(t, "account-only", f.selectedName(repo))
	assert.Equal(t, "alice@work.invalid", f.gitValue(repo, "user.email"))
}

func TestOfflineAccountOverride(t *testing.T) {
	// Given an unbound repository, then a persistent identity for Bob
	f := newOfflineFixture(t, requireOfflineBinary(t))
	repo := f.repo("override-https", "https://"+fixtureHost+"/company/project.git")
	syncArgs := []string{"repo", "sync", "--source", fixtureHost + "/company/project", "--branch", "main", "--account", fixtureAlice}
	f.ghFails(repo, "couldn't find remote ref", syncArgs...)
	assert.Equal(t, fixtureAlice, f.lastEvent().Username)
	assert.Equal(t, aliceToken, f.lastEvent().Token)
	assert.JSONEq(t, "[]", f.gh("", "auth", "identity", "list"))
	f.create("personal", fixtureBob, "bob-ssh", "bob@example.invalid")
	f.create("work", fixtureAlice, "alice-ssh", "alice@work.invalid")
	f.gh(repo, "auth", "use", "personal")
	f.git(repo, "remote", "set-url", "origin", "git@"+fixtureHost+":company/project.git")
	f.git(repo, "ls-remote", "origin")
	assert.Contains(t, f.lastEvent().Args, "git@bob-ssh")
	hostsPath := filepath.Join(f.home, "gh", "hosts.yml")
	before := offlineRead(t, hostsPath)

	// When gh overrides the account for its own Git operation
	f.ghFails(repo, "couldn't find remote ref", syncArgs...)

	// Then only that invocation uses Alice; persistent choices and credentials stay unchanged
	assert.Contains(t, f.lastEvent().Args, "git@alice-ssh")
	f.git(repo, "ls-remote", "origin")
	assert.Contains(t, f.lastEvent().Args, "git@bob-ssh")
	assert.Equal(t, "personal", f.selectedName(repo))
	assert.Equal(t, "bob@example.invalid", f.gitValue(repo, "user.email"))
	assert.Equal(t, "git@"+fixtureHost+":company/project.git", f.gitValue(repo, "remote.origin.url"))
	assert.Equal(t, before, offlineRead(t, hostsPath))

	f.create("alternate", fixtureAlice, "alice-ca", "alice@work.invalid")
	eventsBefore := len(f.events())
	f.ghFails(repo, "multiple SSH routes", syncArgs...)
	for _, event := range f.events()[eventsBefore:] {
		assert.Contains(t, event.Args, "-G", "ambiguous routes must fail before connecting; local ssh -G is allowed")
	}
	f.gh("", "auth", "use", "alternate", "--owner", "company")
	sshConfig := filepath.Join(f.root, "isolated-ssh-config")
	configured := f.using(map[string]string{"GIT_SSH_COMMAND": shellquote.Join(filepath.Join(f.tools, "ssh"), "-F", sshConfig)})
	configured.ghFails(repo, "couldn't find remote ref", syncArgs...)
	assert.Contains(t, f.lastEvent().Args, "git@alice-ca")
	require.GreaterOrEqual(t, len(f.lastEvent().Args), 2)
	assert.Equal(t, []string{"-F", sshConfig}, f.lastEvent().Args[:2])
	assert.Equal(t, "personal", f.selectedName(repo))
	f.using(map[string]string{"GIT_SSH_COMMAND": "ssh -F $IDENTITY_SSH_CONFIG"}).
		ghFails(repo, "SSH shell expressions", syncArgs...)

	f.transport("enterprise-ssh", "ssh")
	configured = f.using(map[string]string{
		"GIT_SSH_COMMAND": shellquote.Join(filepath.Join(f.tools, "enterprise-ssh"), "-F", "company config"),
		"GIT_SSH_VARIANT": "ssh",
	})
	configured.ghFails(repo, "couldn't find remote ref", syncArgs...)
	require.GreaterOrEqual(t, len(f.lastEvent().Args), 2)
	assert.Equal(t, []string{"-F", "company config"}, f.lastEvent().Args[:2])
	assert.Contains(t, f.lastEvent().Args, "git@alice-ca")

	f.transport("custom & tools/SSH.EXE", "ssh")
	f.using(map[string]string{"GIT_SSH": filepath.Join(f.tools, "custom & tools", "SSH.EXE")}).
		git(repo, "ls-remote", "origin")
	assert.Contains(t, f.lastEvent().Args, "git@bob-ssh")

	localCommand := shellquote.Join(filepath.Join(f.tools, "enterprise-ssh"), "-F", "repository config")
	f.git(repo, "config", "core.sshCommand", localCommand)
	f.git(repo, "config", "ssh.variant", "ssh")
	f.ghFails(repo, "couldn't find remote ref", syncArgs...)
	require.GreaterOrEqual(t, len(f.lastEvent().Args), 2)
	assert.Equal(t, []string{"-F", "repository config"}, f.lastEvent().Args[:2])
	assert.Contains(t, f.lastEvent().Args, "git@alice-ca")
	assert.Equal(t, localCommand, f.gitValue(repo, "core.sshCommand"))
	configured.using(map[string]string{"GIT_SSH_VARIANT": "plink"}).
		ghFails(repo, "does not support SSH variant", syncArgs...)
}

func TestOfflineCleanup(t *testing.T) {
	// Given two independent repository choices
	f := newOfflineFixture(t, requireOfflineBinary(t))
	first := f.repo("first", "git@"+fixtureHost+":company/project.git")
	second := f.repo("second", "git@"+fixtureHost+":personal/project.git")
	f.create("work", fixtureAlice, "github-work", "alice@work.invalid")
	f.create("personal", fixtureBob, "personal-ssh", "alice@work.invalid")
	f.gh(first, "auth", "use", "work")
	f.gh(second, "auth", "use", "personal")

	// When deleting one identity, then explicitly clearing the experiment
	f.gh("", "auth", "identity", "delete", "work", "--confirm")

	// Then named deletion preserves the other choice, and full cleanup is repeatable
	assert.Nil(t, f.selected(first))
	assert.Equal(t, "personal", f.selectedName(second))
	f.ghFails("", "--confirm required", "auth", "identity", "delete", "--all")
	f.gh("", "auth", "identity", "delete", "--all", "--confirm")
	assert.JSONEq(t, "[]", f.gh("", "auth", "identity", "list"))
	assert.JSONEq(t, "[]", f.gh("", "auth", "use", "--list"))
	assert.Nil(t, f.selected(second))
	assert.Equal(t, "ordinary@example.invalid", f.gitValue(first, "user.email"))
	f.git(second, "ls-remote", "origin")
	assert.Contains(t, f.lastEvent().Args, "git@"+fixtureHost)
	f.gh("", "auth", "identity", "delete", "--all", "--confirm")
}

func TestOfflineRepositoryFamily(t *testing.T) {
	// Given directory choices and an external worktree of the main repository
	f := newOfflineFixture(t, requireOfflineBinary(t))
	f.create("tree", fixtureAlice, "tree-ssh", "tree@example.invalid")
	f.create("elsewhere", fixtureBob, "elsewhere-ssh", "elsewhere@example.invalid")
	f.create("family", fixtureBob, "family-ssh", "family@example.invalid")
	tree, agents := f.mkdir("work"), f.mkdir("agents")
	f.gh(tree, "auth", "use", "tree")
	f.gh(agents, "auth", "use", "elsewhere")
	main := f.repo("work/project", "git@"+fixtureHost+":company/project.git")
	f.git(main, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "Initial family fixture")
	linked := filepath.Join(agents, "linked")
	f.git(main, "worktree", "add", "--detach", linked)
	configPath := filepath.Join(main, ".git", "config")
	originalConfig := offlineRead(t, configPath)
	for _, repo := range []string{main, linked} {
		assert.Equal(t, "tree", f.selectedName(repo))
		assert.Equal(t, "tree@example.invalid", f.gitValue(repo, "user.email"))
		assert.Contains(t, f.credential(repo, "company", "", ""), aliceToken)
		f.git(repo, "ls-remote", "origin")
		assert.Contains(t, f.lastEvent().Args, "git@tree-ssh")
	}

	// When applying a family choice from the external worktree
	f.gh(linked, "auth", "use", "family")
	future := filepath.Join(f.root, "future")
	f.git(main, "worktree", "add", "--detach", future)

	// Then main, existing and future worktrees share it without per-worktree configuration
	for _, repo := range []string{main, linked, future} {
		assert.Equal(t, "family", f.selectedName(repo))
		assert.Equal(t, "family@example.invalid", f.gitValue(repo, "user.email"))
		assert.Contains(t, f.credential(repo, "company", "", ""), bobToken)
		f.git(repo, "ls-remote", "origin")
		assert.Contains(t, f.lastEvent().Args, "git@family-ssh")
	}
	f.git(future, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "Worktree identity")
	assert.Equal(t, "family@example.invalid|family@example.invalid", strings.TrimSpace(f.git(future, "log", "-1", "--format=%ae|%ce")))
	nested := f.repo("work/project/nested", "")
	assert.Equal(t, "tree", f.selectedName(nested))
	var report struct {
		Selection struct {
			Selected struct{ Repository string }
		}
		Context  struct{ ScopeDirectory string }
		Warnings []json.RawMessage
	}
	require.NoError(t, json.Unmarshal([]byte(f.gh(linked, "auth", "use", "--explain", "--hostname", fixtureHost,
		"--json", "context,selection,attribution,warnings")), &report))
	assert.Equal(t, offlineCanonical(t, filepath.Join(main, ".git")), report.Selection.Selected.Repository)
	assert.Equal(t, offlineCanonical(t, main), report.Context.ScopeDirectory)
	assert.Empty(t, report.Warnings)
	f.gh(future, "auth", "use", "elsewhere")
	assert.Equal(t, "elsewhere", f.selectedName(main))
	f.gh(linked, "auth", "use", "--unset", "--hostname", fixtureHost)
	assert.Equal(t, "tree", f.selectedName(future))
	f.gh(future, "auth", "use", "family")
	f.gh("", "auth", "identity", "delete", "family", "--confirm")
	assert.Equal(t, "tree", f.selectedName(linked))
	assert.Equal(t, originalConfig, offlineRead(t, configPath))
	f.gh("", "auth", "identity", "delete", "--all", "--confirm")
	for _, repo := range []string{main, linked, future} {
		assert.Nil(t, f.selected(repo))
		assert.Equal(t, "ordinary@example.invalid", f.gitValue(repo, "user.email"))
	}
}

func offlineCanonical(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return resolved
}
