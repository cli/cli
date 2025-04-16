package shared

import (
	"errors"
	"net/url"
	"testing"

	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	o "github.com/cli/cli/v2/pkg/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQualifiedHeadRef(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		behavior           string
		ref                string
		expectedString     string
		expectedBranchName string
		expectedError      error
	}{
		{
			behavior:           "when a branch is provided, the parsed qualified head ref only has a branch",
			ref:                "feature-branch",
			expectedString:     "feature-branch",
			expectedBranchName: "feature-branch",
		},
		{
			behavior:           "when an owner and branch are provided, the parsed qualified head ref has both",
			ref:                "owner:feature-branch",
			expectedString:     "owner:feature-branch",
			expectedBranchName: "feature-branch",
		},
		{
			behavior:      "when the structure cannot be interpreted correctly, an error is returned",
			ref:           "owner:feature-branch:extra",
			expectedError: errors.New("invalid qualified head ref format 'owner:feature-branch:extra'"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.behavior, func(t *testing.T) {
			t.Parallel()

			qualifiedHeadRef, err := ParseQualifiedHeadRef(tc.ref)
			if tc.expectedError != nil {
				require.Equal(t, tc.expectedError, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedString, qualifiedHeadRef.String())
			assert.Equal(t, tc.expectedBranchName, qualifiedHeadRef.BranchName())
		})
	}
}

func TestPRFindRefs(t *testing.T) {
	t.Parallel()

	t.Run("qualified head ref with owner", func(t *testing.T) {
		t.Parallel()

		refs := PRFindRefs{
			qualifiedHeadRef: mustParseQualifiedHeadRef("forkowner:feature-branch"),
		}

		require.Equal(t, "forkowner:feature-branch", refs.QualifiedHeadRef())
		require.Equal(t, "feature-branch", refs.UnqualifiedHeadRef())
	})

	t.Run("qualified head ref without owner", func(t *testing.T) {
		t.Parallel()

		refs := PRFindRefs{
			qualifiedHeadRef: mustParseQualifiedHeadRef("feature-branch"),
		}

		require.Equal(t, "feature-branch", refs.QualifiedHeadRef())
		require.Equal(t, "feature-branch", refs.UnqualifiedHeadRef())
	})

	t.Run("base repo", func(t *testing.T) {
		t.Parallel()

		refs := PRFindRefs{
			baseRepo: ghrepo.New("owner", "repo"),
		}

		require.True(t, ghrepo.IsSame(refs.BaseRepo(), ghrepo.New("owner", "repo")), "expected repos to be the same")
	})

	t.Run("matches", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			behavior         string
			refs             PRFindRefs
			baseBranchName   string
			qualifiedHeadRef string
			expectedMatch    bool
		}{
			{
				behavior: "when qualified head refs don't match, returns false",
				refs: PRFindRefs{
					qualifiedHeadRef: mustParseQualifiedHeadRef("owner:feature-branch"),
				},
				baseBranchName:   "feature-branch",
				qualifiedHeadRef: "feature-branch",
				expectedMatch:    false,
			},
			{
				behavior: "when base branches don't match, returns false",
				refs: PRFindRefs{
					qualifiedHeadRef: mustParseQualifiedHeadRef("feature-branch"),
					baseBranchName:   o.Some("not-main"),
				},
				baseBranchName:   "main",
				qualifiedHeadRef: "feature-branch",
				expectedMatch:    false,
			},
			{
				behavior: "when head refs match and there is no base branch, returns true",
				refs: PRFindRefs{
					qualifiedHeadRef: mustParseQualifiedHeadRef("feature-branch"),
					baseBranchName:   o.None[string](),
				},
				baseBranchName:   "main",
				qualifiedHeadRef: "feature-branch",
				expectedMatch:    true,
			},
			{
				behavior: "when head refs match and base branches match, returns true",
				refs: PRFindRefs{
					qualifiedHeadRef: mustParseQualifiedHeadRef("feature-branch"),
					baseBranchName:   o.Some("main"),
				},
				baseBranchName:   "main",
				qualifiedHeadRef: "feature-branch",
				expectedMatch:    true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.behavior, func(t *testing.T) {
				t.Parallel()

				require.Equal(t, tc.expectedMatch, tc.refs.Matches(tc.baseBranchName, tc.qualifiedHeadRef))
			})
		}
	})
}

func TestPullRequestResolution(t *testing.T) {
	t.Parallel()

	baseRepo := ghrepo.New("owner", "repo")
	baseRemote := ghContext.Remote{
		Remote: &git.Remote{
			Name: "upstream",
		},
		Repo: ghrepo.New("owner", "repo"),
	}

	forkRemote := ghContext.Remote{
		Remote: &git.Remote{
			Name: "origin",
		},
		Repo: ghrepo.New("otherowner", "repo-fork"),
	}

	t.Run("when the base repo is nil, returns an error", func(t *testing.T) {
		t.Parallel()

		resolver := NewPullRequestFindRefsResolver(stubGitConfigClient{}, dummyRemotesFn)
		_, err := resolver.ResolvePullRequestRefs(nil, "", "")
		require.Error(t, err)
	})

	t.Run("when the local branch name is empty, returns an error", func(t *testing.T) {
		t.Parallel()

		resolver := NewPullRequestFindRefsResolver(stubGitConfigClient{}, dummyRemotesFn)
		_, err := resolver.ResolvePullRequestRefs(baseRepo, "", "")
		require.Error(t, err)
	})

	t.Run("when the default pr head has a repo, it is used for the refs", func(t *testing.T) {
		t.Parallel()

		// Push revision is the first thing checked for resolution,
		// so nothing else needs to be stubbed.
		repoResolvedFromPushRevisionClient := stubGitConfigClient{
			pushRevisionFn: stubPushRevision(git.RemoteTrackingRef{
				Remote: "origin",
				Branch: "feature-branch",
			}, nil),
		}

		resolver := NewPullRequestFindRefsResolver(
			repoResolvedFromPushRevisionClient,
			stubRemotes(ghContext.Remotes{&baseRemote, &forkRemote}, nil),
		)

		refs, err := resolver.ResolvePullRequestRefs(baseRepo, "main", "feature-branch")
		require.NoError(t, err)

		expectedRefs := PRFindRefs{
			qualifiedHeadRef: QualifiedHeadRef{
				owner:      o.Some("otherowner"),
				branchName: "feature-branch",
			},
			baseRepo:       baseRepo,
			baseBranchName: o.Some("main"),
		}

		require.Equal(t, expectedRefs, refs)
	})

	t.Run("when the default pr head does not have a repo, we use the base repo for the head", func(t *testing.T) {
		t.Parallel()

		// All the values stubbed here result in being unable to resolve a default repo.
		noRepoResolutionStubClient := stubGitConfigClient{
			pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{}, errors.New("test error")),
			readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
			pushDefaultFn:       stubPushDefault("", nil),
			remotePushDefaultFn: stubRemotePushDefault("", nil),
		}

		resolver := NewPullRequestFindRefsResolver(
			noRepoResolutionStubClient,
			stubRemotes(ghContext.Remotes{&baseRemote, &forkRemote}, nil),
		)

		refs, err := resolver.ResolvePullRequestRefs(baseRepo, "main", "feature-branch")
		require.NoError(t, err)

		expectedRefs := PRFindRefs{
			qualifiedHeadRef: QualifiedHeadRef{
				owner:      o.None[string](),
				branchName: "feature-branch",
			},
			baseRepo:       baseRepo,
			baseBranchName: o.Some("main"),
		}
		require.Equal(t, expectedRefs, refs)
	})
}

func TestTryDetermineDefaultPRHead(t *testing.T) {
	t.Parallel()

	baseRepo := ghrepo.New("owner", "repo")
	baseRemote := ghContext.Remote{
		Remote: &git.Remote{
			Name: "upstream",
		},
		Repo: baseRepo,
	}

	forkRepo := ghrepo.New("otherowner", "repo-fork")
	forkRemote := ghContext.Remote{
		Remote: &git.Remote{
			Name: "origin",
		},
		Repo: forkRepo,
	}
	forkRepoURL, err := url.Parse("https://github.com/otherowner/repo-fork.git")
	require.NoError(t, err)

	t.Run("when the push revision is set, use that", func(t *testing.T) {
		t.Parallel()

		repoResolvedFromPushRevisionClient := stubGitConfigClient{
			pushRevisionFn: stubPushRevision(git.RemoteTrackingRef{
				Remote: "origin",
				Branch: "remote-feature-branch",
			}, nil),
		}

		defaultPRHead, err := TryDetermineDefaultPRHead(
			repoResolvedFromPushRevisionClient,
			stubRemoteToRepoResolver(ghContext.Remotes{&baseRemote, &forkRemote}, nil),
			"feature-branch",
		)
		require.NoError(t, err)

		require.True(t, ghrepo.IsSame(defaultPRHead.Repo.Unwrap(), forkRepo), "expected repos to be the same")
		require.Equal(t, "remote-feature-branch", defaultPRHead.BranchName)
	})

	t.Run("when the branch config push remote is set to a name, use that", func(t *testing.T) {
		t.Parallel()

		repoResolvedFromPushRemoteClient := stubGitConfigClient{
			pushRevisionFn: stubPushRevision(git.RemoteTrackingRef{}, errors.New("no push revision")),
			readBranchConfigFn: stubBranchConfig(git.BranchConfig{
				PushRemoteName: "origin",
			}, nil),
			pushDefaultFn: stubPushDefault(git.PushDefaultCurrent, nil),
		}

		defaultPRHead, err := TryDetermineDefaultPRHead(
			repoResolvedFromPushRemoteClient,
			stubRemoteToRepoResolver(ghContext.Remotes{&baseRemote, &forkRemote}, nil),
			"feature-branch",
		)
		require.NoError(t, err)

		require.True(t, ghrepo.IsSame(defaultPRHead.Repo.Unwrap(), forkRepo), "expected repos to be the same")
		require.Equal(t, "feature-branch", defaultPRHead.BranchName)
	})

	t.Run("when the branch config push remote is set to a URL, use that", func(t *testing.T) {
		t.Parallel()

		repoResolvedFromPushRemoteClient := stubGitConfigClient{
			pushRevisionFn: stubPushRevision(git.RemoteTrackingRef{}, errors.New("no push revision")),
			readBranchConfigFn: stubBranchConfig(git.BranchConfig{
				PushRemoteURL: forkRepoURL,
			}, nil),
			pushDefaultFn: stubPushDefault(git.PushDefaultCurrent, nil),
		}

		defaultPRHead, err := TryDetermineDefaultPRHead(
			repoResolvedFromPushRemoteClient,
			dummyRemoteToRepoResolver(),
			"feature-branch",
		)
		require.NoError(t, err)

		require.True(t, ghrepo.IsSame(defaultPRHead.Repo.Unwrap(), forkRepo), "expected repos to be the same")
		require.Equal(t, "feature-branch", defaultPRHead.BranchName)
	})

	t.Run("when a remote push default is set, use that", func(t *testing.T) {
		t.Parallel()

		repoResolvedFromPushRemoteClient := stubGitConfigClient{
			pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{}, errors.New("no push revision")),
			readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
			pushDefaultFn:       stubPushDefault(git.PushDefaultCurrent, nil),
			remotePushDefaultFn: stubRemotePushDefault("origin", nil),
		}

		defaultPRHead, err := TryDetermineDefaultPRHead(
			repoResolvedFromPushRemoteClient,
			stubRemoteToRepoResolver(ghContext.Remotes{&baseRemote, &forkRemote}, nil),
			"feature-branch",
		)
		require.NoError(t, err)

		require.True(t, ghrepo.IsSame(defaultPRHead.Repo.Unwrap(), forkRepo), "expected repos to be the same")
		require.Equal(t, "feature-branch", defaultPRHead.BranchName)
	})

	t.Run("when the branch config remote is set to a name, use that", func(t *testing.T) {
		t.Parallel()

		repoResolvedFromPushRemoteClient := stubGitConfigClient{
			pushRevisionFn: stubPushRevision(git.RemoteTrackingRef{}, errors.New("no push revision")),
			readBranchConfigFn: stubBranchConfig(git.BranchConfig{
				RemoteName: "origin",
			}, nil),
			pushDefaultFn:       stubPushDefault(git.PushDefaultCurrent, nil),
			remotePushDefaultFn: stubRemotePushDefault("", nil),
		}

		defaultPRHead, err := TryDetermineDefaultPRHead(
			repoResolvedFromPushRemoteClient,
			stubRemoteToRepoResolver(ghContext.Remotes{&baseRemote, &forkRemote}, nil),
			"feature-branch",
		)
		require.NoError(t, err)

		require.True(t, ghrepo.IsSame(defaultPRHead.Repo.Unwrap(), forkRepo), "expected repos to be the same")
		require.Equal(t, "feature-branch", defaultPRHead.BranchName)
	})

	t.Run("when the branch config remote is set to a URL, use that", func(t *testing.T) {
		t.Parallel()

		repoResolvedFromPushRemoteClient := stubGitConfigClient{
			pushRevisionFn: stubPushRevision(git.RemoteTrackingRef{}, errors.New("no push revision")),
			readBranchConfigFn: stubBranchConfig(git.BranchConfig{
				RemoteURL: forkRepoURL,
			}, nil),
			pushDefaultFn:       stubPushDefault(git.PushDefaultCurrent, nil),
			remotePushDefaultFn: stubRemotePushDefault("", nil),
		}

		defaultPRHead, err := TryDetermineDefaultPRHead(
			repoResolvedFromPushRemoteClient,
			dummyRemoteToRepoResolver(),
			"feature-branch",
		)
		require.NoError(t, err)

		require.True(t, ghrepo.IsSame(defaultPRHead.Repo.Unwrap(), forkRepo), "expected repos to be the same")
		require.Equal(t, "feature-branch", defaultPRHead.BranchName)
	})

	t.Run("when git didn't provide the necessary information, return none for the remote", func(t *testing.T) {
		t.Parallel()

		// All the values stubbed here result in being unable to resolve a default repo.
		noRepoResolutionStubClient := stubGitConfigClient{
			pushRevisionFn:      stubPushRevision(git.RemoteTrackingRef{}, errors.New("test error")),
			readBranchConfigFn:  stubBranchConfig(git.BranchConfig{}, nil),
			pushDefaultFn:       stubPushDefault("", nil),
			remotePushDefaultFn: stubRemotePushDefault("", nil),
		}

		defaultPRHead, err := TryDetermineDefaultPRHead(
			noRepoResolutionStubClient,
			stubRemoteToRepoResolver(ghContext.Remotes{&baseRemote, &forkRemote}, nil),
			"feature-branch",
		)
		require.NoError(t, err)

		require.True(t, defaultPRHead.Repo.IsNone(), "expected repo to be none")
		require.Equal(t, "feature-branch", defaultPRHead.BranchName)
	})

	t.Run("when the push default is tracking or upstream, use the merge ref", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			pushDefault git.PushDefault
		}{
			{pushDefault: git.PushDefaultTracking},
			{pushDefault: git.PushDefaultUpstream},
		}

		for _, tc := range testCases {
			t.Run(string(tc.pushDefault), func(t *testing.T) {
				t.Parallel()

				repoResolvedFromPushRemoteClient := stubGitConfigClient{
					pushRevisionFn: stubPushRevision(git.RemoteTrackingRef{}, errors.New("test error")),
					readBranchConfigFn: stubBranchConfig(git.BranchConfig{
						PushRemoteName: "origin",
						MergeRef:       "main",
					}, nil),
					pushDefaultFn: stubPushDefault(tc.pushDefault, nil),
				}

				defaultPRHead, err := TryDetermineDefaultPRHead(
					repoResolvedFromPushRemoteClient,
					stubRemoteToRepoResolver(ghContext.Remotes{&baseRemote, &forkRemote}, nil),
					"feature-branch",
				)
				require.NoError(t, err)

				require.True(t, ghrepo.IsSame(defaultPRHead.Repo.Unwrap(), forkRepo), "expected repos to be the same")
				require.Equal(t, "main", defaultPRHead.BranchName)
			})
		}

		t.Run("but if the merge ref is empty, error", func(t *testing.T) {
			t.Parallel()

			repoResolvedFromPushRemoteClient := stubGitConfigClient{
				pushRevisionFn: stubPushRevision(git.RemoteTrackingRef{}, errors.New("test error")),
				readBranchConfigFn: stubBranchConfig(git.BranchConfig{
					PushRemoteName: "origin",
					MergeRef:       "", // intentionally empty
				}, nil),
				pushDefaultFn: stubPushDefault(git.PushDefaultUpstream, nil),
			}

			_, err := TryDetermineDefaultPRHead(
				repoResolvedFromPushRemoteClient,
				stubRemoteToRepoResolver(ghContext.Remotes{&baseRemote, &forkRemote}, nil),
				"feature-branch",
			)
			require.Error(t, err)
		})
	})

}

func dummyRemotesFn() (ghContext.Remotes, error) {
	panic("remotes fn not implemented")
}

func dummyRemoteToRepoResolver() remoteToRepoResolver {
	return NewRemoteToRepoResolver(dummyRemotesFn)
}

func stubRemoteToRepoResolver(remotes ghContext.Remotes, err error) remoteToRepoResolver {
	return NewRemoteToRepoResolver(func() (ghContext.Remotes, error) {
		return remotes, err
	})
}

func mustParseQualifiedHeadRef(ref string) QualifiedHeadRef {
	parsed, err := ParseQualifiedHeadRef(ref)
	if err != nil {
		panic(err)
	}
	return parsed
}
