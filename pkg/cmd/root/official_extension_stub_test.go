package root

import (
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialExtensionStubRun(t *testing.T) {
	ext := &extensions.OfficialExtension{Name: "cool", Owner: "github", Repo: "gh-cool"}

	tests := []struct {
		name          string
		isTTY         bool
		ciEnv         string
		confirmResult bool
		confirmErr    error
		installErr    error
		wantErr       string
		wantStderr    string
		wantInstalled bool
	}{
		{
			name:          "non-TTY in CI auto-installs without prompting",
			isTTY:         false,
			ciEnv:         "1",
			wantStderr:    "Successfully installed github/gh-cool",
			wantInstalled: true,
		},
		{
			name:       "non-TTY outside CI prints install instructions and returns silent error",
			isTTY:      false,
			wantStderr: "gh extension install github/gh-cool",
			wantErr:    "SilentError",
		},
		{
			name:          "TTY confirmed installs",
			isTTY:         true,
			confirmResult: true,
			wantStderr:    "Successfully installed github/gh-cool",
			wantInstalled: true,
		},
		{
			name:          "TTY in CI auto-installs without prompting",
			isTTY:         true,
			ciEnv:         "1",
			wantStderr:    "Successfully installed github/gh-cool",
			wantInstalled: true,
		},
		{
			name:          "TTY declined does not install",
			isTTY:         true,
			confirmResult: false,
		},
		{
			name:       "TTY prompt error is propagated",
			isTTY:      true,
			confirmErr: fmt.Errorf("prompt interrupted"),
			wantErr:    "prompt interrupted",
		},
		{
			name:          "TTY install error is propagated",
			isTTY:         true,
			confirmResult: true,
			installErr:    fmt.Errorf("network error"),
			wantErr:       "network error",
			wantInstalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CI", "")
			t.Setenv("BUILD_NUMBER", "")
			t.Setenv("RUN_ID", "")
			if tt.ciEnv != "" {
				t.Setenv("CI", tt.ciEnv)
			}

			ios, _, _, stderr := iostreams.Test()
			if tt.isTTY {
				ios.SetStdinTTY(true)
				ios.SetStdoutTTY(true)
				ios.SetStderrTTY(true)
			}

			em := &extensions.ExtensionManagerMock{
				InstallFunc: func(_ ghrepo.Interface, _ string) error {
					return tt.installErr
				},
			}
			p := &prompter.PrompterMock{
				ConfirmFunc: func(_ string, _ bool) (bool, error) {
					return tt.confirmResult, tt.confirmErr
				},
			}

			err := officialExtensionStubRun(ios, p, em, ext)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}

			if tt.wantStderr != "" {
				assert.Contains(t, stderr.String(), tt.wantStderr)
			}

			if tt.wantInstalled {
				require.NotEmpty(t, em.InstallCalls())
				repo := em.InstallCalls()[0].InterfaceMoqParam
				assert.Equal(t, "github", repo.RepoOwner())
				assert.Equal(t, "gh-cool", repo.RepoName())
				assert.Equal(t, "github.com", repo.RepoHost())
			} else {
				assert.Empty(t, em.InstallCalls())
			}
		})
	}
}

func TestNewCmdOfficialExtensionStub_Properties(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ext := &extensions.OfficialExtension{Name: "cool", Owner: "github", Repo: "gh-cool"}
	em := &extensions.ExtensionManagerMock{}
	p := &prompter.PrompterMock{}

	cmd := NewCmdOfficialExtensionStub(ios, p, em, ext)

	assert.Equal(t, "cool", cmd.Use)
	assert.True(t, cmd.Hidden)
	assert.Equal(t, "extension", cmd.GroupID)
	assert.True(t, cmd.DisableFlagParsing)
}
