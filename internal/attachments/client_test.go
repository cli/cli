package attachments

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAsset stubs file opening so upload reads body without touching the
// filesystem.
func testAsset(t *testing.T, name, contentType, body string) UserAsset {
	t.Helper()

	previousOpenFile := openFile
	openFile = func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(body)), nil
	}
	t.Cleanup(func() { openFile = previousOpenFile })

	info, err := fs.Stat(fstest.MapFS{
		name: &fstest.MapFile{Data: []byte(body)},
	}, name)
	require.NoError(t, err)

	base := asset{path: "./" + name, info: info, contentType: contentType}
	if strings.HasPrefix(contentType, "video/") {
		return &videoAsset{base}
	}
	return &imageAsset{base}
}

// newTestUploader builds an uploader whose requests land in reg.
func newTestUploader(t *testing.T, reg *httpmock.Registry, host string, targetRepository int64) *Uploader {
	t.Helper()

	uploader, err := NewUploader(&http.Client{Transport: reg}, gh.TokenTypeOAuth, host, targetRepository, "WRITE")
	require.NoError(t, err)
	return uploader
}

type staticTokenConfig map[string]string

func (c staticTokenConfig) ActiveToken(host string) (string, string) {
	return c[host], "oauth_token"
}

func TestUpload(t *testing.T) {
	a := testAsset(t, "shot.png", "image/png", "the bytes")

	var gotBody []byte
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "user-attachments/assets"),
		func(req *http.Request) (*http.Response, error) {
			var err error
			gotBody, err = io.ReadAll(req.Body)
			require.NoError(t, err)
			return httpmock.StatusStringResponse(201, `{"url":"https://github.com/user-attachments/assets/be9b3920"}`)(req)
		},
	)

	assetURL, err := newTestUploader(t, reg, "github.com", 1234).upload(context.Background(), a)

	require.NoError(t, err)
	assert.Equal(t, "https://github.com/user-attachments/assets/be9b3920", assetURL)
	assert.Equal(t, "the bytes", string(gotBody))

	require.Len(t, reg.Requests, 1)
	req := reg.Requests[0]
	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "https://uploads.github.com/user-attachments/assets?content_type=image%2Fpng&name=shot.png&repository_id=1234", req.URL.String())
	assert.Equal(t, "application/octet-stream", req.Header.Get("Content-Type"))
	assert.Equal(t, "application/vnd.github+json", req.Header.Get("Accept"))
	assert.Equal(t, int64(9), req.ContentLength)
}

func TestUploadHostForTenant(t *testing.T) {
	a := testAsset(t, "shot.png", "image/png", "x")

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "user-attachments/assets"),
		httpmock.StatusStringResponse(201, `{"url":"https://acme.ghe.com/user-attachments/assets/1"}`),
	)

	_, err := newTestUploader(t, reg, "acme.ghe.com", 1).upload(context.Background(), a)

	require.NoError(t, err)
	require.Len(t, reg.Requests, 1)
	assert.Equal(t, "uploads.acme.ghe.com", reg.Requests[0].URL.Host)
}

func TestUploadAuthentication(t *testing.T) {
	tests := []struct {
		name              string
		configuredHost    string
		configuredToken   string
		wantRequestHost   string
		wantAuthorization string
	}{
		{
			name:              "GitHub upload host uses github.com token",
			configuredHost:    "github.com",
			configuredToken:   "github-token",
			wantRequestHost:   "uploads.github.com",
			wantAuthorization: "token github-token",
		},
		{
			name:              "tenant upload host uses tenant token",
			configuredHost:    "acme.ghe.com",
			configuredToken:   "tenant-token",
			wantRequestHost:   "uploads.acme.ghe.com",
			wantAuthorization: "token tenant-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := testAsset(t, "shot.png", "image/png", "x")

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(
				httpmock.REST("POST", "user-attachments/assets"),
				httpmock.StatusStringResponse(201, `{"url":"https://github.com/user-attachments/assets/1"}`),
			)

			client := &http.Client{
				Transport: api.AddAuthTokenHeader(
					reg,
					staticTokenConfig{tt.configuredHost: tt.configuredToken},
				),
			}
			uploader, err := NewUploader(client, gh.TokenTypeOAuth, tt.configuredHost, 1, "WRITE")
			require.NoError(t, err)

			_, err = uploader.upload(context.Background(), a)

			require.NoError(t, err)
			require.Len(t, reg.Requests, 1)
			assert.Equal(t, tt.wantRequestHost, reg.Requests[0].URL.Host)
			assert.Equal(t, tt.wantAuthorization, reg.Requests[0].Header.Get("Authorization"))
		})
	}
}

func TestUploadErrors(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		contentType string
		status      int
		response    string
		retryAfter  string
		// nonJSON sends response bytes verbatim instead of marshaling them as
		// json.RawMessage. The "unreadable response" row uses it so the decoder
		// receives invalid JSON rather than an empty body from a failed marshal.
		nonJSON        bool
		wantErr        string
		wantErrPrefix  string
		wantStatusCode int
	}{
		{
			name:           "no write access",
			file:           "login.png",
			contentType:    "image/png",
			status:         404,
			response:       `{"message":"Not Found"}`,
			wantErr:        "could not upload ./login.png: attaching files requires write access to the repository",
			wantStatusCode: 404,
		},
		{
			// The server's limit for a video depends on an account plan gh
			// cannot see, so its own words are what a user can act on.
			name:           "rejected as too large",
			file:           "clip.mp4",
			contentType:    "video/mp4",
			status:         413,
			response:       `{"message":"Payload Too Large"}`,
			wantErrPrefix:  "failed to upload ./clip.mp4: HTTP 413: Payload Too Large",
			wantStatusCode: 413,
		},
		{
			name:        "rejected with one stated cause",
			file:        "clip.mp4",
			contentType: "video/mp4",
			status:      422,
			response: `{"message":"Validation Failed","errors":[
				{"field":"content_type","message":"content_type is not included in the list of allowed content types"}]}`,
			wantErr:        "could not upload ./clip.mp4: Validation Failed; content_type is not included in the list of allowed content types",
			wantStatusCode: 422,
		},
		{
			name:        "rejected with several stated causes",
			file:        "shot.png",
			contentType: "image/png",
			status:      422,
			response: `{"message":"Validation Failed","errors":[
				{"field":"content_type","message":"content_type is not included in the list of allowed content types"},
				{"field":"name","message":"name has a file extension that does not match the content type"}]}`,
			wantErr:        "could not upload ./shot.png: Validation Failed; content_type is not included in the list of allowed content types; name has a file extension that does not match the content type",
			wantStatusCode: 422,
		},
		{
			name:           "rejected with a cause that carries only a code",
			file:           "shot.png",
			contentType:    "image/png",
			status:         422,
			response:       `{"message":"Validation Failed","errors":[{"resource":"Asset","field":"name","code":"invalid"}]}`,
			wantErr:        "could not upload ./shot.png: Validation Failed; Asset.name is invalid",
			wantStatusCode: 422,
		},
		{
			name:           "rejected with no stated cause",
			file:           "shot.png",
			contentType:    "image/png",
			status:         422,
			response:       `{"message":"Validation Failed","errors":[]}`,
			wantErr:        "could not upload ./shot.png: Validation Failed",
			wantStatusCode: 422,
		},
		{
			name:           "rejected and saying nothing",
			file:           "shot.png",
			contentType:    "image/png",
			status:         422,
			response:       `{}`,
			wantErr:        "could not upload ./shot.png",
			wantStatusCode: 422,
		},
		{
			name:           "rate limited",
			file:           "shot.png",
			contentType:    "image/png",
			status:         429,
			response:       `{"message":"Too Many Requests"}`,
			wantErr:        "could not upload ./shot.png: rate limited; wait and try again",
			wantStatusCode: 429,
		},
		{
			name:           "rate limited with retry window",
			file:           "shot.png",
			contentType:    "image/png",
			status:         429,
			response:       `{"message":"Too Many Requests"}`,
			retryAfter:     "120",
			wantErr:        "could not upload ./shot.png: rate limited; retry after 120 seconds",
			wantStatusCode: 429,
		},
		{
			name:           "rate limited until a date",
			file:           "shot.png",
			contentType:    "image/png",
			status:         429,
			response:       `{"message":"Too Many Requests"}`,
			retryAfter:     "Wed, 21 Oct 2015 07:28:00 GMT",
			wantErr:        "could not upload ./shot.png: rate limited; retry after Wed, 21 Oct 2015 07:28:00 GMT",
			wantStatusCode: 429,
		},
		{
			name:           "server error",
			file:           "shot.png",
			contentType:    "image/png",
			status:         500,
			response:       `{"message":"Internal Server Error"}`,
			wantErrPrefix:  "failed to upload ./shot.png: ",
			wantStatusCode: 500,
		},
		{
			name:        "no asset URL in the response",
			file:        "shot.png",
			contentType: "image/png",
			status:      201,
			response:    `{}`,
			wantErr:     "failed to upload ./shot.png: the server returned no asset URL",
		},
		{
			name:          "unreadable response",
			file:          "shot.png",
			contentType:   "image/png",
			status:        201,
			response:      `not json`,
			nonJSON:       true,
			wantErrPrefix: "failed to upload ./shot.png: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := testAsset(t, tt.file, tt.contentType, "x")

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			responder := httpmock.StatusJSONResponse(tt.status, json.RawMessage(tt.response))
			if tt.nonJSON {
				responder = httpmock.StatusStringResponse(tt.status, tt.response)
			}
			if tt.retryAfter != "" {
				next := responder
				responder = func(req *http.Request) (*http.Response, error) {
					resp, err := next(req)
					if err == nil {
						resp.Header.Set("Retry-After", tt.retryAfter)
					}
					return resp, err
				}
			}
			reg.Register(
				httpmock.REST("POST", "user-attachments/assets"),
				responder,
			)

			_, err := newTestUploader(t, reg, "github.com", 1).upload(context.Background(), a)

			require.Error(t, err)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.Contains(t, err.Error(), tt.wantErrPrefix)
			}
			assert.NotContains(t, err.Error(), "\n")

			var uploadErr *uploadError
			require.True(t, errors.As(err, &uploadErr))
			assert.Equal(t, tt.wantStatusCode, uploadErr.StatusCode)
			assert.Equal(t, "./"+tt.file, uploadErr.Path)
			// One attempt, whatever the failure. An upload cannot be deleted,
			// so a retry that the server had already accepted would orphan an
			// asset nobody can remove.
			assert.Len(t, reg.Requests, 1)
		})
	}
}

// A file can be removed between the validation that accepted it and the upload
// that opens it, which is the only way a resolved asset reaches the endpoint
// with nothing behind it.
func TestUploadMissingFile(t *testing.T) {
	a := testAsset(t, "gone.png", "image/png", "the bytes")
	openFile = func(path string) (io.ReadCloser, error) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	_, err := newTestUploader(t, reg, "github.com", 1).upload(context.Background(), a)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to upload ./gone.png: ")
	assert.Empty(t, reg.Requests)
}

func TestNewUploader(t *testing.T) {
	const badTokenErr = "unsupported authentication type"
	const badPermissionErr = "attaching files requires write access to the repository"

	tests := []struct {
		name             string
		tokenType        gh.TokenType
		host             string
		targetRepository int64
		viewerPermission string
		wantErr          string
	}{
		{
			name:             "github.com",
			tokenType:        gh.TokenTypeOAuth,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "WRITE",
		},
		{
			name:             "data residency tenant",
			tokenType:        gh.TokenTypeOAuth,
			host:             "acme.ghe.com",
			targetRepository: 7,
			viewerPermission: "WRITE",
		},
		{
			name:             "classic personal access token",
			tokenType:        gh.TokenTypePersonalAccess,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "WRITE",
		},
		{
			name:             "fine-grained personal access token",
			tokenType:        gh.TokenTypeFineGrainedPAT,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "WRITE",
		},
		{
			name:             "rejects an enterprise server host",
			tokenType:        gh.TokenTypeOAuth,
			host:             "github.example.com",
			targetRepository: 42,
			viewerPermission: "WRITE",
			wantErr:          "attaching files is not supported on GitHub Enterprise Server",
		},
		{
			// This row defends a message, not an order. On an enterprise server
			// the token message would tell the user to re-authenticate, and that
			// remedy does not work there. The other checks are free to move.
			name:             "reports the host before the token",
			tokenType:        gh.TokenTypeServerToServer,
			host:             "github.example.com",
			targetRepository: 42,
			viewerPermission: "WRITE",
			wantErr:          "attaching files is not supported on GitHub Enterprise Server",
		},
		{
			name:             "rejects an App user-to-server token",
			tokenType:        gh.TokenTypeUserToServer,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "WRITE",
			wantErr:          badTokenErr,
		},
		{
			name:             "rejects an App server-to-server token",
			tokenType:        gh.TokenTypeServerToServer,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "WRITE",
			wantErr:          badTokenErr,
		},
		{
			name:             "rejects a refresh token",
			tokenType:        gh.TokenTypeRefresh,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "WRITE",
			wantErr:          badTokenErr,
		},
		{
			// An unrecognised prefix and no token at all reach here the same
			// way. Which is which is decided by ActiveTokenType.
			name:             "rejects a credential gh does not recognise",
			tokenType:        gh.TokenTypeUnknown,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "WRITE",
			wantErr:          badTokenErr,
		},
		{
			// A caller that never fetched the id hands over a zero, and an
			// upload cannot be taken back, so it stops before the endpoint.
			name:             "rejects an unresolved target",
			tokenType:        gh.TokenTypeOAuth,
			host:             "github.com",
			targetRepository: 0,
			viewerPermission: "WRITE",
			wantErr:          "could not determine which repository to attach files to",
		},
		{
			name:             "rejects a negative target",
			tokenType:        gh.TokenTypeOAuth,
			host:             "github.com",
			targetRepository: -1,
			viewerPermission: "WRITE",
			wantErr:          "could not determine which repository to attach files to",
		},
		{
			name:             "admin permission",
			tokenType:        gh.TokenTypeOAuth,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "ADMIN",
		},
		{
			name:             "maintain permission",
			tokenType:        gh.TokenTypeOAuth,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "MAINTAIN",
		},
		{
			name:             "rejects triage permission",
			tokenType:        gh.TokenTypeOAuth,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "TRIAGE",
			wantErr:          badPermissionErr,
		},
		{
			name:             "rejects read permission",
			tokenType:        gh.TokenTypeOAuth,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "READ",
			wantErr:          badPermissionErr,
		},
		{
			name:             "rejects an unrecognized permission",
			tokenType:        gh.TokenTypeOAuth,
			host:             "github.com",
			targetRepository: 42,
			viewerPermission: "UNKNOWN",
			wantErr:          badPermissionErr,
		},
		{
			// Only the user can fix a low permission, so an empty string and a
			// permission that is too low get different messages.
			name:             "rejects an unrequested permission",
			tokenType:        gh.TokenTypeOAuth,
			host:             "github.com",
			targetRepository: 42,
			wantErr:          "could not determine your permission on the repository to attach files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Registered with no stubs, so any request at all fails the test.
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			uploader, err := NewUploader(&http.Client{Transport: reg}, tt.tokenType, tt.host, tt.targetRepository, tt.viewerPermission)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Nil(t, uploader)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.host, uploader.host)
				assert.Equal(t, tt.targetRepository, uploader.targetRepository)
			}
			// Building an uploader resolves nothing.
			assert.Empty(t, reg.Requests)
		})
	}
}

func TestCheckHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr string
	}{
		{name: "github.com", host: "github.com"},
		{name: "data residency tenant", host: "acme.ghe.com"},
		{name: "localhost", host: "github.localhost"},
		{
			name:    "enterprise server",
			host:    "github.example.com",
			wantErr: "attaching files is not supported on GitHub Enterprise Server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkHost(tt.host)

			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr)
		})
	}
}
