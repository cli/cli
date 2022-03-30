package codespace

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestPendingOperationDisallowsSSH(t *testing.T) {
	app := testingSSHApp()

	if err := app.SSH(context.Background(), []string{}, sshOptions{codespace: "disabledCodespace"}); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func testingSSHApp() *App {
	user := &api.User{Login: "monalisa"}
	disabledCodespace := &api.Codespace{
		Name:                           "disabledCodespace",
		PendingOperation:               true,
		PendingOperationDisabledReason: "Some pending operation",
	}
	apiMock := &apiClientMock{
		GetCodespaceFunc: func(_ context.Context, name string, _ bool) (*api.Codespace, error) {
			if name == "disabledCodespace" {
				return disabledCodespace, nil
			}
			return nil, nil
		},
		GetUserFunc: func(_ context.Context) (*api.User, error) {
			return user, nil
		},
		AuthorizedKeysFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte{}, nil
		},
	}

	io, _, _, _ := iostreams.Test()
	return NewApp(io, nil, apiMock, nil)
}
