package branch

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
)

type BranchOptions struct {
	IO *iostreams.IOStreams

	Preview bool

	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
}

func NewCmdBranch(f *cmdutil.Factory, runF func(*BranchOptions) error) *cobra.Command {
	opts := BranchOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "branch",
		Short: "Switch between git branches",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if runF != nil {
				return runF(&opts)
			}
			return branchRun(&opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Preview, "preview", "p", false, "Preview git log for selected branch")
	return cmd
}

func branchRun(opts *BranchOptions) error {
	branches, err := gitBranches()
	if err != nil {
		return err
	}
	sort.Sort(branches)

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	prs, err := pullRequests(httpClient, baseRepo)
	if err != nil {
		return err
	}

	gray := ansi.ColorFunc("black+h")
	green := ansi.ColorFunc("green")
	magenta := ansi.ColorFunc("magenta")
	red := ansi.ColorFunc("red")
	yellow := ansi.ColorFunc("yellow")

	_, ttyWidth := utils.TerminalInfo(opts.IO.Out)
	if ttyWidth < 1 {
		ttyWidth = 80
	}

	fzfArgs := []string{"--ansi", "-n1"}
	if opts.Preview {
		fzfArgs = append(fzfArgs, "--preview=echo {} | cut -f1 -d ' ' | xargs git log -5 --color=always")
	}

	fzf := exec.Command("fzf", fzfArgs...)
	fzf.Stderr = opts.IO.ErrOut
	fzfInput, err := fzf.StdinPipe()
	if err != nil {
		return err
	}
	fzfOutput, err := fzf.StdoutPipe()
	if err != nil {
		return err
	}

	t := utils.NewTablePrinterTerminalInfo(fzfInput, true, ttyWidth)
	for _, r := range branches {
		t.AddField(r.Name, nil, nil)
		if r.IsCurrent {
			t.AddField("(current)", nil, yellow)
		} else {
			t.AddField(utils.FuzzyAgo(time.Since(r.CommitterDate)), nil, gray)
		}

		var pr *pullRequest
		for _, p := range prs {
			if p.HeadRefName == r.Name {
				pr = &p
				break
			}
		}

		if pr == nil {
			t.AddField("", nil, nil)
		} else {
			c := green
			if pr.IsDraft {
				c = nil
			} else if pr.State == "MERGED" {
				c = magenta
			} else if pr.State == "CLOSED" {
				c = red
			}
			t.AddField(fmt.Sprintf("#%d by @%s", pr.Number, pr.Author.Login), nil, c)
		}

		t.EndRow()
	}

	err = t.Render()
	if err != nil {
		return err
	}
	fzfInput.Close()

	err = fzf.Start()
	if err != nil {
		return fmt.Errorf("error starting fzf: %w", err)
	}

	fzfBytes, err := ioutil.ReadAll(fzfOutput)
	if err != nil {
		return fmt.Errorf("error reading fzf output: %w", err)
	}
	fzfOutput.Close()

	err = fzf.Wait()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ProcessState.ExitCode() == 130 {
			return cmdutil.SilentError
		}
		return fmt.Errorf("error waiting on fzf: %w", err)
	}

	fzfResult := string(fzfBytes)
	chosenBranch := fzfResult
	if idx := strings.IndexAny(fzfResult, " \t\n"); idx >= 0 {
		chosenBranch = fzfResult[:idx]
	}

	gitCheckout := git.GitCommand("checkout", chosenBranch)
	gitCheckout.Stdout = opts.IO.Out
	gitCheckout.Stderr = opts.IO.ErrOut
	return gitCheckout.Run()
}

type GitBranch struct {
	Name          string
	CommitterDate time.Time
	IsCurrent     bool
}

// GitBranchSlice is a slice of GitBranch that implements sort.Interface
type GitBranchSlice []GitBranch

func (b GitBranchSlice) Len() int {
	return len(b)
}
func (b GitBranchSlice) Less(i, j int) bool {
	if b[i].IsCurrent != b[j].IsCurrent {
		return b[i].IsCurrent
	}
	if !b[i].CommitterDate.Equal(b[j].CommitterDate) {
		return b[i].CommitterDate.After(b[j].CommitterDate)
	}
	return b[i].Name < b[j].Name
}
func (b GitBranchSlice) Swap(i, j int) {
	b[j], b[i] = b[i], b[j]
}

func gitBranches() (GitBranchSlice, error) {
	gitBranch := git.GitCommand("for-each-ref", "refs/heads/*", "--format=%(refname:short)%09%(committerdate:unix)%09%(HEAD)")
	gitBranchOutput, err := gitBranch.Output()
	if err != nil {
		return nil, err
	}
	branchLines := strings.Split(strings.TrimSuffix(string(gitBranchOutput), "\n"), "\n")

	branches := make(GitBranchSlice, len(branchLines))
	for i, b := range branchLines {
		parts := strings.SplitN(b, "\t", 3)
		isCurrent := len(parts) == 3 && parts[2] == "*"
		sec, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return branches, err
		}
		branches[i] = GitBranch{
			Name:          parts[0],
			CommitterDate: time.Unix(sec, 0),
			IsCurrent:     isCurrent,
		}
	}

	return branches, nil
}
