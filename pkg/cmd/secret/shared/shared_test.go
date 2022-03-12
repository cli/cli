package shared

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetSecretEntity(t *testing.T) {
	tests := []struct {
		name        string
		orgName     string
		envName     string
		userSecrets bool
		want        SecretEntity
		wantErr     bool
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
			name:        "user",
			userSecrets: true,
			want:        User,
		},
		{
			name: "defaults to repo",
			want: Repository,
		},
		{
			name:    "Errors if both org and env are set",
			orgName: "myOrg",
			envName: "myEnv",
			wantErr: true,
		},
		{
			name:        "Errors if both org and user secrets are set",
			orgName:     "myOrg",
			userSecrets: true,
			wantErr:     true,
		},
		{
			name:        "Errors if both env and user secrets are set",
			envName:     "myEnv",
			userSecrets: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity, err := GetSecretEntity(tt.orgName, tt.envName, tt.userSecrets)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, entity)
			}
		})
	}
}

func TestGetSecretApp(t *testing.T) {
	tests := []struct {
		name   string
		app    string
		entity SecretEntity
		want   App
	}{
		{
			name: "Actions",
			app:  "actions",
			want: Actions,
		},
		{
			name: "Codespaces",
			app:  "codespaces",
			want: Codespaces,
		},
		{
			name: "Dependabot",
			app:  "dependabot",
			want: Dependabot,
		},
		{
			name:   "Defaults to Actions for repository",
			app:    "",
			entity: Repository,
			want:   Actions,
		},
		{
			name:   "Defaults to Actions for organization",
			app:    "",
			entity: Organization,
			want:   Actions,
		},
		{
			name:   "Defaults to Actions for environment",
			app:    "",
			entity: Environment,
			want:   Actions,
		},
		{
			name:   "Defaults to Codespaces for user",
			app:    "",
			entity: User,
			want:   Codespaces,
		},
		{
			name: "Unknown for invalid apps",
			app:  "invalid",
			want: Unknown,
		},
		{
			name: "case insensitive",
			app:  "ACTIONS",
			want: Actions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, GetSecretApp(tt.app, tt.entity))
		})
	}
}

func TestIsSupportedSecretEntity(t *testing.T) {
	tests := []struct {
		name                string
		app                 App
		supportedEntities   []SecretEntity
		unsupportedEntities []SecretEntity
	}{
		{
			name: "Actions",
			app:  Actions,
			supportedEntities: []SecretEntity{
				Repository,
				Organization,
				Environment,
			},
			unsupportedEntities: []SecretEntity{
				User,
				Unknown,
			},
		},
		{
			name: "Codespaces",
			app:  Codespaces,
			supportedEntities: []SecretEntity{
				User,
			},
			unsupportedEntities: []SecretEntity{
				Repository,
				Organization,
				Environment,
				Unknown,
			},
		},
		{
			name: "Dependabot",
			app:  Dependabot,
			supportedEntities: []SecretEntity{
				Repository,
				Organization,
			},
			unsupportedEntities: []SecretEntity{
				Environment,
				User,
				Unknown,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, entity := range tt.supportedEntities {
				require.True(t, IsSupportedSecretEntity(tt.app, entity))
			}

			for _, entity := range tt.unsupportedEntities {
				require.False(t, IsSupportedSecretEntity(tt.app, entity))
			}
		})
	}
}
