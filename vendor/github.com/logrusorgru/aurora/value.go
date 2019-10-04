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

package aurora

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

// A Value represents any printable value
// with it's color
type Value interface {
	// String returns string with colors. If there are any color
	// or format the string will be terminated with \033[0m
	fmt.Stringer
	// Format implements fmt.Formatter interface
	fmt.Formatter
	// Color returns value's color
	Color() Color
	// Value returns value's value (welcome to the tautology club)
	Value() interface{}

	//  internals
	tail() Color
	setTail(Color) Value

	// Bleach returns copy of original value without colors
	//
	// Deprecated: use Reset instead.
	Bleach() Value
	// Reset colors and formats
	Reset() Value

	//
	// Formats
	//
	//
	// Bold or increased intensity (1).
	Bold() Value
	// Faint, decreased intensity, reset the Bold (2).
	Faint() Value
	//
	// DoublyUnderline or Bold off, double-underline
	// per ECMA-48 (21). It depends.
	DoublyUnderline() Value
	// Fraktur, rarely supported (20).
	Fraktur() Value
	//
	// Italic, not widely supported, sometimes
	// treated as inverse (3).
	Italic() Value
	// Underline (4).
	Underline() Value
	//
	// SlowBlink, blinking less than 150
	// per minute (5).
	SlowBlink() Value
	// RapidBlink, blinking 150+ per minute,
	// not widely supported (6).
	RapidBlink() Value
	// Blink is alias for the SlowBlink.
	Blink() Value
	//
	// Reverse video, swap foreground and
	// background colors (7).
	Reverse() Value
	// Inverse is alias for the Reverse
	Inverse() Value
	//
	// Conceal, hidden, not widely supported (8).
	Conceal() Value
	// Hidden is alias for the Conceal
	Hidden() Value
	//
	// CrossedOut, characters legible, but
	// marked for deletion (9).
	CrossedOut() Value
	// StrikeThrough is alias for the CrossedOut.
	StrikeThrough() Value
	//
	// Framed (51).
	Framed() Value
	// Encircled (52).
	Encircled() Value
	//
	// Overlined (53).
	Overlined() Value

	//
	// Foreground colors
	//
	//
	// Black foreground color (30)
	Black() Value
	// Red foreground color (31)
	Red() Value
	// Green foreground color (32)
	Green() Value
	// Yellow foreground color (33)
	Yellow() Value
	// Brown foreground color (33)
	//
	// Deprecated: use Yellow instead, following specification
	Brown() Value
	// Blue foreground color (34)
	Blue() Value
	// Magenta foreground color (35)
	Magenta() Value
	// Cyan foreground color (36)
	Cyan() Value
	// White foreground color (37)
	White() Value
	//
	// Bright foreground colors
	//
	// BrightBlack foreground color (90)
	BrightBlack() Value
	// BrightRed foreground color (91)
	BrightRed() Value
	// BrightGreen foreground color (92)
	BrightGreen() Value
	// BrightYellow foreground color (93)
	BrightYellow() Value
	// BrightBlue foreground color (94)
	BrightBlue() Value
	// BrightMagenta foreground color (95)
	BrightMagenta() Value
	// BrightCyan foreground color (96)
	BrightCyan() Value
	// BrightWhite foreground color (97)
	BrightWhite() Value
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
	Index(n uint8) Value
	// Gray from 0 to 24.
	Gray(n uint8) Value

	//
	// Background colors
	//
	//
	// BgBlack background color (40)
	BgBlack() Value
	// BgRed background color (41)
	BgRed() Value
	// BgGreen background color (42)
	BgGreen() Value
	// BgYellow background color (43)
	BgYellow() Value
	// BgBrown background color (43)
	//
	// Deprecated: use BgYellow instead, following specification
	BgBrown() Value
	// BgBlue background color (44)
	BgBlue() Value
	// BgMagenta background color (45)
	BgMagenta() Value
	// BgCyan background color (46)
	BgCyan() Value
	// BgWhite background color (47)
	BgWhite() Value
	//
	// Bright background colors
	//
	// BgBrightBlack background color (100)
	BgBrightBlack() Value
	// BgBrightRed background color (101)
	BgBrightRed() Value
	// BgBrightGreen background color (102)
	BgBrightGreen() Value
	// BgBrightYellow background color (103)
	BgBrightYellow() Value
	// BgBrightBlue background color (104)
	BgBrightBlue() Value
	// BgBrightMagenta background color (105)
	BgBrightMagenta() Value
	// BgBrightCyan background color (106)
	BgBrightCyan() Value
	// BgBrightWhite background color (107)
	BgBrightWhite() Value
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
	BgIndex(n uint8) Value
	// BgGray from 0 to 24.
	BgGray(n uint8) Value

	//
	// Special
	//
	// Colorize removes existing colors and
	// formats of the argument and applies given.
	Colorize(color Color) Value
}

// Value without colors

type valueClear struct {
	value interface{}
}

func (vc valueClear) String() string { return fmt.Sprint(vc.value) }

func (vc valueClear) Color() Color       { return 0 }
func (vc valueClear) Value() interface{} { return vc.value }

func (vc valueClear) tail() Color         { return 0 }
func (vc valueClear) setTail(Color) Value { return vc }

func (vc valueClear) Bleach() Value { return vc }
func (vc valueClear) Reset() Value  { return vc }

func (vc valueClear) Bold() Value            { return vc }
func (vc valueClear) Faint() Value           { return vc }
func (vc valueClear) DoublyUnderline() Value { return vc }
func (vc valueClear) Fraktur() Value         { return vc }
func (vc valueClear) Italic() Value          { return vc }
func (vc valueClear) Underline() Value       { return vc }
func (vc valueClear) SlowBlink() Value       { return vc }
func (vc valueClear) RapidBlink() Value      { return vc }
func (vc valueClear) Blink() Value           { return vc }
func (vc valueClear) Reverse() Value         { return vc }
func (vc valueClear) Inverse() Value         { return vc }
func (vc valueClear) Conceal() Value         { return vc }
func (vc valueClear) Hidden() Value          { return vc }
func (vc valueClear) CrossedOut() Value      { return vc }
func (vc valueClear) StrikeThrough() Value   { return vc }
func (vc valueClear) Framed() Value          { return vc }
func (vc valueClear) Encircled() Value       { return vc }
func (vc valueClear) Overlined() Value       { return vc }

func (vc valueClear) Black() Value         { return vc }
func (vc valueClear) Red() Value           { return vc }
func (vc valueClear) Green() Value         { return vc }
func (vc valueClear) Yellow() Value        { return vc }
func (vc valueClear) Brown() Value         { return vc }
func (vc valueClear) Blue() Value          { return vc }
func (vc valueClear) Magenta() Value       { return vc }
func (vc valueClear) Cyan() Value          { return vc }
func (vc valueClear) White() Value         { return vc }
func (vc valueClear) BrightBlack() Value   { return vc }
func (vc valueClear) BrightRed() Value     { return vc }
func (vc valueClear) BrightGreen() Value   { return vc }
func (vc valueClear) BrightYellow() Value  { return vc }
func (vc valueClear) BrightBlue() Value    { return vc }
func (vc valueClear) BrightMagenta() Value { return vc }
func (vc valueClear) BrightCyan() Value    { return vc }
func (vc valueClear) BrightWhite() Value   { return vc }
func (vc valueClear) Index(uint8) Value    { return vc }
func (vc valueClear) Gray(uint8) Value     { return vc }

func (vc valueClear) BgBlack() Value         { return vc }
func (vc valueClear) BgRed() Value           { return vc }
func (vc valueClear) BgGreen() Value         { return vc }
func (vc valueClear) BgYellow() Value        { return vc }
func (vc valueClear) BgBrown() Value         { return vc }
func (vc valueClear) BgBlue() Value          { return vc }
func (vc valueClear) BgMagenta() Value       { return vc }
func (vc valueClear) BgCyan() Value          { return vc }
func (vc valueClear) BgWhite() Value         { return vc }
func (vc valueClear) BgBrightBlack() Value   { return vc }
func (vc valueClear) BgBrightRed() Value     { return vc }
func (vc valueClear) BgBrightGreen() Value   { return vc }
func (vc valueClear) BgBrightYellow() Value  { return vc }
func (vc valueClear) BgBrightBlue() Value    { return vc }
func (vc valueClear) BgBrightMagenta() Value { return vc }
func (vc valueClear) BgBrightCyan() Value    { return vc }
func (vc valueClear) BgBrightWhite() Value   { return vc }
func (vc valueClear) BgIndex(uint8) Value    { return vc }
func (vc valueClear) BgGray(uint8) Value     { return vc }
func (vc valueClear) Colorize(Color) Value   { return vc }

func (vc valueClear) Format(s fmt.State, verb rune) {
	// it's enough for many cases (%-+020.10f)
	// %          - 1
	// availFlags - 3 (5)
	// width      - 2
	// prec       - 3 (.23)
	// verb       - 1
	// --------------
	//             10
	format := make([]byte, 1, 10)
	format[0] = '%'
	var f byte
	for i := 0; i < len(availFlags); i++ {
		if f = availFlags[i]; s.Flag(int(f)) {
			format = append(format, f)
		}
	}
	var width, prec int
	var ok bool
	if width, ok = s.Width(); ok {
		format = strconv.AppendInt(format, int64(width), 10)
	}
	if prec, ok = s.Precision(); ok {
		format = append(format, '.')
		format = strconv.AppendInt(format, int64(prec), 10)
	}
	if verb > utf8.RuneSelf {
		format = append(format, string(verb)...)
	} else {
		format = append(format, byte(verb))
	}
	fmt.Fprintf(s, string(format), vc.value)
}

// Value within colors

type value struct {
	value     interface{} // value as it
	color     Color       // this color
	tailColor Color       // tail color
}

func (v value) String() string {
	if v.color != 0 {
		if v.tailColor != 0 {
			return esc + v.color.Nos(true) + "m" +
				fmt.Sprint(v.value) +
				esc + v.tailColor.Nos(true) + "m"
		}
		return esc + v.color.Nos(false) + "m" + fmt.Sprint(v.value) + clear
	}
	return fmt.Sprint(v.value)
}

func (v value) Color() Color { return v.color }

func (v value) Bleach() Value {
	v.color, v.tailColor = 0, 0
	return v
}

func (v value) Reset() Value {
	v.color, v.tailColor = 0, 0
	return v
}

func (v value) tail() Color { return v.tailColor }

func (v value) setTail(t Color) Value {
	v.tailColor = t
	return v
}

func (v value) Value() interface{} { return v.value }

func (v value) Format(s fmt.State, verb rune) {

	// it's enough for many cases (%-+020.10f)
	// %          - 1
	// availFlags - 3 (5)
	// width      - 2
	// prec       - 3 (.23)
	// verb       - 1
	// --------------
	//             10
	// +
	// \033[                            5
	// 0;1;3;4;5;7;8;9;20;21;51;52;53  30
	// 38;5;216                         8
	// 48;5;216                         8
	// m                                1
	// +
	// \033[0m                          7
	//
	// x2 (possible tail color)
	//
	// 10 + 59 * 2 = 128

	format := make([]byte, 0, 128)
	if v.color != 0 {
		format = append(format, esc...)
		format = v.color.appendNos(format, v.tailColor != 0)
		format = append(format, 'm')
	}
	format = append(format, '%')
	var f byte
	for i := 0; i < len(availFlags); i++ {
		if f = availFlags[i]; s.Flag(int(f)) {
			format = append(format, f)
		}
	}
	var width, prec int
	var ok bool
	if width, ok = s.Width(); ok {
		format = strconv.AppendInt(format, int64(width), 10)
	}
	if prec, ok = s.Precision(); ok {
		format = append(format, '.')
		format = strconv.AppendInt(format, int64(prec), 10)
	}
	if verb > utf8.RuneSelf {
		format = append(format, string(verb)...)
	} else {
		format = append(format, byte(verb))
	}
	if v.color != 0 {
		if v.tailColor != 0 {
			// set next (previous) format clearing current one
			format = append(format, esc...)
			format = v.tailColor.appendNos(format, true)
			format = append(format, 'm')
		} else {
			format = append(format, clear...) // just clear
		}
	}
	fmt.Fprintf(s, string(format), v.value)
}

func (v value) Bold() Value {
	v.color = (v.color &^ FaintFm) | BoldFm
	return v
}

func (v value) Faint() Value {
	v.color = (v.color &^ BoldFm) | FaintFm
	return v
}

func (v value) DoublyUnderline() Value {
	v.color |= DoublyUnderlineFm
	return v
}

func (v value) Fraktur() Value {
	v.color |= FrakturFm
	return v
}

func (v value) Italic() Value {
	v.color |= ItalicFm
	return v
}

func (v value) Underline() Value {
	v.color |= UnderlineFm
	return v
}

func (v value) SlowBlink() Value {
	v.color = (v.color &^ RapidBlinkFm) | SlowBlinkFm
	return v
}

func (v value) RapidBlink() Value {
	v.color = (v.color &^ SlowBlinkFm) | RapidBlinkFm
	return v
}

func (v value) Blink() Value {
	return v.SlowBlink()
}

func (v value) Reverse() Value {
	v.color |= ReverseFm
	return v
}

func (v value) Inverse() Value {
	return v.Reverse()
}

func (v value) Conceal() Value {
	v.color |= ConcealFm
	return v
}

func (v value) Hidden() Value {
	return v.Conceal()
}

func (v value) CrossedOut() Value {
	v.color |= CrossedOutFm
	return v
}

func (v value) StrikeThrough() Value {
	return v.CrossedOut()
}

func (v value) Framed() Value {
	v.color |= FramedFm
	return v
}

func (v value) Encircled() Value {
	v.color |= EncircledFm
	return v
}

func (v value) Overlined() Value {
	v.color |= OverlinedFm
	return v
}

func (v value) Black() Value {
	v.color = (v.color &^ maskFg) | BlackFg
	return v
}

func (v value) Red() Value {
	v.color = (v.color &^ maskFg) | RedFg
	return v
}

func (v value) Green() Value {
	v.color = (v.color &^ maskFg) | GreenFg
	return v
}

func (v value) Yellow() Value {
	v.color = (v.color &^ maskFg) | YellowFg
	return v
}

func (v value) Brown() Value {
	return v.Yellow()
}

func (v value) Blue() Value {
	v.color = (v.color &^ maskFg) | BlueFg
	return v
}

func (v value) Magenta() Value {
	v.color = (v.color &^ maskFg) | MagentaFg
	return v
}

func (v value) Cyan() Value {
	v.color = (v.color &^ maskFg) | CyanFg
	return v
}

func (v value) White() Value {
	v.color = (v.color &^ maskFg) | WhiteFg
	return v
}

func (v value) BrightBlack() Value {
	v.color = (v.color &^ maskFg) | BrightFg | BlackFg
	return v
}

func (v value) BrightRed() Value {
	v.color = (v.color &^ maskFg) | BrightFg | RedFg
	return v
}

func (v value) BrightGreen() Value {
	v.color = (v.color &^ maskFg) | BrightFg | GreenFg
	return v
}

func (v value) BrightYellow() Value {
	v.color = (v.color &^ maskFg) | BrightFg | YellowFg
	return v
}

func (v value) BrightBlue() Value {
	v.color = (v.color &^ maskFg) | BrightFg | BlueFg
	return v
}

func (v value) BrightMagenta() Value {
	v.color = (v.color &^ maskFg) | BrightFg | MagentaFg
	return v
}

func (v value) BrightCyan() Value {
	v.color = (v.color &^ maskFg) | BrightFg | CyanFg
	return v
}

func (v value) BrightWhite() Value {
	v.color = (v.color &^ maskFg) | BrightFg | WhiteFg
	return v
}

func (v value) Index(n uint8) Value {
	v.color = (v.color &^ maskFg) | (Color(n) << shiftFg) | flagFg
	return v
}

func (v value) Gray(n uint8) Value {
	if n > 23 {
		n = 23
	}
	v.color = (v.color &^ maskFg) | (Color(232+n) << shiftFg) | flagFg
	return v
}

func (v value) BgBlack() Value {
	v.color = (v.color &^ maskBg) | BlackBg
	return v
}

func (v value) BgRed() Value {
	v.color = (v.color &^ maskBg) | RedBg
	return v
}

func (v value) BgGreen() Value {
	v.color = (v.color &^ maskBg) | GreenBg
	return v
}

func (v value) BgYellow() Value {
	v.color = (v.color &^ maskBg) | YellowBg
	return v
}

func (v value) BgBrown() Value {
	return v.BgYellow()
}

func (v value) BgBlue() Value {
	v.color = (v.color &^ maskBg) | BlueBg
	return v
}

func (v value) BgMagenta() Value {
	v.color = (v.color &^ maskBg) | MagentaBg
	return v
}

func (v value) BgCyan() Value {
	v.color = (v.color &^ maskBg) | CyanBg
	return v
}

func (v value) BgWhite() Value {
	v.color = (v.color &^ maskBg) | WhiteBg
	return v
}

func (v value) BgBrightBlack() Value {
	v.color = (v.color &^ maskBg) | BrightBg | BlackBg
	return v
}

func (v value) BgBrightRed() Value {
	v.color = (v.color &^ maskBg) | BrightBg | RedBg
	return v
}

func (v value) BgBrightGreen() Value {
	v.color = (v.color &^ maskBg) | BrightBg | GreenBg
	return v
}

func (v value) BgBrightYellow() Value {
	v.color = (v.color &^ maskBg) | BrightBg | YellowBg
	return v
}

func (v value) BgBrightBlue() Value {
	v.color = (v.color &^ maskBg) | BrightBg | BlueBg
	return v
}

func (v value) BgBrightMagenta() Value {
	v.color = (v.color &^ maskBg) | BrightBg | MagentaBg
	return v
}

func (v value) BgBrightCyan() Value {
	v.color = (v.color &^ maskBg) | BrightBg | CyanBg
	return v
}

func (v value) BgBrightWhite() Value {
	v.color = (v.color &^ maskBg) | BrightBg | WhiteBg
	return v
}

func (v value) BgIndex(n uint8) Value {
	v.color = (v.color &^ maskBg) | (Color(n) << shiftBg) | flagBg
	return v
}

func (v value) BgGray(n uint8) Value {
	if n > 23 {
		n = 23
	}
	v.color = (v.color &^ maskBg) | (Color(232+n) << shiftBg) | flagBg
	return v
}

func (v value) Colorize(color Color) Value {
	v.color = color
	return v
}
