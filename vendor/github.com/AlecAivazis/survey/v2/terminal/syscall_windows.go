package terminal

import (
	"syscall"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procSetConsoleTextAttribute    = kernel32.NewProc("SetConsoleTextAttribute")
	procSetConsoleCursorPosition   = kernel32.NewProc("SetConsoleCursorPosition")
	procFillConsoleOutputCharacter = kernel32.NewProc("FillConsoleOutputCharacterW")
	procGetConsoleCursorInfo       = kernel32.NewProc("GetConsoleCursorInfo")
	procSetConsoleCursorInfo       = kernel32.NewProc("SetConsoleCursorInfo")
)

type wchar uint16
type dword uint32
type word uint16

type smallRect struct {
	left   Short
	top    Short
	right  Short
	bottom Short
}

type consoleScreenBufferInfo struct {
	size              Coord
	cursorPosition    Coord
	attributes        word
	window            smallRect
	maximumWindowSize Coord
}

type consoleCursorInfo struct {
	size    dword
	visible int32
}
