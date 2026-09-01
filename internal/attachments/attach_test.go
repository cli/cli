package attachments

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFiles puts real files in a temporary working directory, because
// uploading opens the path it is given.
func writeFiles(t *testing.T, names ...string) {
	t.Helper()

	t.Chdir(t.TempDir())
	for _, name := range names {
		require.NoError(t, os.WriteFile(name, []byte("the bytes"), 0o600))
	}
}

func testUploader(reg *httpmock.Registry) *Uploader {
	return &Uploader{
		httpClient:       &http.Client{Transport: reg},
		host:             "github.com",
		targetRepository: 1234,
	}
}

func TestUploaderUploadAndAttach(t *testing.T) {
	// Each upload response is consumed in the order the assets were
	// written, so a status here is a status for that asset.
	type upload struct {
		status   int
		response string
	}

	tests := []struct {
		name     string
		files    []string
		args     []string
		body     string
		uploads  []upload
		wantBody string
		// What a caller keys on to decide whether a body is worth writing.
		wantUploaded int
		wantErr      string
	}{
		{
			name:         "appends an image to a body",
			files:        []string{"login.png"},
			args:         []string{"./login.png"},
			body:         "See below",
			uploads:      []upload{{201, `{"url":"https://github.com/user-attachments/assets/1"}`}},
			wantBody:     "See below\n\n![login](https://github.com/user-attachments/assets/1)",
			wantUploaded: 1,
		},
		{
			name:    "appends a video as a paragraph of its own",
			files:   []string{"repro.mp4"},
			args:    []string{"./repro.mp4"},
			body:    "Watch this:",
			uploads: []upload{{201, `{"url":"https://github.com/user-attachments/assets/2"}`}},
			// A bare URL only renders as a player when nothing shares its
			// paragraph, so it must not land on the end of the line above.
			wantBody:     "Watch this:\n\nhttps://github.com/user-attachments/assets/2",
			wantUploaded: 1,
		},
		{
			name:         "appends to an empty body without leading blank lines",
			files:        []string{"login.png"},
			args:         []string{"./login.png"},
			body:         "",
			uploads:      []upload{{201, `{"url":"https://github.com/user-attachments/assets/1"}`}},
			wantBody:     "![login](https://github.com/user-attachments/assets/1)",
			wantUploaded: 1,
		},
		{
			name:         "does not stack blank lines on a body that ends with them",
			files:        []string{"login.png"},
			args:         []string{"./login.png"},
			body:         "See below\n\n\n",
			uploads:      []upload{{201, `{"url":"https://github.com/user-attachments/assets/1"}`}},
			wantBody:     "See below\n\n![login](https://github.com/user-attachments/assets/1)",
			wantUploaded: 1,
		},
		{
			name:    "appends several assets in the order they were written",
			files:   []string{"before.png", "after.png", "repro.mp4"},
			args:    []string{"./before.png#Before the fix", "./after.png#After the fix", "./repro.mp4"},
			body:    "Compare:",
			uploads: []upload{{201, `{"url":"https://example.com/1"}`}, {201, `{"url":"https://example.com/2"}`}, {201, `{"url":"https://example.com/3"}`}},
			wantBody: "Compare:\n\n" +
				"![Before the fix](https://example.com/1)\n\n" +
				"![After the fix](https://example.com/2)\n\n" +
				"https://example.com/3",
			wantUploaded: 3,
		},
		{
			name:         "rewrites a reference in place instead of appending it",
			files:        []string{"login.png"},
			args:         []string{"./login.png"},
			body:         "The error:\n\n![the login screen](./login.png)\n\nThat is all.",
			uploads:      []upload{{201, `{"url":"https://example.com/1"}`}},
			wantBody:     "The error:\n\n![the login screen](https://example.com/1)\n\nThat is all.",
			wantUploaded: 1,
		},
		{
			name:         "rewrites what the body references and appends what it does not",
			files:        []string{"login.png", "after.png"},
			args:         []string{"./login.png", "./after.png"},
			body:         "![the login screen](./login.png)",
			uploads:      []upload{{201, `{"url":"https://example.com/1"}`}, {201, `{"url":"https://example.com/2"}`}},
			wantBody:     "![the login screen](https://example.com/1)\n\n![after](https://example.com/2)",
			wantUploaded: 2,
		},
		{
			// Three files and two replies: c is never attempted, which the
			// registry proves by failing on a stub nothing used.
			name:         "stops at the first failure and writes what got up",
			files:        []string{"a.png", "b.png", "c.png"},
			args:         []string{"./a.png", "./b.png", "./c.png"},
			body:         "Three files",
			uploads:      []upload{{201, `{"url":"https://example.com/1"}`}, {404, `{"message":"Not Found"}`}},
			wantBody:     "Three files\n\n![a](https://example.com/1)",
			wantUploaded: 1,
			wantErr:      "could not upload ./b.png: attaching files requires write access to the repository",
		},
		{
			name:         "leaves a failed reference as the author wrote it",
			files:        []string{"login.png"},
			args:         []string{"./login.png"},
			body:         "![the login screen](./login.png)",
			uploads:      []upload{{404, `{"message":"Not Found"}`}},
			wantBody:     "![the login screen](./login.png)",
			wantUploaded: 0,
			wantErr:      "could not upload ./login.png: attaching files requires write access to the repository",
		},
		{
			name:         "writes nothing when the first upload fails",
			files:        []string{"a.png", "b.png"},
			args:         []string{"./a.png", "./b.png"},
			body:         "",
			uploads:      []upload{{404, `{"message":"Not Found"}`}},
			wantBody:     "",
			wantUploaded: 0,
			wantErr:      "could not upload ./a.png: attaching files requires write access to the repository",
		},
		{
			// Refused before the upload loop, so nothing is stranded, and the
			// body comes back untouched for a caller that assigns in place.
			name:         "refuses a video embedded through a reference definition",
			files:        []string{"repro.mp4"},
			args:         []string{"./repro.mp4"},
			body:         "![clip][c]\n\n[c]: ./repro.mp4",
			wantBody:     "![clip][c]\n\n[c]: ./repro.mp4",
			wantUploaded: 0,
			wantErr:      "cannot embed a video as a reference-style image: ./repro.mp4",
		},
		{
			name:         "uploads a video linked through a reference definition",
			files:        []string{"repro.mp4"},
			args:         []string{"./repro.mp4"},
			body:         "[clip][c]\n\n[c]: ./repro.mp4",
			uploads:      []upload{{201, `{"url":"https://example.com/1"}`}},
			wantBody:     "[clip][c]\n\n[c]: https://example.com/1",
			wantUploaded: 1,
		},
		{
			// The only place a UserAsset becomes an attachmentArg, so the only row
			// that proves the label survives the copy. It has to be an embed,
			// since a link keeps the label the author wrote and never reaches
			// the branch that supplies one.
			name:         "labels a video embed that degrades to a link",
			files:        []string{"repro.mp4"},
			args:         []string{"./repro.mp4"},
			body:         "The crash ![](./repro.mp4) reproduces every time.",
			uploads:      []upload{{201, `{"url":"https://example.com/1"}`}},
			wantBody:     "The crash [repro.mp4](https://example.com/1) reproduces every time.",
			wantUploaded: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeFiles(t, tt.files...)

			assets, err := assetsFromArgs(t, tt.args...)
			require.NoError(t, err)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			for _, u := range tt.uploads {
				reg.Register(
					httpmock.REST("POST", "user-attachments/assets"),
					httpmock.StatusStringResponse(u.status, u.response),
				)
			}

			body, uploaded, err := testUploader(reg).UploadAndAttach(context.Background(), tt.body, assets)

			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.wantBody, body)
			assert.Equal(t, tt.wantUploaded, uploaded)

			// The bytes go up in the order they were attached, so a failure
			// stops the ones after it rather than reordering them.
			if len(tt.uploads) == 0 {
				assert.Empty(t, reg.Requests, "nothing should have been uploaded")
				return
			}
			require.Len(t, reg.Requests, len(tt.uploads))
			for i := range reg.Requests {
				assert.Equal(t, tt.files[i], reg.Requests[i].URL.Query().Get("name"))
			}
		})
	}
}

func TestUploaderUploadAndAttachUploadsOnceForRepeatedReferences(t *testing.T) {
	writeFiles(t, "shot.png")

	assets, err := assetsFromArgs(t, "./shot.png")
	require.NoError(t, err)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "user-attachments/assets"),
		httpmock.StatusStringResponse(201, `{"url":"https://example.com/1"}`),
	)

	body, uploaded, err := testUploader(reg).UploadAndAttach(context.Background(),
		"![one](./shot.png)\n\ntext\n\n![two](./shot.png)", assets)

	require.NoError(t, err)
	assert.Equal(t, 1, uploaded)
	assert.Equal(t, "![one](https://example.com/1)\n\ntext\n\n![two](https://example.com/1)", body)
	assert.Len(t, reg.Requests, 1)
}

func TestUploaderUploadAndAttachNoAssets(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	body, uploaded, err := testUploader(reg).UploadAndAttach(context.Background(), "unchanged\n", nil)

	require.NoError(t, err)
	assert.Zero(t, uploaded)
	assert.Equal(t, "unchanged\n", body)
	assert.Empty(t, reg.Requests)
}

func TestAppendParagraph(t *testing.T) {
	tests := []struct {
		name     string
		md       string
		addition string
		want     string
	}{
		{name: "separates with a blank line", md: "text", addition: "more", want: "text\n\nmore"},
		{name: "empty markdown", md: "", addition: "more", want: "more"},
		{name: "whitespace only markdown", md: "  \n\n", addition: "more", want: "more"},
		{name: "empty addition preserves markdown", md: "text  \n", addition: "", want: "text  \n"},
		{name: "trailing newlines", md: "text\n\n\n", addition: "more", want: "text\n\nmore"},
		{name: "trailing spaces", md: "text  ", addition: "more", want: "text\n\nmore"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, appendParagraph(tt.md, tt.addition))
		})
	}
}

func TestUploaderUploadAndAttachDoesNotLeakTheAssetURLIntoAnError(t *testing.T) {
	writeFiles(t, "a.png", "b.png")

	assets, err := assetsFromArgs(t, "./a.png", "./b.png")
	require.NoError(t, err)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "user-attachments/assets"),
		httpmock.StatusStringResponse(201, `{"url":"https://example.com/secret-asset"}`),
	)
	reg.Register(
		httpmock.REST("POST", "user-attachments/assets"),
		httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
	)

	body, uploaded, err := testUploader(reg).UploadAndAttach(context.Background(), "", assets)

	require.Error(t, err)
	// One asset is up and cannot be deleted, so the caller must write this
	// body even though the call failed.
	assert.Equal(t, 1, uploaded)
	assert.NotContains(t, err.Error(), "secret-asset")
	assert.Contains(t, body, "secret-asset")
}

func TestUploaderUploadAndAttachAbsolutePathReference(t *testing.T) {
	writeFiles(t, "shot.png")
	abs, err := filepath.Abs("shot.png")
	require.NoError(t, err)

	assets, err := assetsFromArgs(t, abs)
	require.NoError(t, err)

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("POST", "user-attachments/assets"),
		httpmock.StatusStringResponse(201, `{"url":"https://example.com/1"}`),
	)

	body, uploaded, err := testUploader(reg).UploadAndAttach(context.Background(), "![shot](./shot.png)", assets)

	require.NoError(t, err)
	assert.Equal(t, 1, uploaded)
	assert.Equal(t, "![shot](https://example.com/1)", body)
}
