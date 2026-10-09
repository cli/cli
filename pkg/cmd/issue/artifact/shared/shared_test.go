package shared

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/attachments"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/issue/artifact/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
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

func TestParseArtifactNumber(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    int
		wantErr string
	}{
		{
			name: "a number",
			arg:  "2",
			want: 2,
		},
		{
			name:    "zero",
			arg:     "0",
			wantErr: `invalid artifact number: "0"`,
		},
		{
			name:    "a negative number",
			arg:     "-2",
			wantErr: `invalid artifact number: "-2"`,
		},
		{
			name:    "a number with a hash",
			arg:     "#2",
			wantErr: `invalid artifact number: "#2"`,
		},
		{
			name:    "a name",
			arg:     "OAuth",
			wantErr: `invalid artifact number: "OAuth"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseArtifactNumber(tt.arg)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindVersion(t *testing.T) {
	// Artifact 2 on the spec's issue 142 keeps its versions newest first.
	// Version 9 renamed it.
	oauthPlan := &client.ArtifactWithVersions{
		Number: 2, Type: "generic", Name: "OAuth callback plan", Body: "# OAuth callback plan",
		Versions: []client.Version{
			{Version: 12, Name: "OAuth callback plan", Body: "# OAuth callback plan"},
			{Version: 9, Name: "OAuth callback plan", Body: "# OAuth callback plan"},
			{Version: 5, Name: "OAuth plan", Body: "# OAuth plan\n\n1. Register the callback URL."},
			{Version: 1, Name: "OAuth plan", Body: "# OAuth plan"},
		},
	}
	// An artifact from a list, which has no edit history.
	listed := &client.ArtifactWithVersions{
		Number: 5, Type: "generic", Name: "Barista feedback notes",
	}

	tests := []struct {
		name     string
		artifact *client.ArtifactWithVersions
		number   int
		want     *client.Version
		wantErr  string
	}{
		{
			name:     "0 means the current version",
			artifact: oauthPlan,
			number:   0,
		},
		{
			name:     "the current version's number means the current version",
			artifact: oauthPlan,
			number:   12,
		},
		{
			name:     "an earlier version",
			artifact: oauthPlan,
			number:   5,
			want:     &client.Version{Version: 5, Name: "OAuth plan", Body: "# OAuth plan\n\n1. Register the callback URL."},
		},
		{
			name:     "the first version",
			artifact: oauthPlan,
			number:   1,
			want:     &client.Version{Version: 1, Name: "OAuth plan", Body: "# OAuth plan"},
		},
		{
			name:     "a version the API didn't return",
			artifact: oauthPlan,
			number:   99,
			wantErr:  "version 99 not found for artifact 2",
		},
		{
			name:     "0 without an edit history",
			artifact: listed,
			number:   0,
		},
		{
			name:     "a version without an edit history",
			artifact: listed,
			number:   1,
			wantErr:  "version 1 not found for artifact 5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindVersion(tt.artifact, tt.number)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
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

func TestCheckUploadTarget(t *testing.T) {
	tests := []struct {
		name      string
		number    int
		target    *client.UploadTarget
		lookupErr error
		want      *client.UploadTarget
		wantErr   string
	}{
		{
			name:   "an issue",
			number: 142,
			target: &client.UploadTarget{RepositoryID: 1234, ViewerPermission: "WRITE"},
			want:   &client.UploadTarget{RepositoryID: 1234, ViewerPermission: "WRITE"},
		},
		{
			name:    "a pull request",
			number:  158,
			target:  &client.UploadTarget{IsPullRequest: true, RepositoryID: 1234, ViewerPermission: "WRITE"},
			wantErr: "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
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
				UploadTargetFunc: func(ghrepo.Interface, int) (*client.UploadTarget, error) {
					return tt.target, tt.lookupErr
				},
			}

			got, err := CheckUploadTarget(mock, repo, tt.number)

			require.Len(t, mock.UploadTargetCalls(), 1)
			assert.Equal(t, repo, mock.UploadTargetCalls()[0].Repo)
			assert.Equal(t, tt.number, mock.UploadTargetCalls()[0].Number)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewUploader(t *testing.T) {
	tests := []struct {
		name       string
		repo       ghrepo.Interface
		number     int
		target     *client.UploadTarget
		lookupErr  error
		httpErr    error
		token      string
		wantUpload bool
		wantErr    string
	}{
		{
			name:       "uploads against the issue's repository",
			repo:       ghrepo.New("monalisa", "monas-cafe"),
			number:     142,
			target:     &client.UploadTarget{RepositoryID: 1234, ViewerPermission: "WRITE"},
			wantUpload: true,
		},
		{
			name:       "uploads to a ghe.com host",
			repo:       ghrepo.NewWithHost("monalisa", "monas-cafe", "monas-cafe.ghe.com"),
			number:     142,
			target:     &client.UploadTarget{RepositoryID: 1234, ViewerPermission: "MAINTAIN"},
			wantUpload: true,
		},
		{
			name:    "a pull request",
			repo:    ghrepo.New("monalisa", "monas-cafe"),
			number:  158,
			target:  &client.UploadTarget{IsPullRequest: true, RepositoryID: 1234, ViewerPermission: "WRITE"},
			wantErr: "monalisa/monas-cafe#158 is a pull request; artifacts are only supported on issues",
		},
		{
			name:      "the lookup's error",
			repo:      ghrepo.New("monalisa", "monas-cafe"),
			number:    999,
			lookupErr: errors.New("GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)"),
			wantErr:   "GraphQL: Could not resolve to an issue or pull request with the number of 999. (repository.issue)",
		},
		{
			name:    "without write access",
			repo:    ghrepo.New("monalisa", "monas-cafe"),
			number:  142,
			target:  &client.UploadTarget{RepositoryID: 1234, ViewerPermission: "READ"},
			wantErr: "attaching files requires write access to the repository",
		},
		{
			name:    "a token that can't upload",
			repo:    ghrepo.New("monalisa", "monas-cafe"),
			number:  142,
			target:  &client.UploadTarget{RepositoryID: 1234, ViewerPermission: "WRITE"},
			token:   "ghs_aninstallationtoken",
			wantErr: "unsupported authentication type",
		},
		{
			name:    "the HTTP client's error",
			repo:    ghrepo.New("monalisa", "monas-cafe"),
			number:  142,
			target:  &client.UploadTarget{RepositoryID: 1234, ViewerPermission: "WRITE"},
			httpErr: errors.New("no token"),
			wantErr: "no token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assets := attachments.NewTestAssets(t, "signin-flow.png")
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.wantUpload {
				attachments.StubUploadToHost(t, reg, "uploads."+tt.repo.RepoHost(), 1234, "signin-flow.png", 200,
					`{"url": "https://github.com/user-attachments/assets/AAA"}`)
			}
			mock := &client.ArtifactClientMock{
				UploadTargetFunc: func(ghrepo.Interface, int) (*client.UploadTarget, error) {
					return tt.target, tt.lookupErr
				},
			}
			httpClient := func() (*http.Client, error) {
				if tt.httpErr != nil {
					return nil, tt.httpErr
				}
				return &http.Client{Transport: reg}, nil
			}
			cfg := func() (gh.Config, error) {
				token := tt.token
				if token == "" {
					token = "gho_atokenthatcanupload"
				}
				return config.NewMockConfigFromString(fmt.Sprintf("hosts:\n  %s:\n    user: monalisa\n    oauth_token: %s\n", tt.repo.RepoHost(), token)), nil
			}

			uploader, err := NewUploader(mock, tt.repo, tt.number, httpClient, cfg)

			require.Len(t, mock.UploadTargetCalls(), 1)
			assert.Equal(t, tt.repo, mock.UploadTargetCalls()[0].Repo)
			assert.Equal(t, tt.number, mock.UploadTargetCalls()[0].Number)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, uploader)
				return
			}
			require.NoError(t, err)
			md, _, err := uploader.UploadAndAttach(context.Background(), "", "", assets)
			require.NoError(t, err)
			assert.Equal(t, "![signin-flow](https://github.com/user-attachments/assets/AAA)", md)
		})
	}
}

func TestCheckType(t *testing.T) {
	tests := []struct {
		name         string
		artifactType string
		wantErr      string
	}{
		{
			name:         "generic",
			artifactType: "generic",
		},
		{
			name:         "plan",
			artifactType: "plan",
		},
		{
			name:         "link",
			artifactType: "link",
		},
		{
			name:         "a type gh doesn't know",
			artifactType: "bad-type",
			wantErr:      `artifact 7 has type "bad-type", which this version of gh doesn't support; upgrade gh`,
		},
		{
			name:         "an empty type",
			artifactType: "",
			wantErr:      `artifact 7 has type "", which this version of gh doesn't support; upgrade gh`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckType(client.Artifact{Number: 7, Type: tt.artifactType, Name: "Espresso checklist"})

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestHTTPURL(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		want   string
		wantOK bool
	}{
		{
			name:   "an https URL",
			value:  "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook",
			want:   "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook",
			wantOK: true,
		},
		{
			name:   "an http URL with a port, query and fragment",
			value:  "http://staging.monas-cafe.example:8080/menu?day=monday#specials",
			want:   "http://staging.monas-cafe.example:8080/menu?day=monday#specials",
			wantOK: true,
		},
		{
			name:   "surrounding whitespace is trimmed",
			value:  " \thttps://github.com/monalisa/monas-cafe/wiki/OAuth-runbook\r\n",
			want:   "https://github.com/monalisa/monas-cafe/wiki/OAuth-runbook",
			wantOK: true,
		},
		{
			name:   "an uppercase scheme",
			value:  "HTTPS://github.com/monalisa/monas-cafe",
			want:   "HTTPS://github.com/monalisa/monas-cafe",
			wantOK: true,
		},
		{
			name:  "another scheme",
			value: "ftp://monas-cafe.example/menu.pdf",
		},
		{
			name:  "a script",
			value: "javascript:alert(1)",
		},
		{
			name:  "a local file",
			value: "file:///etc/hosts",
		},
		{
			name:  "no host",
			value: "https:///monalisa/monas-cafe",
		},
		{
			name:  "no slashes after the scheme",
			value: "https:github.com/monalisa/monas-cafe",
		},
		{
			name:  "a relative URL",
			value: "/monalisa/monas-cafe/wiki/OAuth-runbook",
		},
		{
			name:  "a space inside",
			value: "https://github.com/monalisa/monas cafe",
		},
		{
			name:  "a line break inside",
			value: "https://github.com/monalisa/monas-cafe\nURL=file:///etc/hosts",
		},
		{
			name:  "a control character",
			value: "https://github.com/monalisa/monas-cafe\x1b[31m",
		},
		{
			name:  "a non-ASCII host",
			value: "https://ラテ.example/menu",
		},
		{
			name:  "an invalid escape",
			value: "https://github.com/monalisa/monas-cafe/%zz",
		},
		{
			name:  "empty",
			value: "  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := HTTPURL(tt.value)

			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
