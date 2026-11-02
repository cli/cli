package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHAccountOverrideUsesUniqueRouteWithoutChangingIdentity(t *testing.T) {
	// Given an alice directory identity and two bob profiles sharing one SSH alias
	s := isolatedStore(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	for _, id := range []Identity{
		{Name: "alice", Host: "github.com", Account: "alice", SSHHost: "alice-ssh"},
		{Name: "bob-work", Host: "github.com", Account: "bob", SSHHost: "bob-ssh"},
		{Name: "bob-personal", Host: "github.com", Account: "bob", SSHHost: "bob-ssh"},
	} {
		require.NoError(t, s.Save(id, false))
	}

	require.NoError(t, s.Bind(Binding{Identity: "alice", Directory: dir}, "/path/to/gh"))

	// When gh launches Git with an authentication-only override
	got, err := s.SSHArgumentsForAccount([]string{"-p", "22", "cert-user@github.com", "git-receive-pack 'company/project.git'"}, dir, "github.com", "bob")

	// Then the declared bob route is used while the SSH user and persistent identity stay unchanged
	require.NoError(t, err)
	assert.Equal(t, []string{"-p", "22", "cert-user@bob-ssh", "git-receive-pack 'company/project.git'"}, got)
	selected, err := s.Resolve("github.com", "company", dir)
	require.NoError(t, err)
	assert.Equal(t, "alice", selected.Account)
}

func TestSSHAccountOverrideRejectsConflictingAliasDeclarations(t *testing.T) {
	// Given the same alias has been declared for different accounts
	s := isolatedStore(t)
	for _, id := range []Identity{
		{Name: "alice", Host: "github.com", Account: "alice", SSHHost: "shared-alias"},
		{Name: "bob", Host: "github.com", Account: "bob", SSHHost: "shared-alias"},
	} {
		require.NoError(t, s.Save(id, false))
	}

	// When bob is explicitly requested
	_, err := s.SSHArgumentsForAccount([]string{"git@github.com", "git-upload-pack 'company/project.git'"}, t.TempDir(), "github.com", "bob")

	// Then a contradictory declaration cannot masquerade as an unambiguous route
	require.ErrorContains(t, err, "declared for multiple accounts")
}

func TestSSHAccountOverrideRecognizesAnExistingDeclaredAlias(t *testing.T) {
	// Given a remote or Git URL rewrite already names alice's declared alias
	s := isolatedStore(t)
	require.NoError(t, s.Save(Identity{Name: "alice", Host: "github.com", Account: "alice", SSHHost: "alice-ssh"}, false))
	require.NoError(t, s.Save(Identity{Name: "bob", Host: "github.com", Account: "bob", SSHHost: "bob-ssh"}, false))

	// When this invocation explicitly chooses bob on the same logical host
	got, err := s.SSHArgumentsForAccount([]string{"git@alice-ssh", "git-upload-pack 'company/project.git'"}, t.TempDir(), "github.com", "bob")

	// Then the actual SSH destination is replaced without changing the repository target
	require.NoError(t, err)
	assert.Equal(t, []string{"git@bob-ssh", "git-upload-pack 'company/project.git'"}, got)
}

func TestSSHAccountOverrideFailsWithoutAnUnambiguousRoute(t *testing.T) {
	for _, tt := range []struct {
		name   string
		routes []Identity
		want   string
	}{
		{name: "missing", want: "no declared SSH route"},
		{name: "ambiguous", routes: []Identity{
			{Name: "personal", Host: "github.com", Account: "bob", SSHHost: "personal-ssh"},
			{Name: "company", Host: "github.com", Account: "bob", SSHHost: "company-ca"},
		}, want: "multiple SSH routes"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Given no contextual choice can supply one SSH route for bob
			s := isolatedStore(t)
			for _, id := range tt.routes {
				require.NoError(t, s.Save(id, false))
			}

			// When Git authentication requests bob
			_, err := s.SSHArgumentsForAccount([]string{"git@github.com", "git-upload-pack 'company/project.git'"}, t.TempDir(), "github.com", "bob")

			// Then gh fails rather than falling back to ordinary SSH authentication
			require.ErrorContains(t, err, tt.want)
		})
	}
}
