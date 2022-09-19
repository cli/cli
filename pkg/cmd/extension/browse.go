package extension

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var appStyle = lipgloss.NewStyle().Padding(1, 2)
var sidebarStyle = lipgloss.NewStyle()

type uiModel struct {
	sidebar sidebarModel
	extList extListModel
	// TODO move keymap here i guess? i don't know
}

func newUIModel() uiModel {
	return uiModel{
		extList: newExtListModel(),
		sidebar: newSidebarModel(),
	}
}

func (m uiModel) Init() tea.Cmd {
	// TODO the docs say not to do this but the example code in bubbles does:
	return tea.EnterAltScreen
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd
	var newModel tea.Model

	newModel, cmd = m.extList.Update(msg)
	cmds = append(cmds, cmd)
	m.extList = newModel.(extListModel)

	newModel, cmd = m.sidebar.Update(msg)
	cmds = append(cmds, cmd)
	m.sidebar = newModel.(sidebarModel)

	return m, tea.Batch(cmds...)
}

func (m uiModel) View() string {
	return lipgloss.JoinHorizontal(lipgloss.Top, m.extList.View(), m.sidebar.View())
}

type sidebarModel struct {
	content  string
	viewport viewport.Model
	ready    bool
}

func newSidebarModel() sidebarModel {
	// TODO
	return sidebarModel{}
}

func (m sidebarModel) Init() tea.Cmd {
	return nil
}

func (m sidebarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// TODO
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !m.ready {
			m.viewport = viewport.New(80, msg.Height)
			m.viewport.SetContent("LOL TODO")
			m.ready = true
		} else {
			m.viewport.Height = msg.Height
		}

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

func newItemDelegate(keys *delegateKeyMap) list.DefaultDelegate {
	// TODO unsure if i'll need this
	return list.NewDefaultDelegate()
}

type extListModel struct {
	list list.Model
	keys *keyMap
	// TODO keybindings
}

func newExtListModel() extListModel {
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
	list := list.New(items, newItemDelegate(nil), 0, 0)

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
		list: list,
		keys: keys,
	}
}

func (m extListModel) Init() tea.Cmd {
	return nil
}

func (m extListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// TODO probably fill this in in debugging why list not showing
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w, h := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-w, msg.Height-h)
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch {
		//case.keyMatches(msg, )
		}
	}

	nm, cmd := m.list.Update(msg)
	m.list = nm
	return m, cmd
}

func (m extListModel) View() string {
	return appStyle.Render(m.list.View())
}

// TODO

func extBrowse(cmd *cobra.Command) error {
	// TODO

	return tea.NewProgram(newUIModel()).Start()
}
