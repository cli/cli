package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetVariableEntity(t *testing.T) {
	tests := []struct {
		name    string
		orgName string
		envName string
		want    VariableEntity
		wantErr bool
	}{
		{
			name:    "org",
			orgName: "myOrg",
			want:    Organization,
		},
		{
			name:    "env",
			envName: "myEnv",
			want:    Environment,
		},
		{
			name: "defaults to repo",
			want: Repository,
		},
		{
			name:    "errors when both org and env are set",
			orgName: "myOrg",
			envName: "myEnv",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity, err := GetVariableEntity(tt.orgName, tt.envName)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, entity)
			}
		})
	}
}
