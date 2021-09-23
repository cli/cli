package ghcs

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/github/ghcs/internal/api"
)

func TestDelete(t *testing.T) {
	user := &api.User{Login: "hubot"}
	now, _ := time.Parse(time.RFC3339, "2021-09-22T00:00:00Z")
	daysAgo := func(n int) string {
		return now.Add(time.Hour * -time.Duration(24*n)).Format(time.RFC3339)
	}

	tests := []struct {
		name        string
		opts        deleteOptions
		codespaces  []*api.Codespace
		confirms    map[string]bool
		wantErr     bool
		wantDeleted []string
	}{
		{
			name: "by name",
			opts: deleteOptions{
				codespaceName: "hubot-robawt-abc",
			},
			codespaces: []*api.Codespace{
				{
					Name: "monalisa-spoonknife-123",
				},
				{
					Name: "hubot-robawt-abc",
				},
			},
			wantDeleted: []string{"hubot-robawt-abc"},
		},
		{
			name: "by repo",
			opts: deleteOptions{
				repoFilter: "monalisa/spoon-knife",
			},
			codespaces: []*api.Codespace{
				{
					Name:          "monalisa-spoonknife-123",
					RepositoryNWO: "monalisa/Spoon-Knife",
				},
				{
					Name:          "hubot-robawt-abc",
					RepositoryNWO: "hubot/ROBAWT",
				},
				{
					Name:          "monalisa-spoonknife-c4f3",
					RepositoryNWO: "monalisa/Spoon-Knife",
				},
			},
			wantDeleted: []string{"monalisa-spoonknife-123", "monalisa-spoonknife-c4f3"},
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
					Environment: api.CodespaceEnvironment{
						GitStatus: api.CodespaceEnvironmentGitStatus{
							HasUnpushedChanges: true,
						},
					},
				},
				{
					Name: "hubot-robawt-abc",
					Environment: api.CodespaceEnvironment{
						GitStatus: api.CodespaceEnvironmentGitStatus{
							HasUncommitedChanges: true,
						},
					},
				},
				{
					Name: "monalisa-spoonknife-c4f3",
					Environment: api.CodespaceEnvironment{
						GitStatus: api.CodespaceEnvironmentGitStatus{
							HasUnpushedChanges:   false,
							HasUncommitedChanges: false,
						},
					},
				},
			},
			confirms: map[string]bool{
				"Codespace monalisa-spoonknife-123 has unsaved changes. OK to delete?": false,
				"Codespace hubot-robawt-abc has unsaved changes. OK to delete?":        true,
			},
			wantDeleted: []string{"hubot-robawt-abc", "monalisa-spoonknife-c4f3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiMock := &apiClientMock{
				GetUserFunc: func(_ context.Context) (*api.User, error) {
					return user, nil
				},
				ListCodespacesFunc: func(_ context.Context, userLogin string) ([]*api.Codespace, error) {
					if userLogin != user.Login {
						return nil, fmt.Errorf("unexpected user %q", userLogin)
					}
					return tt.codespaces, nil
				},
				DeleteCodespaceFunc: func(_ context.Context, userLogin, name string) error {
					if userLogin != user.Login {
						return fmt.Errorf("unexpected user %q", userLogin)
					}
					return nil
				},
			}
			opts := tt.opts
			opts.apiClient = apiMock
			opts.now = func() time.Time { return now }
			opts.prompter = &prompterMock{
				ConfirmFunc: func(msg string) (bool, error) {
					res, found := tt.confirms[msg]
					if !found {
						return false, fmt.Errorf("unexpected prompt %q", msg)
					}
					return res, nil
				},
			}

			err := delete(context.Background(), nil, opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("delete() error = %v, wantErr %v", err, tt.wantErr)
			}
			if n := len(apiMock.GetUserCalls()); n != 1 {
				t.Errorf("GetUser invoked %d times, expected %d", n, 1)
			}
			var gotDeleted []string
			for _, delArgs := range apiMock.DeleteCodespaceCalls() {
				gotDeleted = append(gotDeleted, delArgs.Name)
			}
			sort.Strings(gotDeleted)
			if !sliceEquals(gotDeleted, tt.wantDeleted) {
				t.Errorf("deleted %q, want %q", gotDeleted, tt.wantDeleted)
			}
		})
	}
}

func sliceEquals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
