//
// Copyright (c) 2016-2019 The Aurora Authors. All rights reserved.
// This program is free software. It comes without any warranty,
// to the extent permitted by applicable law. You can redistribute
// it and/or modify it under the terms of the Do What The Fuck You
// Want To Public License, Version 2, as published by Sam Hocevar.
// See LICENSE file for more details or see below.
//

//
//        DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE
//                    Version 2, December 2004
//
// Copyright (C) 2004 Sam Hocevar <sam@hocevar.net>
//
// Everyone is permitted to copy and distribute verbatim or modified
// copies of this license document, and changing it is allowed as long
// as the name is changed.
//
//            DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE
//   TERMS AND CONDITIONS FOR COPYING, DISTRIBUTION AND MODIFICATION
//
//  0. You just DO WHAT THE FUCK YOU WANT TO.
//

// Package aurora implements ANSI-colors
package aurora

import (
	"fmt"
)

// An Aurora implements colorizer interface.
// It also can be a non-colorizer
type Aurora interface {

	// Reset wraps given argument returning Value
	// without formats and colors.
	Reset(arg interface{}) Value

	//
	// Formats
	//
	//
	// Bold or increased intensity (1).
	Bold(arg interface{}) Value
	// Faint, decreased intensity (2).
	Faint(arg interface{}) Value
	//
	// DoublyUnderline or Bold off, double-underline
	// per ECMA-48 (21).
	DoublyUnderline(arg interface{}) Value
	// Fraktur, rarely supported (20).
	Fraktur(arg interface{}) Value
	//
	// Italic, not widely supported, sometimes
	// treated as inverse (3).
	Italic(arg interface{}) Value
	// Underline (4).
	Underline(arg interface{}) Value
	//
	// SlowBlink, blinking less than 150
	// per minute (5).
	SlowBlink(arg interface{}) Value
	// RapidBlink, blinking 150+ per minute,
	// not widely supported (6).
	RapidBlink(arg interface{}) Value
	// Blink is alias for the SlowBlink.
	Blink(arg interface{}) Value
	//
	// Reverse video, swap foreground and
	// background colors (7).
	Reverse(arg interface{}) Value
	// Inverse is alias for the Reverse
	Inverse(arg interface{}) Value
	//
	// Conceal, hidden, not widely supported (8).
	Conceal(arg interface{}) Value
	// Hidden is alias for the Conceal
	Hidden(arg interface{}) Value
	//
	// CrossedOut, characters legible, but
	// marked for deletion (9).
	CrossedOut(arg interface{}) Value
	// StrikeThrough is alias for the CrossedOut.
	StrikeThrough(arg interface{}) Value
	//
	// Framed (51).
	Framed(arg interface{}) Value
	// Encircled (52).
	Encircled(arg interface{}) Value
	//
	// Overlined (53).
	Overlined(arg interface{}) Value

	//
	// Foreground colors
	//
	//
	// Black foreground color (30)
	Black(arg interface{}) Value
	// Red foreground color (31)
	Red(arg interface{}) Value
	// Green foreground color (32)
	Green(arg interface{}) Value
	// Yellow foreground color (33)
	Yellow(arg interface{}) Value
	// Brown foreground color (33)
	//
	// Deprecated: use Yellow instead, following specification
	Brown(arg interface{}) Value
	// Blue foreground color (34)
	Blue(arg interface{}) Value
	// Magenta foreground color (35)
	Magenta(arg interface{}) Value
	// Cyan foreground color (36)
	Cyan(arg interface{}) Value
	// White foreground color (37)
	White(arg interface{}) Value
	//
	// Bright foreground colors
	//
	// BrightBlack foreground color (90)
	BrightBlack(arg interface{}) Value
	// BrightRed foreground color (91)
	BrightRed(arg interface{}) Value
	// BrightGreen foreground color (92)
	BrightGreen(arg interface{}) Value
	// BrightYellow foreground color (93)
	BrightYellow(arg interface{}) Value
	// BrightBlue foreground color (94)
	BrightBlue(arg interface{}) Value
	// BrightMagenta foreground color (95)
	BrightMagenta(arg interface{}) Value
	// BrightCyan foreground color (96)
	BrightCyan(arg interface{}) Value
	// BrightWhite foreground color (97)
	BrightWhite(arg interface{}) Value
	//
	// Other
	//
	// Index of pre-defined 8-bit foreground color
	// from 0 to 255 (38;5;n).
	//
	//       0-  7:  standard colors (as in ESC [ 30–37 m)
	//       8- 15:  high intensity colors (as in ESC [ 90–97 m)
	//      16-231:  6 × 6 × 6 cube (216 colors): 16 + 36 × r + 6 × g + b (0 ≤ r, g, b ≤ 5)
	//     232-255:  grayscale from black to white in 24 steps
	//
	Index(n uint8, arg interface{}) Value
	// Gray from 0 to 23.
	Gray(n uint8, arg interface{}) Value

	//
	// Background colors
	//
	//
	// BgBlack background color (40)
	BgBlack(arg interface{}) Value
	// BgRed background color (41)
	BgRed(arg interface{}) Value
	// BgGreen background color (42)
	BgGreen(arg interface{}) Value
	// BgYellow background color (43)
	BgYellow(arg interface{}) Value
	// BgBrown background color (43)
	//
	// Deprecated: use BgYellow instead, following specification
	BgBrown(arg interface{}) Value
	// BgBlue background color (44)
	BgBlue(arg interface{}) Value
	// BgMagenta background color (45)
	BgMagenta(arg interface{}) Value
	// BgCyan background color (46)
	BgCyan(arg interface{}) Value
	// BgWhite background color (47)
	BgWhite(arg interface{}) Value
	//
	// Bright background colors
	//
	// BgBrightBlack background color (100)
	BgBrightBlack(arg interface{}) Value
	// BgBrightRed background color (101)
	BgBrightRed(arg interface{}) Value
	// BgBrightGreen background color (102)
	BgBrightGreen(arg interface{}) Value
	// BgBrightYellow background color (103)
	BgBrightYellow(arg interface{}) Value
	// BgBrightBlue background color (104)
	BgBrightBlue(arg interface{}) Value
	// BgBrightMagenta background color (105)
	BgBrightMagenta(arg interface{}) Value
	// BgBrightCyan background color (106)
	BgBrightCyan(arg interface{}) Value
	// BgBrightWhite background color (107)
	BgBrightWhite(arg interface{}) Value
	//
	// Other
	//
	// BgIndex of 8-bit pre-defined background color
	// from 0 to 255 (48;5;n).
	//
	//       0-  7:  standard colors (as in ESC [ 40–47 m)
	//       8- 15:  high intensity colors (as in ESC [100–107 m)
	//      16-231:  6 × 6 × 6 cube (216 colors): 16 + 36 × r + 6 × g + b (0 ≤ r, g, b ≤ 5)
	//     232-255:  grayscale from black to white in 24 steps
	//
	BgIndex(n uint8, arg interface{}) Value
	// BgGray from 0 to 23.
	BgGray(n uint8, arg interface{}) Value

	//
	// Special
	//
	// Colorize removes existing colors and
	// formats of the argument and applies given.
	Colorize(arg interface{}, color Color) Value

	//
	// Support methods
	//
	// Sprintf allows to use colored format.
	Sprintf(format interface{}, args ...interface{}) string
}

// NewAurora returns a new Aurora interface that
// will support or not support colors depending
// the enableColors argument
func NewAurora(enableColors bool) Aurora {
	if enableColors {
		return aurora{}
	}
	return auroraClear{}
}

// no colors

type auroraClear struct{}

func (auroraClear) Reset(arg interface{}) Value { return valueClear{arg} }

func (auroraClear) Bold(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Faint(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) DoublyUnderline(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Fraktur(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Italic(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Underline(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) SlowBlink(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) RapidBlink(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Blink(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Reverse(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Inverse(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Conceal(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Hidden(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) CrossedOut(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) StrikeThrough(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Framed(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Encircled(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Overlined(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Black(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Red(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Green(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Yellow(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Brown(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Blue(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Magenta(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Cyan(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) White(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BrightBlack(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BrightRed(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BrightGreen(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BrightYellow(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BrightBlue(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BrightMagenta(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BrightCyan(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BrightWhite(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Index(_ uint8, arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Gray(_ uint8, arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBlack(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgRed(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgGreen(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgYellow(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBrown(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBlue(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgMagenta(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgCyan(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgWhite(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBrightBlack(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBrightRed(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBrightGreen(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBrightYellow(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBrightBlue(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBrightMagenta(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBrightCyan(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgBrightWhite(arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgIndex(_ uint8, arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) BgGray(_ uint8, arg interface{}) Value {
	return valueClear{arg}
}

func (auroraClear) Colorize(arg interface{}, _ Color) Value {
	return valueClear{arg}
}

func (auroraClear) Sprintf(format interface{}, args ...interface{}) string {
	if str, ok := format.(string); ok {
		return fmt.Sprintf(str, args...)
	}
	return fmt.Sprintf(fmt.Sprint(format), args...)
}

// colorized

type aurora struct{}

func (aurora) Reset(arg interface{}) Value {
	return Reset(arg)
}

func (aurora) Bold(arg interface{}) Value {
	return Bold(arg)
}

func (aurora) Faint(arg interface{}) Value {
	return Faint(arg)
}

func (aurora) DoublyUnderline(arg interface{}) Value {
	return DoublyUnderline(arg)
}

func (aurora) Fraktur(arg interface{}) Value {
	return Fraktur(arg)
}

func (aurora) Italic(arg interface{}) Value {
	return Italic(arg)
}

func (aurora) Underline(arg interface{}) Value {
	return Underline(arg)
}

func (aurora) SlowBlink(arg interface{}) Value {
	return SlowBlink(arg)
}

func (aurora) RapidBlink(arg interface{}) Value {
	return RapidBlink(arg)
}

func (aurora) Blink(arg interface{}) Value {
	return Blink(arg)
}

func (aurora) Reverse(arg interface{}) Value {
	return Reverse(arg)
}

func (aurora) Inverse(arg interface{}) Value {
	return Inverse(arg)
}

func (aurora) Conceal(arg interface{}) Value {
	return Conceal(arg)
}

func (aurora) Hidden(arg interface{}) Value {
	return Hidden(arg)
}

func (aurora) CrossedOut(arg interface{}) Value {
	return CrossedOut(arg)
}

func (aurora) StrikeThrough(arg interface{}) Value {
	return StrikeThrough(arg)
}

func (aurora) Framed(arg interface{}) Value {
	return Framed(arg)
}

func (aurora) Encircled(arg interface{}) Value {
	return Encircled(arg)
}

func (aurora) Overlined(arg interface{}) Value {
	return Overlined(arg)
}

func (aurora) Black(arg interface{}) Value {
	return Black(arg)
}

func (aurora) Red(arg interface{}) Value {
	return Red(arg)
}

func (aurora) Green(arg interface{}) Value {
	return Green(arg)
}

func (aurora) Yellow(arg interface{}) Value {
	return Yellow(arg)
}

func (aurora) Brown(arg interface{}) Value {
	return Brown(arg)
}

func (aurora) Blue(arg interface{}) Value {
	return Blue(arg)
}

func (aurora) Magenta(arg interface{}) Value {
	return Magenta(arg)
}

func (aurora) Cyan(arg interface{}) Value {
	return Cyan(arg)
}

func (aurora) White(arg interface{}) Value {
	return White(arg)
}

func (aurora) BrightBlack(arg interface{}) Value {
	return BrightBlack(arg)
}

func (aurora) BrightRed(arg interface{}) Value {
	return BrightRed(arg)
}

func (aurora) BrightGreen(arg interface{}) Value {
	return BrightGreen(arg)
}

func (aurora) BrightYellow(arg interface{}) Value {
	return BrightYellow(arg)
}

func (aurora) BrightBlue(arg interface{}) Value {
	return BrightBlue(arg)
}

func (aurora) BrightMagenta(arg interface{}) Value {
	return BrightMagenta(arg)
}

func (aurora) BrightCyan(arg interface{}) Value {
	return BrightCyan(arg)
}

func (aurora) BrightWhite(arg interface{}) Value {
	return BrightWhite(arg)
}

func (aurora) Index(index uint8, arg interface{}) Value {
	return Index(index, arg)
}

func (aurora) Gray(n uint8, arg interface{}) Value {
	return Gray(n, arg)
}

func (aurora) BgBlack(arg interface{}) Value {
	return BgBlack(arg)
}

func (aurora) BgRed(arg interface{}) Value {
	return BgRed(arg)
}

func (aurora) BgGreen(arg interface{}) Value {
	return BgGreen(arg)
}

func (aurora) BgYellow(arg interface{}) Value {
	return BgYellow(arg)
}

func (aurora) BgBrown(arg interface{}) Value {
	return BgBrown(arg)
}

func (aurora) BgBlue(arg interface{}) Value {
	return BgBlue(arg)
}

func (aurora) BgMagenta(arg interface{}) Value {
	return BgMagenta(arg)
}

func (aurora) BgCyan(arg interface{}) Value {
	return BgCyan(arg)
}

func (aurora) BgWhite(arg interface{}) Value {
	return BgWhite(arg)
}

func (aurora) BgBrightBlack(arg interface{}) Value {
	return BgBrightBlack(arg)
}

func (aurora) BgBrightRed(arg interface{}) Value {
	return BgBrightRed(arg)
}

func (aurora) BgBrightGreen(arg interface{}) Value {
	return BgBrightGreen(arg)
}

func (aurora) BgBrightYellow(arg interface{}) Value {
	return BgBrightYellow(arg)
}

func (aurora) BgBrightBlue(arg interface{}) Value {
	return BgBrightBlue(arg)
}

func (aurora) BgBrightMagenta(arg interface{}) Value {
	return BgBrightMagenta(arg)
}

func (aurora) BgBrightCyan(arg interface{}) Value {
	return BgBrightCyan(arg)
}

func (aurora) BgBrightWhite(arg interface{}) Value {
	return BgBrightWhite(arg)
}

func (aurora) BgIndex(n uint8, arg interface{}) Value {
	return BgIndex(n, arg)
}

func (aurora) BgGray(n uint8, arg interface{}) Value {
	return BgGray(n, arg)
}

func (aurora) Colorize(arg interface{}, color Color) Value {
	return Colorize(arg, color)
}

func (aurora) Sprintf(format interface{}, args ...interface{}) string {
	return Sprintf(format, args...)
}
