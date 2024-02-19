package list

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    listOptions
		wantsErr bool
	}{
		{
			name: "no arguments",
			cli:  "",
			wants: listOptions{
				Limit: 10,
			},
		},
		{
			name: "limit flag",
			cli:  "--limit 1000",
			wants: listOptions{
				Limit: 1000,
			},
		},
		{
			name: "filter by environment",
			cli:  "--environment test",
			wants: listOptions{
				Limit:       10,
				Environment: "test",
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

			var gotOpts listOptions
			cmd := NewCmdList(f, func(opts *listOptions) error {
				gotOpts = *opts
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

			assert.Equal(t, tt.wants.Limit, gotOpts.Limit)
		})
	}
}

func Test_listRun(t *testing.T) {
	var now = time.Date(2023, 1, 1, 1, 1, 1, 1, time.UTC)
	tests := []struct {
		name       string
		tty        bool
		opts       *listOptions
		httpStubs  func(*httpmock.Registry)
		wantErr    bool
		wantErrMsg string
		wantStdout string
		wantStderr string
	}{
		{
			name: "no deployments",
			tty:  true,
			opts: &listOptions{},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryDeployments\b`),
					httpmock.StringResponse(`
						{
							"data": {
								"repository": {
									"deployments": {
										"nodes": [],
										"pageInfo": {
											"hasNextPage": false,
											"endCursor": "Y3Vyc29yOnYyOpK5MjAxOS0xMC0xMVQwMTozODowMyswODowMM5f3HZq"
										}
									}
								}
							}
						}`,
					),
				)
			},
			wantErr:    true,
			wantErrMsg: "no deployments found",
		},
		{
			name: "list of deployments",
			tty:  true,
			opts: &listOptions{},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryDeployments\b`),
					httpmock.StringResponse(`
						{
							"data": {
								"repository": {
									"deployments": {
										"nodes": [
											{
												"databaseId": 1,
												"environment": "production",
												"latestStatus": {
													"state": "in_progress",
													"createdAt": "2020-10-11T20:38:03Z"
												},
												"ref": {
													"name": "main"
												},
												"createdAt": "2020-10-11T20:38:03Z"
											},
											{
												"databaseId": 2,
												"environment": "test",
												"latestStatus": {
													"state": "success",
													"createdAt": "2020-10-11T20:38:03Z"
												},
												"ref": {
													"name": "test"
												},
												"createdAt": "2020-10-11T20:38:03Z"
											}
										],
										"pageInfo": {
											"hasNextPage": false,
											"endCursor": "Y3Vyc29yOnYyOpK5MjAxOS0xMC0xMVQwMTozODowMyswODowMM5f3HZq"
										}
									}
								}
							}
						}`,
					),
				)
			},
			wantStdout: heredoc.Doc(`
				ID  ENVIRONMENT  STATUS       REF   CREATED
				1   production   in_progress  main  about 2 years ago
				2   test         success      test  about 2 years ago
			`),
		},
		{
			name: "not tty",
			tty:  false,
			opts: &listOptions{},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryDeployments\b`),
					httpmock.StringResponse(`
						{
							"data": {
								"repository": {
									"deployments": {
										"nodes": [
											{
												"databaseId": 1,
												"environment": "production",
												"latestStatus": {
													"state": "IN_PROGRESS",
													"createdAt": "2020-10-11T20:38:03Z"
												},
												"ref": {
													"name": "main"
												},
												"createdAt": "2020-10-11T20:38:03Z"
											},
											{
												"databaseId": 2,
												"environment": "test",
												"latestStatus": {
													"state": "SUCCESS",
													"createdAt": "2020-10-11T20:38:03Z"
												},
												"ref": {
													"name": "test"
												},
												"createdAt": "2020-10-11T20:38:03Z"
											}
										],
										"pageInfo": {
											"hasNextPage": false,
											"endCursor": "Y3Vyc29yOnYyOpK5MjAxOS0xMC0xMVQwMTozODowMyswODowMM5f3HZq"
										}
									}
								}
							}
						}`,
					),
				)
			},
			wantStdout: heredoc.Doc(`
				1	production	in_progress	main	2020-10-11T20:38:03Z
				2	test	success	test	2020-10-11T20:38:03Z
			`),
		},
		{
			name: "no status deployments",
			tty:  false,
			opts: &listOptions{},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryDeployments\b`),
					httpmock.StringResponse(`
						{
							"data": {
								"repository": {
									"deployments": {
										"nodes": [
											{
												"databaseId": 1,
												"environment": "production",
												"latestStatus": null,
												"ref": {
													"name": "main"
												},
												"createdAt": "2020-10-11T20:38:03Z"
											},
											{
												"databaseId": 2,
												"environment": "test",
												"latestStatus": {
													"state": "SUCCESS",
													"createdAt": "2020-10-11T20:38:03Z"
												},
												"ref": {
													"name": "test"
												},
												"createdAt": "2020-10-11T20:38:03Z"
											}
										],
										"pageInfo": {
											"hasNextPage": false,
											"endCursor": "Y3Vyc29yOnYyOpK5MjAxOS0xMC0xMVQwMTozODowMyswODowMM5f3HZq"
										}
									}
								}
							}
						}`,
					),
				)
			},
			wantStdout: heredoc.Doc(`
				1	production	none	main	2020-10-11T20:38:03Z
				2	test	success	test	2020-10-11T20:38:03Z
			`),
		},
		{
			name: "no ref deployments",
			tty:  false,
			opts: &listOptions{},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryDeployments\b`),
					httpmock.StringResponse(`
						{
							"data": {
								"repository": {
									"deployments": {
										"nodes": [
											{
												"databaseId": 1,
												"environment": "production",
												"latestStatus": null,
												"ref": null,
												"createdAt": "2020-10-11T20:38:03Z"
											},
											{
												"databaseId": 2,
												"environment": "test",
												"latestStatus": {
													"state": "SUCCESS",
													"createdAt": "2020-10-11T20:38:03Z"
												},
												"ref": null,
												"createdAt": "2020-10-11T20:38:03Z"
											}
										],
										"pageInfo": {
											"hasNextPage": false,
											"endCursor": "Y3Vyc29yOnYyOpK5MjAxOS0xMC0xMVQwMTozODowMyswODowMM5f3HZq"
										}
									}
								}
							}
						}`,
					),
				)
			},
			wantStdout: heredoc.Doc(`
				1	production	none	none	2020-10-11T20:38:03Z
				2	test	success	none	2020-10-11T20:38:03Z
			`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)
			ios.SetStdinTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			tt.opts.IO = ios
			tt.opts.Now = now
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			defer reg.Verify(t)
			err := listRun(tt.opts)
			if tt.wantErr {
				if tt.wantErrMsg != "" {
					assert.EqualError(t, err, tt.wantErrMsg)
				} else {
					assert.Error(t, err)
				}
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
