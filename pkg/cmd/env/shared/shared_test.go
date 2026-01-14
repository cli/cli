package shared

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironment_ExportData(t *testing.T) {
	src := heredoc.Doc(
		`
		{
			"id": 123,
			"name": "dev",
			"node_id": "dev-123",
			"created_at": "2025-02-12T18:00:12.456Z",
			"updated_at": "2025-02-12T18:00:12.456Z",
			"can_admins_bypass": false,
			"protection_rules": [],
			"deployment_branch_policy": {
				"protected_branches": true,
				"custom_branch_policies": false
			}
		}
	`)

	tests := []struct {
		name       string
		fields     []string
		inputJSON  string
		outputJSON string
	}{
		{
			name: "basic",
			fields: []string{
				"id",
				"name",
			},
			inputJSON: src,
			outputJSON: heredoc.Doc(`
				{
					"id": 123,
					"name": "dev"
				}
			`),
		},
		{
			name: "full",
			fields: []string{
				"id",
				"name",
				"nodeId",
				"canAdminBypass",
				"protectionRules",
				"protectedBranches",
				"customBranchPolicies",
				"createdAt",
				"updatedAt",
			},
			inputJSON: src,
			outputJSON: heredoc.Doc(`
				{
					"canAdminBypass": false,
					"createdAt": "2025-02-12T18:00:12.456Z",
					"customBranchPolicies": false,
					"id": 123,
					"name": "dev",
					"nodeId": "dev-123",
					"protectedBranches": true,
					"protectionRules": [],
					"updatedAt": "2025-02-12T18:00:12.456Z"
				}
			`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var environment Environment
			dec := json.NewDecoder(strings.NewReader(tt.inputJSON))
			require.NoError(t, dec.Decode(&environment))

			exported := environment.ExportData(tt.fields)

			buf := bytes.Buffer{}
			enc := json.NewEncoder(&buf)
			enc.SetIndent("", "\t")
			require.NoError(t, enc.Encode(exported))
			assert.Equal(t, tt.outputJSON, buf.String())
		})
	}
}
