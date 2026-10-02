package shared

import (
	"errors"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientFunc(t *testing.T) {
	tests := []struct {
		name       string
		httpClient func() (*http.Client, error)
		wantErr    string
	}{
		{
			name:       "creates a client",
			httpClient: func() (*http.Client, error) { return &http.Client{}, nil },
		},
		{
			name:       "returns the HTTP client's error",
			httpClient: func() (*http.Client, error) { return nil, errors.New("no token") },
			wantErr:    "no token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{HttpClient: tt.httpClient}

			c, err := ClientFunc(f)()

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, c)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, c)
		})
	}
}

func TestParseIssueArg(t *testing.T) {
	tests := []struct {
		name              string
		arg               string
		wantNumber        int
		wantRepo          string
		wantBaseRepoCalls int
		wantErr           string
	}{
		{
			name:              "a number uses the base repository",
			arg:               "142",
			wantNumber:        142,
			wantRepo:          "github.com/OWNER/REPO",
			wantBaseRepoCalls: 1,
		},
		{
			name:              "a number with a hash",
			arg:               "#142",
			wantNumber:        142,
			wantRepo:          "github.com/OWNER/REPO",
			wantBaseRepoCalls: 1,
		},
		{
			name:       "an issue URL names its repository",
			arg:        "https://github.com/monalisa/monas-cafe/issues/142",
			wantNumber: 142,
			wantRepo:   "github.com/monalisa/monas-cafe",
		},
		{
			name:       "an issue URL on a ghe.com host",
			arg:        "https://monas-cafe.ghe.com/monalisa/monas-cafe/issues/142",
			wantNumber: 142,
			wantRepo:   "monas-cafe.ghe.com/monalisa/monas-cafe",
		},
		{
			name:       "a pull request URL parses, so the lookup can refuse it",
			arg:        "https://github.com/monalisa/monas-cafe/pull/158",
			wantNumber: 158,
			wantRepo:   "github.com/monalisa/monas-cafe",
		},
		{
			name:    "neither a number nor a URL",
			arg:     "OAuth",
			wantErr: `invalid issue format: "OAuth"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseRepoCalls := 0
			baseRepo := func() (ghrepo.Interface, error) {
				baseRepoCalls++
				return ghrepo.New("OWNER", "REPO"), nil
			}

			number, repoFunc, err := ParseIssueArg(tt.arg, baseRepo)

			assert.Equal(t, 0, baseRepoCalls, "the base repository is resolved later, by the run function")
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNumber, number)
			repo, err := repoFunc()
			require.NoError(t, err)
			assert.Equal(t, tt.wantRepo, repo.RepoHost()+"/"+ghrepo.FullName(repo))
			assert.Equal(t, tt.wantBaseRepoCalls, baseRepoCalls)
		})
	}
}

func TestCheckHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr string
	}{
		{
			name: "github.com",
			host: "github.com",
		},
		{
			name: "a ghe.com host",
			host: "monas-cafe.ghe.com",
		},
		{
			name:    "GitHub Enterprise Server",
			host:    "ghes.monas-cafe.example",
			wantErr: "issue artifacts are not supported on GitHub Enterprise Server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckHost(tt.host)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCheckIssue(t *testing.T) {
	tests := []struct {
		name          string
		number        int
		isPullRequest bool
		lookupErr     error
		wantErr       string
	}{
		{
			name:   "an issue",
			number: 142,
		},
		{
			name:          "a pull request",
			number:        158,
			isPullRequest: true,
			wantErr:       "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
		},
		{
			name:      "the lookup's error",
			number:    999,
			lookupErr: errors.New("GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)"),
			wantErr:   "GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := ghrepo.New("monalisa", "monas-cafe")
			mock := &client.ArtifactClientMock{
				IsPullRequestFunc: func(ghrepo.Interface, int) (bool, error) {
					return tt.isPullRequest, tt.lookupErr
				},
			}

			err := CheckIssue(mock, repo, tt.number)

			require.Len(t, mock.IsPullRequestCalls(), 1)
			assert.Equal(t, repo, mock.IsPullRequestCalls()[0].Repo)
			assert.Equal(t, tt.number, mock.IsPullRequestCalls()[0].Number)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
