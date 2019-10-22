package terminal

import (
	"fmt"
	"io"
)

const (
	KeyArrowLeft       = '\x02'
	KeyArrowRight      = '\x06'
	KeyArrowUp         = '\x10'
	KeyArrowDown       = '\x0e'
	KeySpace           = ' '
	KeyEnter           = '\r'
	KeyBackspace       = '\b'
	KeyDelete          = '\x7f'
	KeyInterrupt       = '\x03'
	KeyEndTransmission = '\x04'
	KeyEscape          = '\x1b'
	KeyDeleteWord      = '\x17' // Ctrl+W
	KeyDeleteLine      = '\x18' // Ctrl+X
	SpecialKeyHome     = '\x01'
	SpecialKeyEnd      = '\x11'
	SpecialKeyDelete   = '\x12'
	IgnoreKey          = '\000'
)

func soundBell(out io.Writer) {
	fmt.Fprint(out, "\a")
}
