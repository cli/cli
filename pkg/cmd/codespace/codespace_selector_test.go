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
		octocatOwner = api.RepositoryOwner{Login: "octocat"}
		cliOwner     = api.RepositoryOwner{Login: "cli"}
		octocatA     = &api.Codespace{
			Name:       "1",
			Repository: api.Repository{FullName: "octocat/A", Owner: octocatOwner},
		}

		octocatA2 = &api.Codespace{
			Name:       "2",
			Repository: api.Repository{FullName: "octocat/A", Owner: octocatOwner},
		}

		cliA = &api.Codespace{
			Name:       "3",
			Repository: api.Repository{FullName: "cli/A", Owner: cliOwner},
		}

		octocatB = &api.Codespace{
			Name:       "4",
			Repository: api.Repository{FullName: "octocat/B", Owner: octocatOwner},
		}
	)

	tests := []struct {
		tName          string
		apiCodespaces  []*api.Codespace
		codespaceName  string
		repoName       string
		repoOwner      string
		wantCodespaces []*api.Codespace
		wantErr        error
	}{
		// Empty case
		{
			tName:          "empty",
			apiCodespaces:  nil,
			wantCodespaces: nil,
			wantErr:        errNoCodespaces,
		},

		// Tests with no filtering
		{
			tName:          "no filtering, single codespaces",
			apiCodespaces:  []*api.Codespace{octocatA},
			wantCodespaces: []*api.Codespace{octocatA},
			wantErr:        nil,
		},
		{
			tName:          "no filtering, multiple codespace",
			apiCodespaces:  []*api.Codespace{octocatA, cliA, octocatB},
			wantCodespaces: []*api.Codespace{octocatA, cliA, octocatB},
		},

		// Test repo filtering
		{
			tName:          "repo name filtering, single codespace",
			apiCodespaces:  []*api.Codespace{octocatA},
			repoName:       "octocat/A",
			wantCodespaces: []*api.Codespace{octocatA},
			wantErr:        nil,
		},
		{
			tName:          "repo name filtering, multiple codespace",
			apiCodespaces:  []*api.Codespace{octocatA, octocatA2, cliA, octocatB},
			repoName:       "octocat/A",
			wantCodespaces: []*api.Codespace{octocatA, octocatA2},
			wantErr:        nil,
		},
		{
			tName:          "repo name filtering, multiple codespace 2",
			apiCodespaces:  []*api.Codespace{octocatA, cliA, octocatB},
			repoName:       "octocat/B",
			wantCodespaces: []*api.Codespace{octocatB},
			wantErr:        nil,
		},
		{
			tName:          "repo name filtering, no matches",
			apiCodespaces:  []*api.Codespace{octocatA, cliA, octocatB},
			repoName:       "Unknown/unknown",
			wantCodespaces: nil,
			wantErr:        errNoFilteredCodespaces,
		},
		{
			tName:          "repo filtering, match with repo owner",
			apiCodespaces:  []*api.Codespace{octocatA, octocatA2, cliA, octocatB},
			repoOwner:      "octocat",
			wantCodespaces: []*api.Codespace{octocatA, octocatA2, octocatB},
			wantErr:        nil,
		},
		{
			tName:          "repo filtering, no match with repo owner",
			apiCodespaces:  []*api.Codespace{octocatA, cliA, octocatB},
			repoOwner:      "unknown",
			wantCodespaces: []*api.Codespace{},
			wantErr:        errNoFilteredCodespaces,
		},
	}

	for _, tt := range tests {
		t.Run(tt.tName, func(t *testing.T) {
			api := &apiClientMock{
				ListCodespacesFunc: func(ctx context.Context, opts api.ListCodespacesOptions) ([]*api.Codespace, error) {
					return tt.apiCodespaces, nil
				},
			}

			cs := &CodespaceSelector{
				api:       api,
				repoName:  tt.repoName,
				repoOwner: tt.repoOwner,
			}

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
