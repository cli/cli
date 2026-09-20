package api

import "testing"

func TestPullRequestGraphQL(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		want   string
	}{
		{
			name:   "empty",
			fields: []string(nil),
			want:   "",
		},
		{
			name:   "simple fields",
			fields: []string{"number", "title"},
			want:   "number,title",
		},
		{
			name:   "fields with nested structures",
			fields: []string{"author", "assignees"},
			want:   "author{login,...on User{id,name}},assignees(first:100){nodes{id,login,name,databaseId},totalCount}",
		},
		{
			name:   "compressed query",
			fields: []string{"files"},
			want:   "files(first: 100) {nodes {additions,deletions,path,changeType}}",
		},
		{
			name:   "invalid fields",
			fields: []string{"isPinned", "stateReason", "number"},
			want:   "number",
		},
		{
			name:   "projectItems",
			fields: []string{"projectItems"},
			want:   `projectItems(first:100){nodes{id, project{id,title}, status:fieldValueByName(name: "Status") { ... on ProjectV2ItemFieldSingleSelectValue{optionId,name}}},totalCount}`,
		},
		{
			name:   "repository",
			fields: []string{"repository"},
			want:   "repository{id,name,nameWithOwner,databaseId,viewerPermission}",
		},
		{
			// headRepository shares a Go type with the selection above, which
			// carries two more fields. They are omitted from JSON when empty,
			// so adding either name here would put them back into the output
			// of `gh pr view --json headRepository`.
			name:   "headRepository",
			fields: []string{"headRepository"},
			want:   "headRepository{id,name,nameWithOwner}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PullRequestGraphQL(tt.fields); got != tt.want {
				t.Errorf("PullRequestGraphQL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIssueGraphQL(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
		want   string
	}{
		{
			name:   "empty",
			fields: []string(nil),
			want:   "",
		},
		{
			name:   "simple fields",
			fields: []string{"number", "title"},
			want:   "number,title",
		},
		{
			name:   "fields with nested structures",
			fields: []string{"author", "assignees"},
			want:   "author{login,...on User{id,name}},assignees(first:100){nodes{id,login,name,databaseId},totalCount}",
		},
		{
			name:   "compressed query",
			fields: []string{"files"},
			want:   "files(first: 100) {nodes {additions,deletions,path,changeType}}",
		},
		{
			name:   "projectItems",
			fields: []string{"projectItems"},
			want:   `projectItems(first:100){nodes{id, project{id,title}, status:fieldValueByName(name: "Status") { ... on ProjectV2ItemFieldSingleSelectValue{optionId,name}}},totalCount}`,
		},
		{
			name:   "repository",
			fields: []string{"repository"},
			want:   "repository{id,name,nameWithOwner,databaseId,viewerPermission}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IssueGraphQL(tt.fields); got != tt.want {
				t.Errorf("IssueGraphQL() = %v, want %v", got, tt.want)
			}
		})
	}
}
