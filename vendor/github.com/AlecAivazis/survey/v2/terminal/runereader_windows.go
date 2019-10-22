package terminal

import (
	"bytes"
	"syscall"
	"unsafe"
)

var (
	dll              = syscall.NewLazyDLL("kernel32.dll")
	setConsoleMode   = dll.NewProc("SetConsoleMode")
	getConsoleMode   = dll.NewProc("GetConsoleMode")
	readConsoleInput = dll.NewProc("ReadConsoleInputW")
)

const (
	EVENT_KEY = 0x0001

	// key codes for arrow keys
	// https://msdn.microsoft.com/en-us/library/windows/desktop/dd375731(v=vs.85).aspx
	VK_DELETE = 0x2E
	VK_END    = 0x23
	VK_HOME   = 0x24
	VK_LEFT   = 0x25
	VK_UP     = 0x26
	VK_RIGHT  = 0x27
	VK_DOWN   = 0x28

	RIGHT_CTRL_PRESSED = 0x0004
	LEFT_CTRL_PRESSED  = 0x0008

	ENABLE_ECHO_INPUT      uint32 = 0x0004
	ENABLE_LINE_INPUT      uint32 = 0x0002
	ENABLE_PROCESSED_INPUT uint32 = 0x0001
)

type inputRecord struct {
	eventType uint16
	padding   uint16
	event     [16]byte
}

type keyEventRecord struct {
	bKeyDown          int32
	wRepeatCount      uint16
	wVirtualKeyCode   uint16
	wVirtualScanCode  uint16
	unicodeChar       uint16
	wdControlKeyState uint32
}

type runeReaderState struct {
	term uint32
}

func newRuneReaderState(input FileReader) runeReaderState {
	return runeReaderState{}
}

func (rr *RuneReader) Buffer() *bytes.Buffer {
	return nil
}

func (rr *RuneReader) SetTermMode() error {
	r, _, err := getConsoleMode.Call(uintptr(rr.stdio.In.Fd()), uintptr(unsafe.Pointer(&rr.state.term)))
	// windows return 0 on error
	if r == 0 {
		return err
	}

	newState := rr.state.term
	newState &^= ENABLE_ECHO_INPUT | ENABLE_LINE_INPUT | ENABLE_PROCESSED_INPUT
	r, _, err = setConsoleMode.Call(uintptr(rr.stdio.In.Fd()), uintptr(newState))
	// windows return 0 on error
	if r == 0 {
		return err
	}
	return nil
}

func (rr *RuneReader) RestoreTermMode() error {
	r, _, err := setConsoleMode.Call(uintptr(rr.stdio.In.Fd()), uintptr(rr.state.term))
	// windows return 0 on error
	if r == 0 {
		return err
	}
	return nil
}

func (rr *RuneReader) ReadRune() (rune, int, error) {
	ir := &inputRecord{}
	bytesRead := 0
	for {
		rv, _, e := readConsoleInput.Call(rr.stdio.In.Fd(), uintptr(unsafe.Pointer(ir)), 1, uintptr(unsafe.Pointer(&bytesRead)))
		// windows returns non-zero to indicate success
		if rv == 0 && e != nil {
			return 0, 0, e
		}

		if ir.eventType != EVENT_KEY {
			continue
		}

		// the event data is really a c struct union, so here we have to do an usafe
		// cast to put the data into the keyEventRecord (since we have already verified
		// above that this event does correspond to a key event
		key := (*keyEventRecord)(unsafe.Pointer(&ir.event[0]))
		// we only care about key down events
		if key.bKeyDown == 0 {
			continue
		}
		if key.wdControlKeyState&(LEFT_CTRL_PRESSED|RIGHT_CTRL_PRESSED) != 0 && key.unicodeChar == 'C' {
			return KeyInterrupt, bytesRead, nil
		}
		// not a normal character so look up the input sequence from the
		// virtual key code mappings (VK_*)
		if key.unicodeChar == 0 {
			switch key.wVirtualKeyCode {
			case VK_DOWN:
				return KeyArrowDown, bytesRead, nil
			case VK_LEFT:
				return KeyArrowLeft, bytesRead, nil
			case VK_RIGHT:
				return KeyArrowRight, bytesRead, nil
			case VK_UP:
				return KeyArrowUp, bytesRead, nil
			case VK_DELETE:
				return SpecialKeyDelete, bytesRead, nil
			case VK_HOME:
				return SpecialKeyHome, bytesRead, nil
			case VK_END:
				return SpecialKeyEnd, bytesRead, nil
			default:
				// not a virtual key that we care about so just continue on to
				// the next input key
				continue
			}
		}
		r := rune(key.unicodeChar)
		return r, bytesRead, nil
	}
}
