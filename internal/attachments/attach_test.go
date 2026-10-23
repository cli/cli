package attachments

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFiles puts real files in a temporary working directory, because
// uploading opens the path it is given. A name may include directories, which
// are created.
func writeFiles(t *testing.T, names ...string) {
	t.Helper()

	t.Chdir(t.TempDir())
	for _, name := range names {
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
		require.NoError(t, os.WriteFile(name, []byte("the bytes"), 0o600))
	}
}

func testUploader(reg *httpmock.Registry) *Uploader {
	return &Uploader{
		client:           api.NewClientFromHTTP(&http.Client{Transport: reg}),
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
		name         string
		files        []string
		args         []string
		body         string
		markdownDir  string
		uploads      []upload
		wantBody     string
		wantUploaded int
		wantAppend   int
		wantReplace  int
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
			wantAppend:   1,
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
			wantAppend:   1,
		},
		{
			name:         "appends to an empty body without leading blank lines",
			files:        []string{"login.png"},
			args:         []string{"./login.png"},
			body:         "",
			uploads:      []upload{{201, `{"url":"https://github.com/user-attachments/assets/1"}`}},
			wantBody:     "![login](https://github.com/user-attachments/assets/1)",
			wantUploaded: 1,
			wantAppend:   1,
		},
		{
			name:         "does not stack blank lines on a body that ends with them",
			files:        []string{"login.png"},
			args:         []string{"./login.png"},
			body:         "See below\n\n\n",
			uploads:      []upload{{201, `{"url":"https://github.com/user-attachments/assets/1"}`}},
			wantBody:     "See below\n\n![login](https://github.com/user-attachments/assets/1)",
			wantUploaded: 1,
			wantAppend:   1,
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
			wantAppend:   3,
		},
		{
			name:         "rewrites a reference in place instead of appending it",
			files:        []string{"login.png"},
			args:         []string{"./login.png"},
			body:         "The error:\n\n![the login screen](./login.png)\n\nThat is all.",
			uploads:      []upload{{201, `{"url":"https://example.com/1"}`}},
			wantBody:     "The error:\n\n![the login screen](https://example.com/1)\n\nThat is all.",
			wantUploaded: 1,
			wantReplace:  1,
		},
		{
			name:         "rewrites what the body references and appends what it does not",
			files:        []string{"login.png", "after.png"},
			args:         []string{"./login.png", "./after.png"},
			body:         "![the login screen](./login.png)",
			uploads:      []upload{{201, `{"url":"https://example.com/1"}`}, {201, `{"url":"https://example.com/2"}`}},
			wantBody:     "![the login screen](https://example.com/1)\n\n![after](https://example.com/2)",
			wantUploaded: 2,
			wantAppend:   1,
			wantReplace:  1,
		},
		{
			name:         "rewrites a reference relative to the directory the markdown was read from",
			files:        []string{"docs/diagram.png"},
			args:         []string{"./docs/diagram.png"},
			body:         "![the diagram](./diagram.png)",
			markdownDir:  "docs",
			uploads:      []upload{{201, `{"url":"https://example.com/1"}`}},
			wantBody:     "![the diagram](https://example.com/1)",
			wantUploaded: 1,
			wantReplace:  1,
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
			wantAppend:   1,
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
			wantReplace:  1,
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
			wantReplace:  1,
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

			body, result, err := testUploader(reg).UploadAndAttach(context.Background(), tt.body, tt.markdownDir, assets)

			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.wantBody, body)
			assert.Equal(t, tt.wantUploaded, result.Uploaded)
			assert.Equal(t, tt.wantAppend, result.AppendOperations)
			assert.Equal(t, tt.wantReplace, result.ReplaceOperations)
			assert.Equal(t, result.Uploaded, result.AppendOperations+result.ReplaceOperations)

			// The bytes go up in the order they were attached, so a failure
			// stops the ones after it rather than reordering them.
			if len(tt.uploads) == 0 {
				assert.Empty(t, reg.Requests, "nothing should have been uploaded")
				return
			}
			require.Len(t, reg.Requests, len(tt.uploads))
			for i := range reg.Requests {
				assert.Equal(t, filepath.Base(tt.files[i]), reg.Requests[i].URL.Query().Get("name"))
			}
		})
	}
}

func TestUploaderUploadAndAttachDocuments(t *testing.T) {
	const (
		url1 = "https://github.com/user-attachments/assets/1"
		url2 = "https://github.com/user-attachments/assets/2"
	)
	okUpload := func(name, url string) UploadStub {
		return UploadStub{Name: name, Status: 201, Body: `{"url":"` + url + `"}`}
	}

	type wantDocument struct {
		markdown string
		uploaded []string
		appended int
		replaced int
		err      string
	}

	tests := []struct {
		name  string
		files []string
		args  []string
		docs  []Document
		// Each file is stubbed by name and every stub answers once. Any other
		// upload, such as a second one of the same file, fails the test.
		uploads          []UploadStub
		want             []wantDocument
		wantErr          string
		wantUnreferenced []string
	}{
		{
			name:  "uploads a file once for every document that references it",
			files: []string{"signin-flow.png"},
			args:  []string{"./signin-flow.png"},
			docs: []Document{
				{Markdown: "# Sign-in plan\n\n![Sign-in flow](./signin-flow.png)"},
				{Markdown: "# Barista notes\n\n![The new sign-in](./signin-flow.png)"},
			},
			uploads: []UploadStub{okUpload("signin-flow.png", url1)},
			want: []wantDocument{
				{markdown: "# Sign-in plan\n\n![Sign-in flow](" + url1 + ")", uploaded: []string{"./signin-flow.png"}, replaced: 1},
				{markdown: "# Barista notes\n\n![The new sign-in](" + url1 + ")", uploaded: []string{"./signin-flow.png"}, replaced: 1},
			},
		},
		{
			name:  "resolves each document's references against its own directory",
			files: []string{"plans/signin-flow.png", "notes/signin-flow.png"},
			args:  []string{"./plans/signin-flow.png", "./notes/signin-flow.png"},
			docs: []Document{
				{Markdown: "![Sign-in flow](./signin-flow.png)", Dir: "plans"},
				{Markdown: "![Sign-in flow](./signin-flow.png)", Dir: "notes"},
			},
			uploads: []UploadStub{okUpload("signin-flow.png", url1), okUpload("signin-flow.png", url2)},
			want: []wantDocument{
				{markdown: "![Sign-in flow](" + url1 + ")", uploaded: []string{"./plans/signin-flow.png"}, replaced: 1},
				{markdown: "![Sign-in flow](" + url2 + ")", uploaded: []string{"./notes/signin-flow.png"}, replaced: 1},
			},
		},
		{
			name:  "falls back to the working directory when nothing beside a document matches",
			files: []string{"plans/signin-flow.png", "menu-photo.png"},
			args:  []string{"./plans/signin-flow.png", "./menu-photo.png"},
			docs: []Document{
				{Markdown: "![Sign-in flow](./signin-flow.png)\n\n![Menu](./menu-photo.png)", Dir: "plans"},
				{Markdown: "![Menu](./menu-photo.png)", Dir: "notes"},
			},
			uploads: []UploadStub{okUpload("signin-flow.png", url1), okUpload("menu-photo.png", url2)},
			want: []wantDocument{
				{markdown: "![Sign-in flow](" + url1 + ")\n\n![Menu](" + url2 + ")", uploaded: []string{"./plans/signin-flow.png", "./menu-photo.png"}, replaced: 2},
				{markdown: "![Menu](" + url2 + ")", uploaded: []string{"./menu-photo.png"}, replaced: 1},
			},
		},
		{
			name:  "refuses a file no document references before uploading anything",
			files: []string{"signin-flow.png", "latte-art.png"},
			args:  []string{"./signin-flow.png", "./latte-art.png"},
			docs: []Document{
				{Markdown: "![Sign-in flow](./signin-flow.png)"},
				{Markdown: "![Sign-in flow](./signin-flow.png)"},
			},
			wantErr:          "no document references ./latte-art.png",
			wantUnreferenced: []string{"./latte-art.png"},
		},
		{
			name:  "names every file no document references, in the order they were attached",
			files: []string{"latte-art.png", "signin-flow.png", "menu-photo.png"},
			args:  []string{"./latte-art.png", "./signin-flow.png", "./menu-photo.png"},
			docs: []Document{
				{Markdown: "![Sign-in flow](./signin-flow.png)"},
				{Markdown: "No pictures."},
			},
			wantErr:          "no document references ./latte-art.png, ./menu-photo.png",
			wantUnreferenced: []string{"./latte-art.png", "./menu-photo.png"},
		},
		{
			name:             "refuses every file when there is no document",
			files:            []string{"signin-flow.png"},
			args:             []string{"./signin-flow.png"},
			wantErr:          "no document references ./signin-flow.png",
			wantUnreferenced: []string{"./signin-flow.png"},
		},
		{
			name:  "refuses a video embedded through a reference definition in any document",
			files: []string{"signin-flow.png", "repro.mp4"},
			args:  []string{"./signin-flow.png", "./repro.mp4"},
			docs: []Document{
				{Markdown: "![Sign-in flow](./signin-flow.png)"},
				{Markdown: "![clip][c]\n\n[c]: ./repro.mp4"},
			},
			wantErr: "cannot embed a video as a reference-style image: ./repro.mp4",
		},
		{
			// latte-art.png fails and menu-photo.png is never tried. Each
			// document with either of them has the failure, and a document
			// whose attachments all uploaded has none of its own.
			name:  "stops at the first failure and reports it on each document it left without a URL",
			files: []string{"signin-flow.png", "latte-art.png", "menu-photo.png"},
			args:  []string{"./signin-flow.png", "./latte-art.png", "./menu-photo.png"},
			docs: []Document{
				{Markdown: "![Sign-in flow](./signin-flow.png)\n\n![Latte art](./latte-art.png)"},
				{Markdown: "![Sign-in flow](./signin-flow.png)"},
				{Markdown: "![Latte art](./latte-art.png)"},
				{Markdown: "![Menu](./menu-photo.png)"},
				{Markdown: "No pictures."},
			},
			uploads: []UploadStub{
				okUpload("signin-flow.png", url1),
				{Name: "latte-art.png", Status: 429, Body: `{"message":"Too Many Requests"}`},
			},
			want: []wantDocument{
				{
					markdown: "![Sign-in flow](" + url1 + ")\n\n![Latte art](./latte-art.png)",
					uploaded: []string{"./signin-flow.png"},
					replaced: 1,
					err:      "could not upload ./latte-art.png: rate limited; wait and try again",
				},
				{markdown: "![Sign-in flow](" + url1 + ")", uploaded: []string{"./signin-flow.png"}, replaced: 1},
				{markdown: "![Latte art](./latte-art.png)", err: "could not upload ./latte-art.png: rate limited; wait and try again"},
				{markdown: "![Menu](./menu-photo.png)", err: "could not upload ./latte-art.png: rate limited; wait and try again"},
				{markdown: "No pictures."},
			},
			wantErr: "could not upload ./latte-art.png: rate limited; wait and try again",
		},
		{
			name:    "with one document, appends the files it does not reference",
			files:   []string{"signin-flow.png", "landing-page.png"},
			args:    []string{"./signin-flow.png", "./landing-page.png"},
			docs:    []Document{{Markdown: "![Sign-in flow](./signin-flow.png)"}},
			uploads: []UploadStub{okUpload("signin-flow.png", url1), okUpload("landing-page.png", url2)},
			want: []wantDocument{
				{
					markdown: "![Sign-in flow](" + url1 + ")\n\n![landing-page](" + url2 + ")",
					uploaded: []string{"./signin-flow.png", "./landing-page.png"},
					appended: 1,
					replaced: 1,
				},
			},
		},
		{
			// A definition continued onto the next line of a blockquote is
			// left as written, so the file is appended to the document that
			// references it there rather than left out of every document.
			name:  "appends a file to a document whose reference to it cannot be rewritten",
			files: []string{"signin-flow.png"},
			args:  []string{"./signin-flow.png"},
			docs: []Document{
				{Markdown: "> [flow]:\n> ./signin-flow.png\n\n![Sign-in flow][flow]"},
				{Markdown: "![Sign-in flow](./signin-flow.png)"},
			},
			uploads: []UploadStub{okUpload("signin-flow.png", url1)},
			want: []wantDocument{
				{
					markdown: "> [flow]:\n> ./signin-flow.png\n\n![Sign-in flow][flow]\n\n![signin-flow](" + url1 + ")",
					uploaded: []string{"./signin-flow.png"},
					appended: 1,
				},
				{markdown: "![Sign-in flow](" + url1 + ")", uploaded: []string{"./signin-flow.png"}, replaced: 1},
			},
		},
		{
			name: "leaves every document as given when nothing is attached",
			docs: []Document{
				{Markdown: "![Sign-in flow](./signin-flow.png)"},
				{Markdown: "No pictures."},
			},
			want: []wantDocument{
				{markdown: "![Sign-in flow](./signin-flow.png)"},
				{markdown: "No pictures."},
			},
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
				StubUpload(reg, 1234, u.Name, u.Status, u.Body)
			}
			reg.Exclude(t, httpmock.REST("POST", "user-attachments/assets"))

			results, err := testUploader(reg).UploadAndAttachDocuments(context.Background(), tt.docs, assets)

			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErr)
			}
			if tt.wantUnreferenced != nil {
				unreferencedErr, ok := errors.AsType[*UnreferencedError](err)
				require.True(t, ok, "want an *UnreferencedError")
				assert.Equal(t, tt.wantUnreferenced, unreferencedErr.Paths)
			}

			// The files go up in the order they were attached.
			require.Len(t, reg.Requests, len(tt.uploads))
			for i, u := range tt.uploads {
				assert.Equal(t, u.Name, reg.Requests[i].URL.Query().Get("name"))
			}

			if tt.want == nil {
				require.Nil(t, results, "a refusal returns no results")
				return
			}
			require.Len(t, results, len(tt.want))
			for i, want := range tt.want {
				got := results[i]
				assert.Equal(t, want.markdown, got.Markdown)
				var gotUploaded []string
				for _, a := range got.Uploaded {
					gotUploaded = append(gotUploaded, a.Path())
				}
				assert.Equal(t, want.uploaded, gotUploaded)
				assert.Equal(t, want.appended, got.AppendOperations)
				assert.Equal(t, want.replaced, got.ReplaceOperations)
				if want.err == "" {
					require.NoError(t, got.Err)
				} else {
					require.EqualError(t, got.Err, want.err)
				}
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

	body, result, err := testUploader(reg).UploadAndAttach(context.Background(),
		"![one](./shot.png)\n\ntext\n\n![two](./shot.png)", "", assets)

	require.NoError(t, err)
	assert.Equal(t, UploadResult{Uploaded: 1, ReplaceOperations: 1}, result)
	assert.Equal(t, "![one](https://example.com/1)\n\ntext\n\n![two](https://example.com/1)", body)
	assert.Len(t, reg.Requests, 1)
}

func TestUploaderUploadAndAttachNoAssets(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	body, result, err := testUploader(reg).UploadAndAttach(context.Background(), "unchanged\n", "", nil)

	require.NoError(t, err)
	assert.Equal(t, UploadResult{}, result)
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

	body, result, err := testUploader(reg).UploadAndAttach(context.Background(), "", "", assets)

	require.Error(t, err)
	// One asset is up and cannot be deleted, so the caller must write this
	// body even though the call failed.
	assert.Equal(t, UploadResult{Uploaded: 1, AppendOperations: 1}, result)
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

	body, result, err := testUploader(reg).UploadAndAttach(context.Background(), "![shot](./shot.png)", "", assets)

	require.NoError(t, err)
	assert.Equal(t, UploadResult{Uploaded: 1, ReplaceOperations: 1}, result)
	assert.Equal(t, "![shot](https://example.com/1)", body)
}
