package shared

import (
	"reflect"
	"strings"
	"time"
)

type Environment struct {
	Id                     int                    `json:"id"`
	NodeId                 string                 `json:"node_id"`
	Name                   string                 `json:"name"`
	Url                    string                 `json:"url"`
	HtmlUrl                string                 `json:"html_url"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
	CanAdminBypass         bool                   `json:"can_admins_bypass"`
	ProtectionRules        []ProtectionRule       `json:"protection_rules"`
	DeploymentBranchPolicy DeploymentBranchPolicy `json:"deployment_branch_policy"`
	SecretsTotalCount      int
	VariablesTotalCount    int
}

type ProtectionRule struct {
	Id     int    `json:"id"`
	NodeId string `json:"node_id"`
	Name   string `json:"name"`
}

type DeploymentBranchPolicy struct {
	ProtectedBranches    bool `json:"protected_branches"`
	CustomBranchPolicies bool `json:"custom_branch_policies"`
}

type EnvironmentPayload struct {
	Environments []Environment `json:"environments"`
}

var EnvironmentFields = []string{
	"id",
	"name",
	"nodeId",
	"url",
	"htmlUrl",
	"createdAt",
	"updatedAt",
	"canAdminBypass",
	"protectionRules",
	"protectedBranches",
	"customBranchPolicies",
}

func (e *Environment) ExportData(fields []string) map[string]interface{} {
	v := reflect.ValueOf(e).Elem()
	data := map[string]interface{}{}
	for _, f := range fields {
		switch f {
		case "protectedBranches":
			data[f] = e.DeploymentBranchPolicy.ProtectedBranches
		case "customBranchPolicies":
			data[f] = e.DeploymentBranchPolicy.CustomBranchPolicies
		default:
			sf := v.FieldByNameFunc(func(s string) bool {
				return strings.EqualFold(f, s)
			})
			data[f] = sf.Interface()
		}
	}
	return data
}
