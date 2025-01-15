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
		cliArgs       []string // alternative to opts; will test command dispatcher
		wantEdits     *api.EditCodespaceParams
		mockCodespace *api.Codespace
		wantStdout    string
		wantStderr    string
		wantErr       bool
		errMsg        string
	}{
		{
			name: "edit codespace display name",
			opts: editOptions{
				selector:    &CodespaceSelector{codespaceName: "hubot"},
				displayName: "hubot-changed",
				machine:     "",
			},
			wantEdits: &api.EditCodespaceParams{
				DisplayName: "hubot-changed",
			},
			mockCodespace: &api.Codespace{
				Name:        "hubot",
				DisplayName: "hubot-changed",
			},
			wantStdout: "",
			wantErr:    false,
		},
		{
			name:    "CLI legacy --displayName",
			cliArgs: []string{"--codespace", "hubot", "--displayName", "hubot-changed"},
			wantEdits: &api.EditCodespaceParams{
				DisplayName: "hubot-changed",
			},
			mockCodespace: &api.Codespace{
				Name:        "hubot",
				DisplayName: "hubot-changed",
			},
			wantStdout: "",
			wantStderr: "Flag --displayName has been deprecated, use `--display-name` instead\n",
			wantErr:    false,
		},
		{
			name: "edit codespace machine",
			opts: editOptions{
				selector:    &CodespaceSelector{codespaceName: "hubot"},
				displayName: "",
				machine:     "machine",
			},
			wantEdits: &api.EditCodespaceParams{
				Machine: "machine",
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
			name:    "no CLI arguments",
			cliArgs: []string{},
			wantErr: true,
			errMsg:  "must provide `--display-name` or `--machine`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotEdits *api.EditCodespaceParams
			apiMock := &apiClientMock{
				EditCodespaceFunc: func(_ context.Context, codespaceName string, params *api.EditCodespaceParams) (*api.Codespace, error) {
					gotEdits = params
					return tt.mockCodespace, nil
				},
			}

			ios, _, stdout, stderr := iostreams.Test()
			a := NewApp(ios, nil, apiMock, nil, nil)

			var err error
			if tt.cliArgs == nil {
				if tt.opts.selector == nil {
					t.Fatalf("selector must be set in opts if cliArgs are not provided")
				}

				tt.opts.selector.api = apiMock
				err = a.Edit(context.Background(), tt.opts)
			} else {
				cmd := newEditCmd(a)
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
			if out := stderr.String(); out != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", out, tt.wantStderr)
			}

			if tt.wantEdits != nil {
				if gotEdits == nil {
					t.Fatalf("EditCodespace() never called")
				}
				if tt.wantEdits.DisplayName != gotEdits.DisplayName {
					t.Errorf("edited display name %q, want %q", gotEdits.DisplayName, tt.wantEdits.DisplayName)
				}
				if tt.wantEdits.Machine != gotEdits.Machine {
					t.Errorf("edited machine type %q, want %q", gotEdits.Machine, tt.wantEdits.Machine)
				}
				if tt.wantEdits.IdleTimeoutMinutes != gotEdits.IdleTimeoutMinutes {
					t.Errorf("edited idle timeout minutes %d, want %d", gotEdits.IdleTimeoutMinutes, tt.wantEdits.IdleTimeoutMinutes)
				}
			}
		})
	}
}
