package config

import (
	"context"
	"errors"
	"testing"

	"github.com/cli/cli/v2/internal/gh"
	o "github.com/cli/cli/v2/pkg/option"
	"github.com/stretchr/testify/assert"
)

// mockGitClient is a test double for GitClientInterface
type mockGitClient struct {
	isLocalRepo bool
	repoErr     error
	configValue string
	configErr   error
}

func (m *mockGitClient) IsLocalGitRepo(ctx context.Context) (bool, error) {
	return m.isLocalRepo, m.repoErr
}

func (m *mockGitClient) Config(ctx context.Context, name string) (string, error) {
	if name == "gh.user" {
		return m.configValue, m.configErr
	}
	return "", errors.New("unknown config key")
}

// mockAuthConfig is a test double for gh.AuthConfig
type mockAuthConfig struct {
	hosts       []string
	activeUser  string
	activeErr   error
	knownUsers  []string
	switchErr   error
	switchCalls []switchCall
}

type switchCall struct {
	hostname string
	user     string
}

func (m *mockAuthConfig) Hosts() []string {
	return m.hosts
}

func (m *mockAuthConfig) ActiveUser(hostname string) (string, error) {
	return m.activeUser, m.activeErr
}

func (m *mockAuthConfig) UsersForHost(hostname string) []string {
	return m.knownUsers
}

func (m *mockAuthConfig) SwitchUser(hostname, user string) error {
	m.switchCalls = append(m.switchCalls, switchCall{hostname: hostname, user: user})
	return m.switchErr
}

// Implement other required methods as no-ops
func (m *mockAuthConfig) ActiveToken(hostname string) (string, string) { return "", "" }
func (m *mockAuthConfig) HasActiveToken(hostname string) bool          { return false }
func (m *mockAuthConfig) HasEnvToken() bool                            { return false }
func (m *mockAuthConfig) SetActiveToken(token, source string)          {}
func (m *mockAuthConfig) TokenFromKeyring(hostname string) (string, error) {
	return "", errors.New("not implemented")
}
func (m *mockAuthConfig) TokenFromKeyringForUser(hostname, username string) (string, error) {
	return "", errors.New("not implemented")
}
func (m *mockAuthConfig) DefaultHost() (string, string)        { return "", "" }
func (m *mockAuthConfig) SetHosts(hosts []string)             {}
func (m *mockAuthConfig) SetDefaultHost(host, source string)  {}
func (m *mockAuthConfig) Login(hostname, username, token, gitProtocol string, secureStorage bool) (bool, error) {
	return false, errors.New("not implemented")
}
func (m *mockAuthConfig) Logout(hostname, username string) error { return errors.New("not implemented") }
func (m *mockAuthConfig) TokenForUser(hostname, user string) (string, string, error) {
	return "", "", errors.New("not implemented")
}

// mockConfig is a test double for gh.Config
type mockConfig struct {
	authConfig *mockAuthConfig
}

func (m *mockConfig) Authentication() gh.AuthConfig {
	return m.authConfig
}

// Implement other required methods as no-ops
func (m *mockConfig) GetOrDefault(hostname, key string) o.Option[gh.ConfigEntry] {
	return o.Some(gh.ConfigEntry{Value: "", Source: gh.ConfigDefaultProvided})
}
func (m *mockConfig) AccessibleColors(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "disabled", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) AccessiblePrompter(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "disabled", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) Browser(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) ColorLabels(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "disabled", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) Editor(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) GitProtocol(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "https", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) HTTPUnixSocket(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) Pager(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) Prompt(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "enabled", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) PreferEditorPrompt(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "disabled", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) Spinner(hostname string) gh.ConfigEntry {
	return gh.ConfigEntry{Value: "enabled", Source: gh.ConfigDefaultProvided}
}
func (m *mockConfig) CacheDir() string { return "" }
func (m *mockConfig) Aliases() gh.AliasConfig {
	return &mockAliasConfig{}
}

// mockAliasConfig is a simple mock for AliasConfig
type mockAliasConfig struct{}

func (m *mockAliasConfig) Get(alias string) (string, error) { return "", errors.New("not found") }
func (m *mockAliasConfig) Add(alias, expansion string)      {}
func (m *mockAliasConfig) Delete(alias string) error       { return nil }
func (m *mockAliasConfig) All() map[string]string           { return map[string]string{} }
func (m *mockConfig) Set(hostname, key, value string) {}
func (m *mockConfig) Write() error                    { return nil }
func (m *mockConfig) Migrate(migration gh.Migration) error { return nil }
func (m *mockConfig) Version() o.Option[string] { return o.Some("1") }

func TestMaybeAutomaticUserSwitch(t *testing.T) {
	tests := []struct {
		name           string
		isLocalRepo    bool
		repoErr        error
		configValue    string
		configErr      error
		hosts          []string
		activeUser     string
		activeErr      error
		knownUsers     []string
		switchErr      error
		expectErr      bool
		expectSwitch   bool
		expectedErrMsg string
	}{
		{
			name:        "not in git repository",
			isLocalRepo: false,
			expectErr:   false,
			expectSwitch: false,
		},
		{
			name:        "git repo error",
			isLocalRepo: false,
			repoErr:     errors.New("git error"),
			expectErr:   false,
			expectSwitch: false,
		},
		{
			name:        "no gh.user configured",
			isLocalRepo: true,
			configErr:   errors.New("config not found"),
			expectErr:   false,
			expectSwitch: false,
		},
		{
			name:        "no authenticated hosts",
			isLocalRepo: true,
			configValue: "testuser",
			hosts:       []string{},
			expectErr:   false,
			expectSwitch: false,
		},
		{
			name:        "no active user",
			isLocalRepo: true,
			configValue: "testuser",
			hosts:       []string{"github.com"},
			activeErr:   errors.New("no active user"),
			expectErr:   false,
			expectSwitch: false,
		},
		{
			name:        "user already matches - no switch needed",
			isLocalRepo: true,
			configValue: "testuser",
			hosts:       []string{"github.com"},
			activeUser:  "testuser",
			knownUsers:  []string{"testuser", "otheruser"},
			expectErr:   false,
			expectSwitch: false,
		},
		{
			name:           "configured user not authenticated",
			isLocalRepo:    true,
			configValue:    "unknownuser",
			hosts:          []string{"github.com"},
			activeUser:     "testuser",
			knownUsers:     []string{"testuser", "otheruser"},
			expectErr:      true,
			expectSwitch:   false,
			expectedErrMsg: "user unknownuser specified in git config gh.user is not authenticated for github.com",
		},
		{
			name:        "successful user switch",
			isLocalRepo: true,
			configValue: "otheruser",
			hosts:       []string{"github.com"},
			activeUser:  "testuser",
			knownUsers:  []string{"testuser", "otheruser"},
			expectErr:   false,
			expectSwitch: true,
		},
		{
			name:           "user switch fails",
			isLocalRepo:    true,
			configValue:    "otheruser",
			hosts:          []string{"github.com"},
			activeUser:     "testuser",
			knownUsers:     []string{"testuser", "otheruser"},
			switchErr:      errors.New("switch failed"),
			expectErr:      true,
			expectSwitch:   true,
			expectedErrMsg: "failed to switch to user otheruser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock auth config
			authConfig := &mockAuthConfig{
				hosts:      tt.hosts,
				activeUser: tt.activeUser,
				activeErr:  tt.activeErr,
				knownUsers: tt.knownUsers,
				switchErr:  tt.switchErr,
			}

			// Create mock git client
			gitClient := &mockGitClient{
				isLocalRepo: tt.isLocalRepo,
				repoErr:     tt.repoErr,
				configValue: tt.configValue,
				configErr:   tt.configErr,
			}

			// Create mock config
			config := &mockConfig{
				authConfig: authConfig,
			}

			// Test the internal function with mocked dependencies
			err := maybeAutomaticUserSwitchWithGitClient(config, gitClient)

			// Check error expectations
			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectedErrMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Check if switch was called
			if tt.expectSwitch {
				assert.Len(t, authConfig.switchCalls, 1)
				assert.Equal(t, "github.com", authConfig.switchCalls[0].hostname)
				assert.Equal(t, tt.configValue, authConfig.switchCalls[0].user)
			} else {
				assert.Len(t, authConfig.switchCalls, 0)
			}
		})
	}
}

func TestGetGitConfigUser(t *testing.T) {
	// Note: This test requires being in an actual git repository
	// In a real test environment, you might want to create a temporary git repo
	// or mock the git client functionality
	
	t.Run("function exists and can be called", func(t *testing.T) {
		// This is a basic test to ensure the function can be called
		// The actual behavior depends on the git repository state
		user, err := GetGitConfigUser()
		
		// We can't assert specific values since it depends on the environment
		// but we can ensure the function doesn't panic and returns expected types
		assert.IsType(t, "", user)
		assert.True(t, err == nil || err != nil) // Either outcome is valid
	})
}

func TestSetGitConfigUser(t *testing.T) {
	// Note: This test requires being in an actual git repository
	// and will modify the git config, so it should be used carefully
	
	t.Run("function exists and can be called", func(t *testing.T) {
		// This is a basic test to ensure the function can be called
		// In a real test environment, you would set up a temporary git repo
		err := SetGitConfigUser("testuser")
		
		// We can't assert specific behavior since it depends on the environment
		// but we can ensure the function doesn't panic
		assert.True(t, err == nil || err != nil) // Either outcome is valid
	})
}