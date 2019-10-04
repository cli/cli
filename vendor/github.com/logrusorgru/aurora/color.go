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

// A Color type is a color. It can contain
// one background color, one foreground color
// and a format, including ideogram related
// formats.
type Color uint

/*

	Developer note.

	The int type is architecture depended and can be
	represented as int32 or int64.

	Thus, we can use 32-bits only to be fast and
	cross-platform.

	All supported formats requires 14 bits. It is
	first 14 bits.

	A foreground color requires 8 bit + 1 bit (presence flag).
	And the same for background color.

	The Color representations

	[ bg 8 bit ] [fg 8 bit ] [ fg/bg 2 bits ] [ fm 14 bits ]

	https://play.golang.org/p/fq2zcNstFoF

*/

// Special formats
const (
	BoldFm       Color = 1 << iota // 1
	FaintFm                        // 2
	ItalicFm                       // 3
	UnderlineFm                    // 4
	SlowBlinkFm                    // 5
	RapidBlinkFm                   // 6
	ReverseFm                      // 7
	ConcealFm                      // 8
	CrossedOutFm                   // 9

	FrakturFm         // 20
	DoublyUnderlineFm // 21 or bold off for some systems

	FramedFm    // 51
	EncircledFm // 52
	OverlinedFm // 53

	InverseFm       = ReverseFm    // alias to ReverseFm
	BlinkFm         = SlowBlinkFm  // alias to SlowBlinkFm
	HiddenFm        = ConcealFm    // alias to ConcealFm
	StrikeThroughFm = CrossedOutFm // alias to CrossedOutFm

	maskFm = BoldFm | FaintFm |
		ItalicFm | UnderlineFm |
		SlowBlinkFm | RapidBlinkFm |
		ReverseFm |
		ConcealFm | CrossedOutFm |

		FrakturFm | DoublyUnderlineFm |

		FramedFm | EncircledFm | OverlinedFm

	flagFg Color = 1 << 14 // presence flag (14th bit)
	flagBg Color = 1 << 15 // presence flag (15th bit)

	shiftFg = 16 // shift for foreground (starting from 16th bit)
	shiftBg = 24 // shift for background (starting from 24th bit)
)

// Foreground colors and related formats
const (

	// 8 bits

	// [  0;   7] - 30-37
	// [  8;  15] - 90-97
	// [ 16; 231] - RGB
	// [232; 255] - grayscale

	BlackFg   Color = (iota << shiftFg) | flagFg // 30, 90
	RedFg                                        // 31, 91
	GreenFg                                      // 32, 92
	YellowFg                                     // 33, 93
	BlueFg                                       // 34, 94
	MagentaFg                                    // 35, 95
	CyanFg                                       // 36, 96
	WhiteFg                                      // 37, 97

	BrightFg Color = ((1 << 3) << shiftFg) | flagFg // -> 90

	// the BrightFg itself doesn't represent
	// a color, thus it has not flagFg

	// 5 bits

	// BrownFg represents brown foreground color.
	//
	// Deprecated: use YellowFg instead, following specifications
	BrownFg = YellowFg

	//
	maskFg = (0xff << shiftFg) | flagFg
)

// Background colors and related formats
const (

	// 8 bits

	// [  0;   7] - 40-47
	// [  8;  15] - 100-107
	// [ 16; 231] - RGB
	// [232; 255] - grayscale

	BlackBg   Color = (iota << shiftBg) | flagBg // 40, 100
	RedBg                                        // 41, 101
	GreenBg                                      // 42, 102
	YellowBg                                     // 43, 103
	BlueBg                                       // 44, 104
	MagentaBg                                    // 45, 105
	CyanBg                                       // 46, 106
	WhiteBg                                      // 47, 107

	BrightBg Color = ((1 << 3) << shiftBg) | flagBg // -> 100

	// the BrightBg itself doesn't represent
	// a color, thus it has not flagBg

	// 5 bits

	// BrownBg represents brown foreground color.
	//
	// Deprecated: use YellowBg instead, following specifications
	BrownBg = YellowBg

	//
	maskBg = (0xff << shiftBg) | flagBg
)

const (
	availFlags = "-+# 0"
	esc        = "\033["
	clear      = esc + "0m"
)

// IsValid returns true always
//
// Deprecated: don't use this method anymore
func (c Color) IsValid() bool {
	return true
}

// Nos returns string like 1;7;31;45. It
// may be an empty string for empty color.
// If the zero is true, then the string
// is prepended with 0;
func (c Color) Nos(zero bool) string {
	return string(c.appendNos(make([]byte, 0, 59), zero))
}

func appendCond(bs []byte, cond, semi bool, vals ...byte) []byte {
	if !cond {
		return bs
	}
	return appendSemi(bs, semi, vals...)
}

// if the semi is true, then prepend with semicolon
func appendSemi(bs []byte, semi bool, vals ...byte) []byte {
	if semi {
		bs = append(bs, ';')
	}
	return append(bs, vals...)
}

func itoa(t byte) string {
	var (
		a [3]byte
		j = 2
	)
	for i := 0; i < 3; i, j = i+1, j-1 {
		a[j] = '0' + t%10
		if t = t / 10; t == 0 {
			break
		}
	}
	return string(a[j:])
}

func (c Color) appendFg(bs []byte, zero bool) []byte {

	if zero || c&maskFm != 0 {
		bs = append(bs, ';')
	}

	// 0- 7 :  30-37
	// 8-15 :  90-97
	// > 15 : 38;5;val

	switch fg := (c & maskFg) >> shiftFg; {
	case fg <= 7:
		// '3' and the value itself
		bs = append(bs, '3', '0'+byte(fg))
	case fg <= 15:
		// '9' and the value itself
		bs = append(bs, '9', '0'+byte(fg&^0x08)) // clear bright flag
	default:
		bs = append(bs, '3', '8', ';', '5', ';')
		bs = append(bs, itoa(byte(fg))...)
	}
	return bs
}

func (c Color) appendBg(bs []byte, zero bool) []byte {

	if zero || c&(maskFm|maskFg) != 0 {
		bs = append(bs, ';')
	}

	// 0- 7 :  40- 47
	// 8-15 : 100-107
	// > 15 : 48;5;val

	switch fg := (c & maskBg) >> shiftBg; {
	case fg <= 7:
		// '3' and the value itself
		bs = append(bs, '4', '0'+byte(fg))
	case fg <= 15:
		// '1', '0' and the value itself
		bs = append(bs, '1', '0', '0'+byte(fg&^0x08)) // clear bright flag
	default:
		bs = append(bs, '4', '8', ';', '5', ';')
		bs = append(bs, itoa(byte(fg))...)
	}
	return bs
}

func (c Color) appendFm9(bs []byte, zero bool) []byte {

	bs = appendCond(bs, c&ItalicFm != 0,
		zero || c&(BoldFm|FaintFm) != 0,
		'3')
	bs = appendCond(bs, c&UnderlineFm != 0,
		zero || c&(BoldFm|FaintFm|ItalicFm) != 0,
		'4')
	// don't combine slow and rapid blink using only
	// on of them, preferring slow blink
	if c&SlowBlinkFm != 0 {
		bs = appendSemi(bs,
			zero || c&(BoldFm|FaintFm|ItalicFm|UnderlineFm) != 0,
			'5')
	} else if c&RapidBlinkFm != 0 {
		bs = appendSemi(bs,
			zero || c&(BoldFm|FaintFm|ItalicFm|UnderlineFm) != 0,
			'6')
	}

	// including 1-2
	const mask6i = BoldFm | FaintFm |
		ItalicFm | UnderlineFm |
		SlowBlinkFm | RapidBlinkFm

	bs = appendCond(bs, c&ReverseFm != 0,
		zero || c&(mask6i) != 0,
		'7')
	bs = appendCond(bs, c&ConcealFm != 0,
		zero || c&(mask6i|ReverseFm) != 0,
		'8')
	bs = appendCond(bs, c&CrossedOutFm != 0,
		zero || c&(mask6i|ReverseFm|ConcealFm) != 0,
		'9')

	return bs
}

// append 1;3;38;5;216 like string that represents ANSI
// color of the Color; the zero argument requires
// appending of '0' before to reset previous format
// and colors
func (c Color) appendNos(bs []byte, zero bool) []byte {

	if zero {
		bs = append(bs, '0') // reset previous
	}

	// formats
	//

	if c&maskFm != 0 {

		// 1-2

		// don't combine bold and faint using only on of them, preferring bold

		if c&BoldFm != 0 {
			bs = appendSemi(bs, zero, '1')
		} else if c&FaintFm != 0 {
			bs = appendSemi(bs, zero, '2')
		}

		// 3-9

		const mask9 = ItalicFm | UnderlineFm |
			SlowBlinkFm | RapidBlinkFm |
			ReverseFm | ConcealFm | CrossedOutFm

		if c&mask9 != 0 {
			bs = c.appendFm9(bs, zero)
		}

		// 20-21

		const (
			mask21 = FrakturFm | DoublyUnderlineFm
			mask9i = BoldFm | FaintFm | mask9
		)

		if c&mask21 != 0 {
			bs = appendCond(bs, c&FrakturFm != 0,
				zero || c&mask9i != 0,
				'2', '0')
			bs = appendCond(bs, c&DoublyUnderlineFm != 0,
				zero || c&(mask9i|FrakturFm) != 0,
				'2', '1')
		}

		// 50-53

		const (
			mask53  = FramedFm | EncircledFm | OverlinedFm
			mask21i = mask9i | mask21
		)

		if c&mask53 != 0 {
			bs = appendCond(bs, c&FramedFm != 0,
				zero || c&mask21i != 0,
				'5', '1')
			bs = appendCond(bs, c&EncircledFm != 0,
				zero || c&(mask21i|FramedFm) != 0,
				'5', '2')
			bs = appendCond(bs, c&OverlinedFm != 0,
				zero || c&(mask21i|FramedFm|EncircledFm) != 0,
				'5', '3')
		}

	}

	// foreground
	if c&maskFg != 0 {
		bs = c.appendFg(bs, zero)
	}

	// background
	if c&maskBg != 0 {
		bs = c.appendBg(bs, zero)
	}

	return bs
}
