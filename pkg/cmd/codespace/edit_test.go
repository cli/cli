package codespace

import (
	"context"
	"errors"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestEdit(t *testing.T) {

	tests := []struct {
		name        string
		opts        deleteOptions
		codespaces  []*api.Codespace
		deleteErr   error
		wantErr     bool
		wantDeleted []string
		wantStdout  string
		wantStderr  string
	}{
		{
			name: "by name",
			opts: deleteOptions{
				codespaceName: "hubot-robawt-abc",
			},
			codespaces: []*api.Codespace{
				{
					Name: "hubot-robawt-abc",
				},
			},
			wantDeleted: []string{"hubot-robawt-abc"},
			wantStdout:  "",
		},
		{
			name: "by repo",
			opts: deleteOptions{
				repoFilter: "monalisa/spoon-knife",
			},
			codespaces: []*api.Codespace{
				{
					Name: "monalisa-spoonknife-123",
					Repository: api.Repository{
						FullName: "monalisa/Spoon-Knife",
					},
				},
				{
					Name: "hubot-robawt-abc",
					Repository: api.Repository{
						FullName: "hubot/ROBAWT",
					},
				},
				{
					Name: "monalisa-spoonknife-c4f3",
					Repository: api.Repository{
						FullName: "monalisa/Spoon-Knife",
					},
				},
			},
			wantDeleted: []string{"monalisa-spoonknife-123", "monalisa-spoonknife-c4f3"},
			wantStdout:  "",
		},
		{
			name: "unused",
			opts: deleteOptions{
				deleteAll: true,
				keepDays:  3,
			},
			codespaces: []*api.Codespace{
				{
					Name:       "monalisa-spoonknife-123",
					LastUsedAt: daysAgo(1),
				},
				{
					Name:       "hubot-robawt-abc",
					LastUsedAt: daysAgo(4),
				},
				{
					Name:       "monalisa-spoonknife-c4f3",
					LastUsedAt: daysAgo(10),
				},
			},
			wantDeleted: []string{"hubot-robawt-abc", "monalisa-spoonknife-c4f3"},
			wantStdout:  "",
		},
		{
			name: "deletion failed",
			opts: deleteOptions{
				deleteAll: true,
			},
			codespaces: []*api.Codespace{
				{
					Name: "monalisa-spoonknife-123",
				},
				{
					Name: "hubot-robawt-abc",
				},
			},
			deleteErr:   errors.New("aborted by test"),
			wantErr:     true,
			wantDeleted: []string{"hubot-robawt-abc", "monalisa-spoonknife-123"},
			wantStderr: heredoc.Doc(`
				error deleting codespace "hubot-robawt-abc": aborted by test
				error deleting codespace "monalisa-spoonknife-123": aborted by test
			`),
		},
		{
			name: "with confirm",
			opts: deleteOptions{
				isInteractive: true,
				deleteAll:     true,
				skipConfirm:   false,
			},
			codespaces: []*api.Codespace{
				{
					Name: "monalisa-spoonknife-123",
					GitStatus: api.CodespaceGitStatus{
						HasUnpushedChanges: true,
					},
				},
				{
					Name: "hubot-robawt-abc",
					GitStatus: api.CodespaceGitStatus{
						HasUncommitedChanges: true,
					},
				},
				{
					Name: "monalisa-spoonknife-c4f3",
					GitStatus: api.CodespaceGitStatus{
						HasUnpushedChanges:   false,
						HasUncommitedChanges: false,
					},
				},
			},
			wantDeleted: []string{"hubot-robawt-abc", "monalisa-spoonknife-c4f3"},
			wantStdout:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiMock := &apiClientMock{
				GetUserFunc: func(_ context.Context) (*api.User, error) {
					return user, nil
				},
				DeleteCodespaceFunc: func(_ context.Context, name string) error {
					if tt.deleteErr != nil {
						return tt.deleteErr
					}
					return nil
				},
			}
			if tt.opts.codespaceName == "" {
				apiMock.ListCodespacesFunc = func(_ context.Context, num int) ([]*api.Codespace, error) {
					return tt.codespaces, nil
				}
			} else {
				apiMock.GetCodespaceFunc = func(_ context.Context, name string, includeConnection bool) (*api.Codespace, error) {
					return tt.codespaces[0], nil
				}
			}

			opts := tt.opts

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			app := NewApp(ios, nil, apiMock, nil)
			err := app.Delete(context.Background(), opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("delete() error = %v, wantErr %v", err, tt.wantErr)
			}
			var gotDeleted []string
			for _, delArgs := range apiMock.DeleteCodespaceCalls() {
				gotDeleted = append(gotDeleted, delArgs.Name)
			}

			if out := stdout.String(); out != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", out, tt.wantStdout)
			}
		})
	}
}
