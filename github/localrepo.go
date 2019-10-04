package github

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/github/gh/git"
)

func LocalRepo() (repo *GitHubRepo, err error) {
	repo = &GitHubRepo{}

	_, err = git.Dir()
	if err != nil {
		err = fmt.Errorf("fatal: Not a git repository")
		return
	}

	return
}

type GitHubRepo struct {
	remotes []Remote
}

func (r *GitHubRepo) loadRemotes() error {
	if r.remotes != nil {
		return nil
	}

	remotes, err := Remotes()
	if err != nil {
		return err
	}
	r.remotes = remotes

	return nil
}

func (r *GitHubRepo) RemoteByName(name string) (*Remote, error) {
	if err := r.loadRemotes(); err != nil {
		return nil, err
	}

	for _, remote := range r.remotes {
		if remote.Name == name {
			return &remote, nil
		}
	}

	return nil, fmt.Errorf("No git remote with name %s", name)
}

func (r *GitHubRepo) remotesForPublish(owner string) (remotes []Remote) {
	r.loadRemotes()
	remotesMap := make(map[string]Remote)

	if owner != "" {
		for _, remote := range r.remotes {
			p, e := remote.Project()
			if e == nil && strings.EqualFold(p.Owner, owner) {
				remotesMap[remote.Name] = remote
			}
		}
	}

	names := OriginNamesInLookupOrder
	for _, name := range names {
		if _, ok := remotesMap[name]; ok {
			continue
		}

		remote, err := r.RemoteByName(name)
		if err == nil {
			remotesMap[remote.Name] = *remote
		}
	}

	for i := len(names) - 1; i >= 0; i-- {
		name := names[i]
		if remote, ok := remotesMap[name]; ok {
			remotes = append(remotes, remote)
			delete(remotesMap, name)
		}
	}

	// anything other than names has higher priority
	for _, remote := range remotesMap {
		remotes = append([]Remote{remote}, remotes...)
	}

	return
}

func (r *GitHubRepo) CurrentBranch() (branch *Branch, err error) {
	head, err := git.Head()
	if err != nil {
		err = fmt.Errorf("Aborted: not currently on any branch.")
		return
	}

	branch = &Branch{r, head}
	return
}

func (r *GitHubRepo) MasterBranch() *Branch {
	if remote, err := r.MainRemote(); err == nil {
		return r.DefaultBranch(remote)
	} else {
		return r.DefaultBranch(nil)
	}
}

func (r *GitHubRepo) DefaultBranch(remote *Remote) *Branch {
	var name string
	if remote != nil {
		name, _ = git.BranchAtRef("refs", "remotes", remote.Name, "HEAD")
	}
	if name == "" {
		name = "refs/heads/master"
	}
	return &Branch{r, name}
}

func (r *GitHubRepo) RemoteBranchAndProject(owner string, preferUpstream bool) (branch *Branch, project *Project, err error) {
	if err = r.loadRemotes(); err != nil {
		return
	}

	for _, remote := range r.remotes {
		if p, err := remote.Project(); err == nil {
			project = p
			break
		}
	}

	branch, err = r.CurrentBranch()
	if err != nil {
		return
	}

	if project == nil {
		return
	}

	pushDefault, _ := git.Config("push.default")
	if pushDefault == "upstream" || pushDefault == "tracking" {
		upstream, e := branch.Upstream()
		if e == nil && upstream.IsRemote() {
			remote, e := r.RemoteByName(upstream.RemoteName())
			if e == nil {
				p, e := remote.Project()
				if e == nil {
					branch = upstream
					project = p
					return
				}
			}
		}
	}

	shortName := branch.ShortName()
	remotes := r.remotesForPublish(owner)
	if preferUpstream {
		// reverse the remote lookup order; see OriginNamesInLookupOrder
		remotesInOrder := []Remote{}
		for i := len(remotes) - 1; i >= 0; i-- {
			remotesInOrder = append(remotesInOrder, remotes[i])
		}
		remotes = remotesInOrder
	}

	for _, remote := range remotes {
		p, e := remote.Project()
		if e != nil {
			continue
		}
		// NOTE: this is similar RemoteForBranch
		if git.HasFile("refs", "remotes", remote.Name, shortName) {
			name := fmt.Sprintf("refs/remotes/%s/%s", remote.Name, shortName)
			branch = &Branch{r, name}
			project = p
			return
		}
	}

	branch = nil
	return
}

func (r *GitHubRepo) RemoteForBranch(branch *Branch, owner string) *Remote {
	branchName := branch.ShortName()
	for _, remote := range r.remotesForPublish(owner) {
		if git.HasFile("refs", "remotes", remote.Name, branchName) {
			return &remote
		}
	}
	return nil
}

func (r *GitHubRepo) RemoteForRepo(repo *Repository) (*Remote, error) {
	if err := r.loadRemotes(); err != nil {
		return nil, err
	}

	repoUrl, err := url.Parse(repo.HtmlUrl)
	if err != nil {
		return nil, err
	}

	project := NewProject(repo.Owner.Login, repo.Name, repoUrl.Host)

	for _, remote := range r.remotes {
		if rp, err := remote.Project(); err == nil {
			if rp.SameAs(project) {
				return &remote, nil
			}
		}
	}
	return nil, fmt.Errorf("could not find a git remote for '%s/%s'", repo.Owner.Login, repo.Name)
}

func (r *GitHubRepo) RemoteForProject(project *Project) (*Remote, error) {
	if err := r.loadRemotes(); err != nil {
		return nil, err
	}

	for _, remote := range r.remotes {
		remoteProject, err := remote.Project()
		if err == nil && remoteProject.SameAs(project) {
			return &remote, nil
		}
	}
	return nil, fmt.Errorf("could not find a git remote for '%s'", project)
}

func (r *GitHubRepo) MainRemote() (*Remote, error) {
	r.loadRemotes()

	if len(r.remotes) > 0 {
		return &r.remotes[0], nil
	} else {
		return nil, fmt.Errorf("no git remotes found")
	}
}

func (r *GitHubRepo) MainProject() (*Project, error) {
	r.loadRemotes()

	for _, remote := range r.remotes {
		if project, err := remote.Project(); err == nil {
			return project, nil
		}
	}
	return nil, fmt.Errorf("Aborted: could not find any git remote pointing to a GitHub repository")
}

func (r *GitHubRepo) CurrentProject() (project *Project, err error) {
	project, err = r.UpstreamProject()
	if err != nil {
		project, err = r.MainProject()
	}

	return
}

func (r *GitHubRepo) UpstreamProject() (project *Project, err error) {
	currentBranch, err := r.CurrentBranch()
	if err != nil {
		return
	}

	upstream, err := currentBranch.Upstream()
	if err != nil {
		return
	}

	remote, err := r.RemoteByName(upstream.RemoteName())
	if err != nil {
		return
	}

	project, err = remote.Project()

	return
}
