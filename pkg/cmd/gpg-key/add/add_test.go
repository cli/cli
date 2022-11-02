package add

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func Test_runAdd(t *testing.T) {
	tests := []struct {
		name       string
		stdin      string
		httpStubs  func(*httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErrMsg string
		wantsOpts  AddOptions
	}{
		{"armored_valid", "-----BEGIN PGP PUBLIC KEY BLOCK-----", func(reg *httpmock.Registry) {
			reg.Register(
				httpmock.REST("POST", "user/gpg_keys"),
				httpmock.WithHeader(
					httpmock.StatusStringResponse(200, `{}`),
					"Content-Type",
					"application/json",
				),
			)
		}, "✓ GPG key added to your account\n", "", "", AddOptions{}},
		{"not_armored", "gCAAAAA7H7MHTZWFLJKD3vP4F7v", func(reg *httpmock.Registry) {
			reg.Register(
				httpmock.REST("POST", "user/gpg_keys"),
				httpmock.WithHeader(
					httpmock.StatusStringResponse(422, `{
                                                "message": "Validation Failed",
                                                "errors": [{
                                                        "resource": "GpgKey",
                                                        "code": "custom",
                                                        "message": "We got an error doing that."
                                                }],
                                                "documentation_url": "https://docs.github.com/v3/users/gpg_keys"
                                        }`),
					"Content-Type",
					"application/json",
				),
			)
		}, "", "X it seems that the GPG key is not armored.\n" +
			"please try to find your GPG key ID using:\n" +
			"\tgpg --list-keys\n" +
			"and use command below to add it to your accont:\n" +
			"\tgpg --armor --export <GPG key ID> | gh gpg-key add -\n", "SilentError", AddOptions{}},
		{"duplicate", "-----BEGIN PGP PUBLIC KEY BLOCK-----", func(reg *httpmock.Registry) {
			reg.Register(
				httpmock.REST("POST", "user/gpg_keys"),
				httpmock.WithHeader(
					httpmock.StatusStringResponse(422, `{
                                                "message": "Validation Failed",
                                                "errors": [{
                                                        "resource": "GpgKey",
                                                        "code": "custom",
                                                        "field": "key_id",
                                                        "message": "key_id already exists"
                                                }, {
                                                        "resource": "GpgKey",
                                                        "code": "custom",
                                                        "field": "public_key",
                                                        "message": "public_key already exists"
                                                }],
                                                "documentation_url": "https://docs.github.com/v3/users/gpg_keys"
                                        }`),
					"Content-Type",
					"application/json",
				),
			)
		}, "", "X Key already exists in your account\n", "SilentError", AddOptions{}},
	}

	for _, tt := range tests {
		ios, stdin, stdout, stderr := iostreams.Test()
		ios.SetStdinTTY(true)
		ios.SetStdoutTTY(true)
		ios.SetStderrTTY(true)
		reg := &httpmock.Registry{}
		defer reg.Verify(t)

		tt.wantsOpts.IO = ios
		tt.wantsOpts.HTTPClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.wantsOpts.Config = func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		}
		tt.wantsOpts.KeyFile = "-"

		stdin.WriteString(tt.stdin)
		t.Run(tt.name, func(t *testing.T) {
			err := runAdd(&tt.wantsOpts)
			if tt.wantErrMsg != "" {
				assert.Equal(t, tt.wantErrMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
