package notification

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// listItem represents one item in a List.
type listItem struct {
	Reason    string
	Unread    bool
	URL       string
	Repo      string
	UpdatedAt string
	Type      string
	Checked   bool

	MainText      string // The main text of the list item.
	SecondaryText string // A secondary text to be shown underneath the main text.
	Shortcut      rune   // The key to select the list item directly, 0 if there is no shortcut.
	Selected      func() // The optional function which is called when the item is selected.
}

// List displays rows of items, each of which can be selected.
//
// See https://github.com/rivo/tview/wiki/List for an example.
type List struct {
	*tview.Box

	// The items of the list.
	items []*listItem

	// The index of the currently selected item.
	currentItem int

	// Whether or not to show the secondary item texts.
	showSecondaryText bool

	// The item main text color.
	mainTextColor tcell.Color

	// The item secondary text color.
	secondaryTextColor tcell.Color

	// The item shortcut text color.
	shortcutColor tcell.Color

	// The text color for selected items.
	selectedTextColor tcell.Color

	// The background color for selected items.
	selectedBackgroundColor tcell.Color

	// If true, the selection is only shown when the list has focus.
	selectedFocusOnly bool

	// If true, the entire row is highlighted when selected.
	highlightFullLine bool

	// Whether or not navigating the list will wrap around.
	wrapAround bool

	// The number of list items skipped at the top before the first item is drawn.
	offset int

	// An optional function which is called when the user has navigated to a list
	// item.
	changed func(index int, mainText, secondaryText string, shortcut rune)

	// An optional function which is called when a list item was selected. This
	// function will be called even if the list item defines its own callback.
	selected func(index int, mainText, secondaryText string, shortcut rune)

	// An optional function which is called when the user presses the Escape key.
	done func()
}

// NewList returns a new form.
func NewList() *List {
	return &List{
		Box:                     tview.NewBox(),
		showSecondaryText:       true,
		wrapAround:              true,
		mainTextColor:           tview.Styles.PrimaryTextColor,
		secondaryTextColor:      tview.Styles.TertiaryTextColor,
		shortcutColor:           tview.Styles.SecondaryTextColor,
		selectedTextColor:       tview.Styles.PrimitiveBackgroundColor,
		selectedBackgroundColor: tview.Styles.PrimaryTextColor,
	}
}

// SetCurrentItem sets the currently selected item by its index, starting at 0
// for the first item. If a negative index is provided, items are referred to
// from the back (-1 = last item, -2 = second-to-last item, and so on). Out of
// range indices are clamped to the beginning/end.
//
// Calling this function triggers a "changed" event if the selection changes.
func (l *List) SetCurrentItem(index int) *List {
	if index < 0 {
		index = len(l.items) + index
	}
	if index >= len(l.items) {
		index = len(l.items) - 1
	}
	if index < 0 {
		index = 0
	}

	if index != l.currentItem && l.changed != nil {
		item := l.items[index]
		l.changed(index, item.MainText, item.SecondaryText, item.Shortcut)
	}

	l.currentItem = index

	return l
}

// GetCurrentItem returns the index of the currently selected list item,
// starting at 0 for the first item.
func (l *List) GetCurrentItem() int {
	return l.currentItem
}

// RemoveItem removes the item with the given index (starting at 0) from the
// list. If a negative index is provided, items are referred to from the back
// (-1 = last item, -2 = second-to-last item, and so on). Out of range indices
// are clamped to the beginning/end, i.e. unless the list is empty, an item is
// always removed.
//
// The currently selected item is shifted accordingly. If it is the one that is
// removed, a "changed" event is fired.
func (l *List) RemoveItem(index int) *List {
	if len(l.items) == 0 {
		return l
	}

	// Adjust index.
	if index < 0 {
		index = len(l.items) + index
	}
	if index >= len(l.items) {
		index = len(l.items) - 1
	}
	if index < 0 {
		index = 0
	}

	// Remove item.
	l.items = append(l.items[:index], l.items[index+1:]...)

	// If there is nothing left, we're done.
	if len(l.items) == 0 {
		return l
	}

	// Shift current item.
	previousCurrentItem := l.currentItem
	if l.currentItem >= index {
		l.currentItem--
	}

	// Fire "changed" event for removed items.
	if previousCurrentItem == index && l.changed != nil {
		item := l.items[l.currentItem]
		l.changed(l.currentItem, item.MainText, item.SecondaryText, item.Shortcut)
	}

	return l
}

// SetMainTextColor sets the color of the items' main text.
func (l *List) SetMainTextColor(color tcell.Color) *List {
	l.mainTextColor = color
	return l
}

// SetSecondaryTextColor sets the color of the items' secondary text.
func (l *List) SetSecondaryTextColor(color tcell.Color) *List {
	l.secondaryTextColor = color
	return l
}

// SetShortcutColor sets the color of the items' shortcut.
func (l *List) SetShortcutColor(color tcell.Color) *List {
	l.shortcutColor = color
	return l
}

// SetSelectedTextColor sets the text color of selected items.
func (l *List) SetSelectedTextColor(color tcell.Color) *List {
	l.selectedTextColor = color
	return l
}

// SetSelectedBackgroundColor sets the background color of selected items.
func (l *List) SetSelectedBackgroundColor(color tcell.Color) *List {
	l.selectedBackgroundColor = color
	return l
}

// SetSelectedFocusOnly sets a flag which determines when the currently selected
// list item is highlighted. If set to true, selected items are only highlighted
// when the list has focus. If set to false, they are always highlighted.
func (l *List) SetSelectedFocusOnly(focusOnly bool) *List {
	l.selectedFocusOnly = focusOnly
	return l
}

// SetHighlightFullLine sets a flag which determines whether the colored
// background of selected items spans the entire width of the view. If set to
// true, the highlight spans the entire view. If set to false, only the text of
// the selected item from beginning to end is highlighted.
func (l *List) SetHighlightFullLine(highlight bool) *List {
	l.highlightFullLine = highlight
	return l
}

// ShowSecondaryText determines whether or not to show secondary item texts.
func (l *List) ShowSecondaryText(show bool) *List {
	l.showSecondaryText = show
	return l
}

// SetWrapAround sets the flag that determines whether navigating the list will
// wrap around. That is, navigating downwards on the last item will move the
// selection to the first item (similarly in the other direction). If set to
// false, the selection won't change when navigating downwards on the last item
// or navigating upwards on the first item.
func (l *List) SetWrapAround(wrapAround bool) *List {
	l.wrapAround = wrapAround
	return l
}

// SetChangedFunc sets the function which is called when the user navigates to
// a list item. The function receives the item's index in the list of items
// (starting with 0), its main text, secondary text, and its shortcut rune.
//
// This function is also called when the first item is added or when
// SetCurrentItem() is called.
func (l *List) SetChangedFunc(handler func(index int, mainText string, secondaryText string, shortcut rune)) *List {
	l.changed = handler
	return l
}

// SetSelectedFunc sets the function which is called when the user selects a
// list item by pressing Enter on the current selection. The function receives
// the item's index in the list of items (starting with 0), its main text,
// secondary text, and its shortcut rune.
func (l *List) SetSelectedFunc(handler func(int, string, string, rune)) *List {
	l.selected = handler
	return l
}

// SetDoneFunc sets a function which is called when the user presses the Escape
// key.
func (l *List) SetDoneFunc(handler func()) *List {
	l.done = handler
	return l
}

// AddItem calls InsertItem() with an index of -1.
func (l *List) AddItem(mainText, secondaryText string, shortcut rune, selected func()) *List {
	l.InsertItem(-1, mainText, secondaryText, shortcut, selected)
	return l
}

// InsertItem adds a new item to the list at the specified index. An index of 0
// will insert the item at the beginning, an index of 1 before the second item,
// and so on. An index of GetItemCount() or higher will insert the item at the
// end of the list. Negative indices are also allowed: An index of -1 will
// insert the item at the end of the list, an index of -2 before the last item,
// and so on. An index of -GetItemCount()-1 or lower will insert the item at the
// beginning.
//
// An item has a main text which will be highlighted when selected. It also has
// a secondary text which is shown underneath the main text (if it is set to
// visible) but which may remain empty.
//
// The shortcut is a key binding. If the specified rune is entered, the item
// is selected immediately. Set to 0 for no binding.
//
// The "selected" callback will be invoked when the user selects the item. You
// may provide nil if no such callback is needed or if all events are handled
// through the selected callback set with SetSelectedFunc().
//
// The currently selected item will shift its position accordingly. If the list
// was previously empty, a "changed" event is fired because the new item becomes
// selected.
func (l *List) InsertItem(index int, mainText, secondaryText string, shortcut rune, selected func()) *List {
	item := &listItem{
		MainText:      mainText,
		SecondaryText: secondaryText,
		Shortcut:      shortcut,
		Selected:      selected,
	}

	// Shift index to range.
	if index < 0 {
		index = len(l.items) + index + 1
	}
	if index < 0 {
		index = 0
	} else if index > len(l.items) {
		index = len(l.items)
	}

	// Shift current item.
	if l.currentItem < len(l.items) && l.currentItem >= index {
		l.currentItem++
	}

	// Insert item (make space for the new item, then shift and insert).
	l.items = append(l.items, nil)
	if index < len(l.items)-1 { // -1 because l.items has already grown by one item.
		copy(l.items[index+1:], l.items[index:])
	}
	l.items[index] = item

	// Fire a "change" event for the first item in the list.
	if len(l.items) == 1 && l.changed != nil {
		item := l.items[0]
		l.changed(0, item.MainText, item.SecondaryText, item.Shortcut)
	}

	return l
}

// GetItemCount returns the number of items in the list.
func (l *List) GetItemCount() int {
	return len(l.items)
}

// GetItemText returns an item's texts (main and secondary). Panics if the index
// is out of range.
func (l *List) GetItemText(index int) (main, secondary string) {
	return l.items[index].MainText, l.items[index].SecondaryText
}

// SetItemText sets an item's main and secondary text. Panics if the index is
// out of range.
func (l *List) SetItemText(index int, main, secondary string) *List {
	item := l.items[index]
	item.MainText = main
	item.SecondaryText = secondary
	return l
}

// FindItems searches the main and secondary texts for the given strings and
// returns a list of item indices in which those strings are found. One of the
// two search strings may be empty, it will then be ignored. Indices are always
// returned in ascending order.
//
// If mustContainBoth is set to true, mainSearch must be contained in the main
// text AND secondarySearch must be contained in the secondary text. If it is
// false, only one of the two search strings must be contained.
//
// Set ignoreCase to true for case-insensitive search.
func (l *List) FindItems(mainSearch, secondarySearch string, mustContainBoth, ignoreCase bool) (indices []int) {
	if mainSearch == "" && secondarySearch == "" {
		return
	}

	if ignoreCase {
		mainSearch = strings.ToLower(mainSearch)
		secondarySearch = strings.ToLower(secondarySearch)
	}

	for index, item := range l.items {
		mainText := item.MainText
		secondaryText := item.SecondaryText
		if ignoreCase {
			mainText = strings.ToLower(mainText)
			secondaryText = strings.ToLower(secondaryText)
		}

		// strings.Contains() always returns true for a "" search.
		mainContained := strings.Contains(mainText, mainSearch)
		secondaryContained := strings.Contains(secondaryText, secondarySearch)
		if mustContainBoth && mainContained && secondaryContained ||
			!mustContainBoth && (mainText != "" && mainContained || secondaryText != "" && secondaryContained) {
			indices = append(indices, index)
		}
	}

	return
}

// Clear removes all items from the list.
func (l *List) Clear() *List {
	l.items = nil
	l.currentItem = 0
	return l
}

// Draw draws this primitive onto the screen.
func (l *List) Draw(screen tcell.Screen) {
	l.Box.DrawForSubclass(screen, l)

	// Determine the dimensions.
	x, y, width, height := l.GetInnerRect()
	bottomLimit := y + height
	_, totalHeight := screen.Size()
	if bottomLimit > totalHeight {
		bottomLimit = totalHeight
	}

	// Do we show any shortcuts?
	// var showShortcuts bool
	// for _, item := range l.items {
	// 	if item.Shortcut != 0 {
	// 		showShortcuts = true
	// 		x += 4
	// 		width -= 4
	// 		break
	// 	}
	// }

	// Adjust offset to keep the current selection in view.
	if l.currentItem < l.offset {
		l.offset = l.currentItem
	} else if l.showSecondaryText {
		if 2*(l.currentItem-l.offset) >= height-1 {
			l.offset = (2*l.currentItem + 3 - height) / 2
		}
	} else {
		if l.currentItem-l.offset >= height {
			l.offset = l.currentItem + 1 - height
		}
	}

	// Draw the list items.
	for index, item := range l.items {
		if index < l.offset {
			continue
		}

		if y >= bottomLimit {
			break
		}

		// Shortcuts.
		// if showShortcuts && item.Shortcut != 0 {
		// 	tview.Print(screen, fmt.Sprintf("(%s)", string(item.Shortcut)), x-5, y, 4, tview.AlignRight, l.shortcutColor)
		// }

		// Selector arror.
		selector := " "
		if index == l.currentItem && (!l.selectedFocusOnly || l.HasFocus()) {
			selector = ">"
		}

		// Selected box.
		checked := "[ ]"
		if item.Checked {
			checked = "[x[]"
		}

		// Main text.
		line := fmt.Sprintf(`%s %s %s`, selector, checked, item.MainText)
		tview.Print(screen, line, x, y, width, tview.AlignLeft, l.mainTextColor)

		// Background color of selected text.
		// if index == l.currentItem && (!l.selectedFocusOnly || l.HasFocus()) {
		// 	textWidth := width
		// 	if !l.highlightFullLine {
		// 		if w := tview.TaggedStringWidth(item.MainText); w < textWidth {
		// 			textWidth = w
		// 		}
		// 	}

		// 	for bx := 0; bx < textWidth; bx++ {
		// 		m, c, style, _ := screen.GetContent(x+bx, y)
		// 		fg, _, _ := style.Decompose()
		// 		if fg == l.mainTextColor {
		// 			fg = l.selectedTextColor
		// 		}
		// 		style = style.Background(l.selectedBackgroundColor).Foreground(fg)
		// 		screen.SetContent(x+bx, y, m, c, style)
		// 	}
		// }

		y++

		if y >= bottomLimit {
			break
		}

		// Secondary text.
		if l.showSecondaryText {
			tview.Print(screen, item.SecondaryText, x, y, width, tview.AlignLeft, l.secondaryTextColor)
			y++
		}
	}
}

// InputHandler returns the handler for this primitive.
func (l *List) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return l.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if event.Key() == tcell.KeyEscape {
			if l.done != nil {
				l.done()
			}
			return
		} else if len(l.items) == 0 {
			return
		}

		previousItem := l.currentItem

		switch key := event.Key(); key {
		case tcell.KeyTab, tcell.KeyDown, tcell.KeyRight:
			l.currentItem++
		case tcell.KeyBacktab, tcell.KeyUp, tcell.KeyLeft:
			l.currentItem--
		case tcell.KeyHome:
			l.currentItem = 0
		case tcell.KeyEnd:
			l.currentItem = len(l.items) - 1
		case tcell.KeyPgDn:
			_, _, _, height := l.GetInnerRect()
			l.currentItem += height
		case tcell.KeyPgUp:
			_, _, _, height := l.GetInnerRect()
			l.currentItem -= height
		case tcell.KeyEnter:
			if l.currentItem >= 0 && l.currentItem < len(l.items) {
				item := l.items[l.currentItem]

				if item.Checked {
					item.Checked = false
				} else {
					item.Checked = true
				}

				if item.Selected != nil {
					item.Selected()
				}
				if l.selected != nil {
					l.selected(l.currentItem, item.MainText, item.SecondaryText, item.Shortcut)
				}
			}
		case tcell.KeyRune:
			ch := event.Rune()
			if ch == 'u' || ch == 'd' {
				for i := len(l.items) - 1; i >= 0; i-- {
					if l.items[i].Checked {
						l.RemoveItem(i)
					}
				}
			}
		}

		if l.currentItem < 0 {
			if l.wrapAround {
				l.currentItem = len(l.items) - 1
			} else {
				l.currentItem = 0
			}
		} else if l.currentItem >= len(l.items) {
			if l.wrapAround {
				l.currentItem = 0
			} else {
				l.currentItem = len(l.items) - 1
			}
		}

		if l.currentItem != previousItem && l.currentItem < len(l.items) && l.changed != nil {
			item := l.items[l.currentItem]
			l.changed(l.currentItem, item.MainText, item.SecondaryText, item.Shortcut)
		}
	})
}

// indexAtPoint returns the index of the list item found at the given position
// or a negative value if there is no such list item.
func (l *List) indexAtPoint(x, y int) int {
	rectX, rectY, width, height := l.GetInnerRect()
	if rectX < 0 || rectX >= rectX+width || y < rectY || y >= rectY+height {
		return -1
	}

	index := y - rectY
	if l.showSecondaryText {
		index /= 2
	}
	index += l.offset

	if index >= len(l.items) {
		return -1
	}
	return index
}

// MouseHandler returns the mouse handler for this primitive.
func (l *List) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return l.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !l.InRect(event.Position()) {
			return false, nil
		}

		// Process mouse event.
		switch action {
		case tview.MouseLeftClick:
			setFocus(l)
			index := l.indexAtPoint(event.Position())
			if index != -1 {
				item := l.items[index]
				if item.Selected != nil {
					item.Selected()
				}
				if l.selected != nil {
					l.selected(index, item.MainText, item.SecondaryText, item.Shortcut)
				}
				if index != l.currentItem && l.changed != nil {
					l.changed(index, item.MainText, item.SecondaryText, item.Shortcut)
				}
				l.currentItem = index
			}
			consumed = true
		case tview.MouseScrollUp:
			if l.offset > 0 {
				l.offset--
			}
			consumed = true
		case tview.MouseScrollDown:
			lines := len(l.items) - l.offset
			if l.showSecondaryText {
				lines *= 2
			}
			if _, _, _, height := l.GetInnerRect(); lines > height {
				l.offset++
			}
			consumed = true
		}

		return
	})
}
