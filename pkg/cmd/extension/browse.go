package extension

import (
	"fmt"
	"log"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/spf13/cobra"
)

var appStyle = lipgloss.NewStyle().Padding(1, 2)
var sidebarStyle = lipgloss.NewStyle()

type uiModel struct {
	sidebar sidebarModel
	extList extListModel
	logger  *log.Logger
}

func newUIModel(l *log.Logger, extEntries []extEntry) uiModel {
	return uiModel{
		extList: newExtListModel(l, extEntries),
		sidebar: newSidebarModel(l),
		logger:  l,
	}
}

func (m uiModel) Init() tea.Cmd {
	// TODO the docs say not to do this but the example code in bubbles does:
	return tea.EnterAltScreen
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.logger.Printf("%#v", msg)

	var cmds []tea.Cmd
	var cmd tea.Cmd
	var newModel tea.Model

	newModel, cmd = m.extList.Update(msg)
	cmds = append(cmds, cmd)
	m.extList = newModel.(extListModel)

	item := newModel.(extListModel).SelectedItem()
	m.sidebar.Content = item.(extEntry).Readme

	newModel, cmd = m.sidebar.Update(msg)
	cmds = append(cmds, cmd)
	m.sidebar = newModel.(sidebarModel)

	return m, tea.Batch(cmds...)
}

func (m uiModel) View() string {
	return lipgloss.JoinHorizontal(lipgloss.Top, m.extList.View(), m.sidebar.View())
}

type sidebarModel struct {
	logger   *log.Logger
	Content  string
	viewport viewport.Model
	ready    bool
}

func newSidebarModel(l *log.Logger) sidebarModel {
	// TODO
	return sidebarModel{
		logger: l,
	}
}

func (m sidebarModel) Init() tea.Cmd {
	return nil
}

func (m sidebarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.logger.Printf("%#v", msg)
	// TODO
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !m.ready {
			m.viewport = viewport.New(80, msg.Height)
			m.viewport.SetContent(m.Content)
			m.ready = true
		} else {
			m.viewport.SetContent(m.Content)
			m.viewport.Height = msg.Height
		}
	default:
		m.viewport.SetContent(m.Content)
	}

	newvp, cmd := m.viewport.Update(msg)
	m.viewport = newvp
	return m, cmd
}

func (m sidebarModel) View() string {
	return sidebarStyle.Render(m.viewport.View())
}

type extEntry struct {
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

	// TODO color -- our thing or lipgloss? probably need to rely on lipgloss.
	var installed string
	var official string

	if e.Installed {
		installed = installedStyle.Render("✓ ")
	}

	if e.Official {
		official = officialStyle.Render("* ")
	}

	return fmt.Sprintf("%s%s%s", installed, official, e.FullName)
}

func (e extEntry) Description() string { return e.description }
func (e extEntry) FilterValue() string { return e.FullName }

type keyMap struct {
	install key.Binding
	remove  key.Binding
	// TODO instead of sorting, consider a toggle for Official Only
	// TODO add key for opening in web
	sort key.Binding
}

func newKeyMap() *keyMap {
	return &keyMap{
		install: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "install"),
		),
		remove: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "remove"),
		),
		sort: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "sort"),
		),
	}
}

type extListModel struct {
	list   list.Model
	keys   *keyMap
	logger *log.Logger
}

func newExtListModel(l *log.Logger, extEntries []extEntry) extListModel {
	items := make([]list.Item, len(extEntries))
	for i := range items {
		items[i] = extEntries[i]
	}
	list := list.New(items, list.NewDefaultDelegate(), 0, 0)

	keys := newKeyMap()
	list.Title = "gh extensions"
	list.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			keys.remove,
			keys.install,
			keys.sort,
		}
	}

	return extListModel{
		logger: l,
		list:   list,
		keys:   keys,
	}
}

func (m extListModel) Init() tea.Cmd {
	return nil
}

func (m extListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.logger.Printf("%#v", msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w, h := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-w, msg.Height-h)
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch {
		// TODO handle install
		// TODO handle remove
		// TODO handle open in web
		//case.keyMatches(msg, )
		}
	}

	nm, cmd := m.list.Update(msg)
	m.list = nm

	return m, cmd
}

func (m extListModel) SelectedItem() list.Item {
	m.logger.Printf("%#v", m.list.SelectedItem())
	return m.list.SelectedItem()
}

func (m extListModel) View() string {
	return appStyle.Render(m.list.View())
}

func extBrowse(cmd *cobra.Command, searcher search.Searcher, em extensions.ExtensionManager) error {
	// TODO support turning debug mode on/off
	f, err := os.CreateTemp("/tmp", "extBrowse-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	l := log.New(f, "", log.Lshortfile)

	// TODO spinner
	// TODO get manager to tell me what's installed so I can cross ref
	installed := em.List()

	result, err := searcher.Repositories(search.Query{
		Kind:  search.KindRepositories,
		Limit: 1000,
		Qualifiers: search.Qualifiers{
			Topic: []string{"gh-extension"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to search for extensions: %w", err)
	}

	extEntries := []extEntry{}

	for _, repo := range result.Items {
		ee := extEntry{
			FullName:    repo.FullName,
			Owner:       repo.Owner.Login,
			Name:        repo.Name,
			Stars:       repo.StargazersCount,
			description: repo.Description,
		}
		for _, v := range installed {
			// TODO the former is git URL and the latter is HTML URL so this doesn't
			// work, do something else. A Repo() method on extension would be ideal.
			l.Printf("%s %s", v.URL(), repo.URL)
			ee.Installed = v.URL() == repo.URL
		}
		if ee.Owner == "cli" || ee.Owner == "github" {
			ee.Official = true
		}

		extEntries = append(extEntries, ee)
	}

	return tea.NewProgram(newUIModel(l, extEntries)).Start()
}
