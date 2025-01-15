package edit

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdEdit(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantOpts EditOptions
		wantErr  string
	}{
		{
			name: "change repo description",
			args: "--description hello",
			wantOpts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					Description: sp("hello"),
				},
			},
		},
		{
			name: "deny public visibility change without accepting consequences",
			args: "--visibility public",
			wantOpts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits:      EditRepositoryInput{},
			},
			wantErr: "use of --visibility flag requires --accept-visibility-change-consequences flag",
		},
		{
			name: "allow public visibility change with accepting consequences",
			args: "--visibility public --accept-visibility-change-consequences",
			wantOpts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					Visibility: sp("public"),
				},
			},
		},
		{
			name: "deny private visibility change without accepting consequences",
			args: "--visibility private",
			wantOpts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits:      EditRepositoryInput{},
			},
			wantErr: "use of --visibility flag requires --accept-visibility-change-consequences flag",
		},
		{
			name: "allow private visibility change with accepting consequences",
			args: "--visibility private --accept-visibility-change-consequences",
			wantOpts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					Visibility: sp("private"),
				},
			},
		},
		{
			name: "deny internal visibility change without accepting consequences",
			args: "--visibility internal",
			wantOpts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits:      EditRepositoryInput{},
			},
			wantErr: "use of --visibility flag requires --accept-visibility-change-consequences flag",
		},
		{
			name: "allow internal visibility change with accepting consequences",
			args: "--visibility internal --accept-visibility-change-consequences",
			wantOpts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					Visibility: sp("internal"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			f := &cmdutil.Factory{
				IOStreams: ios,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				HttpClient: func() (*http.Client, error) {
					return nil, nil
				},
			}

			argv, err := shlex.Split(tt.args)
			assert.NoError(t, err)

			var gotOpts *EditOptions
			cmd := NewCmdEdit(f, func(opts *EditOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, ghrepo.FullName(tt.wantOpts.Repository), ghrepo.FullName(gotOpts.Repository))
			assert.Equal(t, tt.wantOpts.Edits, gotOpts.Edits)
		})
	}
}

func Test_editRun(t *testing.T) {
	tests := []struct {
		name        string
		opts        EditOptions
		httpStubs   func(*testing.T, *httpmock.Registry)
		wantsStderr string
		wantsErr    string
	}{
		{
			name: "change name and description",
			opts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					Homepage:    sp("newURL"),
					Description: sp("hello world!"),
				},
			},
			httpStubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, 2, len(payload))
						assert.Equal(t, "newURL", payload["homepage"])
						assert.Equal(t, "hello world!", payload["description"])
					}))
			},
		},
		{
			name: "add and remove topics",
			opts: EditOptions{
				Repository:   ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				AddTopics:    []string{"topic1", "topic2"},
				RemoveTopics: []string{"topic3"},
			},
			httpStubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/topics"),
					httpmock.StringResponse(`{"names":["topic2", "topic3", "go"]}`))
				r.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/topics"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, 1, len(payload))
						assert.Equal(t, []interface{}{"topic2", "go", "topic1"}, payload["names"])
					}))
			},
		},
		{
			name: "allow update branch",
			opts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					AllowUpdateBranch: bp(true),
				},
			},
			httpStubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, 1, len(payload))
						assert.Equal(t, true, payload["allow_update_branch"])
					}))
			},
		},
		{
			name: "enable/disable security and analysis settings",
			opts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					SecurityAndAnalysis: &SecurityAndAnalysisInput{
						EnableAdvancedSecurity: &SecurityAndAnalysisStatus{
							Status: sp("enabled"),
						},
						EnableSecretScanning: &SecurityAndAnalysisStatus{
							Status: sp("enabled"),
						},
						EnableSecretScanningPushProtection: &SecurityAndAnalysisStatus{
							Status: sp("disabled"),
						},
					},
				},
			},
			httpStubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data": { "repository": { "viewerCanAdminister": true } } }`))

				r.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, 1, len(payload))
						securityAndAnalysis := payload["security_and_analysis"].(map[string]interface{})
						assert.Equal(t, "enabled", securityAndAnalysis["advanced_security"].(map[string]interface{})["status"])
						assert.Equal(t, "enabled", securityAndAnalysis["secret_scanning"].(map[string]interface{})["status"])
						assert.Equal(t, "disabled", securityAndAnalysis["secret_scanning_push_protection"].(map[string]interface{})["status"])
					}))
			},
		},
		{
			name: "does not have sufficient permissions for security edits",
			opts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					SecurityAndAnalysis: &SecurityAndAnalysisInput{
						EnableAdvancedSecurity: &SecurityAndAnalysisStatus{
							Status: sp("enabled"),
						},
						EnableSecretScanning: &SecurityAndAnalysisStatus{
							Status: sp("enabled"),
						},
						EnableSecretScanningPushProtection: &SecurityAndAnalysisStatus{
							Status: sp("disabled"),
						},
					},
				},
			},
			httpStubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data": { "repository": { "viewerCanAdminister": false } } }`))
			},
			wantsErr: "you do not have sufficient permissions to edit repository security and analysis features",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(t, httpReg)
			}

			opts := &tt.opts
			opts.HTTPClient = &http.Client{Transport: httpReg}
			opts.IO = ios
			err := editRun(context.Background(), opts)
			if tt.wantsErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantsErr)
				return
			}
		})
	}
}

func Test_editRun_interactive(t *testing.T) {
	editList := []string{
		"Default Branch Name",
		"Description",
		"Home Page URL",
		"Issues",
		"Merge Options",
		"Projects",
		"Template Repository",
		"Topics",
		"Visibility",
		"Wikis"}

	tests := []struct {
		name        string
		opts        EditOptions
		promptStubs func(*prompter.MockPrompter)
		httpStubs   func(*testing.T, *httpmock.Registry)
		wantsStderr string
		wantsErr    string
	}{
		{
			name: "forking of org repo",
			opts: EditOptions{
				Repository:      ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				InteractiveMode: true,
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				el := append(editList, optionAllowForking)
				pm.RegisterMultiSelect("What do you want to edit?", nil, el,
					func(_ string, _, opts []string) ([]int, error) {
						return []int{10}, nil
					})
				pm.RegisterConfirm("Allow forking (of an organization repository)?", func(_ string, _ bool) (bool, error) {
					return true, nil
				})
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"visibility": "public",
								"description": "description",
								"homePageUrl": "https://url.com",
								"defaultBranchRef": {
									"name": "main"
								},
								"isInOrganization": true,
								"repositoryTopics": {
									"nodes": [{
										"topic": {
											"name": "x"
										}
									}]
								}
							}
						}
					}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, true, payload["allow_forking"])
					}))
			},
		},
		{
			name: "skipping visibility without confirmation",
			opts: EditOptions{
				Repository:      ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				InteractiveMode: true,
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterMultiSelect("What do you want to edit?", nil, editList,
					func(_ string, _, opts []string) ([]int, error) {
						return []int{8}, nil
					})
				pm.RegisterSelect("Visibility", []string{"public", "private", "internal"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "private")
					})
				pm.RegisterConfirm("Do you want to change visibility to private?", func(_ string, _ bool) (bool, error) {
					return false, nil
				})
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"visibility": "public",
								"description": "description",
								"homePageUrl": "https://url.com",
								"defaultBranchRef": {
									"name": "main"
								},
								"stargazerCount": 10,
								"isInOrganization": false,
								"repositoryTopics": {
									"nodes": [{
										"topic": {
											"name": "x"
										}
									}]
								}
							}
						}
					}`))
				reg.Exclude(t, httpmock.REST("PATCH", "repos/OWNER/REPO"))
			},
			wantsStderr: "Changing the repository visibility to private will cause permanent loss of 10 stars and 0 watchers.",
		},
		{
			name: "changing visibility with confirmation",
			opts: EditOptions{
				Repository:      ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				InteractiveMode: true,
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterMultiSelect("What do you want to edit?", nil, editList,
					func(_ string, _, opts []string) ([]int, error) {
						return []int{8}, nil
					})
				pm.RegisterSelect("Visibility", []string{"public", "private", "internal"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "private")
					})
				pm.RegisterConfirm("Do you want to change visibility to private?", func(_ string, _ bool) (bool, error) {
					return true, nil
				})
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"visibility": "public",
								"description": "description",
								"homePageUrl": "https://url.com",
								"defaultBranchRef": {
									"name": "main"
								},
								"stargazerCount": 10,
								"watchers": {
									"totalCount": 15
								},
								"isInOrganization": false,
								"repositoryTopics": {
									"nodes": [{
										"topic": {
											"name": "x"
										}
									}]
								}
							}
						}
					}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, "private", payload["visibility"])
					}))
			},
			wantsStderr: "Changing the repository visibility to private will cause permanent loss of 10 stars and 15 watchers",
		},
		{
			name: "the rest",
			opts: EditOptions{
				Repository:      ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				InteractiveMode: true,
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterMultiSelect("What do you want to edit?", nil, editList,
					func(_ string, _, opts []string) ([]int, error) {
						return []int{0, 2, 3, 5, 6, 9}, nil
					})
				pm.RegisterInput("Default branch name", func(_, _ string) (string, error) {
					return "trunk", nil
				})
				pm.RegisterInput("Repository home page URL", func(_, _ string) (string, error) {
					return "https://zombo.com", nil
				})
				pm.RegisterConfirm("Enable Issues?", func(_ string, _ bool) (bool, error) {
					return true, nil
				})
				pm.RegisterConfirm("Enable Projects?", func(_ string, _ bool) (bool, error) {
					return true, nil
				})
				pm.RegisterConfirm("Convert into a template repository?", func(_ string, _ bool) (bool, error) {
					return true, nil
				})
				pm.RegisterConfirm("Enable Wikis?", func(_ string, _ bool) (bool, error) {
					return true, nil
				})
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"visibility": "public",
								"description": "description",
								"homePageUrl": "https://url.com",
								"defaultBranchRef": {
									"name": "main"
								},
								"stargazerCount": 10,
								"isInOrganization": false,
								"repositoryTopics": {
									"nodes": [{
										"topic": {
											"name": "x"
										}
									}]
								}
							}
						}
					}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, "trunk", payload["default_branch"])
						assert.Equal(t, "https://zombo.com", payload["homepage"])
						assert.Equal(t, true, payload["has_issues"])
						assert.Equal(t, true, payload["has_projects"])
						assert.Equal(t, true, payload["is_template"])
						assert.Equal(t, true, payload["has_wiki"])
					}))
			},
		},
		{
			name: "updates repo description",
			opts: EditOptions{
				Repository:      ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				InteractiveMode: true,
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterMultiSelect("What do you want to edit?", nil, editList,
					func(_ string, _, opts []string) ([]int, error) {
						return []int{1}, nil
					})
				pm.RegisterInput("Description of the repository",
					func(_, _ string) (string, error) {
						return "awesome repo description", nil
					})
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"description": "old description",
								"homePageUrl": "https://url.com",
								"defaultBranchRef": {
									"name": "main"
								},
								"isInOrganization": false,
								"repositoryTopics": {
									"nodes": [{
										"topic": {
											"name": "x"
										}
									}]
								}
							}
						}
					}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, "awesome repo description", payload["description"])
					}))
			},
		},
		{
			name: "updates repo topics",
			opts: EditOptions{
				Repository:      ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				InteractiveMode: true,
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterMultiSelect("What do you want to edit?", nil, editList,
					func(_ string, _, opts []string) ([]int, error) {
						return []int{1, 7}, nil
					})
				pm.RegisterInput("Description of the repository",
					func(_, _ string) (string, error) {
						return "awesome repo description", nil
					})
				pm.RegisterInput("Add topics?(csv format)",
					func(_, _ string) (string, error) {
						return "a, b,c,d ", nil
					})
				pm.RegisterMultiSelect("Remove Topics", nil, []string{"x"},
					func(_ string, _, opts []string) ([]int, error) {
						return []int{0}, nil
					})
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"description": "old description",
								"homePageUrl": "https://url.com",
								"defaultBranchRef": {
									"name": "main"
								},
								"isInOrganization": false,
								"repositoryTopics": {
									"nodes": [{
										"topic": {
											"name": "x"
										}
									}]
								}
							}
						}
					}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, "awesome repo description", payload["description"])
					}))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/topics"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, []interface{}{"a", "b", "c", "d"}, payload["names"])
					}))
			},
		},
		{
			name: "updates repo merge options",
			opts: EditOptions{
				Repository:      ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				InteractiveMode: true,
			},
			promptStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterMultiSelect("What do you want to edit?", nil, editList,
					func(_ string, _, opts []string) ([]int, error) {
						return []int{4}, nil
					})
				pm.RegisterMultiSelect("Allowed merge strategies", nil,
					[]string{allowMergeCommits, allowSquashMerge, allowRebaseMerge},
					func(_ string, _, opts []string) ([]int, error) {
						return []int{0, 2}, nil
					})
				pm.RegisterConfirm("Enable Auto Merge?", func(_ string, _ bool) (bool, error) {
					return false, nil
				})
				pm.RegisterConfirm("Automatically delete head branches after merging?", func(_ string, _ bool) (bool, error) {
					return false, nil
				})
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"description": "old description",
								"homePageUrl": "https://url.com",
								"defaultBranchRef": {
									"name": "main"
								},
								"isInOrganization": false,
								"squashMergeAllowed": false,
								"rebaseMergeAllowed": true,
								"mergeCommitAllowed": true,
								"deleteBranchOnMerge": false,
								"repositoryTopics": {
									"nodes": [{
										"topic": {
											"name": "x"
										}
									}]
								}
							}
						}
					}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, true, payload["allow_merge_commit"])
						assert.Equal(t, false, payload["allow_squash_merge"])
						assert.Equal(t, true, payload["allow_rebase_merge"])
					}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(t, httpReg)
			}

			pm := prompter.NewMockPrompter(t)
			tt.opts.Prompter = pm
			if tt.promptStubs != nil {
				tt.promptStubs(pm)
			}

			opts := &tt.opts
			opts.HTTPClient = &http.Client{Transport: httpReg}
			opts.IO = ios

			err := editRun(context.Background(), opts)
			if tt.wantsErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantsErr)
				return
			}

			assert.Contains(t, stderr.String(), tt.wantsStderr)
		})
	}
}

func Test_transformSecurityAndAnalysisOpts(t *testing.T) {
	tests := []struct {
		name string
		opts EditOptions
		want *SecurityAndAnalysisInput
	}{
		{
			name: "Enable all security and analysis settings",
			opts: EditOptions{
				Edits: EditRepositoryInput{
					enableAdvancedSecurity:             bp(true),
					enableSecretScanning:               bp(true),
					enableSecretScanningPushProtection: bp(true),
				},
			},
			want: &SecurityAndAnalysisInput{
				EnableAdvancedSecurity: &SecurityAndAnalysisStatus{
					Status: sp("enabled"),
				},
				EnableSecretScanning: &SecurityAndAnalysisStatus{
					Status: sp("enabled"),
				},
				EnableSecretScanningPushProtection: &SecurityAndAnalysisStatus{
					Status: sp("enabled"),
				},
			},
		},
		{
			name: "Disable all security and analysis settings",
			opts: EditOptions{
				Edits: EditRepositoryInput{
					enableAdvancedSecurity:             bp(false),
					enableSecretScanning:               bp(false),
					enableSecretScanningPushProtection: bp(false),
				},
			},
			want: &SecurityAndAnalysisInput{
				EnableAdvancedSecurity: &SecurityAndAnalysisStatus{
					Status: sp("disabled"),
				},
				EnableSecretScanning: &SecurityAndAnalysisStatus{
					Status: sp("disabled"),
				},
				EnableSecretScanningPushProtection: &SecurityAndAnalysisStatus{
					Status: sp("disabled"),
				},
			},
		},
		{
			name: "Enable only advanced security",
			opts: EditOptions{
				Edits: EditRepositoryInput{
					enableAdvancedSecurity: bp(true),
				},
			},
			want: &SecurityAndAnalysisInput{
				EnableAdvancedSecurity: &SecurityAndAnalysisStatus{
					Status: sp("enabled"),
				},
				EnableSecretScanning:               nil,
				EnableSecretScanningPushProtection: nil,
			},
		},
		{
			name: "Disable only secret scanning",
			opts: EditOptions{
				Edits: EditRepositoryInput{
					enableSecretScanning: bp(false),
				},
			},
			want: &SecurityAndAnalysisInput{
				EnableAdvancedSecurity: nil,
				EnableSecretScanning: &SecurityAndAnalysisStatus{
					Status: sp("disabled"),
				},
				EnableSecretScanningPushProtection: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &tt.opts
			transformed := transformSecurityAndAnalysisOpts(opts)
			assert.Equal(t, tt.want, transformed)
		})
	}
}

func sp(v string) *string {
	return &v
}

func bp(b bool) *bool {
	return &b
}
