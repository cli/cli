package codespace

import (
	"context"
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func Test_NewCmdView(t *testing.T) {
	tests := []struct {
		tName         string
		codespaceName string
		opts          *viewOptions
		cliArgs       []string
		wantErr       bool
		wantStdout    string
		errMsg        string
	}{
		{
			tName:   "selector throws because no terminal found",
			opts:    &viewOptions{},
			wantErr: true,
			errMsg:  "choosing codespace: error getting answers: no terminal",
		},
		{
			tName:         "command fails because provided codespace doesn't exist",
			codespaceName: "i-dont-exist",
			opts:          &viewOptions{},
			wantErr:       true,
			errMsg:        "getting full codespace details: codespace not found",
		},
		{
			tName:         "command succeeds because codespace exists (no details)",
			codespaceName: "monalisa-cli-cli-abcdef",
			opts:          &viewOptions{},
			wantErr:       false,
			wantStdout:    "Name\tmonalisa-cli-cli-abcdef\nState\t\nRepository\t\nGit Status\t - 0 commits ahead, 0 commits behind\nDevcontainer Path\t\nMachine Display Name\t\nIdle Timeout\t0 minutes\nCreated At\t\nRetention Period\t\n",
		},
		{
			tName:         "command succeeds because codespace exists (with details)",
			codespaceName: "monalisa-cli-cli-hijklm",
			opts:          &viewOptions{},
			wantErr:       false,
			wantStdout:    "Name\tmonalisa-cli-cli-hijklm\nState\tAvailable\nRepository\tcli/cli\nGit Status\tmain* - 1 commit ahead, 2 commits behind\nDevcontainer Path\t.devcontainer/devcontainer.json\nMachine Display Name\tTest Display Name\nIdle Timeout\t30 minutes\nCreated At\t\nRetention Period\t1 day\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.tName, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			a := &App{
				apiClient: testViewApiMock(),
				io:        ios,
			}
			selector := &CodespaceSelector{api: a.apiClient, codespaceName: tt.codespaceName}
			tt.opts.selector = selector

			var err error
			if tt.cliArgs == nil {
				if tt.opts.selector == nil {
					t.Fatalf("selector must be set in opts if cliArgs are not provided")
				}

				err = a.ViewCodespace(context.Background(), tt.opts)
			} else {
				cmd := newViewCmd(a)
				cmd.SilenceUsage = true
				cmd.SilenceErrors = true
				cmd.SetOut(ios.ErrOut)
				cmd.SetErr(ios.ErrOut)
				cmd.SetArgs(tt.cliArgs)
				_, err = cmd.ExecuteC()
			}

			if tt.wantErr {
				if err == nil {
					t.Error("Edit() expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("Edit() error = %q, want %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Edit() expected no error, got %v", err)
			}

			if out := stdout.String(); out != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", out, tt.wantStdout)
			}
		})
	}
}

func testViewApiMock() *apiClientMock {
	codespaceWithNoDetails := &api.Codespace{
		Name: "monalisa-cli-cli-abcdef",
	}
	codespaceWithDetails := &api.Codespace{
		Name: "monalisa-cli-cli-hijklm",
		GitStatus: api.CodespaceGitStatus{
			Ahead:                 1,
			Behind:                2,
			Ref:                   "main",
			HasUnpushedChanges:    true,
			HasUncommittedChanges: true,
		},
		IdleTimeoutMinutes:     30,
		RetentionPeriodMinutes: 1440,
		State:                  "Available",
		Repository:             api.Repository{FullName: "cli/cli"},
		DevContainerPath:       ".devcontainer/devcontainer.json",
		Machine: api.CodespaceMachine{
			DisplayName: "Test Display Name",
		},
	}
	return &apiClientMock{
		GetCodespaceFunc: func(_ context.Context, name string, _ bool) (*api.Codespace, error) {
			if name == codespaceWithDetails.Name {
				return codespaceWithDetails, nil
			} else if name == codespaceWithNoDetails.Name {
				return codespaceWithNoDetails, nil
			}

			return nil, fmt.Errorf("codespace not found")
		},
		ListCodespacesFunc: func(ctx context.Context, opts api.ListCodespacesOptions) ([]*api.Codespace, error) {
			return []*api.Codespace{codespaceWithNoDetails, codespaceWithDetails}, nil
		},
	}
}
