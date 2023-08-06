package codespace

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestAlreadyRebuildingCodespace(t *testing.T) {
	rebuildingCodespace := &api.Codespace{
		Name:  "rebuildingCodespace",
		State: api.CodespaceStateRebuilding,
	}
	app := testingRebuildApp(*rebuildingCodespace)
	selector := &CodespaceSelector{api: app.apiClient, codespaceName: "rebuildingCodespace"}

	err := app.Rebuild(context.Background(), selector, false)
	if err != nil {
		t.Errorf("rebuilding a codespace that was already rebuilding: %v", err)
	}
}

func testingRebuildApp(mockCodespace api.Codespace) *App {
	apiMock := &apiClientMock{
		GetCodespaceFunc: func(_ context.Context, name string, _ bool) (*api.Codespace, error) {
			if name == mockCodespace.Name {
				return &mockCodespace, nil
			}
			return nil, nil
		},
	}

	ios, _, _, _ := iostreams.Test()
	return NewApp(ios, nil, apiMock, nil, nil)
}
