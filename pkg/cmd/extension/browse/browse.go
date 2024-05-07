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

	"github.com/MakeNowJust/heredoc"
	"github.com/charmbracelet/glamour"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

const pagingOffset = 24

type ExtBrowseOpts struct {
	Cmd          *cobra.Command
	Browser      ibrowser
	IO           *iostreams.IOStreams
	Searcher     search.Searcher
	Em           extensions.ExtensionManager
	Client       *http.Client
	Logger       *log.Logger
	Cfg          gh.Config
	Rg           *readmeGetter
	Debug        bool
	SingleColumn bool
}

type ibrowser interface {
	Browse(string) error
}

type uiRegistry struct {
	// references to some of the heavily cross-referenced tview primitives. Not
	// everything is in here because most things are just used once in one place
	// and don't need to be easy to look up like this.
	App       *tview.Application
	Outerflex *tview.Flex
	List      *tview.List
	Pages     *tview.Pages
	CmdFlex   *tview.Flex
}

type extEntry struct {
	URL         string
	Name        string
	FullName    string
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

func (e extEntry) Description() string {
	if e.description == "" {
		return "no description provided"
	}
	return e.description
}

type extList struct {
	ui              uiRegistry
	extEntries      []extEntry
	app             *tview.Application
	filter          string
	opts            ExtBrowseOpts
	QueueUpdateDraw func(func()) *tview.Application
	WaitGroup       wGroup
}

type wGroup interface {
	Add(int)
	Done()
	Wait()
}

type fakeGroup struct{}

func (w *fakeGroup) Add(int) {}
func (w *fakeGroup) Done()   {}
func (w *fakeGroup) Wait()   {}

func newExtList(opts ExtBrowseOpts, ui uiRegistry, extEntries []extEntry) *extList {
	ui.List.SetTitleColor(tcell.ColorWhite)
	ui.List.SetSelectedTextColor(tcell.ColorBlack)
	ui.List.SetSelectedBackgroundColor(tcell.ColorWhite)
	ui.List.SetWrapAround(false)
	ui.List.SetBorderPadding(1, 1, 1, 1)
	ui.List.SetSelectedFunc(func(ix int, _, _ string, _ rune) {
		ui.Pages.SwitchToPage("readme")
	})

	el := &extList{
		ui:              ui,
		extEntries:      extEntries,
		app:             ui.App,
		opts:            opts,
		QueueUpdateDraw: ui.App.QueueUpdateDraw,
		WaitGroup:       &fakeGroup{},
	}

	el.Reset()
	return el
}

func (el *extList) createModal() *tview.Modal {
	m := tview.NewModal()
	m.SetBackgroundColor(tcell.ColorPurple)
	m.SetDoneFunc(func(_ int, _ string) {
		el.ui.Pages.SwitchToPage("main")
		el.Refresh()
	})

	return m
}

func (el *extList) toggleSelected(verb string) {
	ee, ix := el.FindSelected()
	if ix < 0 {
		el.opts.Logger.Println("failed to find selected entry")
		return
	}
	modal := el.createModal()

	if (ee.Installed && verb == "install") || (!ee.Installed && verb == "remove") {
		return
	}

	var action func() error

	if !ee.Installed {
		modal.SetText(fmt.Sprintf("Installing %s...", ee.FullName))
		action = func() error {
			repo, err := ghrepo.FromFullName(ee.FullName)
			if err != nil {
				el.opts.Logger.Println(fmt.Errorf("failed to install '%s': %w", ee.FullName, err))
				return err
			}
			err = el.opts.Em.Install(repo, "")
			if err != nil {
				return fmt.Errorf("failed to install %s: %w", ee.FullName, err)
			}
			return nil
		}
	} else {
		modal.SetText(fmt.Sprintf("Removing %s...", ee.FullName))
		action = func() error {
			name := strings.TrimPrefix(ee.Name, "gh-")
			err := el.opts.Em.Remove(name)
			if err != nil {
				return fmt.Errorf("failed to remove %s: %w", ee.FullName, err)
			}
			return nil
		}
	}

	el.ui.CmdFlex.Clear()
	el.ui.CmdFlex.AddItem(modal, 0, 1, true)
	var err error
	wg := el.WaitGroup
	wg.Add(1)

	go func() {
		el.QueueUpdateDraw(func() {
			el.ui.Pages.SwitchToPage("command")
			wg.Add(1)
			wg.Done()
			go func() {
				el.QueueUpdateDraw(func() {
					err = action()
					if err != nil {
						modal.SetText(err.Error())
					} else {
						modalText := fmt.Sprintf("Installed %s!", ee.FullName)
						if verb == "remove" {
							modalText = fmt.Sprintf("Removed %s!", ee.FullName)
						}
						modal.SetText(modalText)
						modal.AddButtons([]string{"ok"})
						el.app.SetFocus(modal)
					}
					wg.Done()
				})
			}()
		})
	}()

	// TODO blocking the app's thread and deadlocking
	wg.Wait()
	if err == nil {
		el.toggleInstalled(ix)
	}
}

func (el *extList) InstallSelected() {
	el.toggleSelected("install")
}

func (el *extList) RemoveSelected() {
	el.toggleSelected("remove")
}

func (el *extList) toggleInstalled(ix int) {
	ee := el.extEntries[ix]
	ee.Installed = !ee.Installed
	el.extEntries[ix] = ee
}

func (el *extList) Focus() {
	el.app.SetFocus(el.ui.List)
}

func (el *extList) Refresh() {
	el.Reset()
	el.Filter(el.filter)
}

func (el *extList) Reset() {
	el.ui.List.Clear()
	for _, ee := range el.extEntries {
		el.ui.List.AddItem(ee.Title(), ee.Description(), rune(0), func() {})
	}
}

func (el *extList) PageDown() {
	el.ui.List.SetCurrentItem(el.ui.List.GetCurrentItem() + pagingOffset)
}

func (el *extList) PageUp() {
	i := el.ui.List.GetCurrentItem() - pagingOffset
	if i < 0 {
		i = 0
	}
	el.ui.List.SetCurrentItem(i)
}

func (el *extList) ScrollDown() {
	el.ui.List.SetCurrentItem(el.ui.List.GetCurrentItem() + 1)
}

func (el *extList) ScrollUp() {
	i := el.ui.List.GetCurrentItem() - 1
	if i < 0 {
		i = 0
	}
	el.ui.List.SetCurrentItem(i)
}

func (el *extList) FindSelected() (extEntry, int) {
	if el.ui.List.GetItemCount() == 0 {
		return extEntry{}, -1
	}
	title, desc := el.ui.List.GetItemText(el.ui.List.GetCurrentItem())
	for x, e := range el.extEntries {
		if e.Title() == title && e.Description() == desc {
			return e, x
		}
	}
	return extEntry{}, -1
}

func (el *extList) Filter(text string) {
	el.filter = text
	if text == "" {
		return
	}
	el.ui.List.Clear()
	for _, ee := range el.extEntries {
		if strings.Contains(ee.Title()+ee.Description(), text) {
			el.ui.List.AddItem(ee.Title(), ee.Description(), rune(0), func() {})
		}
	}
}

func getSelectedReadme(opts ExtBrowseOpts, readme *tview.TextView, el *extList) (string, error) {
	ee, ix := el.FindSelected()
	if ix < 0 {
		return "", errors.New("failed to find selected entry")
	}
	fullName := ee.FullName
	rm, err := opts.Rg.Get(fullName)
	if err != nil {
		return "", err
	}

	_, _, wrap, _ := readme.GetInnerRect()

	// using glamour directly because if I don't horrible things happen
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(wrap))
	if err != nil {
		return "", err
	}
	rendered, err := renderer.Render(rm)
	if err != nil {
		return "", err
	}

	return rendered, nil
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

	opts.Rg = newReadmeGetter(opts.Client, time.Hour*24)

	app := tview.NewApplication()

	outerFlex := tview.NewFlex()
	innerFlex := tview.NewFlex()

	header := tview.NewTextView().SetText(fmt.Sprintf("browsing %d gh extensions", len(extEntries)))
	header.SetTextAlign(tview.AlignCenter).SetTextColor(tcell.ColorWhite)

	filter := tview.NewInputField().SetLabel("filter: ")
	filter.SetFieldBackgroundColor(tcell.ColorGray)
	filter.SetBorderPadding(0, 0, 20, 20)

	list := tview.NewList()

	readme := tview.NewTextView()
	readme.SetBorderPadding(1, 1, 0, 1)
	readme.SetBorder(true).SetBorderColor(tcell.ColorPurple)

	help := tview.NewTextView()
	help.SetDynamicColors(true)
	help.SetText("[::b]?[-:-:-]: help [::b]j/k[-:-:-]: move [::b]i[-:-:-]: install [::b]r[-:-:-]: remove [::b]w[-:-:-]: web [::b]↵[-:-:-]: view readme [::b]q[-:-:-]: quit")

	cmdFlex := tview.NewFlex()

	pages := tview.NewPages()

	ui := uiRegistry{
		App:       app,
		Outerflex: outerFlex,
		List:      list,
		Pages:     pages,
		CmdFlex:   cmdFlex,
	}

	extList := newExtList(opts, ui, extEntries)

	loadSelectedReadme := func() {
		rendered, err := getSelectedReadme(opts, readme, extList)
		if err != nil {
			opts.Logger.Println(err.Error())
			readme.SetText("unable to fetch readme :(")
			return
		}

		app.QueueUpdateDraw(func() {
			readme.SetText("")
			readme.SetDynamicColors(true)

			w := tview.ANSIWriter(readme)
			_, _ = w.Write([]byte(rendered))

			readme.ScrollToBeginning()
		})
	}

	filter.SetChangedFunc(func(text string) {
		extList.Filter(text)
		go loadSelectedReadme()
	})

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
	if !opts.SingleColumn {
		innerFlex.AddItem(readme, 0, 1, false)
	}

	outerFlex.SetDirection(tview.FlexRow)
	outerFlex.AddItem(header, 1, -1, false)
	outerFlex.AddItem(filter, 1, -1, false)
	outerFlex.AddItem(innerFlex, 0, 1, true)
	outerFlex.AddItem(help, 1, -1, false)

	helpBig := tview.NewTextView()
	helpBig.SetDynamicColors(true)
	helpBig.SetBorderPadding(0, 0, 2, 0)
	helpBig.SetText(heredoc.Doc(`
		[::b]Application[-:-:-]

		?: toggle help
		q: quit

		[::b]Navigation[-:-:-]

		↓, j: scroll list of extensions down by 1
		↑, k: scroll list of extensions up by 1

		shift+j, space:                                   scroll list of extensions down by 25
		shift+k, ctrl+space (mac), shift+space (windows): scroll list of extensions up by 25

		[::b]Extension Management[-:-:-]

		i: install highlighted extension
		r: remove highlighted extension
		w: open highlighted extension in web browser

		[::b]Filtering[-:-:-]

		/:      focus filter
		enter:  finish filtering and go back to list
		escape: clear filter and reset list

		[::b]Readmes[-:-:-]

		enter: open highlighted extension's readme full screen
		page down: scroll readme pane down
		page up:   scroll readme pane up

		(On a mac, page down and page up are fn+down arrow and fn+up arrow)
	`))

	pages.AddPage("main", outerFlex, true, true)
	pages.AddPage("help", helpBig, true, false)
	pages.AddPage("readme", readme, true, false)
	pages.AddPage("command", cmdFlex, true, false)

	app.SetRoot(pages, true)

	// Force fetching of initial readme by loading it just prior to the first
	// draw. The callback is removed immediately after draw.
	app.SetBeforeDrawFunc(func(_ tcell.Screen) bool {
		go loadSelectedReadme()
		return false // returning true would halt drawing which we do not want
	})

	app.SetAfterDrawFunc(func(_ tcell.Screen) {
		app.SetBeforeDrawFunc(nil)
		app.SetAfterDrawFunc(nil)
	})

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if filter.HasFocus() {
			return event
		}

		curPage, _ := pages.GetFrontPage()

		if curPage != "main" {
			if curPage == "command" {
				return event
			}
			if event.Rune() == 'q' || event.Key() == tcell.KeyEscape {
				pages.SwitchToPage("main")
				return nil
			}
			switch curPage {
			case "readme":
				switch event.Key() {
				case tcell.KeyPgUp:
					row, col := readme.GetScrollOffset()
					if row > 0 {
						readme.ScrollTo(row-2, col)
					}
				case tcell.KeyPgDn:
					row, col := readme.GetScrollOffset()
					readme.ScrollTo(row+2, col)
				}
			case "help":
				switch event.Rune() {
				case '?':
					pages.SwitchToPage("main")
				}
			}
			return nil
		}

		switch event.Rune() {
		case '?':
			pages.SwitchToPage("help")
			return nil
		case 'q':
			app.Stop()
		case 'k':
			extList.ScrollUp()
			readme.SetText("...fetching readme...")
			go loadSelectedReadme()
		case 'j':
			extList.ScrollDown()
			readme.SetText("...fetching readme...")
			go loadSelectedReadme()
		case 'w':
			ee, ix := extList.FindSelected()
			if ix < 0 {
				opts.Logger.Println("failed to find selected entry")
				return nil
			}
			err = opts.Browser.Browse(ee.URL)
			if err != nil {
				opts.Logger.Println(fmt.Errorf("could not open browser for '%s': %w", ee.URL, err))
			}
		case 'i':
			extList.InstallSelected()
		case 'r':
			extList.RemoveSelected()
		case ' ':
			// The shift check works on windows and not linux/mac:
			if event.Modifiers()&tcell.ModShift != 0 {
				extList.PageUp()
			} else {
				extList.PageDown()
			}
			go loadSelectedReadme()
		case '/':
			app.SetFocus(filter)
			return nil
		}
		switch event.Key() {
		case tcell.KeyUp:
			extList.ScrollUp()
			go loadSelectedReadme()
			return nil
		case tcell.KeyDown:
			extList.ScrollDown()
			go loadSelectedReadme()
			return nil
		case tcell.KeyEscape:
			filter.SetText("")
			extList.Reset()
		case tcell.KeyCtrlSpace:
			// The ctrl check works on linux/mac and not windows:
			extList.PageUp()
			go loadSelectedReadme()
		case tcell.KeyCtrlJ:
			extList.PageDown()
			go loadSelectedReadme()
		case tcell.KeyCtrlK:
			extList.PageUp()
			go loadSelectedReadme()
		}

		return event
	})

	if err := app.Run(); err != nil {
		return err
	}

	return nil
}
