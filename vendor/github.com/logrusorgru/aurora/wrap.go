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

// Colorize wraps given value into Value with
// given colors. For example
//
//    s := Colorize("some", BlueFg|GreenBg|BoldFm)
//
// returns a Value with blue foreground, green
// background and bold. Unlike functions like
// Red/BgBlue/Bold etc. This function clears
// all previous colors and formats. Thus
//
//    s := Colorize(Red("some"), BgBlue)
//
// clears red color from value
func Colorize(arg interface{}, color Color) Value {
	if val, ok := arg.(value); ok {
		val.color = color
		return val
	}
	return value{arg, color, 0}
}

// Reset wraps given argument returning Value
// without formats and colors.
func Reset(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Reset()
	}
	return value{value: arg}
}

//
// Formats
//

// Bold or increased intensity (1).
func Bold(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Bold()
	}
	return value{value: arg, color: BoldFm}
}

// Faint decreases intensity (2).
// The Faint rejects the Bold.
func Faint(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Faint()
	}
	return value{value: arg, color: FaintFm}
}

// DoublyUnderline or Bold off, double-underline
// per ECMA-48 (21).
func DoublyUnderline(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.DoublyUnderline()
	}
	return value{value: arg, color: DoublyUnderlineFm}
}

// Fraktur is rarely supported (20).
func Fraktur(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Fraktur()
	}
	return value{value: arg, color: FrakturFm}
}

// Italic is not widely supported, sometimes
// treated as inverse (3).
func Italic(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Italic()
	}
	return value{value: arg, color: ItalicFm}
}

// Underline (4).
func Underline(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Underline()
	}
	return value{value: arg, color: UnderlineFm}
}

// SlowBlink makes text blink less than
// 150 per minute (5).
func SlowBlink(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.SlowBlink()
	}
	return value{value: arg, color: SlowBlinkFm}
}

// RapidBlink makes text blink 150+ per
// minute. It is not widely supported (6).
func RapidBlink(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.RapidBlink()
	}
	return value{value: arg, color: RapidBlinkFm}
}

// Blink is alias for the SlowBlink.
func Blink(arg interface{}) Value {
	return SlowBlink(arg)
}

// Reverse video, swap foreground and
// background colors (7).
func Reverse(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Reverse()
	}
	return value{value: arg, color: ReverseFm}
}

// Inverse is alias for the Reverse
func Inverse(arg interface{}) Value {
	return Reverse(arg)
}

// Conceal hides text, preserving an ability to select
// the text and copy it. It is not widely supported (8).
func Conceal(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Conceal()
	}
	return value{value: arg, color: ConcealFm}
}

// Hidden is alias for the Conceal
func Hidden(arg interface{}) Value {
	return Conceal(arg)
}

// CrossedOut makes characters legible, but
// marked for deletion (9).
func CrossedOut(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.CrossedOut()
	}
	return value{value: arg, color: CrossedOutFm}
}

// StrikeThrough is alias for the CrossedOut.
func StrikeThrough(arg interface{}) Value {
	return CrossedOut(arg)
}

// Framed (51).
func Framed(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Framed()
	}
	return value{value: arg, color: FramedFm}
}

// Encircled (52).
func Encircled(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Encircled()
	}
	return value{value: arg, color: EncircledFm}
}

// Overlined (53).
func Overlined(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Overlined()
	}
	return value{value: arg, color: OverlinedFm}
}

//
// Foreground colors
//
//

// Black foreground color (30)
func Black(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Black()
	}
	return value{value: arg, color: BlackFg}
}

// Red foreground color (31)
func Red(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Red()
	}
	return value{value: arg, color: RedFg}
}

// Green foreground color (32)
func Green(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Green()
	}
	return value{value: arg, color: GreenFg}
}

// Yellow foreground color (33)
func Yellow(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Yellow()
	}
	return value{value: arg, color: YellowFg}
}

// Brown foreground color (33)
//
// Deprecated: use Yellow instead, following specification
func Brown(arg interface{}) Value {
	return Yellow(arg)
}

// Blue foreground color (34)
func Blue(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Blue()
	}
	return value{value: arg, color: BlueFg}
}

// Magenta foreground color (35)
func Magenta(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Magenta()
	}
	return value{value: arg, color: MagentaFg}
}

// Cyan foreground color (36)
func Cyan(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Cyan()
	}
	return value{value: arg, color: CyanFg}
}

// White foreground color (37)
func White(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.White()
	}
	return value{value: arg, color: WhiteFg}
}

//
// Bright foreground colors
//

// BrightBlack foreground color (90)
func BrightBlack(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BrightBlack()
	}
	return value{value: arg, color: BrightFg | BlackFg}
}

// BrightRed foreground color (91)
func BrightRed(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BrightRed()
	}
	return value{value: arg, color: BrightFg | RedFg}
}

// BrightGreen foreground color (92)
func BrightGreen(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BrightGreen()
	}
	return value{value: arg, color: BrightFg | GreenFg}
}

// BrightYellow foreground color (93)
func BrightYellow(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BrightYellow()
	}
	return value{value: arg, color: BrightFg | YellowFg}
}

// BrightBlue foreground color (94)
func BrightBlue(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BrightBlue()
	}
	return value{value: arg, color: BrightFg | BlueFg}
}

// BrightMagenta foreground color (95)
func BrightMagenta(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BrightMagenta()
	}
	return value{value: arg, color: BrightFg | MagentaFg}
}

// BrightCyan foreground color (96)
func BrightCyan(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BrightCyan()
	}
	return value{value: arg, color: BrightFg | CyanFg}
}

// BrightWhite foreground color (97)
func BrightWhite(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BrightWhite()
	}
	return value{value: arg, color: BrightFg | WhiteFg}
}

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
func Index(n uint8, arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Index(n)
	}
	return value{value: arg, color: (Color(n) << shiftFg) | flagFg}
}

// Gray from 0 to 24.
func Gray(n uint8, arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.Gray(n)
	}
	if n > 23 {
		n = 23
	}
	return value{value: arg, color: (Color(232+n) << shiftFg) | flagFg}
}

//
// Background colors
//
//

// BgBlack background color (40)
func BgBlack(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgBlack()
	}
	return value{value: arg, color: BlackBg}
}

// BgRed background color (41)
func BgRed(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgRed()
	}
	return value{value: arg, color: RedBg}
}

// BgGreen background color (42)
func BgGreen(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgGreen()
	}
	return value{value: arg, color: GreenBg}
}

// BgYellow background color (43)
func BgYellow(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgYellow()
	}
	return value{value: arg, color: YellowBg}
}

// BgBrown background color (43)
//
// Deprecated: use BgYellow instead, following specification
func BgBrown(arg interface{}) Value {
	return BgYellow(arg)
}

// BgBlue background color (44)
func BgBlue(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgBlue()
	}
	return value{value: arg, color: BlueBg}
}

// BgMagenta background color (45)
func BgMagenta(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgMagenta()
	}
	return value{value: arg, color: MagentaBg}
}

// BgCyan background color (46)
func BgCyan(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgCyan()
	}
	return value{value: arg, color: CyanBg}
}

// BgWhite background color (47)
func BgWhite(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgWhite()
	}
	return value{value: arg, color: WhiteBg}
}

//
// Bright background colors
//

// BgBrightBlack background color (100)
func BgBrightBlack(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgBrightBlack()
	}
	return value{value: arg, color: BrightBg | BlackBg}
}

// BgBrightRed background color (101)
func BgBrightRed(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgBrightRed()
	}
	return value{value: arg, color: BrightBg | RedBg}
}

// BgBrightGreen background color (102)
func BgBrightGreen(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgBrightGreen()
	}
	return value{value: arg, color: BrightBg | GreenBg}
}

// BgBrightYellow background color (103)
func BgBrightYellow(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgBrightYellow()
	}
	return value{value: arg, color: BrightBg | YellowBg}
}

// BgBrightBlue background color (104)
func BgBrightBlue(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgBrightBlue()
	}
	return value{value: arg, color: BrightBg | BlueBg}
}

// BgBrightMagenta background color (105)
func BgBrightMagenta(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgBrightMagenta()
	}
	return value{value: arg, color: BrightBg | MagentaBg}
}

// BgBrightCyan background color (106)
func BgBrightCyan(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgBrightCyan()
	}
	return value{value: arg, color: BrightBg | CyanBg}
}

// BgBrightWhite background color (107)
func BgBrightWhite(arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgBrightWhite()
	}
	return value{value: arg, color: BrightBg | WhiteBg}
}

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
func BgIndex(n uint8, arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgIndex(n)
	}
	return value{value: arg, color: (Color(n) << shiftBg) | flagBg}
}

// BgGray from 0 to 24.
func BgGray(n uint8, arg interface{}) Value {
	if val, ok := arg.(Value); ok {
		return val.BgGray(n)
	}
	if n > 23 {
		n = 23
	}
	return value{value: arg, color: (Color(n+232) << shiftBg) | flagBg}
}
