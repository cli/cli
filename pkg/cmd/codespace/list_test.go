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
		name   string
		fields fields
		opts   *listOptions
	}{
		{
			name: "list codespaces, no flags",
			fields: fields{
				apiClient: &apiClientMock{
					ListCodespacesFunc: func(ctx context.Context, limit int, orgName string, userName string) ([]*api.Codespace, error) {
						if orgName != "" {
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
					ListCodespacesFunc: func(ctx context.Context, limit int, orgName string, userName string) ([]*api.Codespace, error) {
						if orgName != "TestOrg" {
							return nil, fmt.Errorf("Expected orgName to be TestOrg. Got %s", orgName)
						}
						if userName != "" {
							return nil, fmt.Errorf("Expected userName to be blank. Got %s", userName)
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
					ListCodespacesFunc: func(ctx context.Context, limit int, orgName string, userName string) ([]*api.Codespace, error) {
						if orgName != "TestOrg" {
							return nil, fmt.Errorf("Expected orgName to be TestOrg. Got %s", orgName)
						}
						if userName != "jimmy" {
							return nil, fmt.Errorf("Expected userName to be jimmy. Got %s", userName)
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
			if err != nil {
				t.Error(err)
			}
		})
	}
}
