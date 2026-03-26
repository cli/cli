package prompter

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// multiSelectSearchField is a custom huh Field that combines a text input
// for searching with a multi-select list. Unlike huh's built-in OptionsFunc,
// search results are loaded synchronously when the user presses Enter in
// the search input, avoiding goroutine races with selection state.
type multiSelectSearchField struct {
	// configuration
	title       string
	searchTitle string
	searchFunc  func(string) MultiSelectSearchResult

	// state
	mode    msMode // which sub-component has focus
	search  textinput.Model
	cursor  int
	loading bool
	spinner spinner.Model

	// options and selections
	options       []msOption
	selected      map[string]bool   // key → selected (source of truth)
	optionLabels  map[string]string // key → display label
	lastQuery     string
	defaultValues []string
	persistent    []string

	// field metadata
	key       string
	err       error
	focused   bool
	width     int
	height    int
	theme     huh.Theme
	hasDarkBg bool
	position  huh.FieldPosition
}

type msMode int

const (
	msModeSearch msMode = iota
	msModeSelect
)

type msOption struct {
	label string
	value string
}

// msSearchResultMsg carries search results back from the background goroutine.
type msSearchResultMsg struct {
	query  string
	result MultiSelectSearchResult
}

func newMultiSelectSearchField(
	title, searchTitle string,
	defaults, persistent []string,
	searchFunc func(string) MultiSelectSearchResult,
) *multiSelectSearchField {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "Type to search"
	ti.Focus()

	selected := make(map[string]bool)
	for _, k := range defaults {
		selected[k] = true
	}

	m := &multiSelectSearchField{
		title:         title,
		searchTitle:   searchTitle,
		searchFunc:    searchFunc,
		mode:          msModeSearch,
		search:        ti,
		selected:      selected,
		optionLabels:  make(map[string]string),
		defaultValues: defaults,
		persistent:    persistent,
		height:        10,
		spinner:       spinner.New(spinner.WithSpinner(spinner.Line)),
	}

	// Load initial results synchronously (form hasn't started yet).
	m.applySearchResult("", m.searchFunc(""))

	return m
}

// startSearch launches an async search and returns a tea.Cmd that will
// deliver the result via msSearchResultMsg.
func (m *multiSelectSearchField) startSearch(query string) tea.Cmd {
	m.loading = true
	searchFunc := m.searchFunc
	return tea.Batch(
		func() tea.Msg {
			return msSearchResultMsg{query: query, result: searchFunc(query)}
		},
		m.spinner.Tick,
	)
}

// applySearchResult processes a completed search and rebuilds the option list.
func (m *multiSelectSearchField) applySearchResult(query string, result MultiSelectSearchResult) {
	m.loading = false
	m.lastQuery = query
	if result.Err != nil {
		m.err = result.Err
		return
	}
	if len(result.Keys) != len(result.Labels) {
		m.err = fmt.Errorf("search returned mismatched keys and labels: %d keys, %d labels", len(result.Keys), len(result.Labels))
		return
	}

	for i, k := range result.Keys {
		m.optionLabels[k] = result.Labels[i]
	}

	// Build option list: selected items first, then results, then persistent.
	var options []msOption
	seen := make(map[string]bool)

	// 1. Currently selected items.
	for _, k := range m.selectedKeys() {
		if seen[k] {
			continue
		}
		seen[k] = true
		options = append(options, msOption{label: m.label(k), value: k})
	}

	// 2. Search results.
	for i, k := range result.Keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		l := result.Labels[i]
		if l == "" {
			l = k
		}
		options = append(options, msOption{label: l, value: k})
	}

	// 3. Persistent options.
	for _, k := range m.persistent {
		if seen[k] {
			continue
		}
		seen[k] = true
		options = append(options, msOption{label: m.label(k), value: k})
	}

	m.options = options
	m.cursor = 0
	m.err = nil
}

func (m *multiSelectSearchField) selectedKeys() []string {
	keys := make([]string, 0)
	// Maintain order: defaults first, then any added during this session.
	seen := make(map[string]bool)
	for _, k := range m.defaultValues {
		if m.selected[k] && !seen[k] {
			keys = append(keys, k)
			seen[k] = true
		}
	}
	for _, o := range m.options {
		if m.selected[o.value] && !seen[o.value] {
			keys = append(keys, o.value)
			seen[o.value] = true
		}
	}
	return keys
}

func (m *multiSelectSearchField) label(key string) string {
	if l, ok := m.optionLabels[key]; ok && l != "" {
		return l
	}
	return key
}

// --- huh.Field interface ---

func (m *multiSelectSearchField) Init() tea.Cmd {
	return nil
}

func (m *multiSelectSearchField) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.hasDarkBg = msg.IsDark()

	case msSearchResultMsg:
		m.applySearchResult(msg.query, msg.result)
		m.mode = msModeSelect
		m.search.Blur()
		return m, nil

	case spinner.TickMsg:
		if !m.loading {
			break
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		if m.loading {
			return m, nil // ignore keys while loading
		}
		switch m.mode {
		case msModeSearch:
			return m.updateSearch(msg)
		case msModeSelect:
			return m.updateSelect(msg)
		}
	}
	return m, nil
}

func (m *multiSelectSearchField) updateSearch(msg tea.KeyPressMsg) (huh.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter", "tab"))):
		query := m.search.Value()
		if query == m.lastQuery {
			// Query unchanged — just switch to select mode.
			m.mode = msModeSelect
			m.search.Blur()
			return m, nil
		}
		// New query — clear input and search in background with spinner.
		m.search.SetValue("")
		return m, m.startSearch(query)

	case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
		return m, huh.PrevField

	default:
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
}

func (m *multiSelectSearchField) updateSelect(msg tea.KeyPressMsg) (huh.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
		// Back to search mode.
		m.mode = msModeSearch
		m.search.Focus()
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		return m, huh.NextField

	case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
		if m.cursor < len(m.options)-1 {
			m.cursor++
		}
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("space", "x"))):
		if len(m.options) > 0 {
			k := m.options[m.cursor].value
			m.selected[k] = !m.selected[k]
			if !m.selected[k] {
				delete(m.selected, k)
			}
		}
		return m, nil
	}

	return m, nil
}

func (m *multiSelectSearchField) View() string {
	styles := m.activeStyles()
	var sb strings.Builder

	// Title.
	if m.title != "" {
		sb.WriteString(styles.Title.Render(m.title))
		sb.WriteString("\n")
	}

	// Search input.
	if m.searchTitle != "" {
		sb.WriteString(styles.Description.Render(m.searchTitle))
		sb.WriteString("\n")
	}
	sb.WriteString(m.search.View())
	sb.WriteString("\n")

	// Options list.
	if m.loading {
		m.spinner.Style = styles.MultiSelectSelector.UnsetString()
		sb.WriteString(m.spinner.View() + " Loading...")
		sb.WriteString("\n")
	} else if len(m.options) == 0 {
		sb.WriteString(styles.UnselectedOption.Render("  No results"))
		sb.WriteString("\n")
	} else {
		for i, o := range m.options {
			cursor := m.mode == msModeSelect && i == m.cursor
			isSelected := m.selected[o.value]
			sb.WriteString(m.renderOption(o, cursor, isSelected))
			sb.WriteString("\n")
		}
	}

	return styles.Base.Width(m.width).Height(m.height).Render(sb.String())
}

func (m *multiSelectSearchField) renderOption(o msOption, cursor, selected bool) string {
	styles := m.activeStyles()

	var parts []string
	if cursor {
		parts = append(parts, styles.MultiSelectSelector.String())
	} else {
		parts = append(parts, strings.Repeat(" ", lipgloss.Width(styles.MultiSelectSelector.String())))
	}
	if selected {
		parts = append(parts, styles.SelectedPrefix.String())
		parts = append(parts, styles.SelectedOption.Render(o.label))
	} else {
		parts = append(parts, styles.UnselectedPrefix.String())
		parts = append(parts, styles.UnselectedOption.Render(o.label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m *multiSelectSearchField) activeStyles() *huh.FieldStyles {
	theme := m.theme
	if theme == nil {
		theme = huh.ThemeFunc(huh.ThemeCharm)
	}
	if m.focused {
		return &theme.Theme(m.hasDarkBg).Focused
	}
	return &theme.Theme(m.hasDarkBg).Blurred
}

func (m *multiSelectSearchField) Focus() tea.Cmd {
	m.focused = true
	if m.mode == msModeSearch {
		return m.search.Focus()
	}
	return nil
}

func (m *multiSelectSearchField) Blur() tea.Cmd {
	m.focused = false
	m.search.Blur()
	return nil
}

func (m *multiSelectSearchField) Error() error   { return m.err }
func (*multiSelectSearchField) Skip() bool       { return false }
func (*multiSelectSearchField) Zoom() bool       { return false }
func (m *multiSelectSearchField) GetKey() string { return m.key }
func (m *multiSelectSearchField) GetValue() any  { return m.selectedKeys() }
func (m *multiSelectSearchField) Run() error     { return huh.Run(m) }
func (m *multiSelectSearchField) RunAccessible(w io.Writer, r io.Reader) error {
	_, _ = fmt.Fprintln(w, "MultiSelectWithSearch accessible mode not implemented")
	return nil
}

func (m *multiSelectSearchField) KeyBinds() []key.Binding {
	if m.mode == msModeSearch {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "search")),
			key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "back")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "toggle")),
		key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "search")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
	}
}

func (m *multiSelectSearchField) WithTheme(theme huh.Theme) huh.Field {
	if m.theme != nil {
		return m
	}
	m.theme = theme

	styles := theme.Theme(m.hasDarkBg)
	st := m.search.Styles()
	st.Cursor.Color = styles.Focused.TextInput.Cursor.GetForeground()
	st.Focused.Prompt = styles.Focused.TextInput.Prompt
	st.Focused.Text = styles.Focused.TextInput.Text
	st.Focused.Placeholder = styles.Focused.TextInput.Placeholder
	m.search.SetStyles(st)

	return m
}

func (m *multiSelectSearchField) WithKeyMap(k *huh.KeyMap) huh.Field {
	return m
}

func (m *multiSelectSearchField) WithWidth(width int) huh.Field {
	m.width = width
	m.search.SetWidth(width)
	return m
}

func (m *multiSelectSearchField) WithHeight(height int) huh.Field {
	m.height = height
	return m
}

func (m *multiSelectSearchField) WithPosition(p huh.FieldPosition) huh.Field {
	m.position = p
	return m
}
