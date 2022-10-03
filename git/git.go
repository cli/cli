package git

import (
	"context"
	"io"
	"os"
)

func GitCommand(args ...string) (*gitCommand, error) {
	c := &Client{}
	return c.Command(context.Background(), args...)
}

func ShowRefs(ref ...string) ([]Ref, error) {
	c := &Client{}
	return c.ShowRefs(context.Background(), ref...)
}

func CurrentBranch() (string, error) {
	c := &Client{}
	return c.CurrentBranch(context.Background())
}

func Config(name string) (string, error) {
	c := &Client{}
	return c.Config(context.Background(), name)
}

func UncommittedChangeCount() (int, error) {
	c := &Client{}
	return c.UncommittedChangeCount(context.Background())
}

func Commits(baseRef, headRef string) ([]*Commit, error) {
	c := &Client{}
	return c.Commits(context.Background(), baseRef, headRef)
}

func LastCommit() (*Commit, error) {
	c := &Client{}
	return c.LastCommit(context.Background())
}

func CommitBody(sha string) (string, error) {
	c := &Client{}
	return c.CommitBody(context.Background(), sha)
}

func Push(remote string, ref string, cmdIn io.ReadCloser, cmdOut, cmdErr io.Writer) error {
	//TODO: Replace with factory GitClient and use AuthenticatedCommand
	c := &Client{
		Stdin:  cmdIn,
		Stdout: cmdOut,
		Stderr: cmdErr,
	}
	return c.Push(context.Background(), remote, ref)
}

func ReadBranchConfig(branch string) (cfg BranchConfig) {
	c := &Client{}
	return c.ReadBranchConfig(context.Background(), branch)
}

func DeleteLocalBranch(branch string) error {
	c := &Client{}
	return c.DeleteLocalBranch(context.Background(), branch)
}

func HasLocalBranch(branch string) bool {
	c := &Client{}
	return c.HasLocalBranch(context.Background(), branch)
}

func CheckoutBranch(branch string) error {
	c := &Client{}
	return c.CheckoutBranch(context.Background(), branch)
}

func CheckoutNewBranch(remoteName, branch string) error {
	c := &Client{}
	return c.CheckoutNewBranch(context.Background(), remoteName, branch)
}

func Pull(remote, branch string) error {
	//TODO: Replace with factory GitClient and use AuthenticatedCommand
	c := &Client{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	return c.Pull(context.Background(), remote, branch)
}

func RunClone(cloneURL string, args []string) (target string, err error) {
	//TODO: Replace with factory GitClient and use AuthenticatedCommand
	c := &Client{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	return c.Clone(context.Background(), cloneURL, args)
}

func ToplevelDir() (string, error) {
	c := &Client{}
	return c.ToplevelDir(context.Background())
}

func GetDirFromPath(repoDir string) (string, error) {
	c := &Client{
		RepoDir: repoDir,
	}
	return c.GitDir(context.Background())
}

func PathFromRepoRoot() string {
	c := &Client{}
	return c.PathFromRoot(context.Background())
}

func Remotes() (RemoteSet, error) {
	c := &Client{}
	return c.Remotes(context.Background())
}

func RemotesForPath(repoDir string) (RemoteSet, error) {
	c := &Client{
		RepoDir: repoDir,
	}
	return c.Remotes(context.Background())
}

func AddRemote(name, url string) (*Remote, error) {
	c := &Client{}
	return c.AddRemote(context.Background(), name, url, []string{})
}

func AddNamedRemote(url, name, repoDir string, branches []string) error {
	c := &Client{
		RepoDir: repoDir,
	}
	_, err := c.AddRemote(context.Background(), name, url, branches)
	return err
}

func UpdateRemoteURL(name, url string) error {
	c := &Client{}
	return c.UpdateRemoteURL(context.Background(), name, url)
}

func SetRemoteResolution(name, resolution string) error {
	c := &Client{}
	return c.SetRemoteResolution(context.Background(), name, resolution)
}
