package vt10x

import (
	"io"
	"log"
	"sync"
)

const (
	tabspaces = 8
)

const (
	attrReverse = 1 << iota
	attrUnderline
	attrBold
	attrGfx
	attrItalic
	attrBlink
	attrWrap
)

const (
	cursorDefault = 1 << iota
	cursorWrapNext
	cursorOrigin
)

// ModeFlag represents various terminal mode states.
type ModeFlag uint32

// Terminal modes
const (
	ModeWrap ModeFlag = 1 << iota
	ModeInsert
	ModeAppKeypad
	ModeAltScreen
	ModeCRLF
	ModeMouseButton
	ModeMouseMotion
	ModeReverse
	ModeKeyboardLock
	ModeHide
	ModeEcho
	ModeAppCursor
	ModeMouseSgr
	Mode8bit
	ModeBlink
	ModeFBlink
	ModeFocus
	ModeMouseX10
	ModeMouseMany
	ModeMouseMask = ModeMouseButton | ModeMouseMotion | ModeMouseX10 | ModeMouseMany
)

// ChangeFlag represents possible state changes of the terminal.
type ChangeFlag uint32

// Terminal changes to occur in VT.ReadState
const (
	ChangedScreen ChangeFlag = 1 << iota
	ChangedTitle
)

type Glyph struct {
	Char   rune
	Mode   int16
	FG, BG Color
}

type line []Glyph

type Cursor struct {
	Attr  Glyph
	X, Y  int
	State uint8
}

type parseState func(c rune)

// State represents the terminal emulation state. Use Lock/Unlock
// methods to synchronize data access with VT.
type State struct {
	DebugLogger *log.Logger

	w             io.Writer
	mu            sync.Mutex
	changed       ChangeFlag
	cols, rows    int
	lines         []line
	altLines      []line
	dirty         []bool // line dirtiness
	anydirty      bool
	cur, curSaved Cursor
	top, bottom   int // scroll limits
	mode          ModeFlag
	state         parseState
	str           strEscape
	csi           csiEscape
	numlock       bool
	tabs          []bool
	title         string
}

func (t *State) logf(format string, args ...interface{}) {
	if t.DebugLogger != nil {
		t.DebugLogger.Printf(format, args...)
	}
}

func (t *State) logln(s string) {
	if t.DebugLogger != nil {
		t.DebugLogger.Println(s)
	}
}

func (t *State) lock() {
	t.mu.Lock()
}

func (t *State) unlock() {
	t.mu.Unlock()
}

// Lock locks the state object's mutex.
func (t *State) Lock() {
	t.mu.Lock()
}

// Unlock resets change flags and unlocks the state object's mutex.
func (t *State) Unlock() {
	t.resetChanges()
	t.mu.Unlock()
}

// Cell returns the glyph containing the character code, foreground color, and
// background color at position (x, y) relative to the top left of the terminal.
func (t *State) Cell(x, y int) Glyph {
	return t.lines[y][x]
}

// Cursor returns the current position of the cursor.
func (t *State) Cursor() Cursor {
	return t.cur
}

// CursorVisible returns the visible state of the cursor.
func (t *State) CursorVisible() bool {
	return t.mode&ModeHide == 0
}

// Mode returns the current terminal mode.
func (t *State) Mode() ModeFlag {
	return t.mode
}

// Title returns the current title set via the tty.
func (t *State) Title() string {
	return t.title
}

/*
// ChangeMask returns a bitfield of changes that have occured by VT.
func (t *State) ChangeMask() ChangeFlag {
	return t.changed
}
*/

// Changed returns true if change has occured.
func (t *State) Changed(change ChangeFlag) bool {
	return t.changed&change != 0
}

// resetChanges resets the change mask and dirtiness.
func (t *State) resetChanges() {
	for i := range t.dirty {
		t.dirty[i] = false
	}
	t.anydirty = false
	t.changed = 0
}

func (t *State) saveCursor() {
	t.curSaved = t.cur
}

func (t *State) restoreCursor() {
	t.cur = t.curSaved
	t.moveTo(t.cur.X, t.cur.Y)
}

func (t *State) put(c rune) {
	t.state(c)
}

func (t *State) putTab(forward bool) {
	x := t.cur.X
	if forward {
		if x == t.cols {
			return
		}
		for x++; x < t.cols && !t.tabs[x]; x++ {
		}
	} else {
		if x == 0 {
			return
		}
		for x--; x > 0 && !t.tabs[x]; x-- {
		}
	}
	t.moveTo(x, t.cur.Y)
}

func (t *State) newline(firstCol bool) {
	y := t.cur.Y
	if y == t.bottom {
		cur := t.cur
		t.cur = t.defaultCursor()
		t.scrollUp(t.top, 1)
		t.cur = cur
	} else {
		y++
	}
	if firstCol {
		t.moveTo(0, y)
	} else {
		t.moveTo(t.cur.X, y)
	}
}

// table from st, which in turn is from rxvt :)
var gfxCharTable = [62]rune{
	'↑', '↓', '→', '←', '█', '▚', '☃', // A - G
	0, 0, 0, 0, 0, 0, 0, 0, // H - O
	0, 0, 0, 0, 0, 0, 0, 0, // P - W
	0, 0, 0, 0, 0, 0, 0, ' ', // X - _
	'◆', '▒', '␉', '␌', '␍', '␊', '°', '±', // ` - g
	'␤', '␋', '┘', '┐', '┌', '└', '┼', '⎺', // h - o
	'⎻', '─', '⎼', '⎽', '├', '┤', '┴', '┬', // p - w
	'│', '≤', '≥', 'π', '≠', '£', '·', // x - ~
}

func (t *State) setChar(c rune, attr *Glyph, x, y int) {
	if attr.Mode&attrGfx != 0 {
		if c >= 0x41 && c <= 0x7e && gfxCharTable[c-0x41] != 0 {
			c = gfxCharTable[c-0x41]
		}
	}
	t.changed |= ChangedScreen
	t.dirty[y] = true
	t.lines[y][x] = *attr
	t.lines[y][x].Char = c
	//if t.options.BrightBold && attr.Mode&attrBold != 0 && attr.FG < 8 {
	if attr.Mode&attrBold != 0 && attr.FG < 8 {
		t.lines[y][x].FG = attr.FG + 8
	}
	if attr.Mode&attrReverse != 0 {
		t.lines[y][x].FG = attr.BG
		t.lines[y][x].BG = attr.FG
	}
}

func (t *State) defaultCursor() Cursor {
	c := Cursor{}
	c.Attr.FG = DefaultFG
	c.Attr.BG = DefaultBG
	return c
}

func (t *State) reset() {
	t.cur = t.defaultCursor()
	t.saveCursor()
	for i := range t.tabs {
		t.tabs[i] = false
	}
	for i := tabspaces; i < len(t.tabs); i += tabspaces {
		t.tabs[i] = true
	}
	t.top = 0
	t.bottom = t.rows - 1
	t.mode = ModeWrap
	t.clear(0, 0, t.rows-1, t.cols-1)
	t.moveTo(0, 0)
}

// TODO: definitely can improve allocs
func (t *State) resize(cols, rows int) bool {
	if cols == t.cols && rows == t.rows {
		return false
	}
	if cols < 1 || rows < 1 {
		return false
	}
	slide := t.cur.Y - rows + 1
	if slide > 0 {
		copy(t.lines, t.lines[slide:slide+rows])
		copy(t.altLines, t.altLines[slide:slide+rows])
	}

	lines, altLines, tabs := t.lines, t.altLines, t.tabs
	t.lines = make([]line, rows)
	t.altLines = make([]line, rows)
	t.dirty = make([]bool, rows)
	t.tabs = make([]bool, cols)

	minrows := min(rows, t.rows)
	mincols := min(cols, t.cols)
	t.changed |= ChangedScreen
	for i := 0; i < rows; i++ {
		t.dirty[i] = true
		t.lines[i] = make(line, cols)
		t.altLines[i] = make(line, cols)
	}
	for i := 0; i < minrows; i++ {
		copy(t.lines[i], lines[i])
		copy(t.altLines[i], altLines[i])
	}
	copy(t.tabs, tabs)
	if cols > t.cols {
		i := t.cols - 1
		for i > 0 && !tabs[i] {
			i--
		}
		for i += tabspaces; i < len(tabs); i += tabspaces {
			tabs[i] = true
		}
	}

	t.cols = cols
	t.rows = rows
	t.setScroll(0, rows-1)
	t.moveTo(t.cur.X, t.cur.Y)
	for i := 0; i < 2; i++ {
		if mincols < cols && minrows > 0 {
			t.clear(mincols, 0, cols-1, minrows-1)
		}
		if cols > 0 && minrows < rows {
			t.clear(0, minrows, cols-1, rows-1)
		}
		t.swapScreen()
	}
	return slide > 0
}

func (t *State) clear(x0, y0, x1, y1 int) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	x0 = clamp(x0, 0, t.cols-1)
	x1 = clamp(x1, 0, t.cols-1)
	y0 = clamp(y0, 0, t.rows-1)
	y1 = clamp(y1, 0, t.rows-1)
	t.changed |= ChangedScreen
	for y := y0; y <= y1; y++ {
		t.dirty[y] = true
		for x := x0; x <= x1; x++ {
			t.lines[y][x] = t.cur.Attr
			t.lines[y][x].Char = ' '
		}
	}
}

func (t *State) clearAll() {
	t.clear(0, 0, t.cols-1, t.rows-1)
}

func (t *State) moveAbsTo(x, y int) {
	if t.cur.State&cursorOrigin != 0 {
		y += t.top
	}
	t.moveTo(x, y)
}

func (t *State) moveTo(x, y int) {
	var miny, maxy int
	if t.cur.State&cursorOrigin != 0 {
		miny = t.top
		maxy = t.bottom
	} else {
		miny = 0
		maxy = t.rows - 1
	}
	x = clamp(x, 0, t.cols-1)
	y = clamp(y, miny, maxy)
	t.changed |= ChangedScreen
	t.cur.State &^= cursorWrapNext
	t.cur.X = x
	t.cur.Y = y
}

func (t *State) swapScreen() {
	t.lines, t.altLines = t.altLines, t.lines
	t.mode ^= ModeAltScreen
	t.dirtyAll()
}

func (t *State) dirtyAll() {
	t.changed |= ChangedScreen
	for y := 0; y < t.rows; y++ {
		t.dirty[y] = true
	}
}

func (t *State) setScroll(top, bottom int) {
	top = clamp(top, 0, t.rows-1)
	bottom = clamp(bottom, 0, t.rows-1)
	if top > bottom {
		top, bottom = bottom, top
	}
	t.top = top
	t.bottom = bottom
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(val, min, max int) int {
	if val < min {
		return min
	} else if val > max {
		return max
	}
	return val
}

func between(val, min, max int) bool {
	if val < min || val > max {
		return false
	}
	return true
}

func (t *State) scrollDown(orig, n int) {
	n = clamp(n, 0, t.bottom-orig+1)
	t.clear(0, t.bottom-n+1, t.cols-1, t.bottom)
	t.changed |= ChangedScreen
	for i := t.bottom; i >= orig+n; i-- {
		t.lines[i], t.lines[i-n] = t.lines[i-n], t.lines[i]
		t.dirty[i] = true
		t.dirty[i-n] = true
	}

	// TODO: selection scroll
}

func (t *State) scrollUp(orig, n int) {
	n = clamp(n, 0, t.bottom-orig+1)
	t.clear(0, orig, t.cols-1, orig+n-1)
	t.changed |= ChangedScreen
	for i := orig; i <= t.bottom-n; i++ {
		t.lines[i], t.lines[i+n] = t.lines[i+n], t.lines[i]
		t.dirty[i] = true
		t.dirty[i+n] = true
	}

	// TODO: selection scroll
}

func (t *State) modMode(set bool, bit ModeFlag) {
	if set {
		t.mode |= bit
	} else {
		t.mode &^= bit
	}
}

func (t *State) setMode(priv bool, set bool, args []int) {
	if priv {
		for _, a := range args {
			switch a {
			case 1: // DECCKM - cursor key
				t.modMode(set, ModeAppCursor)
			case 5: // DECSCNM - reverse video
				mode := t.mode
				t.modMode(set, ModeReverse)
				if mode != t.mode {
					// TODO: redraw
				}
			case 6: // DECOM - origin
				if set {
					t.cur.State |= cursorOrigin
				} else {
					t.cur.State &^= cursorOrigin
				}
				t.moveAbsTo(0, 0)
			case 7: // DECAWM - auto wrap
				t.modMode(set, ModeWrap)
			// IGNORED:
			case 0, // error
				2,  // DECANM - ANSI/VT52
				3,  // DECCOLM - column
				4,  // DECSCLM - scroll
				8,  // DECARM - auto repeat
				18, // DECPFF - printer feed
				19, // DECPEX - printer extent
				42, // DECNRCM - national characters
				12: // att610 - start blinking cursor
				break
			case 25: // DECTCEM - text cursor enable mode
				t.modMode(!set, ModeHide)
			case 9: // X10 mouse compatibility mode
				t.modMode(false, ModeMouseMask)
				t.modMode(set, ModeMouseX10)
			case 1000: // report button press
				t.modMode(false, ModeMouseMask)
				t.modMode(set, ModeMouseButton)
			case 1002: // report motion on button press
				t.modMode(false, ModeMouseMask)
				t.modMode(set, ModeMouseMotion)
			case 1003: // enable all mouse motions
				t.modMode(false, ModeMouseMask)
				t.modMode(set, ModeMouseMany)
			case 1004: // send focus events to tty
				t.modMode(set, ModeFocus)
			case 1006: // extended reporting mode
				t.modMode(set, ModeMouseSgr)
			case 1034:
				t.modMode(set, Mode8bit)
			case 1049, // = 1047 and 1048
				47, 1047:
				alt := t.mode&ModeAltScreen != 0
				if alt {
					t.clear(0, 0, t.cols-1, t.rows-1)
				}
				if !set || !alt {
					t.swapScreen()
				}
				if a != 1049 {
					break
				}
				fallthrough
			case 1048:
				if set {
					t.saveCursor()
				} else {
					t.restoreCursor()
				}
			case 1001:
				// mouse highlight mode; can hang the terminal by design when
				// implemented
			case 1005:
				// utf8 mouse mode; will confuse applications not supporting
				// utf8 and luit
			case 1015:
				// urxvt mangled mouse mode; incompatiblt and can be mistaken
				// for other control codes
			default:
				t.logf("unknown private set/reset mode %d\n", a)
			}
		}
	} else {
		for _, a := range args {
			switch a {
			case 0: // Error (ignored)
			case 2: // KAM - keyboard action
				t.modMode(set, ModeKeyboardLock)
			case 4: // IRM - insertion-replacement
				t.modMode(set, ModeInsert)
				t.logln("insert mode not implemented")
			case 12: // SRM - send/receive
				t.modMode(set, ModeEcho)
			case 20: // LNM - linefeed/newline
				t.modMode(set, ModeCRLF)
			case 34:
				t.logln("right-to-left mode not implemented")
			case 96:
				t.logln("right-to-left copy mode not implemented")
			default:
				t.logf("unknown set/reset mode %d\n", a)
			}
		}
	}
}

func (t *State) setAttr(attr []int) {
	if len(attr) == 0 {
		attr = []int{0}
	}
	for i := 0; i < len(attr); i++ {
		a := attr[i]
		switch a {
		case 0:
			t.cur.Attr.Mode &^= attrReverse | attrUnderline | attrBold | attrItalic | attrBlink
			t.cur.Attr.FG = DefaultFG
			t.cur.Attr.BG = DefaultBG
		case 1:
			t.cur.Attr.Mode |= attrBold
		case 3:
			t.cur.Attr.Mode |= attrItalic
		case 4:
			t.cur.Attr.Mode |= attrUnderline
		case 5, 6: // slow, rapid blink
			t.cur.Attr.Mode |= attrBlink
		case 7:
			t.cur.Attr.Mode |= attrReverse
		case 21, 22:
			t.cur.Attr.Mode &^= attrBold
		case 23:
			t.cur.Attr.Mode &^= attrItalic
		case 24:
			t.cur.Attr.Mode &^= attrUnderline
		case 25, 26:
			t.cur.Attr.Mode &^= attrBlink
		case 27:
			t.cur.Attr.Mode &^= attrReverse
		case 38:
			if i+2 < len(attr) && attr[i+1] == 5 {
				i += 2
				if between(attr[i], 0, 255) {
					t.cur.Attr.FG = Color(attr[i])
				} else {
					t.logf("bad fgcolor %d\n", attr[i])
				}
			} else {
				t.logf("gfx attr %d unknown\n", a)
			}
		case 39:
			t.cur.Attr.FG = DefaultFG
		case 48:
			if i+2 < len(attr) && attr[i+1] == 5 {
				i += 2
				if between(attr[i], 0, 255) {
					t.cur.Attr.BG = Color(attr[i])
				} else {
					t.logf("bad bgcolor %d\n", attr[i])
				}
			} else {
				t.logf("gfx attr %d unknown\n", a)
			}
		case 49:
			t.cur.Attr.BG = DefaultBG
		default:
			if between(a, 30, 37) {
				t.cur.Attr.FG = Color(a - 30)
			} else if between(a, 40, 47) {
				t.cur.Attr.BG = Color(a - 40)
			} else if between(a, 90, 97) {
				t.cur.Attr.FG = Color(a - 90 + 8)
			} else if between(a, 100, 107) {
				t.cur.Attr.BG = Color(a - 100 + 8)
			} else {
				t.logf("gfx attr %d unknown\n", a)
			}
		}
	}
}

func (t *State) insertBlanks(n int) {
	src := t.cur.X
	dst := src + n
	size := t.cols - dst
	t.changed |= ChangedScreen
	t.dirty[t.cur.Y] = true

	if dst >= t.cols {
		t.clear(t.cur.X, t.cur.Y, t.cols-1, t.cur.Y)
	} else {
		copy(t.lines[t.cur.Y][dst:dst+size], t.lines[t.cur.Y][src:src+size])
		t.clear(src, t.cur.Y, dst-1, t.cur.Y)
	}
}

func (t *State) insertBlankLines(n int) {
	if t.cur.Y < t.top || t.cur.Y > t.bottom {
		return
	}
	t.scrollDown(t.cur.Y, n)
}

func (t *State) deleteLines(n int) {
	if t.cur.Y < t.top || t.cur.Y > t.bottom {
		return
	}
	t.scrollUp(t.cur.Y, n)
}

func (t *State) deleteChars(n int) {
	src := t.cur.X + n
	dst := t.cur.X
	size := t.cols - src
	t.changed |= ChangedScreen
	t.dirty[t.cur.Y] = true

	if src >= t.cols {
		t.clear(t.cur.X, t.cur.Y, t.cols-1, t.cur.Y)
	} else {
		copy(t.lines[t.cur.Y][dst:dst+size], t.lines[t.cur.Y][src:src+size])
		t.clear(t.cols-n, t.cur.Y, t.cols-1, t.cur.Y)
	}
}

func (t *State) setTitle(title string) {
	t.changed |= ChangedTitle
	t.title = title
}

func (t *State) Size() (cols, rows int) {
	return t.cols, t.rows
}

func (t *State) String() string {
	t.Lock()
	defer t.Unlock()

	var view []rune
	for y := 0; y < t.rows; y++ {
		for x := 0; x < t.cols; x++ {
			attr := t.Cell(x, y)
			view = append(view, attr.Char)
		}
		view = append(view, '\n')
	}

	return string(view)
}
