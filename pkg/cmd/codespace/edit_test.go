package codespace

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestEdit(t *testing.T) {

	tests := []struct {
		name          string
		opts          editOptions
		codespaces    []*api.Codespace
		mockCodespace *api.Codespace
		editErr       error
		wantErr       bool
		wantStdout    string
		wantStderr    string
	}{
		{
			name: "edit codespace display name",
			opts: editOptions{
				codespaceName: "hubot",
				displayName:   "hubot-changed",
				machine:       "",
			},
			mockCodespace: &api.Codespace{
				Name:        "hubot",
				DisplayName: "hubot-changed",
			},
			wantStdout: "",
			wantErr:    false,
		},
		{
			name: "edit codespace machine",
			opts: editOptions{
				codespaceName: "hubot",
				displayName:   "",
				machine:       "machine",
			},
			mockCodespace: &api.Codespace{
				Name: "hubot",
				Machine: api.CodespaceMachine{
					Name: "machine",
				},
			},
			wantStdout: "",
			wantErr:    false,
		},
		{
			name: "trying to edit a codespace without anything to edit should return an error",
			opts: editOptions{
				codespaceName: "hubot",
				displayName:   "",
				machine:       "",
			},
			wantStderr: "at least one property has to be edited",
			wantErr:    true,
		},
		{
			name: "select codespace to edit when no codespace input is given",
			opts: editOptions{
				codespaceName: "",
				displayName:   "monalisa-new",
				machine:       "",
			},
			codespaces: []*api.Codespace{
				{
					Name:        "monalisa-123",
					DisplayName: "monalisa-old",
				},
				{
					Name:        "hubot-robawt-abc",
					DisplayName: "hubot",
				},
				{
					Name:        "monalisa-spoonknife-c4f3",
					DisplayName: "c4f3",
				},
			},
			mockCodespace: &api.Codespace{
				Name: "hubot",
				Machine: api.CodespaceMachine{
					Name:        "monalisa-123",
					DisplayName: "monalisa-new",
				},
			},
			wantStdout: "",
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			apiMock := &apiClientMock{
				EditCodespaceFunc: func(_ context.Context, codespaceName string, params *api.EditCodespaceParams) (*api.Codespace, error) {
					if tt.editErr != nil {
						return tt.mockCodespace, tt.editErr
					}
					return tt.mockCodespace, nil
				},
			}

			if tt.opts.codespaceName == "" {
				apiMock.ListCodespacesFunc = func(_ context.Context, num int) ([]*api.Codespace, error) {
					return tt.codespaces, nil
				}
			}

			opts := tt.opts

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			ios.SetStderrTTY(true)
			a := NewApp(ios, nil, apiMock, nil)

			err := a.Edit(context.Background(), opts)

			if (err != nil) != tt.wantErr {
				t.Errorf("App.Edit() error = %v, wantErr %v", err, tt.wantErr)
			}

			if out := stdout.String(); out != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", out, tt.wantStdout)
			}

			if out := stderr.String(); out != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", out, tt.wantStderr)
			}

		})
	}
}
