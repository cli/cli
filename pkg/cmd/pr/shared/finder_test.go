package shared

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type args struct {
	baseRepoFn   func() (ghrepo.Interface, error)
	branchFn     func() (string, error)
	branchConfig func(string) (git.BranchConfig, error)
	remotesFn    func() (context.Remotes, error)
	selector     string
	fields       []string
	baseBranch   string
}

func TestFind(t *testing.T) {
	tests := []struct {
		name     string
		args     args
		httpStub func(*httpmock.Registry)
		wantPR   int
		wantRepo string
		wantErr  bool
	}{
		{
			name: "number argument",
			args: args{
				selector: "13",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequest":{"number":13}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "number argument with base branch",
			args: args{
				selector:   "13",
				baseBranch: "main",
				fields:     []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestForBranch\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequests":{"nodes":[
							{
								"number": 123,
								"state": "OPEN",
								"baseRefName": "main",
								"headRefName": "13",
								"isCrossRepository": false,
								"headRepositoryOwner": {"login":"OWNER"}
							}
						]}
					}}}`))
			},
			wantPR:   123,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "baseRepo is error",
			args: args{
				selector: "13",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return nil, errors.New("baseRepoErr")
				},
			},
			wantErr: true,
		},
		{
			name: "blank fields is error",
			args: args{
				selector: "13",
				fields:   []string{},
			},
			wantErr: true,
		},
		{
			name: "number only",
			args: args{
				selector: "13",
				fields:   []string{"number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
			},
			httpStub: nil,
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "number with hash argument",
			args: args{
				selector: "#13",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequest":{"number":13}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "URL argument",
			args: args{
				selector:   "https://example.org/OWNER/REPO/pull/13/files",
				fields:     []string{"id", "number"},
				baseRepoFn: nil,
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequest":{"number":13}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://example.org/OWNER/REPO",
		},
		{
			name: "branch argument",
			args: args{
				selector: "blueberries",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestForBranch\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequests":{"nodes":[
							{
								"number": 14,
								"state": "CLOSED",
								"baseRefName": "main",
								"headRefName": "blueberries",
								"isCrossRepository": false,
								"headRepositoryOwner": {"login":"OWNER"}
							},
							{
								"number": 13,
								"state": "OPEN",
								"baseRefName": "main",
								"headRefName": "blueberries",
								"isCrossRepository": false,
								"headRepositoryOwner": {"login":"OWNER"}
							}
						]}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "branch argument with base branch",
			args: args{
				selector:   "blueberries",
				baseBranch: "main",
				fields:     []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestForBranch\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequests":{"nodes":[
							{
								"number": 14,
								"state": "OPEN",
								"baseRefName": "dev",
								"headRefName": "blueberries",
								"isCrossRepository": false,
								"headRepositoryOwner": {"login":"OWNER"}
							},
							{
								"number": 13,
								"state": "OPEN",
								"baseRefName": "main",
								"headRefName": "blueberries",
								"isCrossRepository": false,
								"headRepositoryOwner": {"login":"OWNER"}
							}
						]}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "no argument reads current branch",
			args: args{
				selector: "",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				branchConfig: stubBranchConfig(git.BranchConfig{}, nil),
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestForBranch\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequests":{"nodes":[
							{
								"number": 13,
								"state": "OPEN",
								"baseRefName": "main",
								"headRefName": "blueberries",
								"isCrossRepository": false,
								"headRepositoryOwner": {"login":"OWNER"}
							}
						]}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "current branch with merged pr",
			args: args{
				selector: "",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				branchConfig: stubBranchConfig(git.BranchConfig{}, nil),
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestForBranch\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequests":{"nodes":[
							{
								"number": 13,
								"state": "MERGED",
								"baseRefName": "main",
								"headRefName": "blueberries",
								"isCrossRepository": false,
								"headRepositoryOwner": {"login":"OWNER"}
							}
						]},
						"defaultBranchRef":{
							"name": "blueberries"
						}
					}}}`))
			},
			wantErr: true,
		},
		{
			name: "current branch is error",
			args: args{
				selector: "",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
				branchFn: func() (string, error) {
					return "", errors.New("branchErr")
				},
			},
			wantErr: true,
		},
		{
			name: "current branch with upstream configuration",
			args: args{
				selector: "",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				branchConfig: stubBranchConfig(git.BranchConfig{
					MergeRef:   "refs/heads/blue-upstream-berries",
					RemoteName: "origin",
				}, nil),
				remotesFn: func() (context.Remotes, error) {
					return context.Remotes{{
						Remote: &git.Remote{Name: "origin"},
						Repo:   ghrepo.New("UPSTREAMOWNER", "REPO"),
					}}, nil
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestForBranch\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequests":{"nodes":[
							{
								"number": 13,
								"state": "OPEN",
								"baseRefName": "main",
								"headRefName": "blue-upstream-berries",
								"isCrossRepository": true,
								"headRepositoryOwner": {"login":"UPSTREAMOWNER"}
							}
						]}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "current branch with upstream RemoteURL configuration",
			args: args{
				selector: "",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				branchConfig: func(branch string) (git.BranchConfig, error) {
					u, _ := url.Parse("https://github.com/UPSTREAMOWNER/REPO")
					return stubBranchConfig(git.BranchConfig{
						MergeRef:  "refs/heads/blue-upstream-berries",
						RemoteURL: u,
					}, nil)(branch)
				},
				remotesFn: nil,
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestForBranch\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequests":{"nodes":[
							{
								"number": 13,
								"state": "OPEN",
								"baseRefName": "main",
								"headRefName": "blue-upstream-berries",
								"isCrossRepository": true,
								"headRepositoryOwner": {"login":"UPSTREAMOWNER"}
							}
						]}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "current branch with upstream and fork in same org",
			args: args{
				selector: "",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				branchConfig: stubBranchConfig(git.BranchConfig{
					RemoteName: "origin",
				}, nil),
				remotesFn: func() (context.Remotes, error) {
					return context.Remotes{{
						Remote: &git.Remote{Name: "origin"},
						Repo:   ghrepo.New("OWNER", "REPO-FORK"),
					}}, nil
				},
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestForBranch\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequests":{"nodes":[
							{
								"number": 13,
								"state": "OPEN",
								"baseRefName": "main",
								"headRefName": "blueberries",
								"isCrossRepository": true,
								"headRepositoryOwner": {"login":"OWNER"}
							}
						]}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "current branch made by pr checkout",
			args: args{
				selector: "",
				fields:   []string{"id", "number"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				branchConfig: stubBranchConfig(git.BranchConfig{
					MergeRef: "refs/pull/13/head",
				}, nil),
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequest":{"number":13}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
		{
			name: "including project items",
			args: args{
				selector: "",
				fields:   []string{"projectItems"},
				baseRepoFn: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("OWNER/REPO")
				},
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				branchConfig: stubBranchConfig(git.BranchConfig{
					MergeRef: "refs/pull/13/head",
				}, nil),
			},
			httpStub: func(r *httpmock.Registry) {
				r.Register(
					httpmock.GraphQL(`query PullRequestByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"pullRequest":{"number":13}
					}}}`))

				r.Register(
					httpmock.GraphQL(`query PullRequestProjectItems\b`),
					httpmock.GraphQLQuery(`{
                        "data": {
                          "repository": {
                            "pullRequest": {
                              "projectItems": {
                                "nodes": [
                                  {
                                    "id": "PVTI_lADOB-vozM4AVk16zgK6U50",
                                    "project": {
                                      "id": "PVT_kwDOB-vozM4AVk16",
                                      "title": "Test Project"
                                    },
                                    "status": {
                                      "optionId": "47fc9ee4",
                                      "name": "In Progress"
                                    }
                                  }
                                ],
                                "pageInfo": {
                                  "hasNextPage": false,
                                  "endCursor": "MQ"
                                }
                              }
                            }
                          }
                        }
                      }`,
						func(query string, inputs map[string]interface{}) {
							require.Equal(t, float64(13), inputs["number"])
							require.Equal(t, "OWNER", inputs["owner"])
							require.Equal(t, "REPO", inputs["name"])
						}),
				)
			},
			wantPR:   13,
			wantRepo: "https://github.com/OWNER/REPO",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStub != nil {
				tt.httpStub(reg)
			}

			f := finder{
				httpClient: func() (*http.Client, error) {
					return &http.Client{Transport: reg}, nil
				},
				baseRepoFn:   tt.args.baseRepoFn,
				branchFn:     tt.args.branchFn,
				branchConfig: tt.args.branchConfig,
				remotesFn:    tt.args.remotesFn,
			}

			pr, repo, err := f.Find(FindOptions{
				Selector:   tt.args.selector,
				Fields:     tt.args.fields,
				BaseBranch: tt.args.baseBranch,
			})
			if (err != nil) != tt.wantErr {
				t.Errorf("Find() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.wantPR > 0 {
					t.Error("wantPR field is not checked in error case")
				}
				if tt.wantRepo != "" {
					t.Error("wantRepo field is not checked in error case")
				}
				return
			}

			if pr.Number != tt.wantPR {
				t.Errorf("want pr #%d, got #%d", tt.wantPR, pr.Number)
			}
			repoURL := ghrepo.GenerateRepoURL(repo, "")
			if repoURL != tt.wantRepo {
				t.Errorf("want repo %s, got %s", tt.wantRepo, repoURL)
			}
		})
	}
}

func Test_parseCurrentBranch(t *testing.T) {
	tests := []struct {
		name         string
		args         args
		wantSelector string
		wantPR       int
		wantError    error
	}{
		{
			name: "failed branch config",
			args: args{
				branchConfig: stubBranchConfig(git.BranchConfig{}, errors.New("branchConfigErr")),
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
			},
			wantSelector: "",
			wantPR:       0,
			wantError:    errors.New("branchConfigErr"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := finder{
				httpClient: func() (*http.Client, error) {
					return &http.Client{}, nil
				},
				baseRepoFn:   tt.args.baseRepoFn,
				branchFn:     tt.args.branchFn,
				branchConfig: tt.args.branchConfig,
				remotesFn:    tt.args.remotesFn,
			}
			selector, pr, err := f.parseCurrentBranch()
			assert.Equal(t, tt.wantSelector, selector)
			assert.Equal(t, tt.wantPR, pr)
			assert.Equal(t, tt.wantError, err)
		})
	}
}

func stubBranchConfig(branchConfig git.BranchConfig, err error) func(string) (git.BranchConfig, error) {
	return func(branch string) (git.BranchConfig, error) {
		return branchConfig, err
	}
}
