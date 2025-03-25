package list

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name    string
		cli     string
		wants   ListOptions
		wantErr string
	}{
		{
			name:    "no arg",
			cli:     "",
			wantErr: "must specify username",
		},
		{
			name: "normal",
			cli:  "johndoe",
			wants: ListOptions{
				Username: "johndoe",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var listOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
				listOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.Equal(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)

			require.Equal(t, tt.wants.Username, listOpts.Username)
		})
	}
}

func Test_listRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       *ListOptions
		httpStubs  func(*httpmock.Registry)
		wantStdout []string
		wantStderr string
		wantErr    string
	}{
		{
			name: "normal tty",
			tty:  true,
			opts: &ListOptions{
				Username: "johndoe",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserSponsorList\b`),
					httpmock.GraphQLQuery(`
						{
							"data": {
								"user": {
									"sponsors": {
										"edges": [
											{
												"node": {
													"login": "foo"
												}
											},
											{
												"node": {
													"login": "bar"
												}
											}
										]
									}
								}
							}
						}`,
						func(_ string, inputs map[string]interface{}) {
							assert.Equal(t, "johndoe", inputs["login"])
							assert.Equal(t, float64(30), inputs["limit"])
						},
					),
				)
			},
			wantStdout: []string{
				"SPONSOR",
				"foo",
				"bar",
			},
		}, {
			name: "normal tty, no sponsor",
			tty:  true,
			opts: &ListOptions{
				Username: "johndoe",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserSponsorList\b`),
					httpmock.GraphQLQuery(`
						{
							"data": {
								"user": {
									"sponsors": {
										"edges": []
									}
								}
							}
						}`,
						func(_ string, inputs map[string]interface{}) {
							assert.Equal(t, "johndoe", inputs["login"])
							assert.Equal(t, float64(30), inputs["limit"])
						},
					),
				)
			},
			wantStdout: []string{"no sponsor found"},
		}, {
			name: "api error",
			tty:  true,
			opts: &ListOptions{
				Username: "johndoe",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserSponsorList\b`),
					httpmock.GraphQLQuery(
						`{"data":{}, "errors": [{"message": "some gql error"}]}`,
						func(query string, inputs map[string]interface{}) {},
					),
				)
			},
			wantErr: "GraphQL: some gql error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			ios, _, stdout, stderr := iostreams.Test()

			ios.SetStdoutTTY(tt.tty)

			tt.opts.IO = ios
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}

			tt.httpStubs(reg)

			err := listRun(tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			expectedStdout := ""
			if len(tt.wantStdout) > 0 {
				expectedStdout = fmt.Sprintf("%s\n", strings.Join(tt.wantStdout, "\n"))
			}
			assert.Equal(t, expectedStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
