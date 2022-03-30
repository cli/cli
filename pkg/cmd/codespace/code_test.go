package codespace

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestApp_VSCode(t *testing.T) {
	type args struct {
		codespaceName string
		useInsiders   bool
		useWeb        bool
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		wantURL string
	}{
		{
			name: "open VS Code",
			args: args{
				codespaceName: "monalisa-cli-cli-abcdef",
				useInsiders:   false,
			},
			wantErr: false,
			wantURL: "vscode://github.codespaces/connect?name=monalisa-cli-cli-abcdef",
		},
		{
			name: "open VS Code Insiders",
			args: args{
				codespaceName: "monalisa-cli-cli-abcdef",
				useInsiders:   true,
			},
			wantErr: false,
			wantURL: "vscode-insiders://github.codespaces/connect?name=monalisa-cli-cli-abcdef",
		},
		{
			name: "open VS Code Web",
			args: args{
				codespaceName: "monalisa-cli-cli-abcdef",
				useInsiders:   false,
				useWeb:        true,
			},
			wantErr: false,
			wantURL: "https://monalisa-cli-cli-abcdef.github.dev",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &cmdutil.TestBrowser{}
			a := &App{
				browser:   b,
				apiClient: testCodeApiMock(),
			}
			if err := a.VSCode(context.Background(), tt.args.codespaceName, tt.args.useInsiders, tt.args.useWeb); (err != nil) != tt.wantErr {
				t.Errorf("App.VSCode() error = %v, wantErr %v", err, tt.wantErr)
			}
			b.Verify(t, tt.wantURL)
		})
	}
}

func TestPendingOperationDisallowsCode(t *testing.T) {
	app := testingCodeApp()

	if err := app.VSCode(context.Background(), "disabledCodespace", false, false); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func testingCodeApp() *App {
	io, _, _, _ := iostreams.Test()
	return NewApp(io, nil, testCodeApiMock(), nil)
}

func testCodeApiMock() *apiClientMock {
	user := &api.User{Login: "monalisa"}
	testingCodespace := &api.Codespace{
		Name: "monalisa-cli-cli-abcdef",
		WebUrl: "https://monalisa-cli-cli-abcdef.github.dev",
	}
	disabledCodespace := &api.Codespace{
		Name:                           "disabledCodespace",
		PendingOperation:               true,
		PendingOperationDisabledReason: "Some pending operation",
	}
	return &apiClientMock{
		GetCodespaceFunc: func(_ context.Context, name string, _ bool) (*api.Codespace, error) {
			if name == "disabledCodespace" {
				return disabledCodespace, nil
			}
			return testingCodespace, nil
		},
		GetUserFunc: func(_ context.Context) (*api.User, error) {
			return user, nil
		},
		AuthorizedKeysFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte{}, nil
		},
	}
}
