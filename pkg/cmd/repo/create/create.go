package create

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	HttpClient func() (*http.Client, error)
	Config     func() (config.Config, error)
	IO         *iostreams.IOStreams

	Name         string
	Description  string
	Homepage     string
	Team         string
	EnableIssues bool
	EnableWiki   bool
	Public       bool
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:   "create [<name>]",
		Short: "Create a new repository",
		Long:  `Create a new GitHub repository.`,
		Args:  cobra.MaximumNArgs(1),
		Example: heredoc.Doc(`
			# create a repository under your account using the current directory name
			$ gh repo create

			# create a repository with a specific name
			$ gh repo create my-project

			# create a repository in an organization
			$ gh repo create cli/my-project
	  `),
		Annotations: map[string]string{
			"help:arguments": heredoc.Doc(
				`A repository can be supplied as an argument in any of the following formats:
           - <OWNER/REPO>
           - by URL, e.g. "https://github.com/OWNER/REPO"`),
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Name = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return createRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of repository")
	cmd.Flags().StringVarP(&opts.Homepage, "homepage", "h", "", "Repository home page URL")
	cmd.Flags().StringVarP(&opts.Team, "team", "t", "", "The name of the organization team to be granted access")
	cmd.Flags().BoolVar(&opts.EnableIssues, "enable-issues", true, "Enable issues in the new repository")
	cmd.Flags().BoolVar(&opts.EnableWiki, "enable-wiki", true, "Enable wiki in the new repository")
	cmd.Flags().BoolVar(&opts.Public, "public", false, "Make the new repository public (default: private)")

	return cmd
}

func createRun(opts *CreateOptions) error {
	projectDir, projectDirErr := git.ToplevelDir()

	orgName := ""
	name := opts.Name

	if name != "" {
		if strings.Contains(name, "/") {
			newRepo, err := ghrepo.FromFullName(name)
			if err != nil {
				return fmt.Errorf("argument error: %w", err)
			}
			orgName = newRepo.RepoOwner()
			name = newRepo.RepoName()
		}
	} else {
		if projectDirErr != nil {
			return projectDirErr
		}
		name = path.Base(projectDir)
	}

	visibility := "PRIVATE"
	if opts.Public {
		visibility = "PUBLIC"
	}

	input := repoCreateInput{
		Name:             name,
		Visibility:       visibility,
		OwnerID:          orgName,
		TeamID:           opts.Team,
		Description:      opts.Description,
		HomepageURL:      opts.Homepage,
		HasIssuesEnabled: opts.EnableIssues,
		HasWikiEnabled:   opts.EnableWiki,
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	repo, err := repoCreate(httpClient, input)
	if err != nil {
		return err
	}

	stderr := opts.IO.ErrOut
	stdout := opts.IO.Out
	isTTY := opts.IO.IsStdoutTTY()

	if isTTY {
		fmt.Fprintf(stderr, "%s Created repository %s on GitHub\n", utils.GreenCheck(), ghrepo.FullName(repo))
	} else {
		fmt.Fprintln(stdout, repo.URL)
	}

	// TODO This is overly wordy and I'd like to streamline this.
	cfg, err := opts.Config()
	if err != nil {
		return err
	}
	protocol, err := cfg.Get("", "git_protocol")
	if err != nil {
		return err
	}
	remoteURL := ghrepo.FormatRemoteURL(repo, protocol)

	if projectDirErr == nil {
		_, err = git.AddRemote("origin", remoteURL)
		if err != nil {
			return err
		}
		if isTTY {
			fmt.Fprintf(stderr, "%s Added remote %s\n", utils.GreenCheck(), remoteURL)
		}
	} else if isTTY {
		doSetup := false
		err := prompt.Confirm(fmt.Sprintf("Create a local project directory for %s?", ghrepo.FullName(repo)), &doSetup)
		if err != nil {
			return err
		}

		if doSetup {
			path := repo.Name

			gitInit := git.GitCommand("init", path)
			gitInit.Stdout = stdout
			gitInit.Stderr = stderr
			err = run.PrepareCmd(gitInit).Run()
			if err != nil {
				return err
			}
			gitRemoteAdd := git.GitCommand("-C", path, "remote", "add", "origin", remoteURL)
			gitRemoteAdd.Stdout = stdout
			gitRemoteAdd.Stderr = stderr
			err = run.PrepareCmd(gitRemoteAdd).Run()
			if err != nil {
				return err
			}

			fmt.Fprintf(stderr, "%s Initialized repository in './%s/'\n", utils.GreenCheck(), path)
		}
	}
	return nil
}
