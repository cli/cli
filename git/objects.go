package git

import (
	"net/url"
	"strings"
)

// RemoteSet is a slice of git remotes.
type RemoteSet []*Remote

func (r RemoteSet) Len() int      { return len(r) }
func (r RemoteSet) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r RemoteSet) Less(i, j int) bool {
	return remoteNameSortScore(r[i].Name) > remoteNameSortScore(r[j].Name)
}

func remoteNameSortScore(name string) int {
	switch strings.ToLower(name) {
	case "upstream":
		return 3
	case "github":
		return 2
	case "origin":
		return 1
	default:
		return 0
	}
}

// Remote is a parsed git remote.
type Remote struct {
	Name     string
	Resolved string
	FetchURL *url.URL
	PushURL  *url.URL
}

func (r *Remote) String() string {
	return r.Name
}

func NewRemote(name string, u string) *Remote {
	pu, _ := url.Parse(u)
	return &Remote{
		Name:     name,
		FetchURL: pu,
		PushURL:  pu,
	}
}

// Ref represents a git commit reference.
type Ref struct {
	Hash string
	Name string
}

type Commit struct {
	Sha   string
	Title string
	Body  string
}

// These are the keys we read from the git branch.<name> config.
type BranchConfig struct {
	RemoteName     string   // .remote if string
	RemoteURL      *url.URL // .remote if url
	MergeRef       string   // .merge
	PushRemoteName string   // .pushremote if string
	PushRemoteURL  *url.URL // .pushremote if url

	// MergeBase is the optional base branch to target in a new PR if `--base` is not specified.
	MergeBase string
}
