package queries

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOwnerIDAndType(t *testing.T) {
	tests := []struct {
		name         string
		login        string
		httpResponse string
		httpStatus   int
		wantID       string
		wantType     OwnerType
		wantErrMsg   string
	}{
		{
			name:  "user found, org NOT_FOUND",
			login: "monalisa",
			httpResponse: `{
				"data": {
					"user": {"login": "monalisa", "id": "USER_1"},
					"organization": {"login": "", "id": ""}
				},
				"errors": [
					{"type": "NOT_FOUND", "path": ["organization"], "message": "Could not resolve to an Organization"}
				]
			}`,
			httpStatus: 200,
			wantID:     "USER_1",
			wantType:   UserOwner,
		},
		{
			name:  "org found, user NOT_FOUND",
			login: "github",
			httpResponse: `{
				"data": {
					"user": {"login": "", "id": ""},
					"organization": {"login": "github", "id": "ORG_1"}
				},
				"errors": [
					{"type": "NOT_FOUND", "path": ["user"], "message": "Could not resolve to a User"}
				]
			}`,
			httpStatus: 200,
			wantID:     "ORG_1",
			wantType:   OrgOwner,
		},
		{
			name:  "both NOT_FOUND",
			login: "nonexistent",
			httpResponse: `{
				"data": {
					"user": {"login": "", "id": ""},
					"organization": {"login": "", "id": ""}
				},
				"errors": [
					{"type": "NOT_FOUND", "path": ["user"], "message": "Could not resolve to a User"},
					{"type": "NOT_FOUND", "path": ["organization"], "message": "Could not resolve to an Organization"}
				]
			}`,
			httpStatus: 200,
			wantErrMsg: "Could not resolve to a User",
		},
		{
			name:  "non-NOT_FOUND GraphQL error returns original error",
			login: "restricted-org",
			httpResponse: `{
				"data": {
					"user": {"login": "", "id": ""},
					"organization": {"login": "", "id": ""}
				},
				"errors": [
					{"type": "FORBIDDEN", "path": ["organization"], "message": "Resource not accessible"}
				]
			}`,
			httpStatus: 200,
			wantErrMsg: "Resource not accessible",
		},
		{
			name:  "NOT_FOUND for org alongside FORBIDDEN for user returns original error",
			login: "mixed-errors",
			httpResponse: `{
				"data": {
					"user": {"login": "", "id": ""},
					"organization": {"login": "", "id": ""}
				},
				"errors": [
					{"type": "FORBIDDEN", "path": ["user"], "message": "Resource not accessible"},
					{"type": "NOT_FOUND", "path": ["organization"], "message": "Could not resolve to an Organization"}
				]
			}`,
			httpStatus: 200,
			wantErrMsg: "Resource not accessible",
		},
		{
			name:  "NOT_FOUND for user alongside FORBIDDEN for org returns original error",
			login: "mixed-errors-2",
			httpResponse: `{
				"data": {
					"user": {"login": "", "id": ""},
					"organization": {"login": "", "id": ""}
				},
				"errors": [
					{"type": "NOT_FOUND", "path": ["user"], "message": "Could not resolve to a User"},
					{"type": "FORBIDDEN", "path": ["organization"], "message": "Resource not accessible"}
				]
			}`,
			httpStatus: 200,
			wantErrMsg: "Resource not accessible",
		},
		{
			name:  "no error, user result populated",
			login: "monalisa",
			httpResponse: `{
				"data": {
					"user": {"login": "monalisa", "id": "USER_1"},
					"organization": {"login": "", "id": ""}
				}
			}`,
			httpStatus: 200,
			wantID:     "USER_1",
			wantType:   UserOwner,
		},
		{
			name:  "no error, org result populated",
			login: "github",
			httpResponse: `{
				"data": {
					"user": {"login": "", "id": ""},
					"organization": {"login": "github", "id": "ORG_1"}
				}
			}`,
			httpStatus: 200,
			wantID:     "ORG_1",
			wantType:   OrgOwner,
		},
		{
			name:  "viewer login with @me",
			login: "@me",
			httpResponse: `{
				"data": {
					"viewer": {"login": "monalisa", "id": "VIEWER_1"}
				}
			}`,
			httpStatus: 200,
			wantID:     "VIEWER_1",
			wantType:   ViewerOwner,
		},
		{
			name:  "viewer login with empty string",
			login: "",
			httpResponse: `{
				"data": {
					"viewer": {"login": "monalisa", "id": "VIEWER_1"}
				}
			}`,
			httpStatus: 200,
			wantID:     "VIEWER_1",
			wantType:   ViewerOwner,
		},
		{
			name:  "NOT_FOUND for org but user ID empty returns original error",
			login: "ghost",
			httpResponse: `{
				"data": {
					"user": {"login": "", "id": ""},
					"organization": {"login": "", "id": ""}
				},
				"errors": [
					{"type": "NOT_FOUND", "path": ["organization"], "message": "Could not resolve to an Organization"}
				]
			}`,
			httpStatus: 200,
			wantErrMsg: "Could not resolve to an Organization",
		},
		{
			name:  "NOT_FOUND for user but org ID empty returns original error",
			login: "ghost",
			httpResponse: `{
				"data": {
					"user": {"login": "", "id": ""},
					"organization": {"login": "", "id": ""}
				},
				"errors": [
					{"type": "NOT_FOUND", "path": ["user"], "message": "Could not resolve to a User"}
				]
			}`,
			httpStatus: 200,
			wantErrMsg: "Could not resolve to a User",
		},
		{
			name:       "HTTP error returns error",
			login:      "monalisa",
			httpStatus: 502,
			httpResponse: `{
				"message": "Bad Gateway"
			}`,
			wantErrMsg: "non-200 OK status code",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			httpClient := &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: tt.httpStatus,
						Header: http.Header{
							"Content-Type": []string{"application/json"},
						},
						Body: io.NopCloser(strings.NewReader(tt.httpResponse)),
					}, nil
				}),
			}

			client := NewClient(httpClient, "github.com", ios)
			gotID, gotType, err := client.OwnerIDAndType(tt.login)

			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantID, gotID)
			assert.Equal(t, tt.wantType, gotType)
		})
	}
}
