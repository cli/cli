package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArtifactExportData(t *testing.T) {
	createdAt := time.Date(2026, 9, 21, 23, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 9, 24, 21, 0, 0, 0, time.UTC)

	full := Artifact{
		ID:             6626,
		Number:         2,
		Type:           "generic",
		Name:           "OAuth callback plan",
		Body:           "# OAuth callback plan",
		BodyHTML:       "<h1>OAuth callback plan</h1>",
		Description:    "Register the callback URL.",
		Creator:        &Actor{ID: 2, Login: "hubot"},
		UpdatedByActor: &Actor{ID: 1, Login: "monalisa"},
		CreatedAt:      &createdAt,
		UpdatedAt:      &updatedAt,
	}

	tests := []struct {
		name     string
		artifact Artifact
		fields   []string
		wantJSON string
	}{
		{
			name:     "every field",
			artifact: full,
			fields:   ArtifactFields,
			wantJSON: `{
				"body": "# OAuth callback plan",
				"bodyHtml": "<h1>OAuth callback plan</h1>",
				"createdAt": "2026-09-21T23:00:00Z",
				"creator": {"id": 2, "login": "hubot"},
				"description": "Register the callback URL.",
				"id": 6626,
				"name": "OAuth callback plan",
				"number": 2,
				"type": "generic",
				"updatedAt": "2026-09-24T21:00:00Z",
				"updatedByActor": {"id": 1, "login": "monalisa"}
			}`,
		},
		{
			name:     "null users and timestamps stay null",
			artifact: Artifact{ID: 6627, Number: 3, Type: "link", Name: "Staging OAuth runbook"},
			fields:   []string{"creator", "updatedByActor", "createdAt", "updatedAt"},
			wantJSON: `{
				"createdAt": null,
				"creator": null,
				"updatedAt": null,
				"updatedByActor": null
			}`,
		},
		{
			name:     "only the requested fields",
			artifact: full,
			fields:   []string{"number", "name", "type"},
			wantJSON: `{"name": "OAuth callback plan", "number": 2, "type": "generic"}`,
		},
		{
			name:     "a stored type gh doesn't know",
			artifact: Artifact{Number: 7, Type: "bad-type", Name: "Unknown"},
			fields:   []string{"number", "type"},
			wantJSON: `{"number": 7, "type": "bad-type"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.artifact.ExportData(tt.fields))
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(got))
		})
	}
}

func TestArtifactWithVersionsExportData(t *testing.T) {
	createdAt := time.Date(2026, 9, 21, 23, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 9, 24, 21, 0, 0, 0, time.UTC)

	full := ArtifactWithVersions{
		ID:             6626,
		Number:         2,
		Type:           "generic",
		Name:           "OAuth callback plan",
		Body:           "# OAuth callback plan",
		BodyHTML:       "<h1>OAuth callback plan</h1>",
		Description:    "Register the callback URL.",
		Creator:        &Actor{ID: 2, Login: "hubot"},
		UpdatedByActor: &Actor{ID: 1, Login: "monalisa"},
		CreatedAt:      &createdAt,
		UpdatedAt:      &updatedAt,
		Versions: []Version{
			{
				Version:   2,
				Name:      "OAuth callback plan",
				Body:      "# OAuth callback plan",
				BodyHTML:  "<h1>OAuth callback plan</h1>",
				CreatedAt: &updatedAt,
				Actor:     &Actor{ID: 1, Login: "monalisa"},
			},
			{
				Version:   1,
				Name:      "OAuth plan",
				Body:      "# OAuth plan",
				BodyHTML:  "<h1>OAuth plan</h1>",
				CreatedAt: &createdAt,
				Actor:     &Actor{ID: 2, Login: "hubot"},
			},
		},
	}

	tests := []struct {
		name     string
		artifact ArtifactWithVersions
		fields   []string
		wantJSON string
	}{
		{
			name:     "every field, with versions newest first",
			artifact: full,
			fields:   ArtifactWithVersionsFields,
			wantJSON: `{
				"body": "# OAuth callback plan",
				"bodyHtml": "<h1>OAuth callback plan</h1>",
				"createdAt": "2026-09-21T23:00:00Z",
				"creator": {"id": 2, "login": "hubot"},
				"description": "Register the callback URL.",
				"id": 6626,
				"name": "OAuth callback plan",
				"number": 2,
				"type": "generic",
				"updatedAt": "2026-09-24T21:00:00Z",
				"updatedByActor": {"id": 1, "login": "monalisa"},
				"versions": [
					{
						"version": 2,
						"name": "OAuth callback plan",
						"body": "# OAuth callback plan",
						"bodyHtml": "<h1>OAuth callback plan</h1>",
						"createdAt": "2026-09-24T21:00:00Z",
						"actor": {"id": 1, "login": "monalisa"}
					},
					{
						"version": 1,
						"name": "OAuth plan",
						"body": "# OAuth plan",
						"bodyHtml": "<h1>OAuth plan</h1>",
						"createdAt": "2026-09-21T23:00:00Z",
						"actor": {"id": 2, "login": "hubot"}
					}
				]
			}`,
		},
		{
			name:     "only the requested fields",
			artifact: full,
			fields:   []string{"number", "name"},
			wantJSON: `{"name": "OAuth callback plan", "number": 2}`,
		},
		{
			name: "a version's null actor and timestamp stay null",
			artifact: ArtifactWithVersions{
				Artifact: Artifact{Number: 2, Type: "generic", Name: "OAuth plan"},
				Versions: []Version{{Version: 1, Name: "OAuth plan", Body: "# OAuth plan"}},
			},
			fields: []string{"versions"},
			wantJSON: `{"versions": [
				{"version": 1, "name": "OAuth plan", "body": "# OAuth plan", "bodyHtml": "", "createdAt": null, "actor": null}
			]}`,
		},
		{
			name:     "no versions is an empty array",
			artifact: ArtifactWithVersions{Artifact: Artifact{Number: 2, Type: "generic", Name: "OAuth plan"}},
			fields:   []string{"number", "versions"},
			wantJSON: `{"number": 2, "versions": []}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.artifact.ExportData(tt.fields))
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(got))
		})
	}
}
