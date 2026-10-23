package browse

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/spf13/cobra"
)

type ExtBrowseOpts struct {
	Cmd          *cobra.Command
	Browser      ibrowser
	IO           *iostreams.IOStreams
	Searcher     search.Searcher
	Em           extensions.ExtensionManager
	Client       *http.Client
	Logger       *log.Logger
	Cfg          gh.Config
	Rg           readmeLoader
	Debug        bool
	SingleColumn bool
	renderReadme func(string, int) (string, error)
	runProgram   func(tea.Model, ...tea.ProgramOption) error
}

type ibrowser interface {
	Browse(string) error
}

type readmeLoader interface {
	Get(string) (string, error)
}

type extEntry struct {
	URL         string
	Name        string
	FullName    string
	Installed   bool
	Official    bool
	description string
}

func (e extEntry) Description() string {
	if e.description == "" {
		return "no description provided"
	}
	return e.description
}

func getExtensions(opts ExtBrowseOpts) ([]extEntry, error) {
	extEntries := []extEntry{}

	installed := opts.Em.List()

	result, err := opts.Searcher.Repositories(search.Query{
		Kind:  search.KindRepositories,
		Limit: 1000,
		Qualifiers: search.Qualifiers{
			Topic: []string{"gh-extension"},
		},
	})
	if err != nil {
		return extEntries, fmt.Errorf("failed to search for extensions: %w", err)
	}

	host, _ := opts.Cfg.Authentication().DefaultHost()

	for _, repo := range result.Items {
		if !strings.HasPrefix(repo.Name, "gh-") {
			continue
		}
		ee := extEntry{
			URL:         "https://" + host + "/" + repo.FullName,
			FullName:    repo.FullName,
			Name:        repo.Name,
			description: repo.Description,
		}
		for _, installedExtension := range installed {
			var installedRepo string
			if u, err := git.ParseURL(installedExtension.URL()); err == nil {
				if r, err := ghrepo.FromURL(u); err == nil {
					installedRepo = ghrepo.FullName(r)
				}
			}
			if repo.FullName == installedRepo {
				ee.Installed = true
			}
		}
		if repo.Owner.Login == "cli" || repo.Owner.Login == "github" {
			ee.Official = true
		}

		extEntries = append(extEntries, ee)
	}

	return extEntries, nil
}

func ExtBrowse(opts ExtBrowseOpts) error {
	if opts.Debug {
		f, err := os.CreateTemp("", "extBrowse-*.txt")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())

		opts.Logger = log.New(f, "", log.Lshortfile)
	} else {
		opts.Logger = log.New(io.Discard, "", 0)
	}

	opts.IO.StartProgressIndicator()
	extEntries, err := getExtensions(opts)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	opts.Rg = newReadmeGetter(opts.Client, 24*time.Hour)
	model := newBrowseModel(opts, extEntries)

	runProgram := opts.runProgram
	if runProgram == nil {
		runProgram = func(model tea.Model, programOpts ...tea.ProgramOption) error {
			_, err := tea.NewProgram(model, programOpts...).Run()
			return err
		}
	}

	return runProgram(
		model,
		tea.WithInput(opts.IO.In),
		tea.WithOutput(opts.IO.Out),
	)
}
