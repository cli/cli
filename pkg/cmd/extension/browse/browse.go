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

type ExtBrowseOpts struct {
	Cmd      *cobra.Command
	Browser  ibrowser
	IO       *iostreams.IOStreams
	Searcher search.Searcher
	Em       extensions.ExtensionManager
	Client   *http.Client
	Logger   *log.Logger
	Cfg      config.Config
	Rg       *readmeGetter
	Debug    bool
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
	Readme    *tview.TextView
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
	ui         uiRegistry
	extEntries []extEntry
	app        *tview.Application
	filter     string
	opts       ExtBrowseOpts
}

func newExtList(opts ExtBrowseOpts, ui uiRegistry, extEntries []extEntry) *extList {
	ui.List.SetTitleColor(tcell.ColorWhite)
	ui.List.SetSelectedTextColor(tcell.ColorBlack)
	ui.List.SetSelectedBackgroundColor(tcell.ColorWhite)
	ui.List.SetWrapAround(false)
	ui.List.SetBorderPadding(1, 1, 1, 1)

	el := &extList{
		ui:         ui,
		extEntries: extEntries,
		app:        ui.App,
		opts:       opts,
	}

	el.Reset()
	return el
}

func (el *extList) createModal() *tview.Modal {
	m := tview.NewModal()
	m.SetBackgroundColor(tcell.ColorPurple)
	m.SetDoneFunc(func(_ int, _ string) {
		el.ui.App.SetRoot(el.ui.Outerflex, true)
		el.Refresh()
	})

	return m
}

func (el *extList) InstallSelected() {
	ee, ix := el.FindSelected()
	if ix < 0 {
		el.opts.Logger.Println("failed to find selected entry")
		return
	}
	repo, err := ghrepo.FromFullName(ee.FullName)
	if err != nil {
		el.opts.Logger.Println(fmt.Errorf("failed to install '%s't: %w", ee.FullName, err))
		return
	}

	modal := el.createModal()

	modal.SetText(fmt.Sprintf("Installing %s...", ee.FullName))
	el.ui.App.SetRoot(modal, true)
	// I could eliminate this with a goroutine but it seems to be working fine
	el.app.ForceDraw()
	err = el.opts.Em.Install(repo, "")
	if err != nil {
		modal.SetText(fmt.Sprintf("Failed to install %s: %s", ee.FullName, err.Error()))
	} else {
		modal.SetText(fmt.Sprintf("Installed %s!", ee.FullName))
		modal.AddButtons([]string{"ok"})
		el.ui.App.SetFocus(modal)
	}

	el.toggleInstalled(ix)
}

func (el *extList) RemoveSelected() {
	ee, ix := el.FindSelected()
	if ix < 0 {
		el.opts.Logger.Println("failed to find selected extension")
		return
	}

	modal := el.createModal()

	modal.SetText(fmt.Sprintf("Removing %s...", ee.FullName))
	el.ui.App.SetRoot(modal, true)
	// I could eliminate this with a goroutine but it seems to be working fine
	el.ui.App.ForceDraw()

	err := el.opts.Em.Remove(strings.TrimPrefix(ee.Name, "gh-"))
	if err != nil {
		modal.SetText(fmt.Sprintf("Failed to remove %s: %s", ee.FullName, err.Error()))
	} else {
		modal.SetText(fmt.Sprintf("Removed %s.", ee.FullName))
		modal.AddButtons([]string{"ok"})
		el.ui.App.SetFocus(modal)
	}
	el.toggleInstalled(ix)
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

	host, _ := opts.Cfg.DefaultHost()

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
	help.SetText(
		"/: filter  i/r: install/remove  w: open in browser  pgup/pgdn: scroll readme  q: quit")
	help.SetTextAlign(tview.AlignCenter)

	ui := uiRegistry{
		App:       app,
		Outerflex: outerFlex,
		List:      list,
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
	innerFlex.AddItem(readme, 0, 1, false)

	outerFlex.SetDirection(tview.FlexRow)
	outerFlex.AddItem(header, 1, -1, false)
	outerFlex.AddItem(filter, 1, -1, false)
	outerFlex.AddItem(innerFlex, 0, 1, true)
	outerFlex.AddItem(help, 1, -1, false)

	app.SetRoot(outerFlex, true)

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

		switch event.Rune() {
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
			// The ctrl check works on windows/mac and not windows:
			extList.PageUp()
			go loadSelectedReadme()
		case tcell.KeyCtrlJ:
			extList.PageDown()
			go loadSelectedReadme()
		case tcell.KeyCtrlK:
			extList.PageUp()
			go loadSelectedReadme()
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

	// Without this redirection, the git client inside of the extension manager
	// will dump git output to the terminal.
	opts.IO.ErrOut = io.Discard

	if err := app.Run(); err != nil {
		return err
	}

	return nil
}
