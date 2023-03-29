package codespace

import (
	"context"
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
)

func TestSelectWithCodespaceName(t *testing.T) {
	wantName := "mock-codespace"

	api := &apiClientMock{
		GetCodespaceFunc: func(ctx context.Context, name string, includeConnection bool) (*api.Codespace, error) {
			if name != wantName {
				t.Errorf("incorrect name: want %s, got %s", wantName, name)
			}

			return &api.Codespace{}, nil
		},
	}

	cs := &CodespaceSelector{api: api, codespaceName: wantName}

	_, err := cs.Select(context.Background())

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSelectNameWithCodespaceName(t *testing.T) {
	wantName := "mock-codespace"

	cs := &CodespaceSelector{codespaceName: wantName}

	name, err := cs.SelectName(context.Background())

	if name != wantName {
		t.Errorf("incorrect name: want %s, got %s", wantName, name)
	}

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFetchCodespaces(t *testing.T) {
	var (
		repoA1 = &api.Codespace{Name: "1", Repository: api.Repository{FullName: "mock/A"}}
		repoA2 = &api.Codespace{Name: "2", Repository: api.Repository{FullName: "mock/A"}}

		repoB1 = &api.Codespace{Name: "1", Repository: api.Repository{FullName: "mock/B"}}
	)

	tests := []struct {
		tName          string
		apiCodespaces  []*api.Codespace
		repoName       string
		wantCodespaces []*api.Codespace
		wantErr        error
	}{
		// Empty case
		{
			"empty", nil, "", nil, errNoCodespaces,
		},

		// Tests with no filtering
		{
			"no filtering, single codespace",
			[]*api.Codespace{repoA1},
			"",
			[]*api.Codespace{repoA1},
			nil,
		},
		{
			"no filtering, multiple codespaces",
			[]*api.Codespace{repoA1, repoA2, repoB1},
			"",
			[]*api.Codespace{repoA1, repoA2, repoB1},
			nil,
		},

		// Test repo filtering
		{
			"repo filtering, single codespace",
			[]*api.Codespace{repoA1},
			"mock/A",
			[]*api.Codespace{repoA1},
			nil,
		},
		{
			"repo filtering, multiple codespaces",
			[]*api.Codespace{repoA1, repoA2, repoB1},
			"mock/A",
			[]*api.Codespace{repoA1, repoA2},
			nil,
		},
		{
			"repo filtering, multiple codespaces 2",
			[]*api.Codespace{repoA1, repoA2, repoB1},
			"mock/B",
			[]*api.Codespace{repoB1},
			nil,
		},
		{
			"repo filtering, no matches",
			[]*api.Codespace{repoA1, repoA2, repoB1},
			"mock/C",
			nil,
			errNoFilteredCodespaces,
		},
	}

	for _, tt := range tests {
		t.Run(tt.tName, func(t *testing.T) {
			api := &apiClientMock{
				ListCodespacesFunc: func(ctx context.Context, opts api.ListCodespacesOptions) ([]*api.Codespace, error) {
					return tt.apiCodespaces, nil
				},
			}

			cs := &CodespaceSelector{api: api, repoName: tt.repoName}

			codespaces, err := cs.fetchCodespaces(context.Background())

			if err != tt.wantErr {
				t.Errorf("expected error to be %v, got %v", tt.wantErr, err)
			}

			if fmt.Sprintf("%v", tt.wantCodespaces) != fmt.Sprintf("%v", codespaces) {
				t.Errorf("expected codespaces to be %v, got %v", tt.wantCodespaces, codespaces)
			}
		})
	}
}
