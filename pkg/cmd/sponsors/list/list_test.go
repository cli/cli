package list

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
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
			name: "no arg",
			cli:  "",
			wants: ListOptions{
				Username: "",
			},
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
		name          string
		tty           bool
		opts          *ListOptions
		httpStubs     func(*testing.T, *httpmock.Registry)
		prompterStubs func(*testing.T, *prompter.PrompterMock)
		wantStdout    []string
		wantErr       string
	}{
		{
			name: "normal tty",
			tty:  true,
			opts: &ListOptions{
				Username: "johndoe",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
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
			name: "normal tty, no-username",
			tty:  true,
			opts: &ListOptions{},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
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
			prompterStubs: func(t *testing.T, pm *prompter.PrompterMock) {
				pm.InputFunc = func(message, def string) (string, error) {
					assert.Equal(t, "Which user do you want to target?", message)
					assert.Empty(t, def)
					return "johndoe", nil
				}
			},
			wantStdout: []string{
				"SPONSOR",
				"foo",
				"bar",
			},
		}, {
			name: "normal no-tty",
			tty:  false,
			opts: &ListOptions{
				Username: "johndoe",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
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
				"foo",
				"bar",
			},
		}, {
			name: "failure tty, prompt error",
			tty:  true,
			opts: &ListOptions{},
			prompterStubs: func(t *testing.T, pm *prompter.PrompterMock) {
				pm.InputFunc = func(message, def string) (string, error) {
					return "", errors.New("prompt error")
				}
			},
			wantErr: "prompt error",
		}, {
			name:    "failure no-tty, no-username",
			tty:     false,
			opts:    &ListOptions{},
			wantErr: "username not provided",
		}, {
			name: "normal tty, no sponsor",
			tty:  true,
			opts: &ListOptions{
				Username: "johndoe",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
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
			wantErr: "no sponsor found",
		}, {
			name: "normal no-tty, no sponsor",
			tty:  false,
			opts: &ListOptions{
				Username: "johndoe",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
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
			wantErr: "no sponsor found",
		}, {
			name: "api error",
			tty:  true,
			opts: &ListOptions{
				Username: "johndoe",
			},
			httpStubs: func(_ *testing.T, reg *httpmock.Registry) {
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

			pm := &prompter.PrompterMock{}
			if tt.prompterStubs != nil {
				tt.prompterStubs(t, pm)
			}
			tt.opts.Prompter = pm

			ios, _, stdout, _ := iostreams.Test()

			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)

			tt.opts.IO = ios
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}

			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

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
		})
	}
}
