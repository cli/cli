package codespace

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
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
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespaceBillableOwnerFunc: func(ctx context.Context, nwo string) (*api.User, error) {
						return &api.User{
							Login: "monalisa",
							Type:  "User",
						}, nil
					},
					ListDevContainersFunc: func(ctx context.Context, repoID int, branch string, limit int) ([]api.DevContainerEntry, error) {
						return []api.DevContainerEntry{{Path: ".devcontainer/devcontainer.json"}}, nil
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
						if *params.RetentionPeriodMinutes != 2880 {
							return nil, fmt.Errorf("retention period minutes expected 2880, was %v", params.RetentionPeriodMinutes)
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
				repo:            "monalisa/dotfiles",
				branch:          "",
				machine:         "GIGA",
				showStatus:      false,
				idleTimeout:     30 * time.Minute,
				retentionPeriod: NullableDuration{durationPtr(48 * time.Hour)},
			},
			wantStdout: "monalisa-dotfiles-abcd1234\n",
		},
		{
			name: "create codespace with default branch shows idle timeout notice if present",
			fields: fields{
				apiClient: &apiClientMock{
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespaceBillableOwnerFunc: func(ctx context.Context, nwo string) (*api.User, error) {
						return &api.User{
							Login: "monalisa",
							Type:  "User",
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
						if params.RetentionPeriodMinutes != nil {
							return nil, fmt.Errorf("retention period minutes expected nil, was %v", params.RetentionPeriodMinutes)
						}
						if params.DevContainerPath != ".devcontainer/foobar/devcontainer.json" {
							return nil, fmt.Errorf("got dev container path %q, want %q", params.DevContainerPath, ".devcontainer/foobar/devcontainer.json")
						}
						return &api.Codespace{
							Name: "monalisa-dotfiles-abcd1234",
						}, nil
					},
				},
			},
			opts: createOptions{
				repo:             "monalisa/dotfiles",
				branch:           "",
				machine:          "GIGA",
				showStatus:       false,
				idleTimeout:      30 * time.Minute,
				devContainerPath: ".devcontainer/foobar/devcontainer.json",
			},
			wantStdout: "monalisa-dotfiles-abcd1234\n",
		},
		{
			name: "create codespace with default branch with default devcontainer if no path provided and no devcontainer files exist in the repo",
			fields: fields{
				apiClient: &apiClientMock{
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespaceBillableOwnerFunc: func(ctx context.Context, nwo string) (*api.User, error) {
						return &api.User{
							Login: "monalisa",
							Type:  "User",
						}, nil
					},
					ListDevContainersFunc: func(ctx context.Context, repoID int, branch string, limit int) ([]api.DevContainerEntry, error) {
						return []api.DevContainerEntry{}, nil
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
						if params.DevContainerPath != "" {
							return nil, fmt.Errorf("got dev container path %q, want %q", params.DevContainerPath, ".devcontainer/foobar/devcontainer.json")
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
			name: "returns error when getting devcontainer paths fails",
			fields: fields{
				apiClient: &apiClientMock{
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespaceBillableOwnerFunc: func(ctx context.Context, nwo string) (*api.User, error) {
						return &api.User{
							Login: "monalisa",
							Type:  "User",
						}, nil
					},
					ListDevContainersFunc: func(ctx context.Context, repoID int, branch string, limit int) ([]api.DevContainerEntry, error) {
						return nil, fmt.Errorf("some error")
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
			wantErr: fmt.Errorf("error getting devcontainer.json paths: some error"),
		},
		{
			name: "create codespace with default branch does not show idle timeout notice if not conntected to terminal",
			fields: fields{
				apiClient: &apiClientMock{
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespaceBillableOwnerFunc: func(ctx context.Context, nwo string) (*api.User, error) {
						return &api.User{
							Login: "monalisa",
							Type:  "User",
						}, nil
					},
					ListDevContainersFunc: func(ctx context.Context, repoID int, branch string, limit int) ([]api.DevContainerEntry, error) {
						return []api.DevContainerEntry{}, nil
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
					GetCodespaceBillableOwnerFunc: func(ctx context.Context, nwo string) (*api.User, error) {
						return &api.User{
							Login: "monalisa",
							Type:  "User",
						}, nil
					},
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					ListDevContainersFunc: func(ctx context.Context, repoID int, branch string, limit int) ([]api.DevContainerEntry, error) {
						return []api.DevContainerEntry{{Path: ".devcontainer/devcontainer.json"}}, nil
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
		{
			name: "returns error when user can't create codepaces for a repository",
			fields: fields{
				apiClient: &apiClientMock{
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespaceBillableOwnerFunc: func(ctx context.Context, nwo string) (*api.User, error) {
						return nil, fmt.Errorf("some error")
					},
				},
			},
			opts: createOptions{
				repo:        "megacorp/private",
				branch:      "",
				machine:     "GIGA",
				showStatus:  false,
				idleTimeout: 30 * time.Minute,
			},
			wantErr: fmt.Errorf("error checking codespace ownership: some error"),
		},
		{
			name: "mentions billable owner when org covers codepaces for a repository",
			fields: fields{
				apiClient: &apiClientMock{
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespaceBillableOwnerFunc: func(ctx context.Context, nwo string) (*api.User, error) {
						return &api.User{
							Type:  "Organization",
							Login: "megacorp",
						}, nil
					},
					ListDevContainersFunc: func(ctx context.Context, repoID int, branch string, limit int) ([]api.DevContainerEntry, error) {
						return []api.DevContainerEntry{{Path: ".devcontainer/devcontainer.json"}}, nil
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
						return &api.Codespace{
							Name: "megacorp-private-abcd1234",
						}, nil
					},
				},
			},
			opts: createOptions{
				repo:        "megacorp/private",
				branch:      "",
				machine:     "GIGA",
				showStatus:  false,
				idleTimeout: 30 * time.Minute,
			},
			wantStdout: "  ✓ Codespaces usage for this repository is paid for by megacorp\nmegacorp-private-abcd1234\n",
		},
		{
			name: "doesn't mention billable owner when it's the individual",
			fields: fields{
				apiClient: &apiClientMock{
					GetRepositoryFunc: func(ctx context.Context, nwo string) (*api.Repository, error) {
						return &api.Repository{
							ID:            1234,
							FullName:      nwo,
							DefaultBranch: "main",
						}, nil
					},
					GetCodespaceBillableOwnerFunc: func(ctx context.Context, nwo string) (*api.User, error) {
						return &api.User{
							Type:  "User",
							Login: "monalisa",
						}, nil
					},
					ListDevContainersFunc: func(ctx context.Context, repoID int, branch string, limit int) ([]api.DevContainerEntry, error) {
						return []api.DevContainerEntry{{Path: ".devcontainer/devcontainer.json"}}, nil
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
						return &api.Codespace{
							Name: "megacorp-private-abcd1234",
						}, nil
					},
				},
			},
			opts: createOptions{
				repo:        "megacorp/private",
				branch:      "",
				machine:     "GIGA",
				showStatus:  false,
				idleTimeout: 30 * time.Minute,
			},
			wantStdout: "megacorp-private-abcd1234\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			a := &App{
				io:        ios,
				apiClient: tt.fields.apiClient,
			}

			err := a.Create(context.Background(), tt.opts)
			if err != nil && tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
			}
			if err != nil && tt.wantErr == nil {
				t.Logf(err.Error())
			}
			if got := stdout.String(); got != tt.wantStdout {
				t.Logf(t.Name())
				t.Errorf("  stdout = %v, want %v", got, tt.wantStdout)
			}
			if got := stderr.String(); got != tt.wantStderr {
				t.Logf(t.Name())
				t.Errorf("  stderr = %v, want %v", got, tt.wantStderr)
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
		{
			name:                 "prebuild availability is ready",
			prebuildAvailability: "ready",
			expectedDisplayName:  "4 cores, 8 GB RAM, 32 GB storage (Prebuild ready)",
		},
		{
			name:                 "prebuild availability is in_progress",
			prebuildAvailability: "in_progress",
			expectedDisplayName:  "4 cores, 8 GB RAM, 32 GB storage (Prebuild in progress)",
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

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
