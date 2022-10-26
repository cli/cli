package browse

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

// TODO see if there is any way to make readme viewing prettier
// TODO see if it's possible to add padding to list and/or readme viewer
const pagingOffset = 25

type ExtBrowseOpts struct {
	Cmd      *cobra.Command
	Browser  ibrowser
	Searcher search.Searcher
	Em       extensions.ExtensionManager
	Client   *http.Client
	Logger   *log.Logger
	Cfg      config.Config
	Rg       readmeGetter
}

type ibrowser interface {
	Browse(string) error
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
	var installed string
	var official string

	if e.Installed {
		installed = " [installed]"
	}

	if e.Official {
		official = " [official]"
	}

	return fmt.Sprintf("%s%s%s", e.FullName, official, installed)
}

func (e extEntry) Description() string { return e.description }

type extensionListUI interface {
	FindSelected() (extEntry, error)
	Filter(text string)
	Focus()
	Reset()
	PageDown()
	PageUp()
}

type extList struct {
	list       *tview.List
	extEntries []extEntry
	app        *tview.Application
}

func newExtList(app *tview.Application, list *tview.List, extEntries []extEntry) extensionListUI {
	el := &extList{
		list:       list,
		extEntries: extEntries,
		app:        app,
	}
	el.Reset()
	return el
}

func (el *extList) Focus() {
	el.app.SetFocus(el.list)
}

func (el *extList) Reset() {
	el.list.Clear()
	for _, ee := range el.extEntries {
		el.list.AddItem(ee.Title(), ee.Description(), rune(0), func() {})
	}
}

func (el *extList) PageDown() {
	el.list.SetCurrentItem(el.list.GetCurrentItem() + pagingOffset)
}

func (el *extList) PageUp() {
	i := el.list.GetCurrentItem() - pagingOffset
	if i < 0 {
		i = 0
	}
	el.list.SetCurrentItem(i)
}

func (el *extList) FindSelected() (extEntry, error) {
	title, desc := el.list.GetItemText(el.list.GetCurrentItem())
	for _, e := range el.extEntries {
		if e.Title() == title && e.Description() == desc {
			return e, nil
		}
	}
	return extEntry{}, errors.New("not found")
}

func (el *extList) Filter(text string) {
	indices := []int{}
	for x, ee := range el.extEntries {
		if strings.Index(ee.Title()+ee.Description(), text) > -1 {
			indices = append(indices, x)
		}
	}
	el.list.Clear()
	for ex, ee := range el.extEntries {
		for _, fx := range indices {
			if fx == ex {
				el.list.AddItem(ee.Title(), ee.Description(), rune(0), func() {})
			}
		}
	}
}

func ExtBrowse(opts ExtBrowseOpts) error {
	// TODO support turning debug mode on/off
	f, err := os.CreateTemp("/tmp", "extBrowse-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	opts.Logger = log.New(f, "", log.Lshortfile)

	installed := opts.Em.List()

	result, err := opts.Searcher.Repositories(search.Query{
		Kind:  search.KindRepositories,
		Limit: 1000,
		Qualifiers: search.Qualifiers{
			Topic: []string{"gh-extension"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to search for extensions: %w", err)
	}

	host, _ := opts.Cfg.DefaultHost()

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

	cacheTTL, _ := time.ParseDuration("24h")
	opts.Rg = newReadmeGetter(opts.Client, cacheTTL)

	app := tview.NewApplication()

	outerFlex := tview.NewFlex()
	innerFlex := tview.NewFlex()

	header := tview.NewTextView().SetText("gh extensions").SetTextAlign(tview.AlignCenter)
	filter := tview.NewInputField().SetLabel("filter: ")
	list := tview.NewList().SetWrapAround(false)
	readme := tview.NewTextView()
	help := tview.NewTextView().SetText("/: filter i: install r: remove w: open in browser pgup/pgdn: scroll readme q: quit")

	extList := newExtList(app, list, extEntries)

	onSelectItem := func(ix int, _, _ string, _ rune) {
		ee, err := extList.FindSelected()
		if err != nil {
			opts.Logger.Println(fmt.Errorf("tried to find entry, but: %w", err))
			return
		}
		fullName := ee.FullName
		rm, err := opts.Rg.Get(fullName)
		if err != nil {
			opts.Logger.Println(err.Error())
			readme.SetText("unable to fetch readme :(")
			return
		}

		// TODO sanity check what happens for non markdown readmes

		// TODO be more careful about dark/light (sniff it earlier then set overall theming)
		rendered, err := glamour.Render(rm, "dark")
		if err != nil {
			opts.Logger.Println(err.Error())
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

	filter.SetChangedFunc(extList.Filter)

	filter.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			extList.Focus()
		case tcell.KeyEscape:
			filter.SetText("")
			extList.Reset()
			extList.Focus()
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

	// modal is used to output the result of install/remove operations
	modal := tview.NewModal().AddButtons([]string{"ok"}).SetDoneFunc(func(_ int, _ string) {
		app.SetRoot(outerFlex, true)
	})

	app.SetRoot(outerFlex, true)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		opts.Logger.Printf("%#v", event)
		if filter.HasFocus() {
			return event
		}

		switch event.Rune() {
		case 'q':
			app.Stop()
		case 'k':
			return tcell.NewEventKey(tcell.KeyUp, rune(0), 0)
		case 'j':
			return tcell.NewEventKey(tcell.KeyDown, rune(0), 0)
		case 'w':
			ee, err := extList.FindSelected()
			if err != nil {
				opts.Logger.Println(fmt.Errorf("tried to find entry, but: %w", err))
			}
			opts.Browser.Browse(ee.URL)
			return nil
		case 'i':
			// TODO get selected extEntry
			// TODO install
			// TODO set text on modal regarding success/failure
			app.SetRoot(modal, true)
			opts.Logger.Println("INSTALL REQUESTED")
		case 'r':
			// TODO get selected extEntry
			// TODO remove
			// TODO set text on modal regarding success/failure
			opts.Logger.Println("REMOVE REQUESTED")
		case ' ':
			extList.PageDown()
		case '/':
			app.SetFocus(filter)
			return nil
		}
		switch event.Key() {
		case tcell.KeyCtrlSpace:
			extList.PageUp()
		case tcell.KeyCtrlJ:
			extList.PageDown()
			return nil
		case tcell.KeyCtrlK:
			extList.PageUp()
			return nil
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

	if err := app.Run(); err != nil {
		return err
	}

	return nil
}
