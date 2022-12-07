package codespace

import (
	"context"
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestApp_List(t *testing.T) {
	type fields struct {
		apiClient apiClient
	}
	tests := []struct {
		name      string
		fields    fields
		opts      *listOptions
		wantError error
	}{
		{
			name: "list codespaces, no flags",
			fields: fields{
				apiClient: &apiClientMock{
					ListCodespacesFunc: func(ctx context.Context, opts api.ListCodespacesOptions) ([]*api.Codespace, error) {
						if opts.OrgName != "" {
							return nil, fmt.Errorf("should not be called with an orgName")
						}
						return []*api.Codespace{
							{
								DisplayName: "CS1",
							},
						}, nil
					},
				},
			},
			opts: &listOptions{},
		},
		{
			name: "list codespaces, --org flag",
			fields: fields{
				apiClient: &apiClientMock{
					ListCodespacesFunc: func(ctx context.Context, opts api.ListCodespacesOptions) ([]*api.Codespace, error) {
						if opts.OrgName != "TestOrg" {
							return nil, fmt.Errorf("Expected orgName to be TestOrg. Got %s", opts.OrgName)
						}
						if opts.UserName != "" {
							return nil, fmt.Errorf("Expected userName to be blank. Got %s", opts.UserName)
						}
						return []*api.Codespace{
							{
								DisplayName: "CS1",
							},
						}, nil
					},
				},
			},
			opts: &listOptions{
				orgName: "TestOrg",
			},
		},
		{
			name: "list codespaces, --org and --user flag",
			fields: fields{
				apiClient: &apiClientMock{
					ListCodespacesFunc: func(ctx context.Context, opts api.ListCodespacesOptions) ([]*api.Codespace, error) {
						if opts.OrgName != "TestOrg" {
							return nil, fmt.Errorf("Expected orgName to be TestOrg. Got %s", opts.OrgName)
						}
						if opts.UserName != "jimmy" {
							return nil, fmt.Errorf("Expected userName to be jimmy. Got %s", opts.UserName)
						}
						return []*api.Codespace{
							{
								DisplayName: "CS1",
							},
						}, nil
					},
				},
			},
			opts: &listOptions{
				orgName:  "TestOrg",
				userName: "jimmy",
			},
		},
		{
			name: "list codespaces, --repo",
			fields: fields{
				apiClient: &apiClientMock{
					ListCodespacesFunc: func(ctx context.Context, opts api.ListCodespacesOptions) ([]*api.Codespace, error) {
						if opts.RepoName == "" {
							return nil, fmt.Errorf("Expected repository to not be nil")
						}
						if opts.RepoName != "cli/cli" {
							return nil, fmt.Errorf("Expected repository name to be cli/cli. Got %s", opts.RepoName)
						}
						if opts.OrgName != "" {
							return nil, fmt.Errorf("Expected orgName to be blank. Got %s", opts.OrgName)
						}
						if opts.UserName != "" {
							return nil, fmt.Errorf("Expected userName to be blank. Got %s", opts.UserName)
						}
						return []*api.Codespace{
							{
								DisplayName: "CS1",
							},
						}, nil
					},
				},
			},
			opts: &listOptions{
				repo: "cli/cli",
			},
		},
		{
			name: "list codespaces,--repo, --org and --user flag",
			opts: &listOptions{
				repo:     "cli/cli",
				orgName:  "TestOrg",
				userName: "jimmy",
			},
			wantError: fmt.Errorf("using `--org` or `--user` with `--repo` is not allowed"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			a := &App{
				io:        ios,
				apiClient: tt.fields.apiClient,
			}
			var exporter cmdutil.Exporter

			err := a.List(context.Background(), tt.opts, exporter)
			if (err != nil) != (tt.wantError != nil) {
				t.Errorf("error = %v, wantErr %v", err, tt.wantError)
				return
			}

			if err != nil && err.Error() != tt.wantError.Error() {
				t.Errorf("error = %v, wantErr %v", err, tt.wantError)
			}
		})
	}
}
