package commit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"io"
	"net/http"
	"os"
	"strings"
)

var (
	gitClient     *git.Client
	apiClient     *api.Client
	repo          ghrepo.Interface
	host          string
	defaultBranch string
)

type CommitOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Browser    browser.Browser
	GitClient  *git.Client

	// CommitMessage the message designated for the particular commit
	CommitMessage string
	// PatternMatches pattern matches for the files to be committed
	PatternMatches []string
	// Branch the name of the branch the commit will be made to. The default will be the current branch
	Branch string

	// CommitAll commit all changed files
	CommitAll bool
	// Force commit traditionally ignored files
	Force bool
	// IncludeUntracked include untracked files in the commit
	IncludeUntracked bool
	// IncludeStagedFiles include staged files in the commit
	IncludeStagedFiles bool

	// SyncWithRemote will ensure that the local branch is up to date with the remote branch
	SyncWithRemote bool

	// DryRun get a description of the commit that would be made
	DryRun bool
}

func NewCmdCommit(f *cmdutil.Factory) *cobra.Command {
	opts := &CommitOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		GitClient:  f.GitClient,
		Config:     f.Config,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "commit [<files>...] -b [<branch>]",
		Short:                 "Create a new commit.",
		PreRun: func(c *cobra.Command, args []string) {
			opts.BaseRepo = cmdutil.OverrideBaseRepoFunc(f, "")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Set pattern matches from arguments
			opts.PatternMatches = args

			if err := cmdutil.MutuallyExclusive(
				"specify only one of `--commit-all`, `--force`",
				opts.CommitAll,
				opts.Force,
			); err != nil {
				return err
			}

			return createCommit(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "The name of the branch to commit to.")
	cmd.Flags().StringVarP(&opts.CommitMessage, "message", "m", "", "Commit message for the new commit.")
	cmd.Flags().BoolVarP(&opts.CommitAll, "all", "a", false, "Commit all changed files.")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Force the commit of traditionally ignored files.")
	cmd.Flags().BoolVar(&opts.IncludeUntracked, "include-untracked", false, "Include untracked files in the commit.")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Preview the commit without actually creating it.")
	cmd.Flags().BoolVar(&opts.IncludeStagedFiles, "include-staged", false, "Include staged files in the commit.")
	cmd.Flags().BoolVar(&opts.SyncWithRemote, "sync-local", false, "Ensure that the local branch is up to date with the remote branch.")

	// Mark --message as required
	_ = cmd.MarkFlagRequired("message")
	_ = cmd.MarkFlagRequired("branch")

	cmdutil.EnableRepoOverride(cmd, f)
	return cmd
}

func setupContext(opts *CommitOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	apiClient = api.NewClientFromHTTP(httpClient)
	gitClient = opts.GitClient
	repo, err = opts.BaseRepo()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	host, _ = cfg.Authentication().DefaultHost()
	defaultBranch, err = api.RepoDefaultBranch(apiClient, repo)
	if err != nil {
		return err
	}
	return nil
}

func createCommit(opts *CommitOptions) error {
	err := setupContext(opts)
	if err != nil {
		return nil
	}

	alreadyStagedFiles, err := listStagedFiles()
	if len(alreadyStagedFiles) > 0 && !opts.IncludeStagedFiles {
		return errors.New("staged files found, use --include-staged to include them in the commit")
	}

	filesToCommit, err := listFilesForCommit(opts)
	filesToCommit = append(filesToCommit, alreadyStagedFiles...)

	if err != nil {
		return err
	}

	branchExists, latestCommit, err := getLatestCommit(defaultBranch, opts.Branch)
	if err != nil {
		return err
	}
	if !branchExists {
		err = createNewBranch(latestCommit, opts.Branch)
		if err != nil {
			return err
		}
	}
	treeTip, err := getTreeTip(latestCommit)
	if err != nil {
		return err
	}
	blobs, err := createBlobs(filesToCommit)
	if err != nil {
		return err
	}
	newTreeSha, err := createNewTree(treeTip, blobs)
	if err != nil {
		return err
	}
	newCommitSha, err := commitTree(newTreeSha, latestCommit, opts.CommitMessage)
	if err != nil {
		return err
	}
	err = updateBranch(newCommitSha, opts.Branch)
	if err != nil {
		return err
	}

	if opts.SyncWithRemote {
		err = syncWithRemote(opts.Branch)
		if err != nil {
			return err
		}
	}

	return nil
}

func syncWithRemote(branchName string) error {
	// First thing is run git fetch
	_, err := runGitCommandWithOutput([]string{"fetch"})
	if err != nil {
		return err
	}
	// Now stash the latest changes
	_, err = runGitCommandWithOutput([]string{"stash", "push", "--include-untracked"})
	if err != nil {
		return err
	}
	_, err = runGitCommandWithOutput([]string{"checkout", branchName})
	if err != nil {
		return err
	}
	// Apply the stash
	_, err = runGitCommandWithOutput([]string{"stash", "pop"})
	if err != nil {
		return err
	}
	_, err = runGitCommandWithOutput([]string{"pull"})
	if err != nil {
		return err
	}

	return nil
}

func updateBranch(commitSha string, branchName string) error {
	body := map[string]interface{}{
		"sha": commitSha,
	}
	_, err := makeRequest(fmt.Sprintf("/git/refs/heads/%s", branchName), "POST", body, nil)
	return err
}

func commitTree(treeSha string, latestCommit string, commitMessage string) (string, error) {
	body := map[string]interface{}{
		"message": commitMessage,
		"tree":    treeSha,
		"parents": []string{latestCommit},
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	_, err := makeRequest("/git/commits", "POST", body, &commit)
	if err != nil {
		return "", err
	}

	return commit.SHA, nil
}

func createNewBranch(commitSha string, branchName string) error {
	body := map[string]interface{}{
		"ref": fmt.Sprintf("refs/heads/%s", branchName),
		"sha": commitSha,
	}
	_, err := makeRequest("/git/refs", "POST", body, nil)
	return err
}

func createNewTree(treeSha string, blobs []map[string]interface{}) (string, error) {
	tree := map[string]interface{}{
		"base_tree": treeSha,
		"tree":      blobs,
	}

	var treeStruct struct {
		SHA string `json:"sha"`
	}
	_, err := makeRequest("/git/trees", "POST", tree, &treeStruct)
	if err != nil {
		return "", err
	}

	return treeStruct.SHA, nil
}

func listStagedFiles() ([]string, error) {
	command := []string{
		"git",
		"diff",
		"--name-only",
		"--cached",
	}
	return getGitOutput(command[1:])
}

func createBlobs(files []string) ([]map[string]interface{}, error) {
	blobs := make([]map[string]interface{}, 0)
	for _, file := range files {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			blobs = append(blobs, map[string]interface{}{
				"path": file,
				"mode": "100644",
				"type": "blob",
				"sha":  nil,
			})
		} else {
			data, _ := os.ReadFile(file)
			encoded := base64.StdEncoding.EncodeToString(data)

			var blobStruct struct {
				SHA string `json:"sha"`
			}

			body := map[string]interface{}{
				"content":  encoded,
				"encoding": "base64",
			}
			_, err = makeRequest("/git/blobs", "POST", body, &blobStruct)
			if err != nil {
				return nil, err
			}

			blobs = append(blobs, map[string]interface{}{
				"path": file,
				"mode": "100644",
				"type": "blob",
				"sha":  blobStruct.SHA,
			})
		}
	}
	return blobs, nil
}

func listFilesForCommit(opts *CommitOptions) ([]string, error) {
	if !opts.CommitAll && (opts.PatternMatches != nil || len(opts.PatternMatches) == 0) {
		return nil, errors.New("no files to commit")
	}

	if opts.CommitAll {
		return listFilesUsingPatterns([]string{"."}, opts.Force, !opts.IncludeUntracked)
	}
	return listFilesUsingPatterns(opts.PatternMatches, opts.Force, !opts.IncludeUntracked)
}

func listFilesUsingPatterns(patterns []string, force bool, excludeUntracked bool) ([]string, error) {
	command := []string{
		"git",
		"add",
		"--dry-run",
	}
	if excludeUntracked {
		command = append(command, "-u")
	} else if force {
		command = append(command, "-f")
	}

	command = append(command, patterns...)
	commandOutput, err := getGitOutput(command[1:])
	if err != nil {
		return nil, err
	}

	files := make([]string, 0)
	for _, commandResult := range commandOutput {
		// Adding files using git add --dry-run returns "add '<FILENAME>'"
		filename := strings.Trim(strings.Replace(commandResult, "add ", "", 1), "'")
		files = append(files, filename)
	}
	return files, nil
}

// makeRequest make a request to the GitHub API, using a temporary file for the body of the message
func makeRequest(endpoint string, method string, body map[string]interface{}, data interface{}) (map[string]interface{}, error) {
	endpoint = fmt.Sprintf("repos/%s/%s", repo.RepoOwner(), repo.RepoName()) + endpoint

	var responseBody map[string]interface{}

	var ioBody *os.File
	if body != nil {
		tmpFile, err := writeToTempFile(body)
		if err != nil {
			return nil, err
		}
		b, _ := os.Open(tmpFile.Name())
		ioBody = b
	}

	var err error
	if data != nil {
		if method == "GET" {
			err = apiClient.REST(host, method, endpoint, nil, &data)
		} else {
			err = apiClient.REST(host, method, endpoint, ioBody, &data)
		}

	} else {
		if method == "GET" {
			err = apiClient.REST(host, method, endpoint, nil, &responseBody)
		} else {
			err = apiClient.REST(host, method, endpoint, ioBody, &responseBody)
		}
	}

	if err != nil {
		return nil, err
	}
	return responseBody, nil
}

// writeToTempFile writes a map[string]interface{} to a temporary file in JSON format.
func writeToTempFile(data map[string]interface{}) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", "body-*.json")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	encoder := json.NewEncoder(tmpFile)
	if err := encoder.Encode(data); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("failed to write JSON to temp file: %w", err)
	}

	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("failed to reset file pointer: %w", err)
	}

	return tmpFile, nil
}

// runGitCommandWithOutput runs a git command and returns the output as a list of strings.
func runGitCommandWithOutput(command []string) ([]string, error) {
	cmd, err := gitClient.Command(context.Background(), command...)
	cmdOutput, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	outputStr := strings.TrimSpace(string(cmdOutput))
	cmdOutputAsList := strings.Split(outputStr, "\n")
	return cmdOutputAsList, nil
}

func getTreeTip(latestCommit string) (string, error) {
	path := fmt.Sprintf("/git/trees/%s", latestCommit)
	output, err := makeRequest(path, "GET", nil, nil)
	if err != nil {
		return "", err
	}
	return output["sha"].(string), nil
}

// getLatestCommit returns whether the branch exists, the sha of the latest commit (either to the branch if it exists, or the default branch), and any errors
func getLatestCommit(defaultBranch string, branch string) (bool, string, error) {
	var commitResponse struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}

	_, err := makeRequest(fmt.Sprintf("/branches/%s", branch), "GET", nil, &commitResponse)
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) && (httpError.StatusCode != 404 || httpError.Message != "Branch not found") {
			return false, "", err
		}
	} else {
		return true, commitResponse.Commit.SHA, nil
	}

	var defaultCommitResponse struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	_, err = makeRequest(fmt.Sprintf("/branches/%s", defaultBranch), "GET", nil, &defaultCommitResponse)
	return false, defaultCommitResponse.Commit.SHA, nil
}

// getGitOutput runs a git command and returns the output as a list of strings.
func getGitOutput(command []string) ([]string, error) {
	cmd, err := gitClient.Command(context.Background(), command...)
	cmdOutput, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	outputStr := strings.TrimSpace(string(cmdOutput))
	cmdOutputAsList := strings.Split(outputStr, "\n")

	prunedList := make([]string, 0)
	for _, item := range cmdOutputAsList {
		if item != "" {
			prunedList = append(prunedList, item)
		}
	}

	return prunedList, nil
}
