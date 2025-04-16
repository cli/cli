package shared

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"

	"github.com/cli/cli/v2/internal/ghrepo"
	o "github.com/cli/cli/v2/pkg/option"
)

// QualifiedHeadRef represents a git branch with an optional owner, used
// for the head of a pull request. For example, within a single repository,
// we would expect a PR to have a head ref of no owner, and a branch name.
// However, for cross-repository pull requests, we would expect a head ref
// with an owner and a branch name. In string form this is represented as
// <owner>:<branch>. The GitHub API is able to interpret this format in order
// to discover the correct fork repository.
//
// In other parts of the code, you may see this refered to as a HeadLabel.
type QualifiedHeadRef struct {
	owner      o.Option[string]
	branchName string
}

// NewQualifiedHeadRef creates a QualifiedHeadRef. If the empty string is provided
// for the owner, it will be treated as None.
func NewQualifiedHeadRef(owner string, branchName string) QualifiedHeadRef {
	return QualifiedHeadRef{
		owner:      o.SomeIfNonZero(owner),
		branchName: branchName,
	}
}

func NewQualifiedHeadRefWithoutOwner(branchName string) QualifiedHeadRef {
	return QualifiedHeadRef{
		owner:      o.None[string](),
		branchName: branchName,
	}
}

// ParseQualifiedHeadRef takes strings of the form <owner>:<branch> or <branch>
// and returns a QualifiedHeadRef. If the form <owner>:<branch> is used,
// the owner is set to the value of <owner>, and the branch name is set to
// the value of <branch>. If the form <branch> is used, the owner is set to
// None, and the branch name is set to the value of <branch>.
//
// This does no further error checking about the validity of a ref, so
// it is not safe to assume the ref is truly a valid ref, e.g. "my~bad:ref?"
// is going to result in a nonsense result.
func ParseQualifiedHeadRef(ref string) (QualifiedHeadRef, error) {
	if !strings.Contains(ref, ":") {
		return NewQualifiedHeadRefWithoutOwner(ref), nil
	}

	parts := strings.Split(ref, ":")
	if len(parts) != 2 {
		return QualifiedHeadRef{}, fmt.Errorf("invalid qualified head ref format '%s'", ref)
	}

	return NewQualifiedHeadRef(parts[0], parts[1]), nil
}

// A QualifiedHeadRef without an owner returns <branch>, while a QualifiedHeadRef
// with an owner returns <owner>:<branch>.
func (r QualifiedHeadRef) String() string {
	if owner, present := r.owner.Value(); present {
		return fmt.Sprintf("%s:%s", owner, r.branchName)
	}
	return r.branchName
}

func (r QualifiedHeadRef) BranchName() string {
	return r.branchName
}

// PRFindRefs represents the necessary data to find a pull request from the API.
type PRFindRefs struct {
	qualifiedHeadRef QualifiedHeadRef

	baseRepo ghrepo.Interface
	// baseBranchName is an optional branch name, because it is not required for
	// finding a pull request, only for disambiguation if multiple pull requests
	// contain the same head ref.
	baseBranchName o.Option[string]
}

// QualifiedHeadRef returns a stringified form of the head ref, varying depending
// on whether the head ref is in the same repository as the base ref. If they are
// the same repository, we return the branch name only. If they are different repositories,
// we return the owner and branch name in the form <owner>:<branch>.
func (r PRFindRefs) QualifiedHeadRef() string {
	return r.qualifiedHeadRef.String()
}

func (r PRFindRefs) UnqualifiedHeadRef() string {
	return r.qualifiedHeadRef.BranchName()
}

// Matches checks whether the provided baseBranchName and headRef match the refs.
// It is used to determine whether Pull Requests returned from the API
func (r PRFindRefs) Matches(baseBranchName, qualifiedHeadRef string) bool {
	headMatches := qualifiedHeadRef == r.QualifiedHeadRef()
	baseMatches := r.baseBranchName.IsNone() || baseBranchName == r.baseBranchName.Unwrap()
	return headMatches && baseMatches
}

func (r PRFindRefs) BaseRepo() ghrepo.Interface {
	return r.baseRepo
}

type RemoteNameToRepoFn func(remoteName string) (ghrepo.Interface, error)

// PullRequestFindRefsResolver interrogates git configuration to try and determine
// a head repository and a remote branch name, from a local branch name.
type PullRequestFindRefsResolver struct {
	GitConfigClient    GitConfigClient
	RemoteNameToRepoFn RemoteNameToRepoFn
}

func NewPullRequestFindRefsResolver(gitConfigClient GitConfigClient, remotesFn func() (ghContext.Remotes, error)) PullRequestFindRefsResolver {
	return PullRequestFindRefsResolver{
		GitConfigClient:    gitConfigClient,
		RemoteNameToRepoFn: newRemoteNameToRepoFn(remotesFn),
	}
}

// ResolvePullRequests takes a base repository, a base branch name and a local branch name and uses the git configuration to
// determine the head repository and remote branch name. If we were unable to determine this from git, we default the head
// repository to the base repository.
func (r *PullRequestFindRefsResolver) ResolvePullRequestRefs(baseRepo ghrepo.Interface, baseBranchName, localBranchName string) (PRFindRefs, error) {
	if baseRepo == nil {
		return PRFindRefs{}, fmt.Errorf("find pull request ref resolution cannot be performed without a base repository")
	}

	if localBranchName == "" {
		return PRFindRefs{}, fmt.Errorf("find pull request ref resolution cannot be performed without a local branch name")
	}

	headPRRef, err := TryDetermineDefaultPRHead(r.GitConfigClient, remoteToRepoResolver{r.RemoteNameToRepoFn}, localBranchName)
	if err != nil {
		return PRFindRefs{}, err
	}

	// If the headRepo was resolved, we can just convert the response
	// to refs and return it.
	if headRepo, present := headPRRef.Repo.Value(); present {
		qualifiedHeadRef := NewQualifiedHeadRefWithoutOwner(headPRRef.BranchName)
		if !ghrepo.IsSame(headRepo, baseRepo) {
			qualifiedHeadRef = NewQualifiedHeadRef(headRepo.RepoOwner(), headPRRef.BranchName)
		}

		return PRFindRefs{
			qualifiedHeadRef: qualifiedHeadRef,
			baseRepo:         baseRepo,
			baseBranchName:   o.SomeIfNonZero(baseBranchName),
		}, nil
	}

	// If we didn't find a head repo, default to the base repo
	return PRFindRefs{
		qualifiedHeadRef: NewQualifiedHeadRefWithoutOwner(headPRRef.BranchName),
		baseRepo:         baseRepo,
		baseBranchName:   o.SomeIfNonZero(baseBranchName),
	}, nil
}

// DefaultPRHead is a neighbour to defaultPushTarget, but instead of holding
// basic git remote information, it holds a resolved repository in `gh` terms.
//
// Since we may not be able to determine a default remote for a branch, this
// is also true of the resolved repository.
type DefaultPRHead struct {
	Repo       o.Option[ghrepo.Interface]
	BranchName string
}

// TryDetermineDefaultPRHead is a thin wrapper around determineDefaultPushTarget, which attempts to convert
// a present remote into a resolved repository. If the remote is not present, we indicate that to the caller
// by returning a None value for the repo.
func TryDetermineDefaultPRHead(gitClient GitConfigClient, remoteToRepo remoteToRepoResolver, branch string) (DefaultPRHead, error) {
	pushTarget, err := tryDetermineDefaultPushTarget(gitClient, branch)
	if err != nil {
		return DefaultPRHead{}, err
	}

	// If we have no remote, let the caller decide what to do by indicating that with a None.
	if pushTarget.remote.IsNone() {
		return DefaultPRHead{
			Repo:       o.None[ghrepo.Interface](),
			BranchName: pushTarget.branchName,
		}, nil
	}

	repo, err := remoteToRepo.resolve(pushTarget.remote.Unwrap())
	if err != nil {
		return DefaultPRHead{}, err
	}

	return DefaultPRHead{
		Repo:       o.Some(repo),
		BranchName: pushTarget.branchName,
	}, nil
}

// remote represents the value of the remote key in a branch's git configuration.
// This value may be a name or a URL, both of which are strings, but are unfortunately
// parsed by ReadBranchConfig into separate fields, allowing for illegal states to be
// created by accident. This is an attempt to indicate that they are mutally exclusive.
type remote interface{ sealedRemote() }

type remoteName struct{ name string }

func (rn remoteName) sealedRemote() {}

type remoteURL struct{ url *url.URL }

func (ru remoteURL) sealedRemote() {}

// newRemoteNameToRepoFn takes a function that returns a list of remotes and
// returns a function that takes a remote name and returns the corresponding
// repository. It is a convenience function to call sites having to duplicate
// the same logic.
func newRemoteNameToRepoFn(remotesFn func() (ghContext.Remotes, error)) RemoteNameToRepoFn {
	return func(remoteName string) (ghrepo.Interface, error) {
		remotes, err := remotesFn()
		if err != nil {
			return nil, err
		}
		repo, err := remotes.FindByName(remoteName)
		if err != nil {
			return nil, err
		}
		return repo, nil
	}
}

// remoteToRepoResolver provides a utility method to resolve a remote (either name or URL)
// to a repo (ghrepo.Interface).
type remoteToRepoResolver struct {
	remoteNameToRepo RemoteNameToRepoFn
}

func NewRemoteToRepoResolver(remotesFn func() (ghContext.Remotes, error)) remoteToRepoResolver {
	return remoteToRepoResolver{
		remoteNameToRepo: newRemoteNameToRepoFn(remotesFn),
	}
}

// resolve takes a remote and returns a repository representing it.
func (r remoteToRepoResolver) resolve(remote remote) (ghrepo.Interface, error) {
	switch v := remote.(type) {
	case remoteName:
		repo, err := r.remoteNameToRepo(v.name)
		if err != nil {
			return nil, fmt.Errorf("could not resolve remote %q: %w", v.name, err)
		}
		return repo, nil
	case remoteURL:
		repo, err := ghrepo.FromURL(v.url)
		if err != nil {
			return nil, fmt.Errorf("could not parse remote URL %q: %w", v.url, err)
		}
		return repo, nil
	default:
		return nil, fmt.Errorf("unsupported remote type %T, value: %v", v, remote)
	}
}

// A defaultPushTarget represents the remote name or URL and a branch name
// that we would expect a branch to be pushed to if `git push` were run with
// no further arguments. This is the most likely place for the head of the PR
// to be, but it's not guaranteed. The user may have pushed to another branch
// directly via `git push <remote> <local>:<remote>` and not set up tracking information.
// A branch name is always present.
//
// It's possible that we're unable to determine a remote, if the user had pushed directly
// to a URL for example `git push <url> <branch>`, which is why it is optional. When present,
// the remote may either be a name or a URL.
type defaultPushTarget struct {
	remote     o.Option[remote]
	branchName string
}

// newDefaultPushTarget is a thin wrapper over defaultPushTarget to help with
// generic type inference, to reduce verbosity in repeating the parametric type.
func newDefaultPushTarget(remote remote, branchName string) defaultPushTarget {
	return defaultPushTarget{
		remote:     o.Some(remote),
		branchName: branchName,
	}
}

// tryDetermineDefaultPushTarget uses git configuration to make a best guess about where a branch
// is pushed to, and where it would be pushed to if the user ran `git push` with no additional
// arguments.
//
// Firstly, it attempts to resolve the @{push} ref, which is the most reliable method, as this
// is what git uses to determine the remote tracking branch
//
// If this fails, we go through a series of steps to determine the remote:
//
// 1. check branch configuration for `branch.<name>.pushRemote = <name> | <url>`
// 2. check remote configuration for `remote.pushDefault = <name>`
// 3. check branch configuration for `branch.<name>.remote = <name> | <url>`
//
// If none of these are set, we indicate that we were unable to determine the
// remote by returning a None value for the remote.
//
// The branch name is always set. The default configuration for push.default (current) indicates
// that a git push should use the same remote branch name as the local branch name. If push.default
// is set to upstream or tracking (deprecated form of upstream), then we use the branch name from the merge ref.
func tryDetermineDefaultPushTarget(gitClient GitConfigClient, localBranchName string) (defaultPushTarget, error) {
	// If @{push} resolves, then we have the remote tracking branch already, no problem.
	if pushRevisionRef, err := gitClient.PushRevision(context.Background(), localBranchName); err == nil {
		return newDefaultPushTarget(remoteName{pushRevisionRef.Remote}, pushRevisionRef.Branch), nil
	}

	// But it doesn't always resolve, so we can suppress the error and move on to other means
	// of determination. We'll first look at branch and remote configuration to make a determination.
	branchConfig, err := gitClient.ReadBranchConfig(context.Background(), localBranchName)
	if err != nil {
		return defaultPushTarget{}, err
	}

	pushDefault, err := gitClient.PushDefault(context.Background())
	if err != nil {
		return defaultPushTarget{}, err
	}

	// We assume the PR's branch name is the same as whatever was provided, unless the user has specified
	// push.default = upstream or tracking, then we use the branch name from the merge ref.
	remoteBranch := localBranchName
	if pushDefault == git.PushDefaultUpstream || pushDefault == git.PushDefaultTracking {
		remoteBranch = strings.TrimPrefix(branchConfig.MergeRef, "refs/heads/")
		if remoteBranch == "" {
			return defaultPushTarget{}, fmt.Errorf("could not determine remote branch name")
		}
	}

	// To get the remote, we look to the git config. It comes from one of the following, in order of precedence:
	// 1. branch.<name>.pushRemote (which may be a name or a URL)
	// 2. remote.pushDefault (which is a remote name)
	// 3. branch.<name>.remote (which may be a name or a URL)
	if branchConfig.PushRemoteName != "" {
		return newDefaultPushTarget(
			remoteName{branchConfig.PushRemoteName},
			remoteBranch,
		), nil
	}

	if branchConfig.PushRemoteURL != nil {
		return newDefaultPushTarget(
			remoteURL{branchConfig.PushRemoteURL},
			remoteBranch,
		), nil
	}

	remotePushDefault, err := gitClient.RemotePushDefault(context.Background())
	if err != nil {
		return defaultPushTarget{}, err
	}

	if remotePushDefault != "" {
		return newDefaultPushTarget(
			remoteName{remotePushDefault},
			remoteBranch,
		), nil
	}

	if branchConfig.RemoteName != "" {
		return newDefaultPushTarget(
			remoteName{branchConfig.RemoteName},
			remoteBranch,
		), nil
	}

	if branchConfig.RemoteURL != nil {
		return newDefaultPushTarget(
			remoteURL{branchConfig.RemoteURL},
			remoteBranch,
		), nil
	}

	// If we couldn't find the remote, we'll indicate that to the caller via None.
	return defaultPushTarget{
		remote:     o.None[remote](),
		branchName: remoteBranch,
	}, nil
}
