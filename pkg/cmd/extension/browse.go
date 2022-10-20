package extension

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/view"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/spf13/cobra"
)

var appStyle = lipgloss.NewStyle().Padding(1, 2)
var sidebarStyle = lipgloss.NewStyle()

type readmeGetter interface {
	Get(string) (string, error)
}

type cachingReadmeGetter struct {
	client *http.Client
	cache  map[string]string
}

func newReadmeGetter(client *http.Client) readmeGetter {
	return &cachingReadmeGetter{
		client: client,
		cache:  map[string]string{},
	}
}

func (g *cachingReadmeGetter) Get(repoFullName string) (string, error) {
	if readme, ok := g.cache[repoFullName]; ok {
		return readme, nil
	}
	repo, err := ghrepo.FromFullName(repoFullName)
	readme, err := view.RepositoryReadme(g.client, repo, "")
	if err != nil {
		return "", err
	}
	g.cache[repoFullName] = readme.Content
	return readme.Content, nil
}

type extEntry struct {
	URL         string
	Owner       string
	Name        string
	FullName    string
	Readme      string
	Stars       int
	Installed   bool
	Official    bool
	description string
}

func (e extEntry) Title() string {
	installedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#62FF42"))
	officialStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F2DB74"))

	var installed string
	var official string

	if e.Installed {
		installed = installedStyle.Render(" [installed]")
	}

	if e.Official {
		official = officialStyle.Render(" [official]")
	}

	return fmt.Sprintf("%s%s%s", e.FullName, official, installed)
}

func (e extEntry) Description() string { return e.description }
func (e extEntry) FilterValue() string { return e.Title() }

type ibrowser interface {
	Browse(string) error
}

type extBrowseOpts struct {
	cmd      *cobra.Command
	browser  ibrowser
	searcher search.Searcher
	em       extensions.ExtensionManager
	client   *http.Client
	logger   *log.Logger
	cfg      config.Config
	rg       readmeGetter
}

func extBrowse(opts extBrowseOpts) error {
	// TODO support turning debug mode on/off
	f, err := os.CreateTemp("/tmp", "extBrowse-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	opts.logger = log.New(f, "", log.Lshortfile)

	// TODO spinner
	// TODO get manager to tell me what's installed so I can cross ref
	installed := opts.em.List()

	result, err := opts.searcher.Repositories(search.Query{
		Kind:  search.KindRepositories,
		Limit: 1000,
		Qualifiers: search.Qualifiers{
			Topic: []string{"gh-extension"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to search for extensions: %w", err)
	}

	host, _ := opts.cfg.DefaultHost()

	extEntries := []extEntry{}

	for _, repo := range result.Items {
		ee := extEntry{
			URL:         "https://" + host + "/" + repo.FullName,
			FullName:    repo.FullName,
			Owner:       repo.Owner.Login,
			Name:        repo.Name,
			Stars:       repo.StargazersCount,
			description: repo.Description,
		}
		for _, v := range installed {
			// TODO consider a Repo() on Extension interface
			var installedRepo string
			if u, err := git.ParseURL(v.URL()); err == nil {
				if r, err := ghrepo.FromURL(u); err == nil {
					installedRepo = ghrepo.FullName(r)
				}
			}
			if repo.FullName == installedRepo {
				ee.Installed = true
			}
		}
		if ee.Owner == "cli" || ee.Owner == "github" {
			ee.Official = true
		}

		extEntries = append(extEntries, ee)
	}

	opts.rg = newReadmeGetter(opts.client)

	return nil
}
