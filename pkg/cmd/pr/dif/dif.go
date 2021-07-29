package dif

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/AlecAivazis/survey/v2"
	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/spf13/cobra"
)

type Cache interface {
	Exists(string) bool
	Create(string, []byte) error
}

type cache struct{}

func (cache) Exists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

func (cache) Create(path string, content []byte) error {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return err
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = out.Write(content)
	return err
}

type DifOptions struct {
	HttpClient  func() (*http.Client, error)
	IO          *iostreams.IOStreams
	Cache       Cache
	Finder      shared.PRFinder
	SelectorArg string
}

func NewCmdDif(f *cmdutil.Factory, runF func(*DifOptions) error) *cobra.Command {
	opts := &DifOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Cache:      cache{},
	}

	cmd := &cobra.Command{
		Use:   "dif [<number> | <url> | <branch>]",
		Short: "View changes in a pull request",
		Long: heredoc.Doc(`
			View changes in a pull request.

			Without an argument, the pull request that belongs to the current branch
			is selected.
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Finder = shared.NewFinder(f)

			if repoOverride, _ := cmd.Flags().GetString("repo"); repoOverride != "" && len(args) == 0 {
				return &cmdutil.FlagError{Err: errors.New("argument required when using the --repo flag")}
			}

			if len(args) > 0 {
				opts.SelectorArg = args[0]
			}

			if runF != nil {
				return runF(opts)
			}
			return difRun(opts)
		},
	}

	return cmd
}

func difRun(opts *DifOptions) error {
	if !opts.IO.CanPrompt() {
		return errors.New("must be run interactively")
	}
	findOptions := shared.FindOptions{
		Selector: opts.SelectorArg,
		Fields:   []string{"number", "files", "baseRefOid", "headRefOid"},
	}
	pr, repo, err := opts.Finder.Find(findOptions)
	if err != nil {
		return err
	}
	client, err := opts.HttpClient()
	if err != nil {
		return err
	}
	candidates := []string{}
	for _, file := range pr.Files.Nodes {
		candidates = append(candidates, file.Path)
	}
	sort.Strings(candidates)
	for {
		var filename string
		err = prompt.SurveyAskOne(&survey.Select{
			Message: "View which file diff?",
			Options: candidates,
		}, &filename)
		baseFile, err := getFileContent(opts.Cache, client, repo, filename, pr.BaseRefOid)
		if err != nil {
			return err
		}
		headFile, err := getFileContent(opts.Cache, client, repo, filename, pr.HeadRefOid)
		if err != nil {
			return err
		}
		diffCommand := exec.Command("nvim", "-d", "-R", baseFile, headFile)
		diffCommand.Stderr = opts.IO.ErrOut
		diffCommand.Stdout = opts.IO.Out
		diffCommand.Stdin = opts.IO.In
		err = diffCommand.Run()
		if err != nil {
			return err
		}
		choice := ""
		err = prompt.SurveyAskOne(&survey.Select{
			Message: "What next?",
			Options: []string{
				"View another file diff",
				"Cancel",
			},
		}, &choice)
		if err != nil {
			return err
		}
		switch choice {
		case "View another file diff":
			continue
		default:
			return cmdutil.CancelError
		}
	}
}

func storeFileContent(name string, content []byte) (string, error) {
	return "", nil
}
