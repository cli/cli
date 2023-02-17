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
		includeOwner bool
	}
	tests := []struct {
		name   string
		args   args
		fields fields
		want   string
	}{
		{
			name: "No included name or gitstatus",
			args: args{},
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
			want: "cli/cli (trunk): scuba steve",
		},
		{
			name: "No included name - included gitstatus - no unsaved changes",
			args: args{},
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
			want: "cli/cli (trunk): scuba steve",
		},
		{
			name: "No included name - included gitstatus - unsaved changes",
			args: args{},
			fields: fields{
				Codespace: &api.Codespace{
					GitStatus: api.CodespaceGitStatus{
						Ref:                   "trunk",
						HasUncommittedChanges: true,
					},
					Repository: api.Repository{
						FullName: "cli/cli",
					},
					DisplayName: "scuba steve",
				},
			},
			want: "cli/cli (trunk*): scuba steve",
		},
		{
			name: "Included name - included gitstatus - unsaved changes",
			args: args{},
			fields: fields{
				Codespace: &api.Codespace{
					GitStatus: api.CodespaceGitStatus{
						Ref:                   "trunk",
						HasUncommittedChanges: true,
					},
					Repository: api.Repository{
						FullName: "cli/cli",
					},
					DisplayName: "scuba steve",
				},
			},
			want: "cli/cli (trunk*): scuba steve",
		},
		{
			name: "Included name - included gitstatus - no unsaved changes",
			args: args{},
			fields: fields{
				Codespace: &api.Codespace{
					GitStatus: api.CodespaceGitStatus{
						Ref:                   "trunk",
						HasUncommittedChanges: false,
					},
					Repository: api.Repository{
						FullName: "cli/cli",
					},
					DisplayName: "scuba steve",
				},
			},
			want: "cli/cli (trunk): scuba steve",
		},
		{
			name: "with includeOwner true, prefixes the codespace owner",
			args: args{
				includeOwner: true,
			},
			fields: fields{
				Codespace: &api.Codespace{
					Owner: api.User{
						Login: "jimmy",
					},
					GitStatus: api.CodespaceGitStatus{
						Ref:                   "trunk",
						HasUncommittedChanges: false,
					},
					Repository: api.Repository{
						FullName: "cli/cli",
					},
					DisplayName: "scuba steve",
				},
			},
			want: "jimmy           cli/cli (trunk): scuba steve",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := codespace{
				Codespace: tt.fields.Codespace,
			}
			if got := c.displayName(tt.args.includeOwner); got != tt.want {
				t.Errorf("codespace.displayName(includeOwnewr) = %v, want %v", got, tt.want)
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
				"cli/cli (trunk): scuba steve",
			},
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
				"cli/cli (trunk): scuba steve",
				"cli/cli (trunk): flappy bird",
			},
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
				"cli/cli (trunk): scuba steve",
				"cli/cli (feature): flappy bird",
			},
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
				"github/cli (trunk): scuba steve",
				"cli/cli (trunk): flappy bird",
			},
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
							Ref:                   "trunk",
							HasUncommittedChanges: true,
						},
						Repository: api.Repository{
							FullName: "cli/cli",
						},
						DisplayName: "flappy bird",
					},
				},
			},
			wantCodespacesNames: []string{
				"cli/cli (trunk): scuba steve",
				"cli/cli (trunk*): flappy bird",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCodespacesNames := formatCodespacesForSelect(tt.args.codespaces, false)

			if !reflect.DeepEqual(gotCodespacesNames, tt.wantCodespacesNames) {
				t.Errorf("codespacesNames: got %v, want %v", gotCodespacesNames, tt.wantCodespacesNames)
			}
		})
	}
}
