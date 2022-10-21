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
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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
	//installedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#62FF42"))
	//officialStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F2DB74"))

	var installed string
	var official string

	if e.Installed {
		//installed = installedStyle.Render(" [installed]")
		installed = " [installed]"
	}

	if e.Official {
		//official = officialStyle.Render(" [official]")
		official = " [official]"
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

	app := tview.NewApplication()

	outerFlex := tview.NewFlex()
	innerFlex := tview.NewFlex()

	header := tview.NewTextView().SetText("gh extensions").SetTextAlign(tview.AlignCenter)
	filter := tview.NewInputField().SetLabel("filter: ")
	list := tview.NewList()
	readme := tview.NewTextView()
	help := tview.NewTextView().SetText("/: filter i: install r: remove w: open in browser pgup/pgdn: scroll readme q: quit")

	for _, ee := range extEntries {
		list.AddItem(ee.Title(), ee.Description(), ' ', func() {})
	}

	onSelectItem := func(ix int, _, _ string, _ rune) {
		fullName := extEntries[ix].FullName
		rm, err := opts.rg.Get(fullName)
		if err != nil {
			opts.logger.Println(err.Error())
			readme.SetText("unable to fetch readme :(")
			return
		}

		// TODO sanity check what happens for non markdown readmes

		rendered, err := markdown.Render(rm)
		if err != nil {
			opts.logger.Println(err.Error())
			readme.SetText("unable to render readme :(")
			return
		}

		readme.SetText("")
		readme.SetDynamicColors(true)

		w := tview.ANSIWriter(readme)
		w.Write([]byte(rendered))

		readme.ScrollToBeginning()
	}

	list.SetChangedFunc(onSelectItem)
	// Force fetching of initial readme:
	onSelectItem(0, "", "", rune(0))

	filter.SetChangedFunc(func(text string) {
		// TODO filter list
	})
	filter.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			app.SetFocus(list)
			// TODO
		case tcell.KeyEscape:
			filter.SetText("")
			app.SetFocus(list)
			// TODO clear active filter
		}
	})

	innerFlex.SetDirection(tview.FlexColumn)
	innerFlex.AddItem(list, 0, 1, true)
	innerFlex.AddItem(readme, 0, 1, false)

	outerFlex.SetDirection(tview.FlexRow)
	outerFlex.AddItem(header, 1, -1, false)
	outerFlex.AddItem(filter, 1, -1, false)
	outerFlex.AddItem(innerFlex, 0, 1, true)
	outerFlex.AddItem(help, 1, -1, false)

	app.SetRoot(outerFlex, true)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			app.Stop()
		case 'k':
			return tcell.NewEventKey(tcell.KeyUp, rune(0), 0)
		case 'j':
			return tcell.NewEventKey(tcell.KeyDown, rune(0), 0)
		case 'i':
			opts.logger.Println("INSTALL REQUESTED")
		case 'r':
			opts.logger.Println("REMOVE REQUESTED")
		case '/':
			app.SetFocus(filter)
			return nil
		}
		switch event.Key() {
		case tcell.KeyLeft:
			opts.logger.Println("PAGE UP LIST")
		case tcell.KeyRight:
			opts.logger.Println("PAGE DOWN LIST")
		case tcell.KeyPgUp:
			row, col := readme.GetScrollOffset()
			if row > 0 {
				readme.ScrollTo(row-2, col)
			}
			return nil
		case tcell.KeyPgDn:
			row, col := readme.GetScrollOffset()
			readme.ScrollTo(row+2, col)
			return nil
		}
		return event
	})

	// TODO filter input in a modal
	// TODO install/remove feedback in a modal

	if err := app.Run(); err != nil {
		return err
	}

	return nil
}
