package codespace

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestAlreadyRebuildingCodespace(t *testing.T) {
	app := testingRebuildApp()

	err := app.Rebuild(context.Background(), "rebuildingCodespace")
	if err != nil {
		t.Errorf("rebuilding a codespace that was already rebuilding: %v", err)
	}
}

func testingRebuildApp() *App {
	rebuildingCodespace := &api.Codespace{
		Name:  "rebuildingCodespace",
		State: api.CodespaceStateRebuilding,
	}
	apiMock := &apiClientMock{
		GetCodespaceFunc: func(_ context.Context, name string, _ bool) (*api.Codespace, error) {
			if name == rebuildingCodespace.Name {
				return rebuildingCodespace, nil
			}
			return nil, nil
		},
	}

	ios, _, _, _ := iostreams.Test()
	return NewApp(ios, nil, apiMock, nil)
}
