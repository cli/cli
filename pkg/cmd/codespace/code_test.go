package codespace

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
)

func TestApp_VSCode(t *testing.T) {
	type args struct {
		codespaceName string
		useInsiders   bool
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &cmdutil.TestBrowser{}
			a := &App{}
			if err := a.VSCode(context.Background(), b, tt.args.codespaceName, tt.args.useInsiders); (err != nil) != tt.wantErr {
				t.Errorf("App.VSCode() error = %v, wantErr %v", err, tt.wantErr)
			}
			b.Verify(t, tt.wantURL)
		})
	}
}
