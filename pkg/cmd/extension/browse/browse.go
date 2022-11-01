package browse

import (
	"errors"
	"fmt"
	"io"
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
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

const pagingOffset = 24

// TODO saw STDOUT during an install
// TODO update titles after install/remove
// TODO description color is low-contrast in most settings

type ExtBrowseOpts struct {
	Cmd      *cobra.Command
	Browser  ibrowser
	IO       *iostreams.IOStreams
	Searcher search.Searcher
	Em       extensions.ExtensionManager
	Client   *http.Client
	Logger   *log.Logger
	Cfg      config.Config
	Rg       readmeGetter
	Debug    bool
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
		installed = " [green](installed)"
	}

	if e.Official {
		official = " [yellow](official)"
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
	logger     *log.Logger
}

func newExtList(app *tview.Application, list *tview.List, extEntries []extEntry, logger *log.Logger) extensionListUI {
	list.SetWrapAround(false)
	list.SetBorderPadding(1, 1, 1, 1)
	el := &extList{
		list:       list,
		extEntries: extEntries,
		app:        app,
		logger:     logger,
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
		if strings.Contains(ee.Title()+ee.Description(), text) {
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
	if !opts.IO.CanPrompt() {
		return errors.New("command requires interactive terminal")
	}

	if opts.Debug {
		f, err := os.CreateTemp("/tmp", "extBrowse-*.txt")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())

		opts.Logger = log.New(f, "", log.Lshortfile)
	} else {
		opts.Logger = log.New(io.Discard, "", 0)
	}

	installed := opts.Em.List()

	opts.IO.StartProgressIndicator()
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

	header := tview.NewTextView().SetText(fmt.Sprintf("browsing %d gh extensions", len(extEntries)))
	header.SetTextAlign(tview.AlignCenter).SetTextColor(tcell.ColorWhite)

	filter := tview.NewInputField().SetLabel("filter: ")
	filter.SetLabelColor(tcell.ColorGray).SetFieldBackgroundColor(tcell.ColorGray)
	filter.SetBorderPadding(0, 0, 20, 20)

	list := tview.NewList()
	list.SetSelectedTextColor(tcell.ColorWhite)
	list.SetSelectedBackgroundColor(tcell.ColorPurple)
	list.SetSecondaryTextColor(tcell.ColorGray)

	readme := tview.NewTextView()
	readme.SetBorderPadding(1, 1, 0, 1)
	readme.SetBorder(true).SetBorderColor(tcell.ColorPurple)

	help := tview.NewTextView()
	help.SetText(
		"/: filter i/r: install/remove w: open in browser pgup/pgdn: scroll readme q: quit")
	help.SetTextColor(tcell.ColorGray).SetTextAlign(tview.AlignCenter)

	extList := newExtList(app, list, extEntries, opts.Logger)

	onSelectItem := func(ix int, _, _ string, _ rune) {
		readme.SetText("...fetching readme...")
		app.ForceDraw()
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

		// using glamour directly because if I don't horrible things happen
		rendered, err := glamour.Render(rm, "dark")
		if err != nil {
			opts.Logger.Println(err.Error())
			readme.SetText("unable to render readme :(")
			return
		}

		readme.SetText("")
		readme.SetDynamicColors(true)

		w := tview.ANSIWriter(readme)
		_, _ = w.Write([]byte(rendered))

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

	modal.SetBackgroundColor(tcell.ColorPurple)

	app.SetRoot(outerFlex, true)

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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
				return nil
			}
			err = opts.Browser.Browse(ee.URL)
			if err != nil {
				opts.Logger.Println(fmt.Errorf("could not open browser for '%s': %w", ee.URL, err))
			}
		case 'i':
			ee, err := extList.FindSelected()
			if err != nil {
				opts.Logger.Println(fmt.Errorf("failed to find selected ext: %w", err))
				return nil
			}
			repo, err := ghrepo.FromFullName(ee.FullName)
			if err != nil {
				opts.Logger.Println(fmt.Errorf("failed to install '%s't: %w", ee.FullName, err))
				return nil
			}

			app.SetRoot(modal, true)
			modal.SetText(fmt.Sprintf("Installing %s...", ee.FullName))
			app.ForceDraw()
			err = opts.Em.Install(repo, "")
			if err != nil {
				modal.SetText(fmt.Sprintf("Failed to install %s: %s", ee.FullName, err.Error()))
			} else {
				modal.SetText(fmt.Sprintf("Installed %s!", ee.FullName))
			}
		case 'r':
			ee, err := extList.FindSelected()
			if err != nil {
				opts.Logger.Println(fmt.Errorf("failed to find selected ext: %w", err))
				return nil
			}

			err = opts.Em.Remove(strings.TrimPrefix(ee.Name, "gh-"))
			if err != nil {
				modal.SetText(fmt.Sprintf("Failed to remove %s: %s", ee.FullName, err.Error()))
			} else {
				modal.SetText(fmt.Sprintf("Removed %s.", ee.FullName))
			}
			app.SetRoot(modal, true)
		case ' ':
			extList.PageDown()
			// this is a silly hack to trigger the onSelectItem which sadly was not happening
			return tcell.NewEventKey(tcell.KeyDown, rune(0), 0)
		case '/':
			app.SetFocus(filter)
			return nil
		}
		switch event.Key() {
		case tcell.KeyEscape:
			filter.SetText("")
			extList.Reset()
		case tcell.KeyCtrlSpace:
			extList.PageUp()
			// this is a silly hack to trigger the onSelectItem which sadly was not happening
			return tcell.NewEventKey(tcell.KeyUp, rune(0), 0)

		case tcell.KeyCtrlJ:
			extList.PageDown()
			// this is a silly hack to trigger the onSelectItem which sadly was not happening
			return tcell.NewEventKey(tcell.KeyDown, rune(0), 0)
		case tcell.KeyCtrlK:
			extList.PageUp()
			// this is a silly hack to trigger the onSelectItem which sadly was not happening
			return tcell.NewEventKey(tcell.KeyUp, rune(0), 0)
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

	opts.IO.StopProgressIndicator()
	if err := app.Run(); err != nil {
		return err
	}

	return nil
}
