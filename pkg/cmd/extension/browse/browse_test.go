package browse

import (
	"net/http"
	"net/url"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/cli/cli/v2/internal/config"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetExtensionsReturnsBrowsableMetadata(t *testing.T) {
	t.Parallel()

	// Given GitHub repositories tagged as extensions and locally installed extensions
	reg := httpmock.Registry{}
	defer reg.Verify(t)
	client := &http.Client{Transport: &reg}
	values := url.Values{
		"page":     []string{"1"},
		"per_page": []string{"100"},
		"q":        []string{"topic:gh-extension"},
	}
	reg.Register(
		httpmock.QueryMatcher("GET", "search/repositories", values),
		httpmock.JSONResponse(map[string]any{
			"incomplete_results": false,
			"total_count":        3,
			"items": []any{
				map[string]any{
					"name":        "gh-screensaver",
					"full_name":   "vilmibm/gh-screensaver",
					"description": "terminal animations",
					"owner":       map[string]any{"login": "vilmibm"},
				},
				map[string]any{
					"name":        "gh-cool",
					"full_name":   "cli/gh-cool",
					"description": "it's just cool ok",
					"owner":       map[string]any{"login": "cli"},
				},
				map[string]any{
					"name":        "not-an-extension",
					"full_name":   "octo/not-an-extension",
					"description": "wrong prefix",
					"owner":       map[string]any{"login": "octo"},
				},
			},
		}),
	)
	cfg := config.NewMockConfig()
	cfg.AuthenticationFunc = func() gh.AuthConfig {
		authCfg := &config.AuthConfig{}
		authCfg.SetDefaultHost("github.com", "")
		return authCfg
	}
	manager := &extensions.ExtensionManagerMock{
		ListFunc: func() []extensions.Extension {
			return []extensions.Extension{
				&extensions.ExtensionMock{
					URLFunc: func() string {
						return "https://github.com/vilmibm/gh-screensaver"
					},
				},
			}
		},
	}

	// When extension metadata is loaded
	entries, err := getExtensions(ExtBrowseOpts{
		Searcher: search.NewSearcher(client, "github.com", &fd.DisabledDetectorMock{}),
		Em:       manager,
		Cfg:      cfg,
	})

	// Then only extension repositories are returned with visible status metadata
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, extEntry{
		URL:         "https://github.com/vilmibm/gh-screensaver",
		Name:        "gh-screensaver",
		FullName:    "vilmibm/gh-screensaver",
		Installed:   true,
		description: "terminal animations",
	}, entries[0])
	assert.Equal(t, extEntry{
		URL:         "https://github.com/cli/gh-cool",
		Name:        "gh-cool",
		FullName:    "cli/gh-cool",
		Official:    true,
		description: "it's just cool ok",
	}, entries[1])
}

func TestExtBrowseRunsBubbleTeaModel(t *testing.T) {
	t.Parallel()

	// Given one extension is available to browse
	ios, _, _, _ := iostreams.Test()
	cfg := config.NewMockConfig()
	cfg.AuthenticationFunc = func() gh.AuthConfig {
		authCfg := &config.AuthConfig{}
		authCfg.SetDefaultHost("github.com", "")
		return authCfg
	}
	searcher := &search.SearcherMock{
		RepositoriesFunc: func(search.Query) (search.RepositoriesResult, error) {
			return search.RepositoriesResult{
				Items: []search.Repository{
					{
						Name:        "gh-cool",
						FullName:    "cli/gh-cool",
						Description: "terminal tools",
						Owner:       search.User{Login: "cli"},
					},
				},
			}, nil
		},
	}
	manager := &extensions.ExtensionManagerMock{
		ListFunc: func() []extensions.Extension {
			return nil
		},
	}
	var ranModel *browseModel

	// When the extension browser starts
	err := ExtBrowse(ExtBrowseOpts{
		IO:       ios,
		Searcher: searcher,
		Em:       manager,
		Client:   http.DefaultClient,
		Cfg:      cfg,
		runProgram: func(model tea.Model, _ ...tea.ProgramOption) error {
			var ok bool
			ranModel, ok = model.(*browseModel)
			require.True(t, ok, "expected ExtBrowse to run a Bubble Tea browse model")
			return nil
		},
	})

	// Then the Bubble Tea model contains the loaded extension
	require.NoError(t, err)
	require.NotNil(t, ranModel)
	selected, ok := ranModel.selectedEntry()
	require.True(t, ok, "expected the available extension to be selected")
	assert.Equal(t, "cli/gh-cool", selected.FullName)
}

func TestExtensionDescriptionFallback(t *testing.T) {
	t.Parallel()

	// Given an extension has no repository description
	entry := extEntry{FullName: "octo/gh-example"}

	// When its display description is requested
	description := entry.Description()

	// Then a concrete fallback is returned
	assert.Equal(t, "no description provided", description)
}
