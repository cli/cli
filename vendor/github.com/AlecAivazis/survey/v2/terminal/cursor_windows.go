package terminal

import (
	"bytes"
	"syscall"
	"unsafe"
)

var COORDINATE_SYSTEM_BEGIN Short = 0

// shared variable to save the cursor location from CursorSave()
var cursorLoc Coord

type Cursor struct {
	In  FileReader
	Out FileWriter
}

func (c *Cursor) Up(n int) {
	c.cursorMove(0, n)
}

func (c *Cursor) Down(n int) {
	c.cursorMove(0, -1*n)
}

func (c *Cursor) Forward(n int) {
	c.cursorMove(n, 0)
}

func (c *Cursor) Back(n int) {
	c.cursorMove(-1*n, 0)
}

// save the cursor location
func (c *Cursor) Save() {
	cursorLoc, _ = c.Location(nil)
}

func (c *Cursor) Restore() {
	handle := syscall.Handle(c.Out.Fd())
	// restore it to the original position
	procSetConsoleCursorPosition.Call(uintptr(handle), uintptr(*(*int32)(unsafe.Pointer(&cursorLoc))))
}

func (cur Coord) CursorIsAtLineEnd(size *Coord) bool {
	return cur.X == size.X
}

func (cur Coord) CursorIsAtLineBegin() bool {
	return cur.X == 0
}

func (c *Cursor) cursorMove(x int, y int) {
	handle := syscall.Handle(c.Out.Fd())

	var csbi consoleScreenBufferInfo
	procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&csbi)))

	var cursor Coord
	cursor.X = csbi.cursorPosition.X + Short(x)
	cursor.Y = csbi.cursorPosition.Y + Short(y)

	procSetConsoleCursorPosition.Call(uintptr(handle), uintptr(*(*int32)(unsafe.Pointer(&cursor))))
}

func (c *Cursor) NextLine(n int) {
	c.Up(n)
	c.HorizontalAbsolute(0)
}

func (c *Cursor) PreviousLine(n int) {
	c.Down(n)
	c.HorizontalAbsolute(0)
}

// for comparability purposes between windows
// in windows we don't have to print out a new line
func (c *Cursor) MoveNextLine(cur Coord, terminalSize *Coord) {
	c.NextLine(1)
}

func (c *Cursor) HorizontalAbsolute(x int) {
	handle := syscall.Handle(c.Out.Fd())

	var csbi consoleScreenBufferInfo
	procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&csbi)))

	var cursor Coord
	cursor.X = Short(x)
	cursor.Y = csbi.cursorPosition.Y

	if csbi.size.X < cursor.X {
		cursor.X = csbi.size.X
	}

	procSetConsoleCursorPosition.Call(uintptr(handle), uintptr(*(*int32)(unsafe.Pointer(&cursor))))
}

func (c *Cursor) Show() {
	handle := syscall.Handle(c.Out.Fd())

	var cci consoleCursorInfo
	procGetConsoleCursorInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&cci)))
	cci.visible = 1

	procSetConsoleCursorInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&cci)))
}

func (c *Cursor) Hide() {
	handle := syscall.Handle(c.Out.Fd())

	var cci consoleCursorInfo
	procGetConsoleCursorInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&cci)))
	cci.visible = 0

	procSetConsoleCursorInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&cci)))
}

func (c *Cursor) Location(buf *bytes.Buffer) (Coord, error) {
	handle := syscall.Handle(c.Out.Fd())

	var csbi consoleScreenBufferInfo
	procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&csbi)))

	return csbi.cursorPosition, nil
}

func (c *Cursor) Size(buf *bytes.Buffer) (*Coord, error) {
	handle := syscall.Handle(c.Out.Fd())

	var csbi consoleScreenBufferInfo
	procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&csbi)))
	// windows' coordinate system begins at (0, 0)
	csbi.size.X--
	csbi.size.Y--
	return &csbi.size, nil
}
