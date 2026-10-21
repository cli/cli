package browse

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowseModelMovesSelectionDown(t *testing.T) {
	t.Parallel()

	// Given a catalog with two extensions and the first extension selected
	model := newBrowseModel(ExtBrowseOpts{}, []extEntry{
		{FullName: "cli/gh-cool", description: "terminal tools"},
		{FullName: "octo/gh-triage", description: "issue management"},
	})

	// When the user presses j
	updated, _ := model.Update(keyPress('j'))

	// Then the next extension is selected
	got, ok := updated.(*browseModel)
	require.True(t, ok, "expected the updated model to remain a browse model")
	selected, ok := got.selectedEntry()
	require.True(t, ok, "expected an extension to remain selected")
	assert.Equal(t, "octo/gh-triage", selected.FullName)
}

func TestBrowseModelMovesSelectionDownOnePage(t *testing.T) {
	t.Parallel()

	// Given more extensions than fit in the visible list
	entries := make([]extEntry, 10)
	for i := range entries {
		entries[i].FullName = "octo/gh-extension-" + string(rune('a'+i))
	}
	model := newBrowseModel(ExtBrowseOpts{}, entries)
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 10})

	// When the user presses Space
	updateBrowseModel(t, model, specialKey(tea.KeySpace, 0))

	// Then the selection advances by one visible page
	selected, ok := model.selectedEntry()
	require.True(t, ok, "expected an extension to remain selected")
	assert.Equal(t, "octo/gh-extension-d", selected.FullName)
}

func TestBrowseModelQuitsOnControlCWhileFiltering(t *testing.T) {
	t.Parallel()

	// Given the filter input has focus
	model := newBrowseModel(ExtBrowseOpts{}, []extEntry{{FullName: "cli/gh-cool"}})
	updateBrowseModel(t, model, keyPress('/'))

	// When the user presses Ctrl+C
	cmd := updateBrowseModel(t, model, specialKey('c', tea.ModCtrl))

	// Then the Bubble Tea program receives a quit message
	require.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "expected Ctrl+C to quit the extension browser")
}

func TestBrowseModelKeepsMainViewWithinTerminalHeight(t *testing.T) {
	t.Parallel()

	// Given the README fills its viewport with unbreakable lines as wide as the preview
	readmeLines := make([]string, 24)
	for i := range readmeLines {
		readmeLines[i] = strings.Repeat("界", 29)
	}
	model := newBrowseModel(ExtBrowseOpts{
		Rg: readmeLoaderStub{
			content: map[string]string{"cli/gh-cool": strings.Join(readmeLines, "\n")},
		},
		renderReadme: func(markdown string, width int) (string, error) {
			return markdown, nil
		},
	}, []extEntry{
		{
			FullName:    "cli/gh-cool",
			description: strings.Repeat("description ", 30),
		},
	})
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	cmd := model.Init()
	require.NotNil(t, cmd)
	updateBrowseModel(t, model, cmd())

	// When the main view is rendered
	view := model.View().Content

	// Then wrapping does not push content beyond the terminal height
	assert.LessOrEqual(t, strings.Count(view, "\n")+1, 30)
}

func TestBrowseModelUsesCompactFallbackInVeryShortTerminal(t *testing.T) {
	t.Parallel()

	// Given the terminal is too short for the catalog chrome and one extension
	model := newBrowseModel(ExtBrowseOpts{}, []extEntry{
		{FullName: "cli/gh-cool", description: "terminal tools"},
	})
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 60, Height: 5})

	// When the interface is rendered
	view := model.View().Content

	// Then a compact message fits within the available terminal
	assert.Equal(t, "terminal too small", strings.TrimSpace(view))
	assert.LessOrEqual(t, strings.Count(view, "\n")+1, 5)
	assert.LessOrEqual(t, lipgloss.Width(view), 60)
}

func TestBrowseModelKeepsFooterWithinTerminalWidth(t *testing.T) {
	t.Parallel()

	// Given the extension browser is displayed in a narrow terminal
	model := newBrowseModel(ExtBrowseOpts{}, []extEntry{{FullName: "cli/gh-cool"}})
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 60, Height: 20})

	// When the main view is rendered
	view := model.View().Content

	// Then every line fits within the terminal width
	for line := range strings.SplitSeq(view, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 60)
	}
}

func TestBrowseModelProvidesVisualHierarchyInWideLayout(t *testing.T) {
	t.Parallel()

	// Given color is enabled and an installed official extension is selected
	ios, _, _, _ := iostreams.Test()
	ios.SetColorEnabled(true)
	model := newBrowseModel(ExtBrowseOpts{IO: ios}, []extEntry{
		{
			FullName:    "octo/gh-triage",
			description: "issue management",
		},
		{
			FullName:    "cli/gh-cool",
			description: "terminal tools",
			Official:    true,
			Installed:   true,
		},
	})
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 20})

	// When the wide layout is rendered
	view := model.View().Content
	lines := strings.Split(view, "\n")
	plainHeader := ansi.Strip(lines[0])

	// Then the heading, selection, statuses, README, and key hints have distinct styling
	assert.Equal(t, "browsing 2 gh extensions", strings.TrimSpace(plainHeader))
	assert.InDelta(t, strings.Index(plainHeader, "browsing"), len(plainHeader)-strings.LastIndex(plainHeader, "extensions")-len("extensions"), 1)
	assert.Contains(t, view, "\x1b[7m")
	assert.Regexp(t, `\x1b\[[0-9;]*33m\(official\)\x1b\[0m`, view)
	assert.Regexp(t, `\x1b\[[0-9;]*32m\(installed\)\x1b\[0m`, view)
	assert.Contains(t, view, "┌")
	assert.Contains(t, view, "┐")
	assert.Regexp(t, `\x1b\[[0-9;]*1[0-9;]*m\?\x1b\[0m`, view)
}

func TestBrowseModelKeepsDescriptionsReadableWithColorEnabled(t *testing.T) {
	t.Parallel()

	// Given color is enabled for a catalog entry with a description
	ios, _, _, _ := iostreams.Test()
	ios.SetColorEnabled(true)
	model := newBrowseModel(ExtBrowseOpts{IO: ios}, []extEntry{
		{
			FullName:    "octo/gh-triage",
			description: "issue management",
		},
	})

	// When the extension list is rendered
	description := strings.Split(model.listView(), "\n")[1]

	// Then the description uses the terminal's default foreground instead of a fixed low-contrast color
	assert.Equal(t, "issue management", strings.TrimSpace(ansi.Strip(description)))
	assert.NotContains(t, description, "\x1b[")
}

func TestBrowseModelFiltersExtensions(t *testing.T) {
	t.Parallel()

	// Given a catalog with extensions that have different names and descriptions
	model := newBrowseModel(ExtBrowseOpts{}, []extEntry{
		{FullName: "cli/gh-cool", description: "terminal tools"},
		{FullName: "octo/gh-triage", description: "issue management"},
	})

	// When the user focuses the filter and enters cool
	updateBrowseModel(t, model, keyPress('/'))
	for _, r := range "cool" {
		updateBrowseModel(t, model, keyPress(r))
	}

	// Then only the matching extension is displayed
	view := model.View().Content
	assert.Contains(t, view, "cli/gh-cool")
	assert.NotContains(t, view, "octo/gh-triage")
}

func TestBrowseModelFiltersExtensionsByDisplayedStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filter    string
		want      string
		doNotWant string
	}{
		{
			name:      "official",
			filter:    "official",
			want:      "cli/gh-official",
			doNotWant: "octo/gh-installed",
		},
		{
			name:      "installed",
			filter:    "installed",
			want:      "octo/gh-installed",
			doNotWant: "cli/gh-official",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Given the catalog includes extensions with displayed status labels
			model := newBrowseModel(ExtBrowseOpts{}, []extEntry{
				{FullName: "cli/gh-official", Official: true},
				{FullName: "octo/gh-installed", Installed: true},
			})

			// When the user filters by a status label
			updateBrowseModel(t, model, keyPress('/'))
			for _, r := range tt.filter {
				updateBrowseModel(t, model, keyPress(r))
			}

			// Then only extensions displaying that status remain
			view := model.View().Content
			assert.Contains(t, view, tt.want)
			assert.NotContains(t, view, tt.doNotWant)
		})
	}
}

func TestBrowseModelDoesNotReloadReadmeWhenFilteringKeepsSelection(t *testing.T) {
	t.Parallel()

	// Given the selected README is loaded
	renderCount := 0
	model := newBrowseModel(ExtBrowseOpts{
		Rg: readmeLoaderStub{
			content: map[string]string{"cli/gh-cool": "# Cool README"},
		},
		renderReadme: func(markdown string, width int) (string, error) {
			renderCount++
			return markdown, nil
		},
	}, []extEntry{
		{FullName: "cli/gh-cool"},
		{FullName: "octo/gh-triage"},
	})
	initialLoad := model.Init()
	require.NotNil(t, initialLoad)
	updateBrowseModel(t, model, initialLoad())
	require.Equal(t, 1, renderCount)

	// When filtering leaves the same extension selected
	updateBrowseModel(t, model, keyPress('/'))
	cmd := updateBrowseModel(t, model, keyPress('c'))
	if cmd != nil {
		updateBrowseModel(t, model, cmd())
	}

	// Then the already displayed README is not fetched and rendered again
	assert.Equal(t, 1, renderCount)
}

func TestBrowseModelLoadsReadmeForFilteredSelection(t *testing.T) {
	t.Parallel()

	// Given the first extension README is displayed and another extension matches a filter
	model := newBrowseModel(ExtBrowseOpts{
		Rg: readmeLoaderStub{
			content: map[string]string{
				"cli/gh-cool":    "# Cool README",
				"octo/gh-triage": "# Triage README",
			},
		},
		renderReadme: func(markdown string, width int) (string, error) {
			return markdown, nil
		},
	}, []extEntry{
		{FullName: "cli/gh-cool", description: "terminal tools"},
		{FullName: "octo/gh-triage", description: "issue management"},
	})
	initialLoad := model.Init()
	require.NotNil(t, initialLoad)
	updateBrowseModel(t, model, initialLoad())

	// When the user filters the catalog by triage
	updateBrowseModel(t, model, keyPress('/'))
	for _, r := range "triage" {
		if cmd := updateBrowseModel(t, model, keyPress(r)); cmd != nil {
			updateBrowseModel(t, model, cmd())
		}
	}

	// Then the newly selected extension README replaces the previous preview
	view := model.View().Content
	assert.Contains(t, view, "# Triage README")
	assert.NotContains(t, view, "# Cool README")
}

func TestBrowseModelShowsKeyboardHelp(t *testing.T) {
	t.Parallel()

	// Given the main extension catalog is displayed
	ios, _, _, _ := iostreams.Test()
	ios.SetColorEnabled(true)
	model := newBrowseModel(ExtBrowseOpts{IO: ios}, []extEntry{
		{FullName: "cli/gh-cool", description: "terminal tools"},
	})

	// When the user presses ?
	updateBrowseModel(t, model, keyPress('?'))

	// Then the interface displays the available control groups
	view := model.View().Content
	assert.Contains(t, view, "Application")
	assert.Contains(t, view, "Navigation")
	assert.Contains(t, view, "Extension Management")
	assert.Contains(t, view, "Filtering")
	assert.Contains(t, view, "Readmes")
	assert.Regexp(t, `\x1b\[[0-9;]*1[0-9;]*mApplication\x1b\[0m`, view)
	assert.Regexp(t, `\x1b\[[0-9;]*1[0-9;]*m\?\x1b\[0m: toggle help`, view)
}

func TestBrowseModelShowsKeyboardHelpForShiftedQuestionMark(t *testing.T) {
	t.Parallel()

	// Given the terminal reports the printable question mark with shift metadata
	model := newBrowseModel(ExtBrowseOpts{}, []extEntry{{FullName: "cli/gh-cool"}})
	questionMark := tea.KeyPressMsg{Code: '/', Mod: tea.ModShift, Text: "?"}

	// When the user presses ?
	updateBrowseModel(t, model, questionMark)

	// Then the keyboard help opens
	assert.Contains(t, model.View().Content, "Extension Management")
}

func TestBrowseModelPreviewsSelectedReadme(t *testing.T) {
	t.Parallel()

	// Given a wide terminal and an extension with an available README
	model := newBrowseModel(ExtBrowseOpts{
		Rg: readmeLoaderStub{
			content: map[string]string{"cli/gh-cool": "# Cool README"},
		},
		renderReadme: func(markdown string, width int) (string, error) {
			return "rendered: " + markdown, nil
		},
	}, []extEntry{
		{FullName: "cli/gh-cool", description: "terminal tools"},
	})
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})

	// When the initial README load completes
	cmd := model.Init()
	require.NotNil(t, cmd)
	updateBrowseModel(t, model, cmd())

	// Then the rendered README is displayed beside the extension list
	view := model.View().Content
	assert.Contains(t, view, "cli/gh-cool")
	assert.Contains(t, view, "rendered: # Cool README")
}

func TestBrowseModelIgnoresStaleReadmeResult(t *testing.T) {
	t.Parallel()

	// Given README loads are pending for two different selections
	model := newBrowseModel(ExtBrowseOpts{
		Rg: readmeLoaderStub{
			content: map[string]string{
				"cli/gh-cool":    "# Cool README",
				"octo/gh-triage": "# Triage README",
			},
		},
		renderReadme: func(markdown string, width int) (string, error) {
			return markdown, nil
		},
	}, []extEntry{
		{FullName: "cli/gh-cool"},
		{FullName: "octo/gh-triage"},
	})
	firstLoad := model.Init()
	require.NotNil(t, firstLoad)
	secondLoad := updateBrowseModel(t, model, keyPress('j'))
	require.NotNil(t, secondLoad)

	// When the newer load completes before the older load
	updateBrowseModel(t, model, secondLoad())
	updateBrowseModel(t, model, firstLoad())

	// Then the preview still displays the currently selected extension README
	view := model.View().Content
	assert.Contains(t, view, "# Triage README")
	assert.NotContains(t, view, "# Cool README")
}

func TestBrowseModelRerendersReadmeLoadedDuringResize(t *testing.T) {
	t.Parallel()

	// Given the initial README load started before the terminal size was known
	model := newBrowseModel(ExtBrowseOpts{
		Rg: readmeLoaderStub{
			content: map[string]string{"cli/gh-cool": "# Cool README"},
		},
		renderReadme: func(markdown string, width int) (string, error) {
			return fmt.Sprintf("width:%d", width), nil
		},
	}, []extEntry{
		{FullName: "cli/gh-cool"},
	})
	initialLoad := model.Init()
	require.NotNil(t, initialLoad)
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 200, Height: 30})

	// When the initial load completes with content rendered for the old width
	rerender := updateBrowseModel(t, model, initialLoad())

	// Then the README is rerendered for the current preview width
	require.NotNil(t, rerender)
	updateBrowseModel(t, model, rerender())
	assert.Contains(t, model.View().Content, "width:98")
}

func TestBrowseModelRendersFullScreenReadmeWithinTerminalWidth(t *testing.T) {
	t.Parallel()

	// Given the selected README is loaded in the two-column layout
	model := newBrowseModel(ExtBrowseOpts{
		Rg: readmeLoaderStub{
			content: map[string]string{"cli/gh-cool": "# Cool README"},
		},
		renderReadme: func(markdown string, width int) (string, error) {
			return fmt.Sprintf("width:%d", width), nil
		},
	}, []extEntry{{FullName: "cli/gh-cool"}})
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	load := model.Init()
	require.NotNil(t, load)
	updateBrowseModel(t, model, load())

	// When the user opens the README full screen
	rerender := updateBrowseModel(t, model, specialKey(tea.KeyEnter, 0))

	// Then the README is rerendered within the full-screen border
	require.NotNil(t, rerender)
	updateBrowseModel(t, model, rerender())
	assert.Contains(t, model.View().Content, "width:118")
}

func TestBrowseModelRestoresPreviewWidthAfterClosingFullScreenReadme(t *testing.T) {
	t.Parallel()

	// Given a README is displayed full screen in a wide terminal
	model := newBrowseModel(ExtBrowseOpts{
		Rg: readmeLoaderStub{
			content: map[string]string{"cli/gh-cool": "# Cool README"},
		},
		renderReadme: func(markdown string, width int) (string, error) {
			return fmt.Sprintf("width:%d", width), nil
		},
	}, []extEntry{{FullName: "cli/gh-cool"}})
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})
	load := model.Init()
	require.NotNil(t, load)
	updateBrowseModel(t, model, load())
	fullScreenRender := updateBrowseModel(t, model, specialKey(tea.KeyEnter, 0))
	require.NotNil(t, fullScreenRender)
	updateBrowseModel(t, model, fullScreenRender())
	require.Contains(t, model.View().Content, "width:118")

	// When the user closes the full-screen README
	previewRender := updateBrowseModel(t, model, keyPress('q'))

	// Then the catalog returns with the README rendered at preview width
	require.NotNil(t, previewRender)
	updateBrowseModel(t, model, previewRender())
	view := model.View().Content
	assert.Contains(t, view, "cli/gh-cool")
	assert.Contains(t, view, "width:58")
}

func TestBrowseModelUsesSingleColumnInNarrowTerminal(t *testing.T) {
	t.Parallel()

	// Given a loaded README and a terminal too narrow for two useful columns
	model := newBrowseModel(ExtBrowseOpts{
		Rg: readmeLoaderStub{
			content: map[string]string{"cli/gh-cool": "# Cool README"},
		},
		renderReadme: func(markdown string, width int) (string, error) {
			return markdown, nil
		},
	}, []extEntry{
		{FullName: "cli/gh-cool"},
	})
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 80, Height: 24})

	// When the initial README load completes
	cmd := model.Init()
	require.NotNil(t, cmd)
	updateBrowseModel(t, model, cmd())

	// Then the main view displays the extension list without a side-by-side README
	view := model.View().Content
	assert.Contains(t, view, "cli/gh-cool")
	assert.NotContains(t, view, "# Cool README")
}

func TestBrowseModelOpensReadmeFromSingleColumn(t *testing.T) {
	t.Parallel()

	// Given the single-column catalog has loaded the selected extension README
	model := newBrowseModel(ExtBrowseOpts{
		SingleColumn: true,
		Rg: readmeLoaderStub{
			content: map[string]string{"cli/gh-cool": "# Cool README"},
		},
		renderReadme: func(markdown string, width int) (string, error) {
			return markdown, nil
		},
	}, []extEntry{
		{FullName: "cli/gh-cool"},
	})
	cmd := model.Init()
	require.NotNil(t, cmd)
	updateBrowseModel(t, model, cmd())

	// When the user presses Enter
	updateBrowseModel(t, model, specialKey(tea.KeyEnter, 0))

	// Then the selected README is displayed full screen
	assert.Contains(t, model.View().Content, "# Cool README")
}

func TestBrowseModelShowsReadmeFailure(t *testing.T) {
	t.Parallel()

	// Given the selected extension README is unavailable
	model := newBrowseModel(ExtBrowseOpts{
		Rg: readmeLoaderStub{
			errs: map[string]error{"cli/gh-cool": errors.New("not found")},
		},
	}, []extEntry{
		{FullName: "cli/gh-cool", description: "terminal tools"},
	})

	// When the initial README load fails
	cmd := model.Init()
	require.NotNil(t, cmd)
	updateBrowseModel(t, model, cmd())

	// Then the interface displays a visible README error
	assert.Contains(t, model.View().Content, "unable to fetch readme :(")
}

func TestBrowseModelShowsBrowserFailure(t *testing.T) {
	t.Parallel()

	// Given the selected extension repository cannot be opened in a browser
	browser := &browserStub{err: errors.New("browser unavailable")}
	model := newBrowseModel(ExtBrowseOpts{
		Browser: browser,
	}, []extEntry{
		{
			FullName: "cli/gh-cool",
			URL:      "https://github.com/cli/gh-cool",
		},
	})

	// When the user presses w and the browser command completes
	cmd := updateBrowseModel(t, model, keyPress('w'))
	require.NotNil(t, cmd)
	updateBrowseModel(t, model, cmd())

	// Then the attempted URL and visible failure identify the selected extension
	assert.Equal(t, "https://github.com/cli/gh-cool", browser.url)
	assert.Contains(t, model.View().Content, "could not open browser for 'https://github.com/cli/gh-cool'")
}

func TestBrowseModelClearsTransientStatusOnNextInteraction(t *testing.T) {
	t.Parallel()

	// Given a completed browser action is displayed in the status line
	model := newBrowseModel(ExtBrowseOpts{
		Browser: &browserStub{},
	}, []extEntry{
		{FullName: "cli/gh-cool", URL: "https://github.com/cli/gh-cool"},
		{FullName: "octo/gh-triage", URL: "https://github.com/octo/gh-triage"},
	})
	open := updateBrowseModel(t, model, keyPress('w'))
	require.NotNil(t, open)
	updateBrowseModel(t, model, open())
	assert.Contains(t, model.View().Content, "Opened https://github.com/cli/gh-cool")

	// When the user moves to another extension
	updateBrowseModel(t, model, keyPress('j'))

	// Then the normal keyboard help replaces the stale action status
	view := model.View().Content
	assert.NotContains(t, view, "Opened https://github.com/cli/gh-cool")
	assert.Contains(t, view, "? help")
}

func TestBrowseModelForwardsCursorMessagesWhileFiltering(t *testing.T) {
	t.Parallel()

	// Given focusing the filter schedules its initial cursor message
	model := newBrowseModel(ExtBrowseOpts{}, []extEntry{{FullName: "cli/gh-cool"}})
	focus := updateBrowseModel(t, model, keyPress('/'))
	require.NotNil(t, focus)

	// When the cursor message is delivered
	cursorCommand := updateBrowseModel(t, model, focus())

	// Then the text input schedules its cursor blink
	assert.NotNil(t, cursorCommand)
}

func TestBrowseModelInstallsSelectedExtension(t *testing.T) {
	t.Parallel()

	// Given an extension is selected and not installed
	manager := &extensions.ExtensionManagerMock{
		InstallFunc: func(repo ghrepo.Interface, _ string) error {
			assert.Equal(t, "cli/gh-cool", ghrepo.FullName(repo))
			return nil
		},
	}
	model := newBrowseModel(ExtBrowseOpts{
		Em: manager,
	}, []extEntry{
		{
			Name:     "gh-cool",
			FullName: "cli/gh-cool",
		},
	})

	// When the user presses i and installation completes
	cmd := updateBrowseModel(t, model, keyPress('i'))
	require.NotNil(t, cmd)
	assert.Contains(t, model.View().Content, "Installing cli/gh-cool...")
	updateBrowseModel(t, model, cmd())

	// Then the extension is visibly marked as installed
	assert.Contains(t, model.View().Content, "Installed cli/gh-cool!")
	assert.Contains(t, model.View().Content, "(installed)")
}

func TestBrowseModelDoesNotStartDuplicateInstall(t *testing.T) {
	t.Parallel()

	// Given installation of the selected extension is already in progress
	model := newBrowseModel(ExtBrowseOpts{
		Em: &extensions.ExtensionManagerMock{
			InstallFunc: func(ghrepo.Interface, string) error {
				return nil
			},
		},
	}, []extEntry{
		{Name: "gh-cool", FullName: "cli/gh-cool"},
	})
	firstInstall := updateBrowseModel(t, model, keyPress('i'))
	require.NotNil(t, firstInstall)

	// When the user presses i again before installation completes
	secondInstall := updateBrowseModel(t, model, keyPress('i'))

	// Then no duplicate installation command is started
	assert.Nil(t, secondInstall)
}

func TestBrowseModelDoesNotQuitWhileInstallIsRunning(t *testing.T) {
	t.Parallel()

	// Given installation of the selected extension is in progress
	model := newBrowseModel(ExtBrowseOpts{
		Em: &extensions.ExtensionManagerMock{
			InstallFunc: func(ghrepo.Interface, string) error {
				return nil
			},
		},
	}, []extEntry{{Name: "gh-cool", FullName: "cli/gh-cool"}})
	install := updateBrowseModel(t, model, keyPress('i'))
	require.NotNil(t, install)

	// When the user presses q before installation completes
	quit := updateBrowseModel(t, model, keyPress('q'))

	// Then the extension browser stays open so the filesystem operation can finish
	assert.Nil(t, quit)
	assert.Contains(t, model.View().Content, "Installing cli/gh-cool...")
}

func TestBrowseModelPreservesActionResultWhenLeavingHelp(t *testing.T) {
	t.Parallel()

	// Given an installation fails while keyboard help is open
	model := newBrowseModel(ExtBrowseOpts{
		Em: &extensions.ExtensionManagerMock{
			InstallFunc: func(ghrepo.Interface, string) error {
				return errors.New("permission denied")
			},
		},
	}, []extEntry{{Name: "gh-cool", FullName: "cli/gh-cool"}})
	install := updateBrowseModel(t, model, keyPress('i'))
	require.NotNil(t, install)
	updateBrowseModel(t, model, keyPress('?'))
	updateBrowseModel(t, model, install())
	require.Contains(t, model.View().Content, "Extension Management")

	// When the user closes keyboard help
	updateBrowseModel(t, model, keyPress('q'))

	// Then the unseen installation result is visible in the catalog footer
	assert.Contains(t, model.View().Content, "failed to install cli/gh-cool: permission denied")
}

func TestBrowseModelRemovesSelectedExtension(t *testing.T) {
	t.Parallel()

	// Given an installed extension is selected
	manager := &extensions.ExtensionManagerMock{
		RemoveFunc: func(name string) error {
			assert.Equal(t, "cool", name)
			return nil
		},
	}
	model := newBrowseModel(ExtBrowseOpts{
		Em: manager,
	}, []extEntry{
		{
			Name:      "gh-cool",
			FullName:  "cli/gh-cool",
			Installed: true,
		},
	})

	// When the user presses r and removal completes
	cmd := updateBrowseModel(t, model, keyPress('r'))
	require.NotNil(t, cmd)
	assert.Contains(t, model.View().Content, "Removing cli/gh-cool...")
	updateBrowseModel(t, model, cmd())

	// Then the extension is visibly marked as removed
	assert.Contains(t, model.View().Content, "Removed cli/gh-cool!")
	assert.NotContains(t, model.View().Content, "(installed)")
}

func TestBrowseModelKeepsExtensionUninstalledWhenInstallationFails(t *testing.T) {
	t.Parallel()

	// Given installation of the selected extension will fail
	manager := &extensions.ExtensionManagerMock{
		InstallFunc: func(ghrepo.Interface, string) error {
			return errors.New("permission denied")
		},
	}
	model := newBrowseModel(ExtBrowseOpts{Em: manager}, []extEntry{
		{Name: "gh-cool", FullName: "cli/gh-cool"},
	})

	// When the user presses i and installation fails
	cmd := updateBrowseModel(t, model, keyPress('i'))
	require.NotNil(t, cmd)
	updateBrowseModel(t, model, cmd())

	// Then the error is visible and the extension remains uninstalled
	view := model.View().Content
	assert.Contains(t, view, "failed to install cli/gh-cool: permission denied")
	assert.NotContains(t, view, "(installed)")
}

func TestBrowseModelShowsCompleteMultilineInstallationError(t *testing.T) {
	t.Parallel()

	// Given installation fails with platform guidance spanning multiple lines
	installError := strings.Join([]string{
		"gh-jj unsupported for darwin-arm64.",
		"",
		"To request support for darwin-arm64, open an issue on the extension's repo by running the following command:",
		"",
		"\t`gh issue create -R justDeeevin/gh-jj --title \"Add support for the darwin-arm64 architecture\" --body \"This extension does not support the darwin-arm64 architecture.\"`",
	}, "\n")
	model := newBrowseModel(ExtBrowseOpts{
		Em: &extensions.ExtensionManagerMock{
			InstallFunc: func(ghrepo.Interface, string) error {
				return errors.New(installError)
			},
		},
	}, []extEntry{{Name: "gh-jj", FullName: "justDeeevin/gh-jj"}})
	updateBrowseModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 30})

	// When the user installs the extension
	install := updateBrowseModel(t, model, keyPress('i'))
	require.NotNil(t, install)
	updateBrowseModel(t, model, install())
	view := model.View().Content

	// Then the full guidance remains visible without exceeding the terminal
	assert.Contains(t, view, "gh-jj unsupported for darwin-arm64.")
	assert.Contains(t, view, "To request support for darwin-arm64")
	assert.Contains(t, view, "gh issue create -R justDeeevin/gh-jj")
	var commandContinuation string
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, "not support the darwin-arm64 architecture") {
			commandContinuation = line
			break
		}
	}
	require.NotEmpty(t, commandContinuation)
	assert.True(t, strings.HasPrefix(commandContinuation, "    "), "expected the wrapped command to keep its indentation")
	assert.LessOrEqual(t, strings.Count(view, "\n")+1, 30)
}

func TestBrowseModelKeepsExtensionInstalledWhenRemovalFails(t *testing.T) {
	t.Parallel()

	// Given removal of the selected installed extension will fail
	manager := &extensions.ExtensionManagerMock{
		RemoveFunc: func(string) error {
			return errors.New("permission denied")
		},
	}
	model := newBrowseModel(ExtBrowseOpts{Em: manager}, []extEntry{
		{Name: "gh-cool", FullName: "cli/gh-cool", Installed: true},
	})

	// When the user presses r and removal fails
	cmd := updateBrowseModel(t, model, keyPress('r'))
	require.NotNil(t, cmd)
	updateBrowseModel(t, model, cmd())

	// Then the error is visible and the extension remains installed
	view := model.View().Content
	assert.Contains(t, view, "failed to remove cli/gh-cool: permission denied")
	assert.Contains(t, view, "(installed)")
}

func keyPress(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func specialKey(code rune, mod tea.KeyMod) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: mod}
}

func updateBrowseModel(t *testing.T, model *browseModel, msg tea.Msg) tea.Cmd {
	t.Helper()
	updated, cmd := model.Update(msg)
	got, ok := updated.(*browseModel)
	require.True(t, ok, "expected the updated model to remain a browse model")
	require.Same(t, model, got)
	return cmd
}

type readmeLoaderStub struct {
	content map[string]string
	errs    map[string]error
}

type browserStub struct {
	url string
	err error
}

func (s *browserStub) Browse(url string) error {
	s.url = url
	return s.err
}

func (s readmeLoaderStub) Get(fullName string) (string, error) {
	if err := s.errs[fullName]; err != nil {
		return "", err
	}
	return s.content[fullName], nil
}
