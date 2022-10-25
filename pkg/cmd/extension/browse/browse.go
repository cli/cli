package browse

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/markdown"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

// TODO add caching to search
// TODO make readme getter cache to disk
// TODO see if there is any way to make readme viewing prettier
// TODO see if it's possible to add padding to list and/or readme viewer

const pagingOffset = 25

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

func filterEntries(extEntries []extEntry, term string) []int {
	indices := []int{}
	for x, ee := range extEntries {
		if strings.Index(ee.Title()+ee.Description(), term) > -1 {
			indices = append(indices, x)
		}
	}
	return indices
}

// findCurrentEntry returns whatever extEntry is currently selected by the list
func findCurrentEntry(list *tview.List, extEntries []extEntry) (extEntry, error) {
	title, desc := list.GetItemText(list.GetCurrentItem())
	for _, e := range extEntries {
		if e.Title() == title && e.Description() == desc {
			return e, nil
		}
	}
	return extEntry{}, errors.New("not found")
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
func (e extEntry) FilterValue() string { return e.Title() }

type ibrowser interface {
	Browse(string) error
}

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

	// TODO pass a cache TTL
	opts.Rg = newReadmeGetter(opts.Client)

	app := tview.NewApplication()

	outerFlex := tview.NewFlex()
	innerFlex := tview.NewFlex()

	header := tview.NewTextView().SetText("gh extensions").SetTextAlign(tview.AlignCenter)
	filter := tview.NewInputField().SetLabel("filter: ")
	list := tview.NewList().SetWrapAround(false)
	readme := tview.NewTextView()
	help := tview.NewTextView().SetText("/: filter i: install r: remove w: open in browser pgup/pgdn: scroll readme q: quit")

	for _, ee := range extEntries {
		list.AddItem(ee.Title(), ee.Description(), rune(0), func() {})
	}

	onSelectItem := func(ix int, _, _ string, _ rune) {
		fullName := extEntries[ix].FullName
		rm, err := opts.Rg.Get(fullName)
		if err != nil {
			opts.Logger.Println(err.Error())
			readme.SetText("unable to fetch readme :(")
			return
		}

		// TODO sanity check what happens for non markdown readmes

		rendered, err := markdown.Render(rm)
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

	filter.SetChangedFunc(func(text string) {
		indices := filterEntries(extEntries, text)
		list.Clear()
		for ex, ee := range extEntries {
			for _, fx := range indices {
				if fx == ex {
					list.AddItem(ee.Title(), ee.Description(), rune(0), func() {})
				}
			}
		}
	})
	filter.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			app.SetFocus(list)
		case tcell.KeyEscape:
			filter.SetText("")
			list.Clear()
			for _, ee := range extEntries {
				list.AddItem(ee.Title(), ee.Description(), rune(0), func() {})
			}
			app.SetFocus(list)
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

	// TODO make functions for this stuff (scrolling stuff, install/remove, etc)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			app.Stop()
		case 'k':
			return tcell.NewEventKey(tcell.KeyUp, rune(0), 0)
		case 'j':
			return tcell.NewEventKey(tcell.KeyDown, rune(0), 0)
		case 'w':
			ee, err := findCurrentEntry(list, extEntries)
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
			// TODO
			// TODO get selected extEntry
			// TODO remove
			// TODO set text on modal regarding success/failure
			opts.Logger.Println("REMOVE REQUESTED")
		case ' ':
			// TODO the Modifiers check isn't working. add some logging.
			if event.Modifiers()&tcell.ModShift != 0 {
				i := list.GetCurrentItem() - pagingOffset
				if i < 0 {
					i = 0
				}
				list.SetCurrentItem(i)
			} else {
				list.SetCurrentItem(list.GetCurrentItem() + pagingOffset)
			}
			return nil
		case '/':
			app.SetFocus(filter)
			return nil
		}
		switch event.Key() {
		case tcell.KeyCtrlJ:
			list.SetCurrentItem(list.GetCurrentItem() + pagingOffset)
			return nil
		case tcell.KeyCtrlK:
			i := list.GetCurrentItem() - pagingOffset
			if i < 0 {
				i = 0
			}
			list.SetCurrentItem(i)
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
