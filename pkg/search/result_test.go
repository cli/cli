package search

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodeExportData(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		code   Code
		output string
	}{
		{
			name:   "exports requested fields",
			fields: []string{"path", "textMatches"},
			code: Code{
				Repository: Repository{
					Name: "repo",
				},
				Path: "path",
				Name: "name",
				TextMatches: []TextMatch{
					{
						Fragment: "fragment",
						Matches: []Match{
							{
								Text: "fr",
								Indices: []int{
									0,
									1,
								},
							},
						},
						Property: "property",
						Type:     "type",
					},
				},
			},
			output: `{"path":"path","textMatches":[{"fragment":"fragment","matches":[{"indices":[0,1],"text":"fr"}],"property":"property","type":"type"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exported := tt.code.ExportData(tt.fields)
			buf := bytes.Buffer{}
			enc := json.NewEncoder(&buf)
			require.NoError(t, enc.Encode(exported))
			assert.Equal(t, tt.output, strings.TrimSpace(buf.String()))
		})
	}
}

func TestCommitExportData(t *testing.T) {
	var authoredAt = time.Date(2021, 2, 27, 11, 30, 0, 0, time.UTC)
	var committedAt = time.Date(2021, 2, 28, 12, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		fields []string
		commit Commit
		output string
	}{
		{
			name:   "exports requested fields",
			fields: []string{"author", "commit", "committer", "sha"},
			commit: Commit{
				Author:    User{Login: "foo"},
				Committer: User{Login: "bar", ID: "123"},
				Info: CommitInfo{
					Author:    CommitUser{Date: authoredAt, Name: "Foo"},
					Committer: CommitUser{Date: committedAt, Name: "Bar"},
					Message:   "test message",
				},
				Sha: "8dd03144ffdc6c0d",
			},
			output: `{"author":{"id":"","is_bot":true,"login":"app/foo","type":"","url":""},"commit":{"author":{"date":"2021-02-27T11:30:00Z","email":"","name":"Foo"},"comment_count":0,"committer":{"date":"2021-02-28T12:30:00Z","email":"","name":"Bar"},"message":"test message","tree":{"sha":""}},"committer":{"id":"123","is_bot":false,"login":"bar","type":"","url":""},"sha":"8dd03144ffdc6c0d"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exported := tt.commit.ExportData(tt.fields)
			buf := bytes.Buffer{}
			enc := json.NewEncoder(&buf)
			require.NoError(t, enc.Encode(exported))
			assert.Equal(t, tt.output, strings.TrimSpace(buf.String()))
		})
	}
}

func TestRepositoryExportData(t *testing.T) {
	var createdAt = time.Date(2021, 2, 28, 12, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		fields []string
		repo   Repository
		output string
	}{
		{
			name:   "exports requested fields",
			fields: []string{"createdAt", "description", "fullName", "isArchived", "isFork", "isPrivate", "pushedAt"},
			repo: Repository{
				CreatedAt:   createdAt,
				Description: "description",
				FullName:    "cli/cli",
				IsArchived:  true,
				IsFork:      false,
				IsPrivate:   false,
				PushedAt:    createdAt,
			},
			output: `{"createdAt":"2021-02-28T12:30:00Z","description":"description","fullName":"cli/cli","isArchived":true,"isFork":false,"isPrivate":false,"pushedAt":"2021-02-28T12:30:00Z"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exported := tt.repo.ExportData(tt.fields)
			buf := bytes.Buffer{}
			enc := json.NewEncoder(&buf)
			require.NoError(t, enc.Encode(exported))
			assert.Equal(t, tt.output, strings.TrimSpace(buf.String()))
		})
	}
}

func TestIssueExportData(t *testing.T) {
	var updatedAt = time.Date(2021, 2, 28, 12, 30, 0, 0, time.UTC)
	trueValue := true
	tests := []struct {
		name   string
		fields []string
		issue  Issue
		output string
	}{
		{
			name:   "exports requested fields",
			fields: []string{"assignees", "body", "commentsCount", "labels", "isLocked", "repository", "title", "updatedAt"},
			issue: Issue{
				Assignees:     []User{{Login: "test", ID: "123"}, {Login: "foo"}},
				Body:          "body",
				CommentsCount: 1,
				Labels:        []Label{{Name: "label1"}, {Name: "label2"}},
				IsLocked:      true,
				RepositoryURL: "https://github.com/owner/repo",
				Title:         "title",
				UpdatedAt:     updatedAt,
			},
			output: `{"assignees":[{"id":"123","is_bot":false,"login":"test","type":"","url":""},{"id":"","is_bot":true,"login":"app/foo","type":"","url":""}],"body":"body","commentsCount":1,"isLocked":true,"labels":[{"color":"","description":"","id":"","name":"label1"},{"color":"","description":"","id":"","name":"label2"}],"repository":{"name":"repo","nameWithOwner":"owner/repo"},"title":"title","updatedAt":"2021-02-28T12:30:00Z"}`,
		},
		{
			name:   "state when issue",
			fields: []string{"isPullRequest", "state"},
			issue: Issue{
				StateInternal: "closed",
			},
			output: `{"isPullRequest":false,"state":"closed"}`,
		},
		{
			name:   "state when pull request",
			fields: []string{"isPullRequest", "state"},
			issue: Issue{
				PullRequest: PullRequest{
					MergedAt: time.Now(),
					URL:      "a-url",
				},
				StateInternal: "closed",
			},
			output: `{"isPullRequest":true,"state":"merged"}`,
		},
		{
			name:   "isDraft when pull request",
			fields: []string{"isDraft", "state"},
			issue: Issue{
				PullRequest: PullRequest{
					URL: "a-url",
				},
				StateInternal: "open",
				IsDraft:       &trueValue,
			},
			output: `{"isDraft":true,"state":"open"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exported := tt.issue.ExportData(tt.fields)
			buf := bytes.Buffer{}
			enc := json.NewEncoder(&buf)
			require.NoError(t, enc.Encode(exported))
			assert.Equal(t, tt.output, strings.TrimSpace(buf.String()))
		})
	}
}
