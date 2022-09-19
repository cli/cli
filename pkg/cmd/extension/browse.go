package extension

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

type extEntry struct {
	Owner     string
	Name      string
	Readme    string
	Stars     int
	Installed bool
	Official  bool
}

func (e extEntry) FilterValue() string { return fmt.Sprintf("%s/%s", e.Owner, e.Name) }

// TODO what is this
type delegateKeyMap struct{}

func newItemDelegate(keys *delegateKeyMap) list.DefaultDelegate {
	// TODO
	return list.NewDefaultDelegate()
}

type model struct {
	list list.Model
	// TODO keybindings
}

func newModel() model {
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

	return model{
		list: list,
	}
}

func (m model) Init() tea.Cmd {
	// TODO the docs say not to do this but the example code in bubbles does:
	return tea.EnterAltScreen
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// TODO probably fill this in in debugging why list not showing
	return m, nil
}

func (m model) View() string {
	return m.list.View()
}

// TODO

func extBrowse(cmd *cobra.Command) error {
	// TODO

	return tea.NewProgram(newModel()).Start()
}
