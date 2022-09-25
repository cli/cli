package git

import (
	"io"
	"os"
)

func GitCommand(args ...string) (*gitCommand, error) {
	c := &Client{}
	return c.Command(args...)
}

func ShowRefs(ref ...string) ([]Ref, error) {
	c := &Client{}
	return c.ShowRefs(ref...)
}

func CurrentBranch() (string, error) {
	c := &Client{}
	return c.CurrentBranch()
}

func Config(name string) (string, error) {
	c := &Client{}
	return c.Config(name)
}

func UncommittedChangeCount() (int, error) {
	c := &Client{}
	return c.UncommittedChangeCount()
}

func Commits(baseRef, headRef string) ([]*Commit, error) {
	c := &Client{}
	return c.Commits(baseRef, headRef)
}

func LastCommit() (*Commit, error) {
	c := &Client{}
	return c.LastCommit()
}

func CommitBody(sha string) (string, error) {
	c := &Client{}
	return c.CommitBody(sha)
}

func Push(remote string, ref string, cmdIn io.ReadCloser, cmdOut, cmdErr io.Writer) error {
	//TODO: Replace with factory GitClient
	c := &Client{
		Stdin:  cmdIn,
		Stdout: cmdOut,
		Stderr: cmdErr,
	}
	return c.Push(remote, ref)
}

func ReadBranchConfig(branch string) (cfg BranchConfig) {
	c := &Client{}
	return c.ReadBranchConfig(branch)
}

func DeleteLocalBranch(branch string) error {
	c := &Client{}
	return c.DeleteLocalBranch(branch)
}

func HasLocalBranch(branch string) bool {
	c := &Client{}
	return c.HasLocalBranch(branch)
}

func CheckoutBranch(branch string) error {
	c := &Client{}
	return c.CheckoutBranch(branch)
}

func CheckoutNewBranch(remoteName, branch string) error {
	c := &Client{}
	return c.CheckoutNewBranch(remoteName, branch)
}

func Pull(remote, branch string) error {
	//TODO: Replace with factory GitClient
	c := &Client{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	return c.Pull(remote, branch)
}

func RunClone(cloneURL string, args []string) (target string, err error) {
	//TODO: Replace with factory GitClient
	c := &Client{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	return c.Clone(cloneURL, args)
}

func ToplevelDir() (string, error) {
	c := &Client{}
	return c.ToplevelDir()
}

func GetDirFromPath(repoDir string) (string, error) {
	c := &Client{
		RepoDir: repoDir,
	}
	return c.GitDir()
}

func PathFromRepoRoot() string {
	c := &Client{}
	return c.PathFromRoot()
}

func Remotes() (RemoteSet, error) {
	c := &Client{}
	return c.Remotes()
}

func RemotesForPath(repoDir string) (RemoteSet, error) {
	c := &Client{
		RepoDir: repoDir,
	}
	return c.Remotes()
}

func AddRemote(name, url string) (*Remote, error) {
	c := &Client{}
	return c.AddRemote(name, url, []string{})
}

func AddNamedRemote(url, name, repoDir string, branches []string) error {
	c := &Client{
		RepoDir: repoDir,
	}
	_, err := c.AddRemote(name, url, branches)
	return err
}

func UpdateRemoteURL(name, url string) error {
	c := &Client{}
	return c.UpdateRemoteURL(name, url)
}

func SetRemoteResolution(name, resolution string) error {
	c := &Client{}
	return c.SetRemoteResolution(name, resolution)
}
