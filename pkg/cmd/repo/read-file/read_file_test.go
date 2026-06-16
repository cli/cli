package readfile

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/jsonfieldstest"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFields(t *testing.T) {
	jsonfieldstest.ExpectCommandToSupportJSONFields(t, NewCmdReadFile, []string{
		"name",
		"path",
		"gitSHA",
		"size",
		"url",
		"htmlUrl",
		"gitUrl",
		"downloadUrl",
		"type",
		"encoding",
		"content",
	})
}

func Test_repoFile_ExportData(t *testing.T) {
	file := &repoFile{
		Name:        "README.md",
		Path:        "docs/README.md",
		SHA:         "abc",
		Size:        5,
		URL:         "https://api.github.com/repos/OWNER/REPO/contents/docs/README.md",
		HTMLURL:     "https://github.com/OWNER/REPO/blob/main/docs/README.md",
		GitURL:      "https://api.github.com/repos/OWNER/REPO/git/blobs/abc",
		DownloadURL: "https://raw.githubusercontent.com/OWNER/REPO/main/docs/README.md",
		Type:        "file",
		Content:     []byte("hello"),
	}

	data := file.ExportData(fileFields)

	assert.Equal(t, "README.md", data["name"])
	assert.Equal(t, "docs/README.md", data["path"])
	assert.Equal(t, "abc", data["gitSHA"])
	assert.Equal(t, 5, data["size"])
	assert.Equal(t, "https://api.github.com/repos/OWNER/REPO/contents/docs/README.md", data["url"])
	assert.Equal(t, "https://github.com/OWNER/REPO/blob/main/docs/README.md", data["htmlUrl"])
	assert.Equal(t, "https://api.github.com/repos/OWNER/REPO/git/blobs/abc", data["gitUrl"])
	assert.Equal(t, "https://raw.githubusercontent.com/OWNER/REPO/main/docs/README.md", data["downloadUrl"])
	assert.Equal(t, "file", data["type"])
	assert.Equal(t, "base64", data["encoding"])
	assert.Equal(t, "aGVsbG8=", data["content"])
}

func TestNewCmdReadFile(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantOpts ReadFileOptions
		wantErr  string
	}{
		{
			name: "path only",
			args: "README.md",
			wantOpts: ReadFileOptions{
				Path: "README.md",
			},
		},
		{
			name: "with ref",
			args: "README.md --ref v1.2.3",
			wantOpts: ReadFileOptions{
				Path: "README.md",
				Ref:  "v1.2.3",
			},
		},
		{
			name: "with output and clobber",
			args: "README.md --output out.md --clobber",
			wantOpts: ReadFileOptions{
				Path:    "README.md",
				Output:  "out.md",
				Clobber: true,
			},
		},
		{
			name: "with allow-escape-sequences",
			args: "README.md --allow-escape-sequences",
			wantOpts: ReadFileOptions{
				Path:                 "README.md",
				AllowEscapeSequences: true,
			},
		},
		{
			name:    "no arguments",
			args:    "",
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:    "too many arguments",
			args:    "a.md b.md",
			wantErr: "accepts 1 arg(s), received 2",
		},
		{
			name:    "json and output are mutually exclusive",
			args:    "README.md --json name --output out.md",
			wantErr: "specify only one of `--json` or `--output`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var gotOpts *ReadFileOptions
			cmd := NewCmdReadFile(f, func(opts *ReadFileOptions) error {
				gotOpts = opts
				return nil
			})

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.Path, gotOpts.Path)
			assert.Equal(t, tt.wantOpts.Ref, gotOpts.Ref)
			assert.Equal(t, tt.wantOpts.Output, gotOpts.Output)
			assert.Equal(t, tt.wantOpts.Clobber, gotOpts.Clobber)
			assert.Equal(t, tt.wantOpts.AllowEscapeSequences, gotOpts.AllowEscapeSequences)
		})
	}
}

func Test_readFileRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       ReadFileOptions
		httpStubs  func(*httpmock.Registry)
		jsonFields []string
		wantOut    string
		wantStderr string
		wantErrMsg string
	}{
		{
			name: "base repo resolution error is wrapped with a hint",
			tty:  false,
			opts: ReadFileOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return nil, errors.New("some error")
				},
			},
			wantErrMsg: "some error. Run this command from within a git repository, or use the `--repo` flag to specify one",
		},
		{
			name: "writes file content to stdout (non-tty)",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					func(req *http.Request) bool {
						return req.Method == "GET" &&
							req.URL.EscapedPath() == "/repos/OWNER/REPO/contents/README.md" &&
							req.Header.Get("Accept") == "application/vnd.github.object+json"
					},
					httpmock.JSONResponse(fileContentResponse("README.md", "hello world\n")),
				)
			},
			wantOut: "hello world\n",
		},
		{
			name: "writes file content through pager (tty)",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/README.md"),
					httpmock.JSONResponse(fileContentResponse("README.md", "hello world\n")),
				)
			},
			wantOut: "hello world\n",
		},
		{
			name: "reads from a ref",
			tty:  false,
			opts: ReadFileOptions{Ref: "v1.2.3"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					func(req *http.Request) bool {
						return req.Method == "GET" && strings.HasSuffix(req.URL.String(), "/repos/OWNER/REPO/contents/README.md?ref=v1.2.3")
					},
					httpmock.JSONResponse(fileContentResponse("README.md", "tagged\n")),
				)
			},
			wantOut: "tagged\n",
		},
		{
			name: "path and ref are URL escaped",
			tty:  false,
			opts: ReadFileOptions{Path: "dir with spaces/file.txt", Ref: "feature/branch"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					func(req *http.Request) bool {
						return req.Method == "GET" &&
							req.URL.EscapedPath() == "/repos/OWNER/REPO/contents/dir%20with%20spaces%2Ffile.txt" &&
							req.URL.RawQuery == "ref=feature%2Fbranch"
					},
					httpmock.JSONResponse(fileContentResponse("file.txt", "escaped\n")),
				)
			},
			wantOut: "escaped\n",
		},
		{
			name: "path not found propagates the api error",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/missing.md"),
					httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
				)
			},
			opts:       ReadFileOptions{Path: "missing.md"},
			wantErrMsg: "HTTP 404",
		},
		{
			name: "json output maps all metadata fields",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/meta.md"),
					httpmock.JSONResponse(map[string]interface{}{
						"type":         "file",
						"name":         "meta.md",
						"path":         "meta.md",
						"size":         5,
						"encoding":     "base64",
						"content":      "aGVsbG8=",
						"sha":          "deadbeef",
						"url":          "https://api.github.com/repos/OWNER/REPO/contents/meta.md",
						"html_url":     "https://github.com/OWNER/REPO/blob/main/meta.md",
						"git_url":      "https://api.github.com/repos/OWNER/REPO/git/blobs/deadbeef",
						"download_url": "https://raw.githubusercontent.com/OWNER/REPO/main/meta.md",
					}),
				)
			},
			opts:       ReadFileOptions{Path: "meta.md"},
			jsonFields: []string{"name", "path", "gitSHA", "size", "url", "htmlUrl", "gitUrl", "downloadUrl", "type"},
			wantOut: "{" +
				"\"downloadUrl\":\"https://raw.githubusercontent.com/OWNER/REPO/main/meta.md\"," +
				"\"gitSHA\":\"deadbeef\"," +
				"\"gitUrl\":\"https://api.github.com/repos/OWNER/REPO/git/blobs/deadbeef\"," +
				"\"htmlUrl\":\"https://github.com/OWNER/REPO/blob/main/meta.md\"," +
				"\"name\":\"meta.md\"," +
				"\"path\":\"meta.md\"," +
				"\"size\":5," +
				"\"type\":\"file\"," +
				"\"url\":\"https://api.github.com/repos/OWNER/REPO/contents/meta.md\"" +
				"}\n",
		},
		{
			name: "empty file warns and writes nothing",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/empty.txt"),
					httpmock.JSONResponse(fileContentResponse("empty.txt", "")),
				)
			},
			opts: ReadFileOptions{
				Path: "empty.txt",
			},
			wantOut:    "",
			wantStderr: "! file is empty\n",
		},
		{
			name: "directory path errors",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/src"),
					httpmock.JSONResponse(map[string]interface{}{"type": "dir", "path": "src"}),
				)
			},
			opts:       ReadFileOptions{Path: "src"},
			wantErrMsg: "path \"src\" is a directory; use `gh repo read-dir` instead",
		},
		{
			name: "broken symlink path errors",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/link"),
					httpmock.JSONResponse(map[string]interface{}{"type": "symlink", "path": "link", "target": "missing.txt"}),
				)
			},
			opts:       ReadFileOptions{Path: "link"},
			wantErrMsg: `path "link" is a symlink to "missing.txt" which does not exist`,
		},
		{
			name: "submodule path errors",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/sub"),
					httpmock.JSONResponse(map[string]interface{}{
						"type":              "submodule",
						"path":              "sub",
						"submodule_git_url": "https://github.com/OWNER/sub",
						"sha":               "abc123",
					}),
				)
			},
			opts:       ReadFileOptions{Path: "sub"},
			wantErrMsg: `path "sub" is a submodule (https://github.com/OWNER/sub at abc123)`,
		},
		{
			name: "binary file in tty errors",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/img.png"),
					httpmock.JSONResponse(fileContentResponseBytes("img.png", pngBytes())),
				)
			},
			opts:       ReadFileOptions{Path: "img.png"},
			wantErrMsg: "binary file (image/png",
		},
		{
			name: "binary file in non-tty writes bytes",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/img.png"),
					httpmock.JSONResponse(fileContentResponseBytes("img.png", pngBytes())),
				)
			},
			opts:    ReadFileOptions{Path: "img.png"},
			wantOut: string(pngBytes()),
		},
		{
			name: "escape sequences refused without allow-escape-sequences (non-tty)",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/esc.txt"),
					httpmock.JSONResponse(fileContentResponse("esc.txt", "danger\x1b[31m")),
				)
			},
			opts:       ReadFileOptions{Path: "esc.txt"},
			wantErrMsg: "file contains terminal escape sequences; use --allow-escape-sequences to read anyway",
		},
		{
			name: "escape sequences refused without allow-escape-sequences (tty)",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/esc.txt"),
					httpmock.JSONResponse(fileContentResponse("esc.txt", "danger\x1b[31m")),
				)
			},
			opts:       ReadFileOptions{Path: "esc.txt"},
			wantErrMsg: "file contains terminal escape sequences; use --allow-escape-sequences to read anyway",
		},
		{
			name: "escape sequences allowed with allow-escape-sequences (non-tty)",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/esc.txt"),
					httpmock.JSONResponse(fileContentResponse("esc.txt", "danger\x1b[31m")),
				)
			},
			opts:    ReadFileOptions{Path: "esc.txt", AllowEscapeSequences: true},
			wantOut: "danger\x1b[31m",
		},
		{
			name: "escape sequences allowed with allow-escape-sequences (tty)",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/esc.txt"),
					httpmock.JSONResponse(fileContentResponse("esc.txt", "danger\x1b[31m")),
				)
			},
			opts:    ReadFileOptions{Path: "esc.txt", AllowEscapeSequences: true},
			wantOut: "danger\x1b[31m",
		},
		{
			name: "large file fetches raw content on demand",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/big.txt"),
					httpmock.JSONResponse(map[string]interface{}{
						"type":     "file",
						"name":     "big.txt",
						"path":     "big.txt",
						"size":     99999999, // Some big number
						"encoding": "none",   // API returns "none" encoding when content is not included
						"content":  "",       // Confirmed via the live API: content is present but empty, not omitted
					}),
				)
				reg.Register(
					func(req *http.Request) bool {
						return req.URL.EscapedPath() == "/repos/OWNER/REPO/contents/big.txt" &&
							req.Header.Get("Accept") == "application/vnd.github.raw"
					},
					httpmock.StringResponse("huge!"),
				)
			},
			opts:    ReadFileOptions{Path: "big.txt"},
			wantOut: "huge!",
		},
		{
			name: "large file raw content fetch fails",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/big.txt"),
					httpmock.JSONResponse(map[string]interface{}{
						"type":     "file",
						"name":     "big.txt",
						"path":     "big.txt",
						"size":     99999999,
						"encoding": "none",
						"content":  "",
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/big.txt"),
					httpmock.StatusStringResponse(404, `{"message":"Not Found"}`),
				)
			},
			opts:       ReadFileOptions{Path: "big.txt"},
			wantErrMsg: "HTTP 404",
		},
		{
			name: "json with content uses inline content without extra fetch",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/small.txt"),
					httpmock.JSONResponse(fileContentResponse("small.txt", "hello")),
				)
			},
			opts:       ReadFileOptions{Path: "small.txt"},
			jsonFields: []string{"name", "encoding", "content"},
			wantOut:    "{\"content\":\"aGVsbG8=\",\"encoding\":\"base64\",\"name\":\"small.txt\"}\n",
		},
		{
			name: "json metadata only skips content fetch",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/big.txt"),
					httpmock.JSONResponse(map[string]interface{}{
						"type":     "file",
						"name":     "big.txt",
						"path":     "big.txt",
						"size":     99999999, // Some big number
						"encoding": "none",
						"content":  "",
					}),
				)
			},
			opts:       ReadFileOptions{Path: "big.txt"},
			jsonFields: []string{"name", "size", "encoding"}, // "encoding" doesn't make much sense, but it's included to confirm the "none" returned by the API is not passed through
			wantOut:    "{\"encoding\":\"base64\",\"name\":\"big.txt\",\"size\":99999999}\n",
		},
		{
			name: "json with content fetches raw bytes",
			tty:  false,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/big.txt"),
					httpmock.JSONResponse(map[string]interface{}{
						"type":     "file",
						"name":     "big.txt",
						"path":     "big.txt",
						"size":     99999999, // Some big number
						"encoding": "none",
						"content":  "",
					}),
				)
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/contents/big.txt"),
					httpmock.StringResponse("huge!"),
				)
			},
			opts:       ReadFileOptions{Path: "big.txt"},
			jsonFields: []string{"name", "encoding", "content"},
			wantOut:    "{\"content\":\"aHVnZSE=\",\"encoding\":\"base64\",\"name\":\"big.txt\"}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)

			opts := tt.opts
			opts.IO = ios
			if opts.Path == "" {
				opts.Path = "README.md"
			}
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			if opts.BaseRepo == nil {
				opts.BaseRepo = func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				}
			}

			if tt.jsonFields != nil {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(tt.jsonFields)
				opts.Exporter = exporter
			}

			err := readFileRun(&opts)
			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func Test_writeToOutput(t *testing.T) {
	tests := []struct {
		name      string
		file      *repoFile
		output    string
		clobber   bool
		lstat     func(path string) (*lstatResult, error)
		wantDest  string
		wantWrite string
		wantDir   string
		wantErr   string
	}{
		{
			name:      "writes to a new file path",
			file:      &repoFile{Name: "README.md", Content: []byte("hi")},
			output:    "out/README.md",
			lstat:     func(string) (*lstatResult, error) { return nil, fs.ErrNotExist },
			wantDest:  "out/README.md",
			wantWrite: "hi",
			wantDir:   "out",
		},
		{
			name:   "directory target uses remote basename",
			file:   &repoFile{Name: "README.md", Content: []byte("hi")},
			output: "out/",
			lstat: func(path string) (*lstatResult, error) {
				if path == "out/" {
					return &lstatResult{isDir: true}, nil
				}
				return nil, fs.ErrNotExist
			},
			wantDest:  filepath.Join("out", "README.md"),
			wantWrite: "hi",
			wantDir:   "out",
		},
		{
			name:    "existing file without clobber errors",
			file:    &repoFile{Name: "README.md", Content: []byte("hi")},
			output:  "README.md",
			lstat:   func(string) (*lstatResult, error) { return &lstatResult{}, nil },
			wantErr: `output path already exists: "README.md" (use --clobber to overwrite)`,
		},
		{
			name:      "existing file with clobber overwrites",
			file:      &repoFile{Name: "README.md", Content: []byte("hi")},
			output:    "README.md",
			clobber:   true,
			lstat:     func(string) (*lstatResult, error) { return &lstatResult{}, nil },
			wantDest:  "README.md",
			wantWrite: "hi",
		},
		{
			name:    "symlink target is refused",
			file:    &repoFile{Name: "README.md", Content: []byte("hi")},
			output:  "link.md",
			lstat:   func(string) (*lstatResult, error) { return &lstatResult{isSymlink: true}, nil },
			wantErr: "output path is a symlink",
		},
		{
			name:    "initial path lstat error is propagated",
			file:    &repoFile{Name: "README.md", Content: []byte("hi")},
			output:  "out/README.md",
			lstat:   func(string) (*lstatResult, error) { return nil, fmt.Errorf("something went wrong") },
			wantErr: "something went wrong",
		},
		{
			name:   "final path lstat error is propagated",
			file:   &repoFile{Name: "README.md", Content: []byte("hi")},
			output: "out/",
			lstat: func(path string) (*lstatResult, error) {
				if path == "out/" {
					return &lstatResult{isDir: true}, nil
				}
				return nil, fmt.Errorf("something went wrong")
			},
			wantErr: "something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotDir string
			var gotWritePath string
			var gotWriteContent []byte

			origLstat, origMkdir, origWrite := lstatF, mkdirAllF, writeFileF
			defer func() {
				lstatF, mkdirAllF, writeFileF = origLstat, origMkdir, origWrite
			}()

			lstatF = tt.lstat
			mkdirAllF = func(path string, _ fs.FileMode) error {
				gotDir = path
				return nil
			}
			writeFileF = func(path string, data []byte, _ fs.FileMode) error {
				gotWritePath = path
				gotWriteContent = data
				return nil
			}

			dest, err := writeToOutput(tt.file, tt.output, tt.clobber)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDest, dest)
			assert.Equal(t, tt.wantDest, gotWritePath)
			assert.Equal(t, tt.wantWrite, string(gotWriteContent))
			if tt.wantDir != "" {
				assert.Equal(t, tt.wantDir, gotDir)
			}
		})
	}
}

// fileContentResponse builds a Contents API object response for a regular file
// with base64-encoded inline content.
func fileContentResponse(name, content string) map[string]interface{} {
	return fileContentResponseBytes(name, []byte(content))
}

func fileContentResponseBytes(name string, content []byte) map[string]interface{} {
	return map[string]interface{}{
		"type":         "file",
		"name":         name,
		"path":         name,
		"size":         len(content),
		"encoding":     "base64",
		"content":      base64.StdEncoding.EncodeToString(content),
		"sha":          "deadbeef",
		"url":          "https://api.github.com/repos/OWNER/REPO/contents/" + name,
		"git_url":      "https://api.github.com/repos/OWNER/REPO/git/blobs/deadbeef",
		"html_url":     "https://github.com/OWNER/REPO/blob/main/" + name,
		"download_url": "https://raw.githubusercontent.com/OWNER/REPO/main/" + name,
		"_links": map[string]interface{}{
			"self": "https://api.github.com/repos/OWNER/REPO/contents/" + name,
			"git":  "https://api.github.com/repos/OWNER/REPO/git/blobs/deadbeef",
			"html": "https://github.com/OWNER/REPO/blob/main/" + name,
		},
	}
}

func pngBytes() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 16)...)
}

func Test_contentsAPIPath(t *testing.T) {
	repo := ghrepo.New("OWNER", "REPO")

	tests := []struct {
		name     string
		filePath string
		ref      string
		want     string
	}{
		{
			name:     "simple path",
			filePath: "README.md",
			want:     "https://api.github.com/repos/OWNER/REPO/contents/README.md",
		},
		{
			name:     "leading slash trimmed",
			filePath: "/README.md",
			want:     "https://api.github.com/repos/OWNER/REPO/contents/README.md",
		},
		{
			name:     "nested path encodes separators",
			filePath: "pkg/cmd/create.go",
			want:     "https://api.github.com/repos/OWNER/REPO/contents/pkg%2Fcmd%2Fcreate.go",
		},
		{
			name:     "spaces are encoded",
			filePath: "dir with spaces/file.txt",
			want:     "https://api.github.com/repos/OWNER/REPO/contents/dir%20with%20spaces%2Ffile.txt",
		},
		{
			name:     "ref is appended",
			filePath: "README.md",
			ref:      "feature/branch",
			want:     "https://api.github.com/repos/OWNER/REPO/contents/README.md?ref=feature%2Fbranch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contentsAPIPath(repo, tt.filePath, tt.ref)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_binaryContentType(t *testing.T) {
	tests := []struct {
		name       string
		content    []byte
		wantMIME   string
		wantBinary bool
	}{
		{
			name:       "empty content is not binary",
			content:    []byte{},
			wantBinary: false,
		},
		{
			name:       "plain text is not binary",
			content:    []byte("hello world\n"),
			wantBinary: false,
		},
		{
			name:       "png is binary",
			content:    append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 16)...),
			wantMIME:   "image/png",
			wantBinary: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, ok := binaryContentType(tt.content)
			assert.Equal(t, tt.wantBinary, ok)
			assert.Equal(t, tt.wantMIME, mime)
		})
	}
}

func Test_containsEscapeSequence(t *testing.T) {
	assert.False(t, containsEscapeSequence([]byte("plain text")))
	assert.True(t, containsEscapeSequence([]byte("danger\x1b[31m")))
}
