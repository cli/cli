package codespace

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestApp_Create(t *testing.T) {
	type fields struct {
		apiClient apiClient
	}
	tests := []struct {
		name       string
		fields     fields
		opts       createOptions
		wantErr    error
		wantStdout string
		wantStderr string
		isTTY      bool
	}{
		{
			name: "create codespace with default branch and 30m idle timeout",
			fields: fields{
				apiClient: &apiClientMock{
					GetCodespaceRegionLocationFunc: func(ctx context.Context) (string, error) {
						return "EUROPE", nil
					},
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespacesMachinesFunc: func(ctx context.Context, repoID int, branch, location string) ([]*api.Machine, error) {
						return []*api.Machine{
							{
								Name:        "GIGA",
								DisplayName: "Gigabits of a machine",
							},
						}, nil
					},
					CreateCodespaceFunc: func(ctx context.Context, params *api.CreateCodespaceParams) (*api.Codespace, error) {
						if params.Branch != "main" {
							return nil, fmt.Errorf("got branch %q, want %q", params.Branch, "main")
						}
						if params.IdleTimeoutMinutes != 30 {
							return nil, fmt.Errorf("idle timeout minutes was %v", params.IdleTimeoutMinutes)
						}
						return &api.Codespace{
							Name: "monalisa-dotfiles-abcd1234",
						}, nil
					},
					GetCodespaceRepoSuggestionsFunc: func(ctx context.Context, partialSearch string, params api.RepoSearchParameters) ([]string, error) {
						return nil, nil // We can't ask for suggestions without a terminal.
					},
				},
			},
			opts: createOptions{
				repo:        "monalisa/dotfiles",
				branch:      "",
				machine:     "GIGA",
				showStatus:  false,
				idleTimeout: 30 * time.Minute,
			},
			wantStdout: "monalisa-dotfiles-abcd1234\n",
		},
		{
			name: "create codespace with default branch shows idle timeout notice if present",
			fields: fields{
				apiClient: &apiClientMock{
					GetCodespaceRegionLocationFunc: func(ctx context.Context) (string, error) {
						return "EUROPE", nil
					},
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespacesMachinesFunc: func(ctx context.Context, repoID int, branch, location string) ([]*api.Machine, error) {
						return []*api.Machine{
							{
								Name:        "GIGA",
								DisplayName: "Gigabits of a machine",
							},
						}, nil
					},
					CreateCodespaceFunc: func(ctx context.Context, params *api.CreateCodespaceParams) (*api.Codespace, error) {
						if params.Branch != "main" {
							return nil, fmt.Errorf("got branch %q, want %q", params.Branch, "main")
						}
						if params.IdleTimeoutMinutes != 30 {
							return nil, fmt.Errorf("idle timeout minutes was %v", params.IdleTimeoutMinutes)
						}
						return &api.Codespace{
							Name:              "monalisa-dotfiles-abcd1234",
							IdleTimeoutNotice: "Idle timeout for this codespace is set to 10 minutes in compliance with your organization's policy",
						}, nil
					},
					GetCodespaceRepoSuggestionsFunc: func(ctx context.Context, partialSearch string, params api.RepoSearchParameters) ([]string, error) {
						return nil, nil // We can't ask for suggestions without a terminal.
					},
				},
			},
			opts: createOptions{
				repo:        "monalisa/dotfiles",
				branch:      "",
				machine:     "GIGA",
				showStatus:  false,
				idleTimeout: 30 * time.Minute,
			},
			wantStdout: "monalisa-dotfiles-abcd1234\n",
			wantStderr: "Notice: Idle timeout for this codespace is set to 10 minutes in compliance with your organization's policy\n",
			isTTY:      true,
		},
		{
			name: "create codespace with default branch does not show idle timeout notice if not conntected to terminal",
			fields: fields{
				apiClient: &apiClientMock{
					GetCodespaceRegionLocationFunc: func(ctx context.Context) (string, error) {
						return "EUROPE", nil
					},
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespacesMachinesFunc: func(ctx context.Context, repoID int, branch, location string) ([]*api.Machine, error) {
						return []*api.Machine{
							{
								Name:        "GIGA",
								DisplayName: "Gigabits of a machine",
							},
						}, nil
					},
					CreateCodespaceFunc: func(ctx context.Context, params *api.CreateCodespaceParams) (*api.Codespace, error) {
						if params.Branch != "main" {
							return nil, fmt.Errorf("got branch %q, want %q", params.Branch, "main")
						}
						if params.IdleTimeoutMinutes != 30 {
							return nil, fmt.Errorf("idle timeout minutes was %v", params.IdleTimeoutMinutes)
						}
						return &api.Codespace{
							Name:              "monalisa-dotfiles-abcd1234",
							IdleTimeoutNotice: "Idle timeout for this codespace is set to 10 minutes in compliance with your organization's policy",
						}, nil
					},
					GetCodespaceRepoSuggestionsFunc: func(ctx context.Context, partialSearch string, params api.RepoSearchParameters) ([]string, error) {
						return nil, nil // We can't ask for suggestions without a terminal.
					},
				},
			},
			opts: createOptions{
				repo:        "monalisa/dotfiles",
				branch:      "",
				machine:     "GIGA",
				showStatus:  false,
				idleTimeout: 30 * time.Minute,
			},
			wantStdout: "monalisa-dotfiles-abcd1234\n",
			wantStderr: "",
			isTTY:      false,
		},
		{
			name: "create codespace that requires accepting additional permissions",
			fields: fields{
				apiClient: &apiClientMock{
					GetCodespaceRegionLocationFunc: func(ctx context.Context) (string, error) {
						return "EUROPE", nil
					},
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespacesMachinesFunc: func(ctx context.Context, repoID int, branch, location string) ([]*api.Machine, error) {
						return []*api.Machine{
							{
								Name:        "GIGA",
								DisplayName: "Gigabits of a machine",
							},
						}, nil
					},
					CreateCodespaceFunc: func(ctx context.Context, params *api.CreateCodespaceParams) (*api.Codespace, error) {
						if params.Branch != "main" {
							return nil, fmt.Errorf("got branch %q, want %q", params.Branch, "main")
						}
						if params.IdleTimeoutMinutes != 30 {
							return nil, fmt.Errorf("idle timeout minutes was %v", params.IdleTimeoutMinutes)
						}
						return &api.Codespace{}, api.AcceptPermissionsRequiredError{
							AllowPermissionsURL: "https://example.com/permissions",
						}
					},
					GetCodespaceRepoSuggestionsFunc: func(ctx context.Context, partialSearch string, params api.RepoSearchParameters) ([]string, error) {
						return nil, nil // We can't ask for suggestions without a terminal.
					},
				},
			},
			opts: createOptions{
				repo:        "monalisa/dotfiles",
				branch:      "",
				machine:     "GIGA",
				showStatus:  false,
				idleTimeout: 30 * time.Minute,
			},
			wantErr: cmdutil.SilentError,
			wantStderr: `You must authorize or deny additional permissions requested by this codespace before continuing.
Open this URL in your browser to review and authorize additional permissions: example.com/permissions
Alternatively, you can run "create" with the "--default-permissions" option to continue without authorizing additional permissions.
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			a := &App{
				io:        io,
				apiClient: tt.fields.apiClient,
			}
			if err := a.Create(context.Background(), tt.opts); err != tt.wantErr {
				t.Errorf("App.Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got := stdout.String(); got != tt.wantStdout {
				t.Errorf("stdout = %v, want %v", got, tt.wantStdout)
			}
			if got := stderr.String(); got != tt.wantStderr {
				t.Errorf("stderr = %v, want %v", got, tt.wantStderr)
			}
		})
	}
}

func TestBuildDisplayName(t *testing.T) {
	tests := []struct {
		name                 string
		prebuildAvailability string
		expectedDisplayName  string
	}{
		{
			name:                 "prebuild availability is pool",
			prebuildAvailability: "pool",
			expectedDisplayName:  "4 cores, 8 GB RAM, 32 GB storage (Prebuild ready)",
		},
		{
			name:                 "prebuild availability is blob",
			prebuildAvailability: "blob",
			expectedDisplayName:  "4 cores, 8 GB RAM, 32 GB storage (Prebuild ready)",
		},
		{
			name:                 "prebuild availability is none",
			prebuildAvailability: "none",
			expectedDisplayName:  "4 cores, 8 GB RAM, 32 GB storage",
		},
		{
			name:                 "prebuild availability is empty",
			prebuildAvailability: "",
			expectedDisplayName:  "4 cores, 8 GB RAM, 32 GB storage",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			displayName := buildDisplayName("4 cores, 8 GB RAM, 32 GB storage", tt.prebuildAvailability)

			if displayName != tt.expectedDisplayName {
				t.Errorf("displayName = %q, expectedDisplayName %q", displayName, tt.expectedDisplayName)
			}
		})
	}
}
