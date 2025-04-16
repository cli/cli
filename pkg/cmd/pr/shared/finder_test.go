package shared

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/require"
)

type args struct {
	baseRepoFn      func() (ghrepo.Interface, error)
	branchFn        func() (string, error)
	gitConfigClient stubGitConfigClient
	selector        string
	fields          []string
	baseBranch      string
}

func TestFind(t *testing.T) {
	originOwnerUrl, err := url.Parse("https://github.com/ORIGINOWNER/REPO.git")
	if err != nil {
		t.Fatal(err)
	}
	remoteOrigin := ghContext.Remote{
		Remote: &git.Remote{
			Name:     "origin",
			FetchURL: originOwnerUrl,
		},
		Repo: ghrepo.New("ORIGINOWNER", "REPO"),
	}
	remoteOther := ghContext.Remote{
		Remote: &git.Remote{
			Name:     "other",
			FetchURL: originOwnerUrl,
		},
		Repo: ghrepo.New("ORIGINOWNER", "OTHER-REPO"),
	}

	upstreamOwnerUrl, err := url.Parse("https://github.com/UPSTREAMOWNER/REPO.git")
	if err != nil {
		t.Fatal(err)
	}
	remoteUpstream := ghContext.Remote{
		Remote: &git.Remote{
			Name:     "upstream",
			FetchURL: upstreamOwnerUrl,
		},
		Repo: ghrepo.New("UPSTREAMOWNER", "REPO"),
	}

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
				selector:   "13",
				fields:     []string{"id", "number"},
				baseRepoFn: stubBaseRepoFn(ghrepo.New("ORIGINOWNER", "REPO"), nil),
				branchFn: func() (string, error) {
					return "blueberries", nil
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
			wantRepo: "https://github.com/ORIGINOWNER/REPO",
		},
		{
			name: "number argument with base branch",
			args: args{
				selector:   "13",
				baseBranch: "main",
				fields:     []string{"id", "number"},
				baseRepoFn: stubBaseRepoFn(ghrepo.New("ORIGINOWNER", "REPO"), nil),
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn: stubBranchConfig(git.BranchConfig{
						PushRemoteName: remoteOrigin.Remote.Name,
					}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
					pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{}, errors.New("testErr")),
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
								"headRepositoryOwner": {"login":"ORIGINOWNER"}
							}
						]}
					}}}`))
			},
			wantPR:   123,
			wantRepo: "https://github.com/ORIGINOWNER/REPO",
		},
		{
			name: "baseRepo is error",
			args: args{
				selector:   "13",
				fields:     []string{"id", "number"},
				baseRepoFn: stubBaseRepoFn(nil, errors.New("baseRepoErr")),
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
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
				selector:   "13",
				fields:     []string{"number"},
				baseRepoFn: stubBaseRepoFn(ghrepo.New("ORIGINOWNER", "REPO"), nil),
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
				},
			},
			httpStub: nil,
			wantPR:   13,
			wantRepo: "https://github.com/ORIGINOWNER/REPO",
		},
		{
			name: "number with hash argument",
			args: args{
				selector:   "#13",
				fields:     []string{"id", "number"},
				baseRepoFn: stubBaseRepoFn(ghrepo.New("ORIGINOWNER", "REPO"), nil),
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
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
			wantRepo: "https://github.com/ORIGINOWNER/REPO",
		},
		{
			name: "PR URL argument",
			args: args{
				selector:   "https://example.org/OWNER/REPO/pull/13/files",
				fields:     []string{"id", "number"},
				baseRepoFn: nil,
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
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
			wantRepo: "https://example.org/OWNER/REPO",
		},
		{
			name: "PR URL argument and not in a local git repo",
			args: args{
				selector:   "https://example.org/OWNER/REPO/pull/13/files",
				fields:     []string{"id", "number"},
				baseRepoFn: nil,
				branchFn: func() (string, error) {
					return "", &git.GitError{
						Stderr:   "fatal: branchFn error",
						ExitCode: 128,
					}
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn: stubBranchConfig(git.BranchConfig{}, &git.GitError{
						Stderr:   "fatal: branchConfig error",
						ExitCode: 128,
					}),
					pushDefaultFn: stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", &git.GitError{
						Stderr:   "fatal: remotePushDefault error",
						ExitCode: 128,
					}),
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
			wantRepo: "https://example.org/OWNER/REPO",
		},
		{
			name: "when provided branch argument with an open and closed PR for that branch name, it returns the open PR",
			args: args{
				selector:   "blueberries",
				fields:     []string{"id", "number"},
				baseRepoFn: stubBaseRepoFn(ghrepo.New("ORIGINOWNER", "REPO"), nil),
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{}, errors.New("testErr")),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
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
								"headRepositoryOwner": {"login":"ORIGINOWNER"}
							},
							{
								"number": 13,
								"state": "OPEN",
								"baseRefName": "main",
								"headRefName": "blueberries",
								"isCrossRepository": false,
								"headRepositoryOwner": {"login":"ORIGINOWNER"}
							}
						]}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/ORIGINOWNER/REPO",
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
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
					pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{}, errors.New("testErr")),
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
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
					pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{}, errors.New("testErr")),
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
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
					pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{}, errors.New("testErr")),
				},
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
			name: "when the current branch is configured to push to and pull from 'upstream' and push.default = upstream but the repo push/pulls from 'origin', it finds the PR associated with the upstream repo and returns origin as the base repo",
			args: args{
				selector:   "",
				fields:     []string{"id", "number"},
				baseRepoFn: stubBaseRepoFn(ghrepo.New("ORIGINOWNER", "REPO"), nil),
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn: stubBranchConfig(git.BranchConfig{
						MergeRef:       "refs/heads/blue-upstream-berries",
						PushRemoteName: "upstream",
					}, nil),
					pushDefaultFn:       stubPushDefault("upstream", nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
					pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{}, errors.New("testErr")),
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
			wantRepo: "https://github.com/ORIGINOWNER/REPO",
		},
		{
			// The current BRANCH is configured to push to and pull from a URL (upstream, in this example)
			// which is different from what the REPO is configured to push to and pull from (origin, in this example)
			// and push.default = upstream. It should find the PR associated with the upstream repo and return
			// origin as the base repo
			name: "when push.default = upstream and the current branch is configured to push/pull from a different remote than the repo",
			args: args{
				selector:   "",
				fields:     []string{"id", "number"},
				baseRepoFn: stubBaseRepoFn(ghrepo.New("ORIGINOWNER", "REPO"), nil),
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn: stubBranchConfig(git.BranchConfig{
						MergeRef:      "refs/heads/blue-upstream-berries",
						PushRemoteURL: remoteUpstream.Remote.FetchURL,
					}, nil),
					pushDefaultFn:       stubPushDefault("upstream", nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
					pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{}, errors.New("testErr")),
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
			wantRepo: "https://github.com/ORIGINOWNER/REPO",
		},
		{
			name: "current branch with upstream and fork in same org",
			args: args{
				selector:   "",
				fields:     []string{"id", "number"},
				baseRepoFn: stubBaseRepoFn(ghrepo.New("ORIGINOWNER", "REPO"), nil),
				branchFn: func() (string, error) {
					return "blueberries", nil
				},
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
					pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{Remote: "other", Branch: "blueberries"}, nil),
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
								"headRepositoryOwner": {"login":"ORIGINOWNER"}
							}
						]}
					}}}`))
			},
			wantPR:   13,
			wantRepo: "https://github.com/ORIGINOWNER/REPO",
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
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn: stubBranchConfig(git.BranchConfig{
						MergeRef: "refs/pull/13/head",
					}, nil),
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
				gitConfigClient: stubGitConfigClient{
					readBranchConfigFn: stubBranchConfig(git.BranchConfig{
						MergeRef: "refs/pull/13/head",
					}, nil),
					pushDefaultFn:       stubPushDefault(git.PushDefaultSimple, nil),
					remotePushDefaultFn: stubRemotePushDefault("", nil),
				},
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
				baseRepoFn:      tt.args.baseRepoFn,
				branchFn:        tt.args.branchFn,
				gitConfigClient: tt.args.gitConfigClient,
				remotesFn: stubRemotes(ghContext.Remotes{
					&remoteOrigin,
					&remoteOther,
					&remoteUpstream,
				}, nil),
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

func stubBranchConfig(branchConfig git.BranchConfig, err error) func(context.Context, string) (git.BranchConfig, error) {
	return func(_ context.Context, branch string) (git.BranchConfig, error) {
		return branchConfig, err
	}
}

func stubRemotes(remotes ghContext.Remotes, err error) func() (ghContext.Remotes, error) {
	return func() (ghContext.Remotes, error) {
		return remotes, err
	}
}

func stubBaseRepoFn(baseRepo ghrepo.Interface, err error) func() (ghrepo.Interface, error) {
	return func() (ghrepo.Interface, error) {
		return baseRepo, err
	}
}

func stubPushDefault(pushDefault git.PushDefault, err error) func(context.Context) (git.PushDefault, error) {
	return func(_ context.Context) (git.PushDefault, error) {
		return pushDefault, err
	}
}

func stubRemotePushDefault(remotePushDefault string, err error) func(context.Context) (string, error) {
	return func(_ context.Context) (string, error) {
		return remotePushDefault, err
	}
}

func stubPushRevision(parsedPushRevision git.RemoteTrackingRef, err error) func(context.Context, string) (git.RemoteTrackingRef, error) {
	return func(_ context.Context, _ string) (git.RemoteTrackingRef, error) {
		return parsedPushRevision, err
	}
}

type stubGitConfigClient struct {
	readBranchConfigFn  func(ctx context.Context, branchName string) (git.BranchConfig, error)
	pushDefaultFn       func(ctx context.Context) (git.PushDefault, error)
	remotePushDefaultFn func(ctx context.Context) (string, error)
	pushRevisionFn      func(ctx context.Context, branchName string) (git.RemoteTrackingRef, error)
}

func (s stubGitConfigClient) ReadBranchConfig(ctx context.Context, branchName string) (git.BranchConfig, error) {
	if s.readBranchConfigFn == nil {
		panic("unexpected call to ReadBranchConfig")
	}
	return s.readBranchConfigFn(ctx, branchName)
}

func (s stubGitConfigClient) PushDefault(ctx context.Context) (git.PushDefault, error) {
	if s.pushDefaultFn == nil {
		panic("unexpected call to PushDefault")
	}
	return s.pushDefaultFn(ctx)
}

func (s stubGitConfigClient) RemotePushDefault(ctx context.Context) (string, error) {
	if s.remotePushDefaultFn == nil {
		panic("unexpected call to RemotePushDefault")
	}
	return s.remotePushDefaultFn(ctx)
}

func (s stubGitConfigClient) PushRevision(ctx context.Context, branchName string) (git.RemoteTrackingRef, error) {
	if s.pushRevisionFn == nil {
		panic("unexpected call to PushRevision")
	}
	return s.pushRevisionFn(ctx, branchName)
}
