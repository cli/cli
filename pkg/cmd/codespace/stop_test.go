package codespace

import (
	"context"
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestApp_StopCodespace(t *testing.T) {
	type fields struct {
		apiClient apiClient
	}
	tests := []struct {
		name   string
		fields fields
		opts   *stopOptions
	}{
		{
			name: "Stop a codespace I own",
			opts: &stopOptions{
				selector: &CodespaceSelector{codespaceName: "test-codespace"},
			},
			fields: fields{
				apiClient: &apiClientMock{
					GetCodespaceFunc: func(ctx context.Context, name string, includeConnection bool) (*api.Codespace, error) {
						if name != "test-codespace" {
							return nil, fmt.Errorf("got codespace name %s, wanted %s", name, "test-codespace")
						}

						return &api.Codespace{
							State: api.CodespaceStateAvailable,
						}, nil
					},
					StopCodespaceFunc: func(ctx context.Context, name string, orgName string, userName string) error {
						if name != "test-codespace" {
							return fmt.Errorf("got codespace name %s, wanted %s", name, "test-codespace")
						}

						if orgName != "" {
							return fmt.Errorf("got orgName %s, expected none", orgName)
						}

						return nil
					},
				},
			},
		},
		{
			name: "Stop a codespace as an org admin",
			opts: &stopOptions{
				selector: &CodespaceSelector{codespaceName: "test-codespace"},
				orgName:  "test-org",
				userName: "test-user",
			},
			fields: fields{
				apiClient: &apiClientMock{
					GetOrgMemberCodespaceFunc: func(ctx context.Context, orgName string, userName string, codespaceName string) (*api.Codespace, error) {
						if codespaceName != "test-codespace" {
							return nil, fmt.Errorf("got codespace name %s, wanted %s", codespaceName, "test-codespace")
						}
						if orgName != "test-org" {
							return nil, fmt.Errorf("got org name %s, wanted %s", orgName, "test-org")
						}
						if userName != "test-user" {
							return nil, fmt.Errorf("got user name %s, wanted %s", userName, "test-user")
						}

						return &api.Codespace{
							State: api.CodespaceStateAvailable,
						}, nil
					},
					StopCodespaceFunc: func(ctx context.Context, codespaceName string, orgName string, userName string) error {
						if codespaceName != "test-codespace" {
							return fmt.Errorf("got codespace name %s, wanted %s", codespaceName, "test-codespace")
						}
						if orgName != "test-org" {
							return fmt.Errorf("got org name %s, wanted %s", orgName, "test-org")
						}
						if userName != "test-user" {
							return fmt.Errorf("got user name %s, wanted %s", userName, "test-user")
						}

						return nil
					},
				},
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
			err := a.StopCodespace(context.Background(), tt.opts)
			assert.NoError(t, err)
		})
	}
}
