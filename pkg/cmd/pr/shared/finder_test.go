package shared

import (
	"errors"
	"fmt"
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
	baseRepoFn        func() (ghrepo.Interface, error)
	branchFn          func() (string, error)
	branchConfig      func(string) (git.BranchConfig, error)
	pushDefault       func() (string, error)
	remotePushDefault func() (string, error)
	parsePushRevision func(string) (string, error)
	selector          string
	fields            []string
	baseBranch        string
}

func TestFind(t *testing.T) {
	// TODO: Abstract these out meaningfully for reuse in parsePRRefs tests
	originOwnerUrl, err := url.Parse("https://github.com/ORIGINOWNER/REPO.git")
	if err != nil {
		t.Fatal(err)
	}
	remoteOrigin := context.Remote{
		Remote: &git.Remote{
			Name:     "origin",
			FetchURL: originOwnerUrl,
		},
		Repo: ghrepo.New("ORIGINOWNER", "REPO"),
	}
	remoteOther := context.Remote{
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
	remoteUpstream := context.Remote{
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
				branchConfig: stubBranchConfig(git.BranchConfig{}, nil),
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
				branchConfig: stubBranchConfig(git.BranchConfig{
					PushRemoteName: remoteOrigin.Remote.Name,
				}, nil),
				pushDefault:       stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
				parsePushRevision: stubParsedPushRevision("", nil),
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
				branchConfig:      stubBranchConfig(git.BranchConfig{}, nil),
				pushDefault:       stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
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
				branchConfig:      stubBranchConfig(git.BranchConfig{}, nil),
				pushDefault:       stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
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
				branchConfig:      stubBranchConfig(git.BranchConfig{}, nil),
				pushDefault:       stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
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
				branchConfig:      stubBranchConfig(git.BranchConfig{}, nil),
				pushDefault:       stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
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
				branchConfig: stubBranchConfig(git.BranchConfig{}, &git.GitError{
					Stderr:   "fatal: branchConfig error",
					ExitCode: 128,
				}),
				pushDefault: stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", &git.GitError{
					Stderr:   "fatal: remotePushDefault error",
					ExitCode: 128,
				}),
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
				branchConfig:      stubBranchConfig(git.BranchConfig{}, nil),
				pushDefault:       stubPushDefault("simple", nil),
				parsePushRevision: stubParsedPushRevision("", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
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
				branchConfig:      stubBranchConfig(git.BranchConfig{}, nil),
				pushDefault:       stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
				parsePushRevision: stubParsedPushRevision("", nil),
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
				branchConfig:      stubBranchConfig(git.BranchConfig{}, nil),
				pushDefault:       stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
				parsePushRevision: stubParsedPushRevision("", nil),
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
				branchConfig:      stubBranchConfig(git.BranchConfig{}, nil),
				pushDefault:       stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
				parsePushRevision: stubParsedPushRevision("", nil),
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
				branchConfig: stubBranchConfig(git.BranchConfig{
					MergeRef:       "refs/heads/blue-upstream-berries",
					PushRemoteName: "upstream",
				}, nil),
				pushDefault:       stubPushDefault("upstream", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
				parsePushRevision: stubParsedPushRevision("", nil),
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
				branchConfig: stubBranchConfig(git.BranchConfig{
					MergeRef:      "refs/heads/blue-upstream-berries",
					PushRemoteURL: remoteUpstream.Remote.FetchURL,
				}, nil),
				pushDefault:       stubPushDefault("upstream", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
				parsePushRevision: stubParsedPushRevision("", nil),
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
				branchConfig:      stubBranchConfig(git.BranchConfig{}, nil),
				pushDefault:       stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
				parsePushRevision: stubParsedPushRevision("other/blueberries", nil),
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
				pushDefault:       stubPushDefault("simple", nil),
				remotePushDefault: stubRemotePushDefault("", nil),
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
				baseRepoFn:        tt.args.baseRepoFn,
				branchFn:          tt.args.branchFn,
				branchConfig:      tt.args.branchConfig,
				pushDefault:       tt.args.pushDefault,
				remotePushDefault: tt.args.remotePushDefault,
				parsePushRevision: tt.args.parsePushRevision,
				remotesFn: stubRemotes(context.Remotes{
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

func TestParsePRRefs(t *testing.T) {
	originOwnerUrl, err := url.Parse("https://github.com/ORIGINOWNER/REPO.git")
	if err != nil {
		t.Fatal(err)
	}
	remoteOrigin := context.Remote{
		Remote: &git.Remote{
			Name:     "origin",
			FetchURL: originOwnerUrl,
		},
		Repo: ghrepo.New("ORIGINOWNER", "REPO"),
	}
	remoteOther := context.Remote{
		Remote: &git.Remote{
			Name:     "other",
			FetchURL: originOwnerUrl,
		},
		Repo: ghrepo.New("ORIGINOWNER", "REPO"),
	}

	upstreamOwnerUrl, err := url.Parse("https://github.com/UPSTREAMOWNER/REPO.git")
	if err != nil {
		t.Fatal(err)
	}
	remoteUpstream := context.Remote{
		Remote: &git.Remote{
			Name:     "upstream",
			FetchURL: upstreamOwnerUrl,
		},
		Repo: ghrepo.New("UPSTREAMOWNER", "REPO"),
	}

	tests := []struct {
		name               string
		branchConfig       git.BranchConfig
		pushDefault        string
		parsedPushRevision string
		remotePushDefault  string
		currentBranchName  string
		baseRefRepo        ghrepo.Interface
		rems               context.Remotes
		wantPRRefs         PullRequestRefs
		wantErr            error
	}{
		{
			name:              "When the branch is called 'blueberries' with an empty branch config, it returns the correct PullRequestRefs",
			branchConfig:      git.BranchConfig{},
			currentBranchName: "blueberries",
			baseRefRepo:       remoteOrigin.Repo,
			wantPRRefs: PullRequestRefs{
				BranchName: "blueberries",
				HeadRepo:   remoteOrigin.Repo,
				BaseRepo:   remoteOrigin.Repo,
			},
			wantErr: nil,
		},
		{
			name:              "When the branch is called 'otherBranch' with an empty branch config, it returns the correct PullRequestRefs",
			branchConfig:      git.BranchConfig{},
			currentBranchName: "otherBranch",
			baseRefRepo:       remoteOrigin.Repo,
			wantPRRefs: PullRequestRefs{
				BranchName: "otherBranch",
				HeadRepo:   remoteOrigin.Repo,
				BaseRepo:   remoteOrigin.Repo,
			},
			wantErr: nil,
		},
		{
			name:               "When the branch name doesn't match the branch name in BranchConfig.Push, it returns the BranchConfig.Push branch name",
			parsedPushRevision: "origin/pushBranch",
			currentBranchName:  "blueberries",
			baseRefRepo:        remoteOrigin.Repo,
			rems: context.Remotes{
				&remoteOrigin,
			},
			wantPRRefs: PullRequestRefs{
				BranchName: "pushBranch",
				HeadRepo:   remoteOrigin.Repo,
				BaseRepo:   remoteOrigin.Repo,
			},
			wantErr: nil,
		},
		{
			name:               "When the push revision doesn't match a remote, it returns an error",
			parsedPushRevision: "origin/differentPushBranch",
			currentBranchName:  "blueberries",
			baseRefRepo:        remoteOrigin.Repo,
			rems: context.Remotes{
				&remoteUpstream,
				&remoteOther,
			},
			wantPRRefs: PullRequestRefs{},
			wantErr:    fmt.Errorf("no remote for %q found in %q", "origin/differentPushBranch", "upstream, other"),
		},
		{
			name:               "When the branch name doesn't match a different branch name in BranchConfig.Push and the remote isn't 'origin', it returns the BranchConfig.Push branch name",
			parsedPushRevision: "other/pushBranch",
			currentBranchName:  "blueberries",
			baseRefRepo:        remoteOrigin.Repo,
			rems: context.Remotes{
				&remoteOther,
			},
			wantPRRefs: PullRequestRefs{
				BranchName: "pushBranch",
				HeadRepo:   remoteOther.Repo,
				BaseRepo:   remoteOrigin.Repo,
			},
			wantErr: nil,
		},
		{
			name: "When the push remote is the same as the baseRepo, it returns the baseRepo as the PullRequestRefs HeadRepo",
			branchConfig: git.BranchConfig{
				PushRemoteName: remoteOrigin.Remote.Name,
			},
			currentBranchName: "blueberries",
			baseRefRepo:       remoteOrigin.Repo,
			rems: context.Remotes{
				&remoteOrigin,
				&remoteUpstream,
			},
			wantPRRefs: PullRequestRefs{
				BranchName: "blueberries",
				HeadRepo:   remoteOrigin.Repo,
				BaseRepo:   remoteOrigin.Repo,
			},
			wantErr: nil,
		},
		{
			name: "When the push remote is different from the baseRepo, it returns the push remote repo as the PullRequestRefs HeadRepo",
			branchConfig: git.BranchConfig{
				PushRemoteName: remoteOrigin.Remote.Name,
			},
			currentBranchName: "blueberries",
			baseRefRepo:       remoteUpstream.Repo,
			rems: context.Remotes{
				&remoteOrigin,
				&remoteUpstream,
			},
			wantPRRefs: PullRequestRefs{
				BranchName: "blueberries",
				HeadRepo:   remoteOrigin.Repo,
				BaseRepo:   remoteUpstream.Repo,
			},
			wantErr: nil,
		},
		{
			name: "When the push remote defined by a URL and the baseRepo is different from the push remote, it returns the push remote repo as the PullRequestRefs HeadRepo",
			branchConfig: git.BranchConfig{
				PushRemoteURL: remoteOrigin.Remote.FetchURL,
			},
			currentBranchName: "blueberries",
			baseRefRepo:       remoteUpstream.Repo,
			rems: context.Remotes{
				&remoteOrigin,
				&remoteUpstream,
			},
			wantPRRefs: PullRequestRefs{
				BranchName: "blueberries",
				HeadRepo:   remoteOrigin.Repo,
				BaseRepo:   remoteUpstream.Repo,
			},
			wantErr: nil,
		},
		{
			name: "When the push remote and merge ref are configured to a different repo and push.default = upstream, it should return the branch name from the other repo",
			branchConfig: git.BranchConfig{
				PushRemoteName: remoteUpstream.Remote.Name,
				MergeRef:       "refs/heads/blue-upstream-berries",
			},
			pushDefault:       "upstream",
			currentBranchName: "blueberries",
			baseRefRepo:       remoteOrigin.Repo,
			rems: context.Remotes{
				&remoteOrigin,
				&remoteUpstream,
			},
			wantPRRefs: PullRequestRefs{
				BranchName: "blue-upstream-berries",
				HeadRepo:   remoteUpstream.Repo,
				BaseRepo:   remoteOrigin.Repo,
			},
			wantErr: nil,
		},
		{
			name: "When the push remote and merge ref are configured to a different repo and push.default = tracking, it should return the branch name from the other repo",
			branchConfig: git.BranchConfig{
				PushRemoteName: remoteUpstream.Remote.Name,
				MergeRef:       "refs/heads/blue-upstream-berries",
			},
			pushDefault:       "tracking",
			currentBranchName: "blueberries",
			baseRefRepo:       remoteOrigin.Repo,
			rems: context.Remotes{
				&remoteOrigin,
				&remoteUpstream,
			},
			wantPRRefs: PullRequestRefs{
				BranchName: "blue-upstream-berries",
				HeadRepo:   remoteUpstream.Repo,
				BaseRepo:   remoteOrigin.Repo,
			},
			wantErr: nil,
		},
		{
			name:              "When remote.pushDefault is set, it returns the correct PullRequestRefs",
			branchConfig:      git.BranchConfig{},
			remotePushDefault: remoteUpstream.Remote.Name,
			currentBranchName: "blueberries",
			baseRefRepo:       remoteOrigin.Repo,
			rems: context.Remotes{
				&remoteOrigin,
				&remoteUpstream,
			},
			wantPRRefs: PullRequestRefs{
				BranchName: "blueberries",
				HeadRepo:   remoteUpstream.Repo,
				BaseRepo:   remoteOrigin.Repo,
			},
			wantErr: nil,
		},
		{
			name: "When the remote name is set on the branch, it returns the correct PullRequestRefs",
			branchConfig: git.BranchConfig{
				RemoteName: remoteUpstream.Remote.Name,
			},
			currentBranchName: "blueberries",
			baseRefRepo:       remoteOrigin.Repo,
			rems: context.Remotes{
				&remoteOrigin,
				&remoteUpstream,
			},
			wantPRRefs: PullRequestRefs{
				BranchName: "blueberries",
				HeadRepo:   remoteUpstream.Repo,
				BaseRepo:   remoteOrigin.Repo,
			},
			wantErr: nil,
		},
		{
			name: "When the remote URL is set on the branch, it returns the correct PullRequestRefs",
			branchConfig: git.BranchConfig{
				RemoteURL: remoteUpstream.Remote.FetchURL,
			},
			currentBranchName: "blueberries",
			baseRefRepo:       remoteOrigin.Repo,
			rems: context.Remotes{
				&remoteOrigin,
				&remoteUpstream,
			},
			wantPRRefs: PullRequestRefs{
				BranchName: "blueberries",
				HeadRepo:   remoteUpstream.Repo,
				BaseRepo:   remoteOrigin.Repo,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prRefs, err := ParsePRRefs(tt.currentBranchName, tt.branchConfig, tt.parsedPushRevision, tt.pushDefault, tt.remotePushDefault, tt.baseRefRepo, tt.rems)
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantPRRefs, prRefs)
		})
	}
}

func TestPRRefs_GetPRHeadLabel(t *testing.T) {
	originRepo := ghrepo.New("ORIGINOWNER", "REPO")
	upstreamRepo := ghrepo.New("UPSTREAMOWNER", "REPO")
	tests := []struct {
		name   string
		prRefs PullRequestRefs
		want   string
	}{
		{
			name: "When the HeadRepo and BaseRepo match, it returns the branch name",
			prRefs: PullRequestRefs{
				BranchName: "blueberries",
				HeadRepo:   originRepo,
				BaseRepo:   originRepo,
			},
			want: "blueberries",
		},
		{
			name: "When the HeadRepo and BaseRepo do not match, it returns the prepended HeadRepo owner to the branch name",
			prRefs: PullRequestRefs{
				BranchName: "blueberries",
				HeadRepo:   originRepo,
				BaseRepo:   upstreamRepo,
			},
			want: "ORIGINOWNER:blueberries",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.prRefs.GetPRHeadLabel())
		})
	}
}

func stubBranchConfig(branchConfig git.BranchConfig, err error) func(string) (git.BranchConfig, error) {
	return func(branch string) (git.BranchConfig, error) {
		return branchConfig, err
	}
}

func stubRemotes(remotes context.Remotes, err error) func() (context.Remotes, error) {
	return func() (context.Remotes, error) {
		return remotes, err
	}
}

func stubBaseRepoFn(baseRepo ghrepo.Interface, err error) func() (ghrepo.Interface, error) {
	return func() (ghrepo.Interface, error) {
		return baseRepo, err
	}
}

func stubPushDefault(pushDefault string, err error) func() (string, error) {
	return func() (string, error) {
		return pushDefault, err
	}
}

func stubRemotePushDefault(remotePushDefault string, err error) func() (string, error) {
	return func() (string, error) {
		return remotePushDefault, err
	}
}

func stubParsedPushRevision(parsedPushRevision string, err error) func(string) (string, error) {
	return func(_ string) (string, error) {
		return parsedPushRevision, err
	}
}
