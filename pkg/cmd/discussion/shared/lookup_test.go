package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDiscussionArg(t *testing.T) {
	tests := []struct {
		name      string
		arg       string
		wantNum   int32
		wantOwner string
		wantRepo  string
		wantHost  string
		wantErr   string
	}{
		{
			name:    "empty",
			arg:     "",
			wantErr: `invalid discussion argument: ""`,
		},
		{
			name:    "whitespaces",
			arg:     "  ",
			wantErr: `invalid discussion argument: "  "`,
		},
		{
			name:    "invalid string",
			arg:     "not-a-number",
			wantErr: `invalid discussion argument: "not-a-number"`,
		},
		{
			name:    "hash only",
			arg:     "#",
			wantErr: `invalid discussion argument: "#"`,
		},
		{
			name:    "hash non-numeric",
			arg:     "#abc",
			wantErr: `invalid discussion argument: "#abc"`,
		},
		{
			name:    "URL with wrong path",
			arg:     "https://github.com/owner/repo/issues/10",
			wantErr: `invalid discussion URL: "https://github.com/owner/repo/issues/10"`,
		},
		{
			name:    "URL missing number",
			arg:     "https://github.com/owner/repo/discussions/",
			wantErr: `invalid discussion URL: "https://github.com/owner/repo/discussions/"`,
		},
		{
			name:    "URL with overflowing number",
			arg:     "https://github.com/owner/repo/discussions/99999999999999999999",
			wantErr: `invalid discussion number in URL: "99999999999999999999"`,
		},
		{
			name:    "zero",
			arg:     "0",
			wantNum: 0,
		},
		{
			name:    "plain number",
			arg:     "42",
			wantNum: 42,
		},
		{
			name:    "hash number",
			arg:     "#99",
			wantNum: 99,
		},
		{
			name:      "HTTPS URL",
			arg:       "https://github.com/cli/cli/discussions/123",
			wantNum:   123,
			wantOwner: "cli",
			wantRepo:  "cli",
			wantHost:  "github.com",
		},
		{
			name:      "HTTP URL",
			arg:       "http://github.com/owner/repo/discussions/7",
			wantNum:   7,
			wantOwner: "owner",
			wantRepo:  "repo",
			wantHost:  "github.com",
		},
		{
			name:      "GHES URL",
			arg:       "https://git.example.com/org/project/discussions/55",
			wantNum:   55,
			wantOwner: "org",
			wantRepo:  "project",
			wantHost:  "git.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			num, repo, err := ParseDiscussionArg(tt.arg)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantNum, num)

			if tt.wantOwner != "" || tt.wantRepo != "" || tt.wantHost != "" {
				require.NotNil(t, repo)
				assert.Equal(t, tt.wantOwner, repo.RepoOwner())
				assert.Equal(t, tt.wantRepo, repo.RepoName())
				assert.Equal(t, tt.wantHost, repo.RepoHost())
			} else {
				assert.Nil(t, repo)
			}
		})
	}
}

func TestParseDiscussionOrCommentArg(t *testing.T) {
	tests := []struct {
		name              string
		arg               string
		wantNumber        int32
		wantOwner         string
		wantRepo          string
		wantHost          string
		wantCommentNodeID string
		wantCommentDBID   int64
		wantErr           string
	}{
		// Same cases as ParseDiscussionArg
		{
			name:    "empty",
			arg:     "",
			wantErr: `invalid argument: "" (expected a discussion number, URL, or comment ID)`,
		},
		{
			name:    "whitespaces",
			arg:     "  ",
			wantErr: `invalid argument: "  " (expected a discussion number, URL, or comment ID)`,
		},
		{
			name:    "invalid string",
			arg:     "not-a-number",
			wantErr: `invalid argument: "not-a-number" (expected a discussion number, URL, or comment ID)`,
		},
		{
			name:    "hash only",
			arg:     "#",
			wantErr: `invalid argument: "#" (expected a discussion number, URL, or comment ID)`,
		},
		{
			name:    "hash non-numeric",
			arg:     "#abc",
			wantErr: `invalid argument: "#abc" (expected a discussion number, URL, or comment ID)`,
		},
		{
			name:    "URL with wrong path",
			arg:     "https://github.com/owner/repo/issues/10",
			wantErr: `invalid discussion URL: "https://github.com/owner/repo/issues/10"`,
		},
		{
			name:    "URL missing number",
			arg:     "https://github.com/owner/repo/discussions/",
			wantErr: `invalid discussion URL: "https://github.com/owner/repo/discussions/"`,
		},
		{
			name:    "URL with overflowing number",
			arg:     "https://github.com/owner/repo/discussions/99999999999999999999",
			wantErr: `invalid discussion number in URL: "99999999999999999999"`,
		},
		{
			name:    "comment URL with invalid fragment",
			arg:     "https://github.com/owner/repo/discussions/5#discussioncomment-abc",
			wantErr: `invalid comment ID in URL fragment: "discussioncomment-abc"`,
		},
		{
			name:       "zero",
			arg:        "0",
			wantNumber: 0,
		},
		{
			name:       "plain number",
			arg:        "42",
			wantNumber: 42,
		},
		{
			name:       "hash number",
			arg:        "#99",
			wantNumber: 99,
		},
		{
			name:       "HTTPS discussion URL",
			arg:        "https://github.com/cli/cli/discussions/123",
			wantNumber: 123,
			wantOwner:  "cli",
			wantRepo:   "cli",
			wantHost:   "github.com",
		},
		{
			name:            "HTTPS comment URL",
			arg:             "https://github.com/cli/cli/discussions/123#discussioncomment-789",
			wantNumber:      123,
			wantOwner:       "cli",
			wantRepo:        "cli",
			wantHost:        "github.com",
			wantCommentDBID: 789,
		},
		{
			name:       "HTTP discussion URL",
			arg:        "http://github.com/owner/repo/discussions/7",
			wantNumber: 7,
			wantOwner:  "owner",
			wantRepo:   "repo",
			wantHost:   "github.com",
		},
		{
			name:            "HTTP comment URL",
			arg:             "http://github.com/owner/repo/discussions/7#discussioncomment-456",
			wantNumber:      7,
			wantOwner:       "owner",
			wantRepo:        "repo",
			wantHost:        "github.com",
			wantCommentDBID: 456,
		},
		{
			name:       "GHES discussion URL",
			arg:        "https://git.example.com/org/project/discussions/55",
			wantNumber: 55,
			wantOwner:  "org",
			wantRepo:   "project",
			wantHost:   "git.example.com",
		},
		{
			name:            "GHES comment URL",
			arg:             "https://git.example.com/org/project/discussions/55#discussioncomment-100",
			wantNumber:      55,
			wantOwner:       "org",
			wantRepo:        "project",
			wantHost:        "git.example.com",
			wantCommentDBID: 100,
		},
		{
			name:              "comment node ID",
			arg:               "DC_kwDOOokwWs4BBmcq",
			wantCommentNodeID: "DC_kwDOOokwWs4BBmcq",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDiscussionOrCommentArg(tt.arg)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantNumber, result.Number)
			assert.Equal(t, tt.wantCommentNodeID, result.CommentNodeID)
			assert.Equal(t, tt.wantCommentDBID, result.CommentDatabaseID)

			if tt.wantOwner != "" || tt.wantRepo != "" || tt.wantHost != "" {
				require.NotNil(t, result.Repo)
				assert.Equal(t, tt.wantOwner, result.Repo.RepoOwner())
				assert.Equal(t, tt.wantRepo, result.Repo.RepoName())
				assert.Equal(t, tt.wantHost, result.Repo.RepoHost())
			} else {
				assert.Nil(t, result.Repo)
			}
		})
	}
}
