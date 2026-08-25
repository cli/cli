package shared

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/attachments"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// commentableCmd builds the flag set the comment commands share, so that
// CommentablePreRun sees the same command shape it does in production.
// withAttach is false for a command that has not registered --attach, which
// shared code still has to run on.
func commentableCmd(t *testing.T, opts *CommentableOptions, withAttach bool, input string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "comment"}
	cmd.Flags().String("body", "", "")
	cmd.Flags().String("body-file", "", "")
	cmd.Flags().Bool("web", false, "")
	cmd.Flags().Bool("editor", false, "")
	cmd.Flags().BoolVar(&opts.EditLast, "edit-last", false, "")
	cmd.Flags().BoolVar(&opts.DeleteLast, "delete-last", false, "")
	cmd.Flags().BoolVar(&opts.DeleteLastConfirmed, "yes", false, "")
	cmd.Flags().BoolVar(&opts.CreateIfNone, "create-if-none", false, "")
	if withAttach {
		attachments.AddFlag(cmd)
	}

	argv, err := shlex.Split(input)
	require.NoError(t, err)
	require.NoError(t, cmd.Flags().Parse(argv))

	return cmd
}

func TestCommentablePreRun(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		withAttach bool
		canPrompt  bool
		wantErr    string
		// wantErrIsNotExist covers an error whose text the operating system
		// words differently, so the assertion cannot be on the message.
		wantErrIsNotExist bool
		wantInputType     InputType
		wantAssetPaths    []string
		wantInteractive   bool
		wantBodyProvided  bool
	}{
		{
			// A change that separates counting the input from resolving it
			// fails here, since this row asserts both from one run.
			name:           "attach alone is a body input and is resolved",
			input:          "--attach ./shot.png",
			withAttach:     true,
			wantInputType:  InputTypeInline,
			wantAssetPaths: []string{"./shot.png"},
		},
		{
			name:             "attach with body is still resolved",
			input:            "--attach ./shot.png --body 'see below'",
			withAttach:       true,
			wantInputType:    InputTypeInline,
			wantBodyProvided: true,
			wantAssetPaths:   []string{"./shot.png"},
		},
		{
			name:              "a file that does not exist stops the command",
			input:             "--attach ./gone.png",
			withAttach:        true,
			wantErr:           "./gone.png: ",
			wantErrIsNotExist: true,
		},
		{
			// `--body ""` clears a comment, and only the flag being present can
			// say so.
			name:             "attach with an empty body",
			input:            "--attach ./shot.png --body ''",
			withAttach:       true,
			wantInputType:    InputTypeInline,
			wantBodyProvided: true,
			wantAssetPaths:   []string{"./shot.png"},
		},
		{
			name:             "attach with body-file",
			input:            "--attach ./shot.png --body-file ./body.md",
			withAttach:       true,
			wantInputType:    InputTypeInline,
			wantBodyProvided: true,
			wantAssetPaths:   []string{"./shot.png"},
		},
		{
			name:           "attach with editor",
			input:          "--attach ./shot.png --editor",
			withAttach:     true,
			wantInputType:  InputTypeEditor,
			wantAssetPaths: []string{"./shot.png"},
		},
		{
			name:       "attach with web",
			input:      "--attach ./shot.png --web",
			withAttach: true,
			wantErr:    "`--attach` is not supported when using `--web`",
		},
		{
			name:       "a flag conflict is reported before a missing file",
			input:      "--attach ./gone.png --web",
			withAttach: true,
			wantErr:    "`--attach` is not supported when using `--web`",
		},
		{
			name:       "attach with delete-last",
			input:      "--attach ./shot.png --delete-last --yes",
			withAttach: true,
			wantErr:    "`--attach` is not supported when using `--delete-last`",
		},
		{
			// Reaching shared code without registering --attach is a missing
			// AddFlag call rather than a bad invocation.
			name:      "attach not registered",
			input:     "",
			canPrompt: true,
			wantErr:   "comment does not register --attach",
		},
		{
			name:    "attach not registered and body passed",
			input:   "--body 'hello'",
			wantErr: "comment does not register --attach",
		},
		{
			name:       "attach registered but not passed",
			input:      "",
			withAttach: true,
			wantErr:    "flags required when not running interactively",
		},
		{
			name:       "body and editor still conflict",
			input:      "--attach ./shot.png --body hello --editor",
			withAttach: true,
			wantErr:    "specify only one of `--body`, `--body-file`, `--editor`, or `--web`",
		},
		{
			name:       "delete-last with a body",
			input:      "--delete-last --yes --body hello",
			withAttach: true,
			wantErr:    "should not provide comment body when using `--delete-last`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			require.NoError(t, os.WriteFile("shot.png", []byte("the bytes"), 0o600))

			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.canPrompt)
			ios.SetStdoutTTY(tt.canPrompt)
			ios.SetStderrTTY(tt.canPrompt)

			opts := &CommentableOptions{IO: ios}
			cmd := commentableCmd(t, opts, tt.withAttach, tt.input)

			err := CommentablePreRun(cmd, opts)

			if tt.wantErr != "" {
				if tt.wantErrIsNotExist {
					require.ErrorIs(t, err, fs.ErrNotExist)
					require.ErrorContains(t, err, tt.wantErr)
				} else {
					require.EqualError(t, err, tt.wantErr)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantInputType, opts.InputType)

			var assetPaths []string
			for _, a := range opts.Assets {
				assetPaths = append(assetPaths, a.Path())
			}
			assert.Equal(t, tt.wantAssetPaths, assetPaths)
			assert.Equal(t, tt.wantInteractive, opts.Interactive)
			assert.Equal(t, tt.wantBodyProvided, opts.BodyProvided)
		})
	}
}

func TestCommentableRunUploadsAndWritesBodies(t *testing.T) {
	tests := []struct {
		name       string
		opts       CommentableOptions
		attach     []string
		host       string
		hostTokens map[string]string
		// Empty means the default, which can upload.
		viewerPermission     string
		repositoryDatabaseID int64
		uploads              []attachments.UploadStub
		wantQuery            string
		writeFails           bool
		wantBody             string
		wantStdout           string
		wantErr              string
		wantUploads          int
	}{
		{
			name:       "creating with no asset uploads nothing",
			opts:       CommentableOptions{Body: "just text", InputType: InputTypeInline},
			wantQuery:  `mutation CommentCreate\b`,
			wantBody:   "just text",
			wantStdout: "https://github.com/OWNER/REPO/pull/123#issuecomment-456\n",
		},
		{
			name:                 "creating uploads the asset and writes the reference",
			opts:                 CommentableOptions{Body: "see below", InputType: InputTypeInline},
			attach:               []string{"shot.png"},
			repositoryDatabaseID: 1234,
			uploads:              []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			wantQuery:            `mutation CommentCreate\b`,
			wantBody:             "see below\n\n![shot](https://example.com/1)",
			wantStdout:           "https://github.com/OWNER/REPO/pull/123#issuecomment-456\n",
			wantUploads:          1,
		},
		{
			name:                 "creating writes what uploaded when one upload fails",
			opts:                 CommentableOptions{Body: "see below", InputType: InputTypeInline},
			attach:               []string{"a.png", "b.png"},
			repositoryDatabaseID: 1234,
			uploads: []attachments.UploadStub{
				{Name: "a.png", Status: 201, Body: `{"url":"https://example.com/1"}`},
				{Name: "b.png", Status: 404, Body: `{"message":"Not Found"}`},
			},
			wantQuery:   `mutation CommentCreate\b`,
			wantBody:    "see below\n\n![a](https://example.com/1)",
			wantStdout:  "https://github.com/OWNER/REPO/pull/123#issuecomment-456\n",
			wantErr:     "could not upload ./b.png: attaching files requires write access to the repository",
			wantUploads: 2,
		},
		{
			name:                 "creating writes nothing when every upload fails",
			opts:                 CommentableOptions{Body: "", InputType: InputTypeInline},
			attach:               []string{"a.png"},
			repositoryDatabaseID: 1234,
			uploads:              []attachments.UploadStub{{Name: "a.png", Status: 404, Body: `{"message":"Not Found"}`}},
			wantErr:              "could not upload ./a.png: attaching files requires write access to the repository\nno comment was posted",
			wantUploads:          1,
		},
		{
			name:                 "creating writes nothing when every upload fails and the body has text",
			opts:                 CommentableOptions{Body: "see below", InputType: InputTypeInline},
			attach:               []string{"a.png"},
			repositoryDatabaseID: 1234,
			uploads:              []attachments.UploadStub{{Name: "a.png", Status: 404, Body: `{"message":"Not Found"}`}},
			wantErr:              "could not upload ./a.png: attaching files requires write access to the repository\nno comment was posted",
			wantUploads:          1,
		},
		{
			name:                 "creating writes nothing when the body fails validation",
			opts:                 CommentableOptions{Body: "![clip][c]\n\n[c]: ./repro.mp4", InputType: InputTypeInline},
			attach:               []string{"repro.mp4"},
			repositoryDatabaseID: 1234,
			wantErr:              "cannot embed a video as a reference-style image: ./repro.mp4\nno comment was posted",
		},
		{
			name:                 "creating reports the upload and the write when both fail",
			opts:                 CommentableOptions{Body: "see below", InputType: InputTypeInline},
			attach:               []string{"a.png", "b.png"},
			repositoryDatabaseID: 1234,
			uploads: []attachments.UploadStub{
				{Name: "a.png", Status: 201, Body: `{"url":"https://example.com/1"}`},
				{Name: "b.png", Status: 404, Body: `{"message":"Not Found"}`},
			},
			wantQuery:   `mutation CommentCreate\b`,
			writeFails:  true,
			wantErr:     "could not upload ./b.png: attaching files requires write access to the repository\nGraphQL: the write failed",
			wantUploads: 2,
		},
		{
			name: "editing keeps the comment when no body flag was given",
			opts: CommentableOptions{
				Body:             "",
				EditLast:         true,
				KeepExistingBody: true,
				InputType:        InputTypeInline,
			},
			attach:               []string{"shot.png"},
			repositoryDatabaseID: 1234,
			uploads:              []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			wantQuery:            `mutation CommentUpdate\b`,
			wantBody:             "the original comment\n\n![shot](https://example.com/1)",
			wantStdout:           "https://github.com/OWNER/REPO/pull/123#issuecomment-111\n",
			wantUploads:          1,
		},
		{
			name: "editing clears the comment when the body given was empty",
			opts: CommentableOptions{
				Body:             "",
				BodyProvided:     true,
				EditLast:         true,
				KeepExistingBody: true,
				InputType:        InputTypeInline,
			},
			attach:               []string{"shot.png"},
			repositoryDatabaseID: 1234,
			uploads:              []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			wantQuery:            `mutation CommentUpdate\b`,
			wantBody:             "![shot](https://example.com/1)",
			wantStdout:           "https://github.com/OWNER/REPO/pull/123#issuecomment-111\n",
			wantUploads:          1,
		},
		{
			name: "editing writes nothing when every upload fails and there is no text",
			opts: CommentableOptions{
				Body:      "",
				EditLast:  true,
				InputType: InputTypeInline,
			},
			attach:               []string{"a.png"},
			repositoryDatabaseID: 1234,
			uploads:              []attachments.UploadStub{{Name: "a.png", Status: 404, Body: `{"message":"Not Found"}`}},
			wantErr:              "could not upload ./a.png: attaching files requires write access to the repository\nthe comment was not changed",
			wantUploads:          1,
		},
		{
			name: "editing replaces when appending was not asked for",
			opts: CommentableOptions{
				Body:      "a replacement",
				EditLast:  true,
				InputType: InputTypeInline,
			},
			wantQuery:  `mutation CommentUpdate\b`,
			wantBody:   "a replacement",
			wantStdout: "https://github.com/OWNER/REPO/pull/123#issuecomment-111\n",
		},
		{
			name: "editing does not append what the editor already carried",
			opts: CommentableOptions{
				EditLast:         true,
				KeepExistingBody: true,
				InputType:        InputTypeEditor,
				EditSurvey:       func(initial string) (string, error) { return initial + "\n\nplus an edit", nil },
			},
			wantQuery:  `mutation CommentUpdate\b`,
			wantBody:   "the original comment\n\nplus an edit",
			wantStdout: "https://github.com/OWNER/REPO/pull/123#issuecomment-111\n",
		},
		{
			// Both write paths return early on the web path, so an unguarded
			// precondition check would fail this run.
			name: "the web path on an enterprise host is untouched when nothing is attached",
			opts: CommentableOptions{
				InputType:     InputTypeWeb,
				OpenInBrowser: func(string) error { return nil },
			},
			host: "github.example.com",
		},
		{
			name: "a token that cannot upload fails before the editor opens",
			opts: CommentableOptions{
				InputType: InputTypeEditor,
				EditSurvey: func(string) (string, error) {
					return "", errors.New("the editor must not open")
				},
			},
			attach:               []string{"shot.png"},
			hostTokens:           map[string]string{"github.com": "ghs_anactionstoken"},
			repositoryDatabaseID: 1234,
			wantErr:              "unsupported authentication type",
		},
		{
			// This covers the shared comment path only. A command that builds
			// its own uploader decides its own token and needs its own row.
			name:                 "the token comes from the host the commentable was fetched from",
			opts:                 CommentableOptions{Body: "see below", InputType: InputTypeInline},
			attach:               []string{"shot.png"},
			host:                 "acme.ghe.com",
			hostTokens:           map[string]string{"github.com": "ghs_anactionstoken", "acme.ghe.com": "gho_atenanttoken"},
			repositoryDatabaseID: 1234,
			uploads:              []attachments.UploadStub{{Name: "shot.png", Status: 201, Body: `{"url":"https://example.com/1"}`}},
			wantQuery:            `mutation CommentCreate\b`,
			wantBody:             "see below\n\n![shot](https://example.com/1)",
			wantStdout:           "https://github.com/OWNER/REPO/pull/123#issuecomment-456\n",
			wantUploads:          1,
		},
		{
			name:                 "read access stops the upload before anything is written",
			opts:                 CommentableOptions{Body: "see below", InputType: InputTypeInline},
			attach:               []string{"shot.png"},
			repositoryDatabaseID: 1234,
			viewerPermission:     "READ",
			wantErr:              "attaching files requires write access to the repository",
		},
		{
			// An empty permission means the command forgot to request the
			// repository field, which is a different fault from too low a
			// permission, and only one of the two has a remedy.
			name:                 "an unknown permission stops the upload",
			opts:                 CommentableOptions{Body: "see below", InputType: InputTypeInline},
			attach:               []string{"shot.png"},
			repositoryDatabaseID: 1234,
			viewerPermission:     "none",
			wantErr:              "could not determine your permission on the repository to attach files",
		},
		{
			name:                 "a missing repository id stops the upload",
			opts:                 CommentableOptions{Body: "see below", InputType: InputTypeInline},
			attach:               []string{"shot.png"},
			repositoryDatabaseID: 0,
			wantErr:              "could not determine which repository to attach files to",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()

			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			for _, u := range tt.uploads {
				attachments.StubUpload(reg, tt.repositoryDatabaseID, u.Name, u.Status, u.Body)
			}

			var gotBody string
			if tt.wantQuery != "" && tt.writeFails {
				reg.Register(
					httpmock.GraphQL(tt.wantQuery),
					httpmock.StringResponse(`{"errors":[{"message":"the write failed"}]}`),
				)
			} else if tt.wantQuery != "" {
				response := `{"data":{"addComment":{"commentEdge":{"node":{
					"url":"https://github.com/OWNER/REPO/pull/123#issuecomment-456"}}}}}`
				if tt.wantQuery == `mutation CommentUpdate\b` {
					response = `{"data":{"updateIssueComment":{"issueComment":{
						"url":"https://github.com/OWNER/REPO/pull/123#issuecomment-111"}}}}`
				}
				reg.Register(
					httpmock.GraphQL(tt.wantQuery),
					httpmock.GraphQLMutation(response, func(inputs map[string]interface{}) {
						gotBody, _ = inputs["body"].(string)
					}),
				)
			}

			opts := tt.opts
			opts.IO = ios
			opts.HttpClient = func() (*http.Client, error) { return &http.Client{Transport: reg}, nil }
			if len(tt.attach) > 0 {
				opts.Assets = attachments.NewTestAssets(t, tt.attach...)
			}

			host := tt.host
			if host == "" {
				host = "github.com"
			}

			hostTokens := tt.hostTokens
			if hostTokens == nil {
				hostTokens = map[string]string{host: "gho_atokenthatcanupload"}
			}
			opts.Config = func() (gh.Config, error) {
				return config.NewMockConfigFromString(hostsConfig(hostTokens)), nil
			}
			viewerPermission := tt.viewerPermission
			switch viewerPermission {
			case "":
				viewerPermission = "WRITE"
			case "none":
				viewerPermission = ""
			}
			opts.RetrieveCommentable = func() (Commentable, ghrepo.Interface, error) {
				return &api.PullRequest{
					Number: 123,
					URL:    "https://github.com/OWNER/REPO/pull/123",
					Repository: &api.PRRepository{
						DatabaseID:       tt.repositoryDatabaseID,
						ViewerPermission: viewerPermission,
					},
					Comments: api.Comments{Nodes: []api.Comment{{
						ID:              "id1",
						Body:            "the original comment",
						Author:          api.CommentAuthor{Login: "octocat"},
						URL:             "https://github.com/OWNER/REPO/pull/123#issuecomment-111",
						ViewerDidAuthor: true,
					}}},
				}, ghrepo.NewWithHost("OWNER", "REPO", host), nil
			}

			err := CommentableRun(&opts)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantBody, gotBody)
			assert.Equal(t, tt.wantStdout, stdout.String())

			uploads := 0
			for _, req := range reg.Requests {
				if req.URL.Path == "/user-attachments/assets" {
					uploads++
				}
			}
			assert.Equal(t, tt.wantUploads, uploads, "asset uploads")
		})
	}
}

func hostsConfig(tokens map[string]string) string {
	var b strings.Builder
	b.WriteString("hosts:\n")
	for host, token := range tokens {
		fmt.Fprintf(&b, "  %s:\n    user: monalisa\n    oauth_token: %s\n", host, token)
	}
	return b.String()
}
