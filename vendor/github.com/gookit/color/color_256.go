package color

import (
	"fmt"
	"strings"
)

/*
from wikipedia, 256 color:
   ESC[ … 38;5;<n> … m选择前景色
   ESC[ … 48;5;<n> … m选择背景色
     0-  7：标准颜色（同 ESC[30–37m）
     8- 15：高强度颜色（同 ESC[90–97m）
    16-231：6 × 6 × 6 立方（216色）: 16 + 36 × r + 6 × g + b (0 ≤ r, g, b ≤ 5)
   232-255：从黑到白的24阶灰度色
*/

// tpl for 8 bit 256 color(`2^8`)
//
// format:
// 	ESC[ … 38;5;<n> … m // 选择前景色
//  ESC[ … 48;5;<n> … m // 选择背景色
//
// example:
//  fg "\x1b[38;5;242m"
//  bg "\x1b[48;5;208m"
//  both "\x1b[38;5;242;48;5;208m"
//
// links:
// 	https://zh.wikipedia.org/wiki/ANSI%E8%BD%AC%E4%B9%89%E5%BA%8F%E5%88%97#8位
const (
	TplFg256 = "38;5;%d"
	TplBg256 = "48;5;%d"
)

/*************************************************************
 * 8bit(256) Color: Bit8Color Color256
 *************************************************************/

// Color256 256 (8 bit) color, uint8 range at 0 - 255
//
// 颜色值使用10进制和16进制都可 0x98 = 152
//
// 颜色有两位uint8组成,
// 	0: color value
// 	1: color type, Fg=0 Bg=1
// 	>1: unset value
// 	fg color: [152, 0]
//  bg color: [152, 1]
type Color256 [2]uint8

// Bit8 create a color256
func Bit8(val uint8, isBg ...bool) Color256 {
	return C256(val, isBg...)
}

// C256 create a color256
func C256(val uint8, isBg ...bool) Color256 {
	bc := Color256{val}

	// mark is bg color
	if len(isBg) > 0 && isBg[0] {
		bc[1] = AsBg
	}

	return bc
}

// Print print message
func (c Color256) Print(a ...interface{}) {
	fmt.Print(RenderCode(c.String(), a...))
}

// Printf format and print message
func (c Color256) Printf(format string, a ...interface{}) {
	fmt.Print(RenderString(c.String(), fmt.Sprintf(format, a...)))
}

// Println print message with newline
func (c Color256) Println(a ...interface{}) {
	fmt.Println(RenderString(c.String(), formatArgsForPrintln(a)))
}

// Sprint returns rendered message
func (c Color256) Sprint(a ...interface{}) string {
	return RenderCode(c.String(), a...)
}

// Sprintf returns format and rendered message
func (c Color256) Sprintf(format string, a ...interface{}) string {
	return RenderString(c.String(), fmt.Sprintf(format, a...))
}

// Value return color value
func (c Color256) Value() uint8 {
	return c[0]
}

// String convert to color code string.
func (c Color256) String() string {
	if c[1] == AsFg { // 0 is Fg
		return fmt.Sprintf(TplFg256, c[0])
	}

	if c[1] == AsBg { // 1 is Bg
		return fmt.Sprintf(TplBg256, c[0])
	}

	// empty
	return ""
}

// IsEmpty value
func (c Color256) IsEmpty() bool {
	return c[1] > 1
}

/*************************************************************
 * 8bit(256) Style
 *************************************************************/

// Style256 definition
//
// 前/背景色
// 都是由两位uint8组成, 第一位是色彩值；
// 第二位与Bit8Color不一样的是，在这里表示是否设置了值 0 未设置 ^0 已设置
type Style256 struct {
	p *Printer
	// Name of the style
	Name string
	// fg and bg color
	fg, bg Color256
}

// S256 create a color256 style
// Usage:
// 	s := color.S256()
// 	s := color.S256(132) // fg
// 	s := color.S256(132, 203) // fg and bg
func S256(fgAndBg ...uint8) *Style256 {
	s := &Style256{}
	vl := len(fgAndBg)
	if vl > 0 { // with fg
		s.fg = Color256{fgAndBg[0], 1}

		if vl > 1 { // and with bg
			s.bg = Color256{fgAndBg[1], 1}
		}
	}

	return s
}

// Set fg and bg color value
func (s *Style256) Set(fgVal, bgVal uint8) *Style256 {
	s.fg = Color256{fgVal, 1}
	s.bg = Color256{bgVal, 1}
	return s
}

// SetBg set bg color value
func (s *Style256) SetBg(bgVal uint8) *Style256 {
	s.bg = Color256{bgVal, 1}
	return s
}

// SetFg set fg color value
func (s *Style256) SetFg(fgVal uint8) *Style256 {
	s.fg = Color256{fgVal, 1}
	return s
}

// Print print message
func (s *Style256) Print(a ...interface{}) {
	fmt.Print(RenderCode(s.String(), a...))
}

// Printf format and print message
func (s *Style256) Printf(format string, a ...interface{}) {
	fmt.Print(RenderString(s.String(), fmt.Sprintf(format, a...)))
}

// Println print message with newline
func (s *Style256) Println(a ...interface{}) {
	fmt.Println(RenderString(s.String(), formatArgsForPrintln(a)))
}

// Sprint returns rendered message
func (s *Style256) Sprint(a ...interface{}) string {
	return RenderCode(s.Code(), a...)
}

// Sprintf returns format and rendered message
func (s *Style256) Sprintf(format string, a ...interface{}) string {
	return RenderString(s.Code(), fmt.Sprintf(format, a...))
}

// Code convert to color code string
func (s *Style256) Code() string {
	return s.String()
}

// String convert to color code string
func (s *Style256) String() string {
	var ss []string
	if s.fg[1] > 0 {
		ss = append(ss, fmt.Sprintf(TplFg256, s.fg[0]))
	}

	if s.bg[1] > 0 {
		ss = append(ss, fmt.Sprintf(TplBg256, s.bg[0]))
	}

	return strings.Join(ss, ";")
}
