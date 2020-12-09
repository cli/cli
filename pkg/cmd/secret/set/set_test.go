package set

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmd/secret/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdSet(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    SetOptions
		stdinTTY bool
		wantsErr bool
	}{
		{
			name:     "invalid visibility",
			cli:      "cool_secret --org -v'mistyVeil'",
			wantsErr: true,
		},
		{
			name:     "invalid visibility",
			cli:      "cool_secret --org -v'selected'",
			wantsErr: true,
		},
		{
			name:     "no name",
			cli:      "",
			wantsErr: true,
		},
		{
			name:     "multiple names",
			cli:      "cool_secret good_secret",
			wantsErr: true,
		},
		{
			name:     "no body, stdin is terminal",
			cli:      "cool_secret",
			stdinTTY: true,
			wantsErr: true,
		},
		{
			name:     "visibility without org",
			cli:      "cool_secret -vall",
			wantsErr: true,
		},
		{
			name: "explicit org with selected repo",
			cli:  "--org=coolOrg -vselected -rcoolRepo cool_secret",
			wants: SetOptions{
				SecretName:      "cool_secret",
				Visibility:      shared.VisSelected,
				RepositoryNames: []string{"coolRepo"},
				Body:            "-",
				OrgName:         "coolOrg",
			},
		},
		{
			name: "explicit org with selected repos",
			cli:  `--org=coolOrg -vselected -r="coolRepo,radRepo,goodRepo" cool_secret`,
			wants: SetOptions{
				SecretName:      "cool_secret",
				Visibility:      shared.VisSelected,
				RepositoryNames: []string{"coolRepo", "goodRepo", "radRepo"},
				Body:            "-",
				OrgName:         "coolOrg",
			},
		},
		{
			name: "repo",
			cli:  `cool_secret -b"a secret"`,
			wants: SetOptions{
				SecretName: "cool_secret",
				Visibility: shared.VisPrivate,
				Body:       "a secret",
				OrgName:    "",
			},
		},
		{
			name: "implicit org",
			cli:  `cool_secret --org -b"@cool.json"`,
			wants: SetOptions{
				SecretName: "cool_secret",
				Visibility: shared.VisPrivate,
				Body:       "@cool.json",
				OrgName:    "@owner",
			},
		},
		{
			name: "vis all",
			cli:  `cool_secret --org -b"@cool.json" -vall`,
			wants: SetOptions{
				SecretName: "cool_secret",
				Visibility: shared.VisAll,
				Body:       "@cool.json",
				OrgName:    "@owner",
			},
		},
		{
			name:     "bad name prefix",
			cli:      `GITHUB_SECRET -b"cool"`,
			wantsErr: true,
		},
		{
			name:     "leading numbers in name",
			cli:      `123_SECRET -b"cool"`,
			wantsErr: true,
		},
		{
			name:     "invalid characters in name",
			cli:      `BAD-SECRET -b"cool"`,
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}

			io.SetStdinTTY(tt.stdinTTY)

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *SetOptions
			cmd := NewCmdSet(f, func(opts *SetOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.SecretName, gotOpts.SecretName)
			assert.Equal(t, tt.wants.Body, gotOpts.Body)
			assert.Equal(t, tt.wants.Visibility, gotOpts.Visibility)
			assert.Equal(t, tt.wants.OrgName, gotOpts.OrgName)
			assert.ElementsMatch(t, tt.wants.RepositoryNames, gotOpts.RepositoryNames)
		})
	}
}

func Test_setRun_repo(t *testing.T) {
	reg := &httpmock.Registry{}

	reg.Register(httpmock.REST("GET", "repos/owner/repo/actions/secrets/public-key"),
		httpmock.JSONResponse(PubKey{ID: "123", Key: "CDjXqf7AJBXWhMczcy+Fs7JlACEptgceysutztHaFQI="}))

	reg.Register(httpmock.REST("PUT", "repos/owner/repo/actions/secrets/cool_secret"), httpmock.StatusStringResponse(201, `{}`))

	mockClient := func() (*http.Client, error) {
		return &http.Client{Transport: reg}, nil
	}

	io, _, _, _ := iostreams.Test()

	opts := &SetOptions{
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("owner/repo")
		},
		HttpClient: mockClient,
		IO:         io,
		SecretName: "cool_secret",
		Body:       "a secret",
		// Cribbed from https://github.com/golang/crypto/commit/becbf705a91575484002d598f87d74f0002801e7
		RandomOverride: bytes.NewReader([]byte{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5}),
	}

	err := setRun(opts)
	assert.NoError(t, err)

	reg.Verify(t)

	data, err := ioutil.ReadAll(reg.Requests[1].Body)
	assert.NoError(t, err)
	var payload SecretPayload
	err = json.Unmarshal(data, &payload)
	assert.NoError(t, err)
	assert.Equal(t, payload.KeyID, "123")
	assert.Equal(t, payload.EncryptedValue, "UKYUCbHd0DJemxa3AOcZ6XcsBwALG9d4bpB8ZT0gSV39vl3BHiGSgj8zJapDxgB2BwqNqRhpjC4=")
}

func Test_setRun_org(t *testing.T) {
	tests := []struct {
		name             string
		opts             *SetOptions
		wantVisibility   string
		wantRepositories []int
	}{
		{
			name: "explicit org name",
			opts: &SetOptions{
				OrgName:    "UmbrellaCorporation",
				Visibility: shared.VisAll,
			},
		},
		{
			name: "implicit org name",
			opts: &SetOptions{
				OrgName:    "@owner",
				Visibility: shared.VisPrivate,
			},
		},
		{
			name: "selected visibility",
			opts: &SetOptions{
				OrgName:         "UmbrellaCorporation",
				Visibility:      shared.VisSelected,
				RepositoryNames: []string{"birkin", "wesker"},
			},
			wantRepositories: []int{1, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}

			orgName := tt.opts.OrgName
			if orgName == "@owner" {
				orgName = "NeoUmbrella"
			}

			reg.Register(httpmock.REST("GET",
				fmt.Sprintf("orgs/%s/actions/secrets/public-key", orgName)),
				httpmock.JSONResponse(PubKey{ID: "123", Key: "CDjXqf7AJBXWhMczcy+Fs7JlACEptgceysutztHaFQI="}))

			reg.Register(httpmock.REST("PUT",
				fmt.Sprintf("orgs/%s/actions/secrets/cool_secret", orgName)),
				httpmock.StatusStringResponse(201, `{}`))

			if len(tt.opts.RepositoryNames) > 0 {
				reg.Register(httpmock.GraphQL(`query MapRepositoryNames\b`),
					httpmock.StringResponse(`{"data":{"birkin":{"databaseId":1},"wesker":{"databaseId":2}}}`))
			}

			io, _, _, _ := iostreams.Test()

			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("NeoUmbrella/repo")
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.opts.IO = io
			tt.opts.SecretName = "cool_secret"
			tt.opts.Body = "a secret"
			// Cribbed from https://github.com/golang/crypto/commit/becbf705a91575484002d598f87d74f0002801e7
			tt.opts.RandomOverride = bytes.NewReader([]byte{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5})

			err := setRun(tt.opts)
			assert.NoError(t, err)

			reg.Verify(t)

			data, err := ioutil.ReadAll(reg.Requests[len(reg.Requests)-1].Body)
			assert.NoError(t, err)
			var payload SecretPayload
			err = json.Unmarshal(data, &payload)
			assert.NoError(t, err)
			assert.Equal(t, payload.KeyID, "123")
			assert.Equal(t, payload.EncryptedValue, "UKYUCbHd0DJemxa3AOcZ6XcsBwALG9d4bpB8ZT0gSV39vl3BHiGSgj8zJapDxgB2BwqNqRhpjC4=")
			assert.Equal(t, payload.Visibility, tt.opts.Visibility)
			assert.ElementsMatch(t, payload.Repositories, tt.wantRepositories)
		})
	}
}

func Test_getBody(t *testing.T) {
	tests := []struct {
		name     string
		bodyArg  string
		want     string
		stdin    string
		fromFile bool
	}{
		{
			name:    "literal value",
			bodyArg: "a secret",
			want:    "a secret",
		},
		{
			name:    "from stdin",
			bodyArg: "-",
			want:    "a secret",
			stdin:   "a secret",
		},
		{
			name:     "from file",
			fromFile: true,
			want:     "a secret from a file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, stdin, _, _ := iostreams.Test()

			io.SetStdinTTY(false)

			_, err := stdin.WriteString(tt.stdin)
			assert.NoError(t, err)

			if tt.fromFile {
				dir := os.TempDir()
				tmpfile, err := ioutil.TempFile(dir, "testfile*")
				assert.NoError(t, err)
				_, err = tmpfile.WriteString(tt.want)
				assert.NoError(t, err)
				tt.bodyArg = fmt.Sprintf("@%s", tmpfile.Name())
			}

			body, err := getBody(&SetOptions{
				Body: tt.bodyArg,
				IO:   io,
			})
			assert.NoError(t, err)

			assert.Equal(t, string(body), tt.want)

		})

	}

}

// TODO test updating org secret's repo lists
