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

func newUIModel(l *log.Logger) uiModel {
	return uiModel{
		extList: newExtListModel(l),
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
	Owner     string
	Name      string
	Readme    string
	Stars     int
	Installed bool
	Official  bool
}

func (e extEntry) Title() string       { return fmt.Sprintf("%s/%s", e.Owner, e.Name) }
func (e extEntry) Description() string { return fmt.Sprintf("%s/%s", e.Owner, e.Name) }
func (e extEntry) FilterValue() string { return fmt.Sprintf("%s/%s", e.Owner, e.Name) }

// TODO what is this
type delegateKeyMap struct{}

type keyMap struct {
	install key.Binding
	remove  key.Binding
	sort    key.Binding
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

func newExtListModel(l *log.Logger) extListModel {
	items := make([]list.Item, 5)
	items[0] = extEntry{
		Owner:     "cli",
		Name:      "user-status",
		Readme:    "It's good",
		Stars:     1000,
		Installed: true,
		Official:  true,
	}
	items[1] = extEntry{
		Owner:     "github",
		Name:      "something",
		Readme:    "It's pretty good",
		Stars:     10000,
		Installed: false,
		Official:  true,
	}
	items[2] = extEntry{
		Owner:     "vilmibm",
		Name:      "screenssaver",
		Readme:    "rainbow characters",
		Stars:     0,
		Installed: true,
		Official:  false,
	}
	items[3] = extEntry{
		Owner:     "mislav",
		Name:      "branch",
		Readme:    "trees are nice",
		Stars:     100,
		Installed: true,
		Official:  false,
	}
	items[4] = extEntry{
		Owner:     "samcoe",
		Name:      "triage",
		Readme:    "things are sometimes",
		Stars:     10,
		Installed: false,
		Official:  false,
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

func extBrowse(cmd *cobra.Command, searcher search.Searcher) error {
	// TODO support turning debug mode on/off
	f, err := os.CreateTemp("/tmp", "extBrowse-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	l := log.New(f, "", log.Lshortfile)

	// TODO spinner

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

	l.Printf("%#v", result)

	return tea.NewProgram(newUIModel(l)).Start()
}
