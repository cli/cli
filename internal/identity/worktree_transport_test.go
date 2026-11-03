package identity_test

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIFollowsRepositoryChoiceFromExternalWorktree(t *testing.T) {
	// Given a repository-family choice and an external agent worktree
	cfg, s := isolatedAccounts(t)
	main := t.TempDir()
	runGit(t, main, "init", "-q")
	runGit(t, main, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test",
		"-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "initial")
	linked := filepath.Join(t.TempDir(), "agent")
	runGit(t, main, "worktree", "add", "--detach", linked)
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Bind(identity.Binding{Identity: "work", Directory: linked}, "/path/to/gh"))
	t.Chdir(linked)
	var auth string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		auth = req.Header.Get("Authorization")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"login":"alice"}`))}, nil
	})
	client := &http.Client{Transport: api.AddAuthTokenHeaderWithResolver(base, identity.TokenResolver(cfg.Authentication()))}

	// When gh makes a context-only API request from the linked worktree
	resp, err := client.Get("https://api.github.com/user")

	// Then the shared repository identity supplies authentication without owner inference
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "token alice-token", auth)
}
