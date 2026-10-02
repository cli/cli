package browse

import (
	"fmt"
	"io"
	"log"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type browseModel struct {
	opts        ExtBrowseOpts
	entries     []extEntry
	filtered    []int
	cursor      int
	filterInput textinput.Model
	filtering   bool
	page        browsePage
	width       int
	height      int
	readme      viewport.Model
	readmeName  string
	readmeRaw   string
	readmeState readmeState
	status      string
	actionBusy  bool
}

type browsePage int

const (
	mainPage browsePage = iota
	helpPage
	readmePage
)

type readmeState int

const (
	readmeIdle readmeState = iota
	readmeLoading
	readmeReady
	readmeFailed
)

type readmeLoadedMsg struct {
	fullName string
	raw      string
	rendered string
	width    int
	err      error
}

type browserResultMsg struct {
	url string
	err error
}

type installResultMsg struct {
	fullName string
	err      error
}

type removeResultMsg struct {
	fullName string
	err      error
}

const helpText = `Application

?: toggle help
q: quit

Navigation

down, j: move down by 1
up, k: move up by 1
shift+j, space, ctrl+j: move down by one page
shift+k, shift+space, ctrl+space, ctrl+k: move up by one page

Extension Management

i: install highlighted extension
r: remove highlighted extension
w: open highlighted extension in web browser

Filtering

/: focus filter
enter: finish filtering and return to the list
escape: clear filter and reset list

Readmes

enter: open highlighted extension's readme full screen
page down: scroll readme down
page up: scroll readme up`

func newBrowseModel(opts ExtBrowseOpts, entries []extEntry) *browseModel {
	filterInput := textinput.New()
	filterInput.Prompt = "filter: "

	filtered := make([]int, len(entries))
	for i := range entries {
		filtered[i] = i
	}
	if opts.renderReadme == nil {
		opts.renderReadme = renderReadmeMarkdown
	}
	if opts.Logger == nil {
		opts.Logger = log.New(io.Discard, "", 0)
	}

	model := &browseModel{
		opts:        opts,
		entries:     entries,
		filtered:    filtered,
		filterInput: filterInput,
		readme:      viewport.New(),
		width:       120,
		height:      30,
	}
	model.resizeReadme()
	return model
}

func (m *browseModel) Init() tea.Cmd {
	return m.loadSelectedReadme()
}

func (m *browseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeReadme()
		if m.readmeRaw != "" {
			return m, m.renderLoadedReadme()
		}
		return m, nil
	case readmeLoadedMsg:
		selected, ok := m.selectedEntry()
		if !ok || selected.FullName != msg.fullName {
			return m, nil
		}
		m.readmeName = msg.fullName
		if msg.err != nil {
			m.opts.Logger.Printf("failed to load README for %s: %v", msg.fullName, msg.err)
			m.readmeState = readmeFailed
			m.readme.SetContent("unable to fetch readme :(")
			return m, nil
		}
		m.readmeRaw = msg.raw
		if msg.width != m.readmeWidth() {
			return m, m.renderLoadedReadme()
		}
		m.readmeState = readmeReady
		m.readme.SetContent(truncateLines(msg.rendered, m.readmeWidth()))
		m.readme.GotoTop()
		return m, nil
	case browserResultMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("could not open browser for '%s': %v", msg.url, msg.err)
			m.opts.Logger.Print(m.status)
			return m, nil
		}
		m.status = fmt.Sprintf("Opened %s in the browser", msg.url)
		return m, nil
	case installResultMsg:
		m.actionBusy = false
		if msg.err != nil {
			m.status = msg.err.Error()
			m.opts.Logger.Print(m.status)
			return m, nil
		}
		m.setInstalled(msg.fullName, true)
		m.status = fmt.Sprintf("Installed %s!", msg.fullName)
		return m, nil
	case removeResultMsg:
		m.actionBusy = false
		if msg.err != nil {
			m.status = msg.err.Error()
			m.opts.Logger.Print(m.status)
			return m, nil
		}
		m.setInstalled(msg.fullName, false)
		m.status = fmt.Sprintf("Removed %s!", msg.fullName)
		return m, nil
	}

	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if m.filtering {
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			return m, cmd
		}
		if m.page == readmePage {
			var cmd tea.Cmd
			m.readme, cmd = m.readme.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	keystroke := key.Keystroke()
	if keystroke == "ctrl+c" {
		return m, tea.Quit
	}
	if !m.actionBusy {
		m.status = ""
	}

	if m.page == readmePage {
		switch keystroke {
		case "q", "esc":
			m.page = mainPage
			m.resizeReadme()
			return m, m.renderLoadedReadme()
		}
		var cmd tea.Cmd
		m.readme, cmd = m.readme.Update(msg)
		return m, cmd
	}

	if m.page == helpPage {
		switch keystroke {
		case "?", "q", "esc":
			m.page = mainPage
		}
		return m, nil
	}

	if m.filtering {
		switch keystroke {
		case "enter":
			m.filtering = false
			m.filterInput.Blur()
			return m, nil
		case "esc":
			m.filterInput.SetValue("")
			m.filtering = false
			m.filterInput.Blur()
			m.applyFilter()
			return m, m.loadSelectedReadme()
		}

		previous := m.filterInput.Value()
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		if m.filterInput.Value() != previous {
			m.applyFilter()
			return m, m.loadSelectedReadme()
		}
		return m, cmd
	}

	switch keystroke {
	case "?":
		m.page = helpPage
	case "q":
		return m, tea.Quit
	case "j", "down":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			return m, m.loadSelectedReadme()
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			return m, m.loadSelectedReadme()
		}
	case "space", "ctrl+j", "shift+j", "J":
		return m, m.moveByPage(1)
	case "shift+space", "ctrl+space", "ctrl+k", "shift+k", "K":
		return m, m.moveByPage(-1)
	case "/":
		m.filtering = true
		return m, m.filterInput.Focus()
	case "enter":
		if _, ok := m.selectedEntry(); ok {
			m.page = readmePage
			m.resizeReadme()
			return m, m.renderLoadedReadme()
		}
	case "w":
		selected, ok := m.selectedEntry()
		if !ok || m.opts.Browser == nil {
			return m, nil
		}
		m.status = fmt.Sprintf("Opening %s in the browser...", selected.URL)
		url := selected.URL
		browser := m.opts.Browser
		return m, func() tea.Msg {
			return browserResultMsg{url: url, err: browser.Browse(url)}
		}
	case "i":
		selected, ok := m.selectedEntry()
		if !ok || selected.Installed || m.opts.Em == nil || m.actionBusy {
			return m, nil
		}
		m.actionBusy = true
		m.status = fmt.Sprintf("Installing %s...", selected.FullName)
		fullName := selected.FullName
		manager := m.opts.Em
		return m, func() tea.Msg {
			repo, err := ghrepo.FromFullName(fullName)
			if err == nil {
				err = manager.Install(repo, "")
			}
			if err != nil {
				err = fmt.Errorf("failed to install %s: %w", fullName, err)
			}
			return installResultMsg{fullName: fullName, err: err}
		}
	case "r":
		selected, ok := m.selectedEntry()
		if !ok || !selected.Installed || m.opts.Em == nil || m.actionBusy {
			return m, nil
		}
		m.actionBusy = true
		m.status = fmt.Sprintf("Removing %s...", selected.FullName)
		fullName := selected.FullName
		name := strings.TrimPrefix(selected.Name, "gh-")
		manager := m.opts.Em
		return m, func() tea.Msg {
			err := manager.Remove(name)
			if err != nil {
				err = fmt.Errorf("failed to remove %s: %w", fullName, err)
			}
			return removeResultMsg{fullName: fullName, err: err}
		}
	case "esc":
		m.filterInput.SetValue("")
		m.applyFilter()
		return m, m.loadSelectedReadme()
	}

	return m, nil
}

func (m *browseModel) View() tea.View {
	if m.page == helpPage {
		view := tea.NewView(helpText)
		view.AltScreen = true
		return view
	}

	if m.page == readmePage {
		view := tea.NewView(m.readme.View())
		view.AltScreen = true
		return view
	}

	var content strings.Builder
	fmt.Fprintf(&content, "browsing %d gh extensions\n", len(m.entries))
	content.WriteString(m.filterInput.View())
	content.WriteByte('\n')
	content.WriteString(m.mainContent())
	content.WriteByte('\n')
	content.WriteString(m.footerView())

	view := tea.NewView(content.String())
	view.AltScreen = true
	return view
}

func (m *browseModel) selectedEntry() (extEntry, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return extEntry{}, false
	}
	return m.entries[m.filtered[m.cursor]], true
}

func (m *browseModel) applyFilter() {
	filter := m.filterInput.Value()
	m.filtered = m.filtered[:0]
	for i, entry := range m.entries {
		if filter == "" || strings.Contains(entry.FullName+entry.Description(), filter) {
			m.filtered = append(m.filtered, i)
		}
	}
	m.cursor = 0
}

func (m *browseModel) setInstalled(fullName string, installed bool) {
	for i := range m.entries {
		if m.entries[i].FullName == fullName {
			m.entries[i].Installed = installed
			return
		}
	}
}

func (m *browseModel) mainContent() string {
	list := m.listView()
	if m.singleColumn() {
		return list
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, list, " │ ", m.readme.View())
}

func (m *browseModel) footerView() string {
	var footer string
	switch {
	case m.status != "":
		footer = m.status
	case m.readmeState == readmeLoading:
		footer = "fetching readme..."
	default:
		const (
			full    = "? help  j/k move  i install  r remove  w web  enter view readme  q quit"
			compact = "? help  j/k move  enter readme  q quit"
			minimal = "? help  q quit"
		)
		switch {
		case m.width <= 0 || lipgloss.Width(full) <= m.width:
			footer = full
		case lipgloss.Width(compact) <= m.width:
			footer = compact
		default:
			footer = minimal
		}
	}
	if m.width <= 0 {
		return footer
	}
	return ansi.Truncate(footer, m.width, "")
}

func (m *browseModel) listView() string {
	if len(m.filtered) == 0 {
		return "no matching extensions"
	}

	var content strings.Builder
	pageSize := m.pageSize()
	start := min((m.cursor/pageSize)*pageSize, max(len(m.filtered)-pageSize, 0))
	end := min(start+pageSize, len(m.filtered))
	for i := start; i < end; i++ {
		entry := m.entries[m.filtered[i]]
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		title := prefix + entry.FullName
		if entry.Official {
			title += " (official)"
		}
		if entry.Installed {
			title += " (installed)"
		}
		content.WriteString(renderListLine(title, m.listWidth()))
		content.WriteByte('\n')
		content.WriteString(renderListLine("  "+entry.Description(), m.listWidth()))
		content.WriteByte('\n')
	}

	return content.String()
}

func (m *browseModel) singleColumn() bool {
	return m.opts.SingleColumn || (m.width > 0 && m.width < 100)
}

func (m *browseModel) listWidth() int {
	if m.singleColumn() || m.width <= 0 {
		return m.width
	}
	return max((m.width-3)/2, 1)
}

func (m *browseModel) readmeWidth() int {
	if m.page == readmePage {
		return max(m.width, 20)
	}
	if m.singleColumn() {
		return max(m.width-2, 20)
	}
	return max(m.width-m.listWidth()-3, 20)
}

func (m *browseModel) pageSize() int {
	if m.height <= 5 {
		return 1
	}
	return max((m.height-4)/2, 1)
}

func (m *browseModel) moveByPage(direction int) tea.Cmd {
	if len(m.filtered) == 0 {
		return nil
	}
	m.cursor = min(max(m.cursor+direction*m.pageSize(), 0), len(m.filtered)-1)
	return m.loadSelectedReadme()
}

func (m *browseModel) resizeReadme() {
	m.readme.SetWidth(m.readmeWidth())
	if m.page == readmePage {
		m.readme.SetHeight(max(m.height, 1))
		return
	}
	m.readme.SetHeight(max(m.height-4, 1))
}

func (m *browseModel) loadSelectedReadme() tea.Cmd {
	selected, ok := m.selectedEntry()
	if !ok {
		m.readmeName = ""
		m.readmeRaw = ""
		m.readmeState = readmeIdle
		m.readme.SetContent("")
		return nil
	}
	if m.opts.Rg == nil {
		return nil
	}
	m.readmeName = selected.FullName
	m.readmeRaw = ""
	m.readmeState = readmeLoading
	m.readme.SetContent("fetching readme...")

	fullName := selected.FullName
	loader := m.opts.Rg
	renderer := m.opts.renderReadme
	width := m.readmeWidth()
	return func() tea.Msg {
		raw, err := loader.Get(fullName)
		if err != nil {
			return readmeLoadedMsg{fullName: fullName, err: err}
		}
		rendered, err := renderer(raw, width)
		return readmeLoadedMsg{fullName: fullName, raw: raw, rendered: rendered, width: width, err: err}
	}
}

func (m *browseModel) renderLoadedReadme() tea.Cmd {
	if m.readmeRaw == "" || m.readmeName == "" {
		return nil
	}
	fullName := m.readmeName
	raw := m.readmeRaw
	renderer := m.opts.renderReadme
	width := m.readmeWidth()
	return func() tea.Msg {
		rendered, err := renderer(raw, width)
		return readmeLoadedMsg{fullName: fullName, raw: raw, rendered: rendered, width: width, err: err}
	}
}

func renderReadmeMarkdown(markdown string, width int) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(max(width-2, 1)),
	)
	if err != nil {
		return "", err
	}
	return renderer.Render(markdown)
}

func renderListLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	line = ansi.Truncate(line, width, "")
	return lipgloss.NewStyle().Width(width).Render(line)
}

func truncateLines(content string, width int) string {
	if width <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], width, "")
	}
	return strings.Join(lines, "\n")
}
