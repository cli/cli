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
			want:   "author{login},assignees(first:100){nodes{id,login,name},totalCount}",
		},
		{
			name:   "compressed query",
			fields: []string{"files"},
			want:   "files(first: 100) {nodes {additions,deletions,path}}",
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
