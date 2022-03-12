package codespace

import (
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
