package codespace

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/codespaces/api"
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
			wantURL: "vscode://github.codespaces/connect?name=monalisa-cli-cli-abcdef&windowId=_blank",
		},
		{
			name: "open VS Code Insiders",
			args: args{
				codespaceName: "monalisa-cli-cli-abcdef",
				useInsiders:   true,
			},
			wantErr: false,
			wantURL: "vscode-insiders://github.codespaces/connect?name=monalisa-cli-cli-abcdef&windowId=_blank",
		},
		{
			name: "open VS Code web",
			args: args{
				codespaceName: "monalisa-cli-cli-abcdef",
				useInsiders:   false,
				useWeb:        true,
			},
			wantErr: false,
			wantURL: "https://monalisa-cli-cli-abcdef.github.dev",
		},
		{
			name: "open VS Code web with Insiders",
			args: args{
				codespaceName: "monalisa-cli-cli-abcdef",
				useInsiders:   true,
				useWeb:        true,
			},
			wantErr: false,
			wantURL: "https://monalisa-cli-cli-abcdef.github.dev?vscodeChannel=insiders",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &browser.Stub{}
			ios, _, stdout, stderr := iostreams.Test()
			a := &App{
				browser:   b,
				apiClient: testCodeApiMock(),
				io:        ios,
			}
			selector := &CodespaceSelector{api: a.apiClient, codespaceName: tt.args.codespaceName}

			if err := a.VSCode(context.Background(), selector, tt.args.useInsiders, tt.args.useWeb); (err != nil) != tt.wantErr {
				t.Errorf("App.VSCode() error = %v, wantErr %v", err, tt.wantErr)
			}
			b.Verify(t, tt.wantURL)
			if got := stdout.String(); got != "" {
				t.Errorf("stdout = %q, want %q", got, "")
			}
			if got := stderr.String(); got != "" {
				t.Errorf("stderr = %q, want %q", got, "")
			}
		})
	}
}

func TestPendingOperationDisallowsCode(t *testing.T) {
	app := testingCodeApp()
	selector := &CodespaceSelector{api: app.apiClient, codespaceName: "disabledCodespace"}

	if err := app.VSCode(context.Background(), selector, false, false); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func testingCodeApp() *App {
	ios, _, _, _ := iostreams.Test()
	return NewApp(ios, nil, testCodeApiMock(), nil, nil)
}

func testCodeApiMock() *apiClientMock {
	testingCodespace := &api.Codespace{
		Name:   "monalisa-cli-cli-abcdef",
		WebURL: "https://monalisa-cli-cli-abcdef.github.dev",
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
	}
}
