package codespace

import (
	"reflect"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
)

func Test_codespace_displayName(t *testing.T) {
	type fields struct {
		Codespace *api.Codespace
	}
	type args struct {
		includeName      bool
		includeGitStatus bool
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
	}{
		{
			name: "No included name or gitstatus",
			fields: fields{
				Codespace: &api.Codespace{
					GitStatus: api.CodespaceGitStatus{
						Ref: "trunk",
					},
					Repository: api.Repository{
						FullName: "cli/cli",
					},
					DisplayName: "scuba steve",
				},
			},
			args: args{
				includeName:      false,
				includeGitStatus: false,
			},
			want: "cli/cli: trunk",
		},
		{
			name: "No included name - included gitstatus - no unsaved changes",
			fields: fields{
				Codespace: &api.Codespace{
					GitStatus: api.CodespaceGitStatus{
						Ref: "trunk",
					},
					Repository: api.Repository{
						FullName: "cli/cli",
					},
					DisplayName: "scuba steve",
				},
			},
			args: args{
				includeName:      false,
				includeGitStatus: true,
			},
			want: "cli/cli: trunk",
		},
		{
			name: "No included name - included gitstatus - unsaved changes",
			fields: fields{
				Codespace: &api.Codespace{
					GitStatus: api.CodespaceGitStatus{
						Ref:                  "trunk",
						HasUncommitedChanges: true,
					},
					Repository: api.Repository{
						FullName: "cli/cli",
					},
					DisplayName: "scuba steve",
				},
			},
			args: args{
				includeName:      false,
				includeGitStatus: true,
			},
			want: "cli/cli: trunk*",
		},
		{
			name: "Included name - included gitstatus - unsaved changes",
			fields: fields{
				Codespace: &api.Codespace{
					GitStatus: api.CodespaceGitStatus{
						Ref:                  "trunk",
						HasUncommitedChanges: true,
					},
					Repository: api.Repository{
						FullName: "cli/cli",
					},
					DisplayName: "scuba steve",
				},
			},
			args: args{
				includeName:      true,
				includeGitStatus: true,
			},
			want: "cli/cli: scuba steve (trunk*)",
		},
		{
			name: "Included name - included gitstatus - no unsaved changes",
			fields: fields{
				Codespace: &api.Codespace{
					GitStatus: api.CodespaceGitStatus{
						Ref:                  "trunk",
						HasUncommitedChanges: false,
					},
					Repository: api.Repository{
						FullName: "cli/cli",
					},
					DisplayName: "scuba steve",
				},
			},
			args: args{
				includeName:      true,
				includeGitStatus: true,
			},
			want: "cli/cli: scuba steve (trunk)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := codespace{
				Codespace: tt.fields.Codespace,
			}
			if got := c.displayName(tt.args.includeName, tt.args.includeGitStatus); got != tt.want {
				t.Errorf("codespace.displayName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_formatCodespacesForSelect(t *testing.T) {
	type args struct {
		codespaces []*api.Codespace
	}
	tests := []struct {
		name                string
		args                args
		wantCodespacesNames []string
		wantCodespacesDirty map[string]bool
	}{
		{
			name: "One codespace: Shows only repo and branch name",
			args: args{
				codespaces: []*api.Codespace{
					{
						GitStatus: api.CodespaceGitStatus{
							Ref: "trunk",
						},
						Repository: api.Repository{
							FullName: "cli/cli",
						},
						DisplayName: "scuba steve",
					},
				},
			},
			wantCodespacesNames: []string{
				"cli/cli: trunk",
			},
			wantCodespacesDirty: map[string]bool{},
		},
		{
			name: "Two codespaces on the same repo/branch: Adds the codespace's display name",
			args: args{
				codespaces: []*api.Codespace{
					{
						GitStatus: api.CodespaceGitStatus{
							Ref: "trunk",
						},
						Repository: api.Repository{
							FullName: "cli/cli",
						},
						DisplayName: "scuba steve",
					},
					{
						GitStatus: api.CodespaceGitStatus{
							Ref: "trunk",
						},
						Repository: api.Repository{
							FullName: "cli/cli",
						},
						DisplayName: "flappy bird",
					},
				},
			},
			wantCodespacesNames: []string{
				"cli/cli: scuba steve (trunk)",
				"cli/cli: flappy bird (trunk)",
			},
			wantCodespacesDirty: map[string]bool{},
		},
		{
			name: "Two codespaces on the different branches: Shows only repo and branch name",
			args: args{
				codespaces: []*api.Codespace{
					{
						GitStatus: api.CodespaceGitStatus{
							Ref: "trunk",
						},
						Repository: api.Repository{
							FullName: "cli/cli",
						},
						DisplayName: "scuba steve",
					},
					{
						GitStatus: api.CodespaceGitStatus{
							Ref: "feature",
						},
						Repository: api.Repository{
							FullName: "cli/cli",
						},
						DisplayName: "flappy bird",
					},
				},
			},
			wantCodespacesNames: []string{
				"cli/cli: trunk",
				"cli/cli: feature",
			},
			wantCodespacesDirty: map[string]bool{},
		},
		{
			name: "Two codespaces on the different repos: Shows only repo and branch name",
			args: args{
				codespaces: []*api.Codespace{
					{
						GitStatus: api.CodespaceGitStatus{
							Ref: "trunk",
						},
						Repository: api.Repository{
							FullName: "github/cli",
						},
						DisplayName: "scuba steve",
					},
					{
						GitStatus: api.CodespaceGitStatus{
							Ref: "trunk",
						},
						Repository: api.Repository{
							FullName: "cli/cli",
						},
						DisplayName: "flappy bird",
					},
				},
			},
			wantCodespacesNames: []string{
				"github/cli: trunk",
				"cli/cli: trunk",
			},
			wantCodespacesDirty: map[string]bool{},
		},
		{
			name: "Two codespaces on the same repo/branch, one dirty: Adds the codespace's display name and *",
			args: args{
				codespaces: []*api.Codespace{
					{
						GitStatus: api.CodespaceGitStatus{
							Ref: "trunk",
						},
						Repository: api.Repository{
							FullName: "cli/cli",
						},
						DisplayName: "scuba steve",
					},
					{
						GitStatus: api.CodespaceGitStatus{
							Ref:                  "trunk",
							HasUncommitedChanges: true,
						},
						Repository: api.Repository{
							FullName: "cli/cli",
						},
						DisplayName: "flappy bird",
					},
				},
			},
			wantCodespacesNames: []string{
				"cli/cli: scuba steve (trunk)",
				"cli/cli: flappy bird (trunk*)",
			},
			wantCodespacesDirty: map[string]bool{
				"cli/cli: flappy bird (trunk*)": true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCodespacesNames, gotCodespacesDirty, _ := formatCodespacesForSelect(tt.args.codespaces)

			if !reflect.DeepEqual(gotCodespacesNames, tt.wantCodespacesNames) {
				t.Errorf("codespacesNames: got %v, want %v", gotCodespacesNames, tt.wantCodespacesNames)
			}
			if !reflect.DeepEqual(gotCodespacesDirty, tt.wantCodespacesDirty) {
				t.Errorf("codespacesDirty: got %v, want %v", gotCodespacesDirty, tt.wantCodespacesDirty)
			}
		})
	}
}
