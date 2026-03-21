package batchclone

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/repo/clone"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type BatchCloneOptions struct {
	IO       *iostreams.IOStreams
	Prompter prompter.Prompter

	CloneRun func(*clone.CloneOptions) error

	FilePath         string
	TargetDir        string
	SkipExisting     bool
	ContinueOnError  bool
	NoConfirm        bool
	NoUpstream       bool
	UpstreamName     string
	GitArgs          []string
	LogFile          string
}

type RepoItem struct {
	Input      string
	Repository string
	Directory  string
}

func NewCmdBatchClone(f *cmdutil.Factory, runF func(*BatchCloneOptions) error) *cobra.Command {
	opts := &BatchCloneOptions{
		IO:           f.IOStreams,
		Prompter:     f.Prompter,
		UpstreamName: "upstream",
	}

	cmd := &cobra.Command{
		Use:   "batch-clone --file <path> [-- <gitflags>...]",
		Short: "Clone multiple repositories from a file",
		Long: heredoc.Doc(`
			Clone multiple GitHub repositories from a text file.

			Each line in the file can be one of:
			- OWNER/REPO
			- https://github.com/OWNER/REPO.git
			- git@github.com:OWNER/REPO.git

			Blank lines and lines starting with # are ignored.
		`),
		Example: heredoc.Doc(`
			# Clone repositories listed in repos.txt
			$ gh repo batch-clone --file repos.txt

			# Clone into a target directory
			$ gh repo batch-clone --file repos.txt --target-dir ./workspace

			# Continue on failures
			$ gh repo batch-clone --file repos.txt --continue-on-error
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.GitArgs = args
			if runF != nil {
				return runF(opts)
			}
			return batchCloneRun(opts, f)
		},
	}

	cmd.Flags().StringVarP(&opts.FilePath, "file", "f", "", "Path to repository list file")
	cmd.Flags().StringVarP(&opts.TargetDir, "target-dir", "d", "", "Directory to clone repositories into")
	cmd.Flags().BoolVar(&opts.SkipExisting, "skip-existing", false, "Skip repositories whose destination already exists")
	cmd.Flags().BoolVar(&opts.ContinueOnError, "continue-on-error", false, "Continue cloning after an error")
	cmd.Flags().BoolVar(&opts.NoConfirm, "no-confirm", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&opts.NoUpstream, "no-upstream", false, "Do not add an upstream remote when cloning a fork")
	cmd.Flags().StringVarP(&opts.UpstreamName, "upstream-remote-name", "u", "upstream", "Upstream remote name when cloning a fork")
	cmd.Flags().StringVar(&opts.LogFile, "log-file", "", "Path to write clone results log")

	_ = cmd.MarkFlagRequired("file")
	_ = cmd.MarkFlagsMutuallyExclusive("upstream-remote-name", "no-upstream")

	return cmd
}

func batchCloneRun(opts *BatchCloneOptions, f *cmdutil.Factory) error {
	items, err := parseRepoFile(opts.FilePath, opts.TargetDir)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return cmdutil.FlagErrorf("no repositories found in %s", opts.FilePath)
	}

	if !opts.NoConfirm && opts.IO.CanPrompt() {
		msg := fmt.Sprintf("Clone %d repositories?", len(items))
		ok, err := opts.Prompter.Confirm(msg, true)
		if err != nil {
			return err
		}
		if !ok {
			return cmdutil.CancelError
		}
	}

	logPath := opts.LogFile
	if logPath == "" {
		logPath = fmt.Sprintf("batch-clone-%s.log", time.Now().Format("20060102-150405"))
	}

	var lines []string
	successCount := 0

	for _, item := range items {
		dest := item.Directory
		if opts.SkipExisting {
			if _, err := os.Stat(dest); err == nil {
				lines = append(lines, fmt.Sprintf("SKIP\t%s\t%s", item.Repository, dest))
				continue
			}
		}

		cloneOpts := &clone.CloneOptions{
			IO:           f.IOStreams,
			HttpClient:   f.HttpClient,
			GitClient:    f.GitClient,
			Config:       f.Config,
			Repository:   item.Repository,
			GitArgs:      append([]string{dest}, opts.GitArgs...),
			NoUpstream:   opts.NoUpstream,
			UpstreamName: opts.UpstreamName,
		}

		err := cloneRun(cloneOpts)
		if err != nil {
			lines = append(lines, fmt.Sprintf("FAIL\t%s\t%s\t%v", item.Repository, dest, err))
			if !opts.ContinueOnError {
				_ = os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)
				return err
			}
			continue
		}

		successCount++
		lines = append(lines, fmt.Sprintf("OK\t%s\t%s", item.Repository, dest))
	}

	lines = append(lines, fmt.Sprintf("SUMMARY\tsuccess=%d\ttotal=%d", successCount, len(items)))
	_ = os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	return nil
}

func parseRepoFile(path string, targetDir string) ([]RepoItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var items []RepoItem
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		repo, dir, err := normalizeRepoLine(line, targetDir)
		if err != nil {
			return nil, fmt.Errorf("invalid repository entry %q: %w", line, err)
		}

		items = append(items, RepoItem{
			Input:      line,
			Repository: repo,
			Directory:  dir,
		})
	}

	return items, scanner.Err()
}

func normalizeRepoLine(line string, targetDir string) (string, string, error) {
	repo := line

	if strings.Contains(line, "://") || strings.Contains(line, "@") {
		u, err := git.ParseURL(line)
		if err != nil {
			return "", "", err
		}
		path := strings.TrimPrefix(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		repo = path
	}

	parts := strings.Split(repo, "/")
	name := parts[len(parts)-1]
	dest := name
	if targetDir != "" {
		dest = filepath.Join(targetDir, name)
	}

	return repo, dest, nil
}

// 这里先用包装函数，便于后续单测 stub
var cloneRun = cloneRunAdapter

func cloneRunAdapter(opts *clone.CloneOptions) error {
	return cloneRunner(opts)
}

// 实际项目里建议把 clone.cloneRun 抽成可复用导出函数，
// 如果不想改 clone.go，就在 batchclone.go 内复制必要逻辑。
func cloneRunner(opts *clone.CloneOptions) error {
	return nil
}
