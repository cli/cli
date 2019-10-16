package color

import (
	"fmt"
	"strconv"
	"strings"
)

// 24 bit RGB color
// RGB:
// 	R 0-255 G 0-255 B 0-255
// 	R 00-FF G 00-FF B 00-FF (16进制)
//
// Format:
// 	ESC[ … 38;2;<r>;<g>;<b> … m // 选择RGB前景色
// 	ESC[ … 48;2;<r>;<g>;<b> … m // 选择RGB背景色
//
// links:
// 	https://zh.wikipedia.org/wiki/ANSI%E8%BD%AC%E4%B9%89%E5%BA%8F%E5%88%97#24位
//
// example:
// 	fg: \x1b[38;2;30;144;255mMESSAGE\x1b[0m
// 	bg: \x1b[48;2;30;144;255mMESSAGE\x1b[0m
// 	both: \x1b[38;2;233;90;203;48;2;30;144;255mMESSAGE\x1b[0m
const (
	TplFgRGB = "38;2;%d;%d;%d"
	TplBgRGB = "48;2;%d;%d;%d"
)

// mark color is fg or bg.
const (
	AsFg uint8 = iota
	AsBg
)

/*************************************************************
 * RGB Color(Bit24Color, TrueColor)
 *************************************************************/

// RGBColor definition.
//
// The first to third digits represent the color value.
// The last digit represents the foreground(0), background(1), >1 is unset value
//
// Usage:
// 	// 0, 1, 2 is R,G,B.
// 	// 3th: Fg=0, Bg=1, >1: unset value
// 	RGBColor{30,144,255, 0}
// 	RGBColor{30,144,255, 1}
type RGBColor [4]uint8

// create a empty RGBColor
var emptyRGBColor = RGBColor{3: 99}

// RGB color create.
// Usage:
// 	c := RGB(30,144,255)
// 	c := RGB(30,144,255, true)
// 	c.Print("message")
func RGB(r, g, b uint8, isBg ...bool) RGBColor {
	rgb := RGBColor{r, g, b}
	if len(isBg) > 0 && isBg[0] {
		rgb[3] = AsBg
	}

	return rgb
}

// HEX create RGB color from a HEX color string.
// Usage:
// 	c := HEX("ccc") // rgb: [204 204 204]
// 	c := HEX("aabbcc") // rgb: [170 187 204]
// 	c := HEX("#aabbcc")
// 	c := HEX("0xaabbcc")
// 	c.Print("message")
func HEX(hex string, isBg ...bool) RGBColor {
	if rgb := HexToRGB(hex); len(rgb) > 0 {
		return RGB(uint8(rgb[0]), uint8(rgb[1]), uint8(rgb[2]), isBg...)
	}

	// mark is empty
	return emptyRGBColor
}

// RGBFromString create RGB color from a string.
// Usage:
// 	c := RGBFromString("170,187,204")
// 	c.Print("message")
func RGBFromString(rgb string, isBg ...bool) RGBColor {
	ss := stringToArr(rgb, ",")
	if len(ss) != 3 {
		return emptyRGBColor
	}

	var ar [3]int
	for i, val := range ss {
		iv, err := strconv.Atoi(val)
		if err != nil {
			return emptyRGBColor
		}

		ar[i] = iv
	}

	return RGB(uint8(ar[0]), uint8(ar[1]), uint8(ar[2]), isBg...)
}

// Print print message
func (c RGBColor) Print(a ...interface{}) {
	fmt.Print(RenderCode(c.String(), a...))
}

// Printf format and print message
func (c RGBColor) Printf(format string, a ...interface{}) {
	fmt.Print(RenderString(c.String(), fmt.Sprintf(format, a...)))
}

// Println print message with newline
func (c RGBColor) Println(a ...interface{}) {
	fmt.Println(RenderString(c.String(), formatArgsForPrintln(a)))
}

// Sprint returns rendered message
func (c RGBColor) Sprint(a ...interface{}) string {
	return RenderCode(c.String(), a...)
}

// Sprintf returns format and rendered message
func (c RGBColor) Sprintf(format string, a ...interface{}) string {
	return RenderString(c.Code(), fmt.Sprintf(format, a...))
}

// Values to RGB values
func (c RGBColor) Values() []int {
	return []int{int(c[0]), int(c[1]), int(c[2])}
}

// Code to color code string
func (c RGBColor) Code() string {
	return c.String()
}

// String to color code string
func (c RGBColor) String() string {
	if c[3] == AsFg { // 0 is Fg
		return fmt.Sprintf(TplFgRGB, c[0], c[1], c[2])
	}

	if c[3] == AsBg { // 1 is Bg
		return fmt.Sprintf(TplBgRGB, c[0], c[1], c[2])

	}

	// >1 is empty
	return ""
}

// IsEmpty value
func (c RGBColor) IsEmpty() bool {
	return c[3] > 1
}

// HexToRGB hex color string to RGB numbers
// Usage:
// 	rgb := HexToRGB("ccc") // rgb: [204 204 204]
// 	rgb := HexToRGB("aabbcc") // rgb: [170 187 204]
// 	rgb := HexToRGB("#aabbcc") // rgb: [170 187 204]
// 	rgb := HexToRGB("0xad99c0") // rgb: [170 187 204]
func HexToRGB(hex string) (rgb []int) {
	hex = strings.TrimSpace(hex)
	if hex == "" {
		return
	}

	// like from css. eg "#ccc" "#ad99c0"
	if hex[0] == '#' {
		hex = hex[1:]
	}

	hex = strings.ToLower(hex)
	switch len(hex) {
	case 3: // "ccc"
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	case 8: // "0xad99c0"
		hex = strings.TrimPrefix(hex, "0x")
	}

	// recheck
	if len(hex) != 6 {
		return
	}

	// convert string to int64
	if i64, err := strconv.ParseInt(hex, 16, 32); err == nil {
		color := int(i64)
		// parse int
		rgb = make([]int, 3)
		rgb[0] = color >> 16
		rgb[1] = (color & 0x00FF00) >> 8
		rgb[2] = color & 0x0000FF
	}

	return
}

/*************************************************************
 * RGB Style
 *************************************************************/

// RGBStyle definition.
//
// 前/背景色
// 都是由4位uint8组成, 前三位是色彩值；
// 最后一位与RGBColor不一样的是，在这里表示是否设置了值 1 表示已设置 ^1 未设置
type RGBStyle struct {
	// Name of the style
	Name string
	// fg and bg color
	fg, bg RGBColor
}

// NewRGBStyle create a RGBStyle.
func NewRGBStyle(fg RGBColor, bg ...RGBColor) *RGBStyle {
	s := &RGBStyle{}
	if len(bg) > 0 {
		s.SetBg(bg[0])
	}

	return s.SetFg(fg)
}

// HEXStyle create a RGBStyle from HEX color string.
// Usage:
// 	s := HEXStyle("aabbcc", "eee")
// 	s.Print("message")
func HEXStyle(fg string, bg ...string) *RGBStyle {
	s := &RGBStyle{}
	if len(bg) > 0 {
		s.SetBg(HEX(bg[0]))
	}

	return s.SetFg(HEX(fg))
}

// RGBStyleFromString create a RGBStyle from color value string.
// Usage:
// 	s := RGBStyleFromString("170,187,204", "70,87,4")
// 	s.Print("message")
func RGBStyleFromString(fg string, bg ...string) *RGBStyle {
	s := &RGBStyle{}
	if len(bg) > 0 {
		s.SetBg(RGBFromString(bg[0]))
	}

	return s.SetFg(RGBFromString(fg))
}

// Set fg and bg color
func (s *RGBStyle) Set(fg, bg RGBColor) *RGBStyle {
	return s.SetFg(fg).SetBg(bg)
}

// SetFg set fg color
func (s *RGBStyle) SetFg(fg RGBColor) *RGBStyle {
	fg[3] = 1
	s.fg = fg
	return s
}

// SetBg set bg color
func (s *RGBStyle) SetBg(bg RGBColor) *RGBStyle {
	bg[3] = 1
	s.bg = bg
	return s
}

// Print print message
func (s *RGBStyle) Print(a ...interface{}) {
	fmt.Print(RenderCode(s.String(), a...))
}

// Printf format and print message
func (s *RGBStyle) Printf(format string, a ...interface{}) {
	fmt.Print(RenderString(s.String(), fmt.Sprintf(format, a...)))
}

// Println print message with newline
func (s *RGBStyle) Println(a ...interface{}) {
	fmt.Println(RenderString(s.String(), formatArgsForPrintln(a)))
}

// Sprint returns rendered message
func (s *RGBStyle) Sprint(a ...interface{}) string {
	return RenderCode(s.String(), a...)
}

// Sprintf returns format and rendered message
func (s *RGBStyle) Sprintf(format string, a ...interface{}) string {
	return RenderString(s.Code(), fmt.Sprintf(format, a...))
}

// Code convert to color code string
func (s *RGBStyle) Code() string {
	return s.String()
}

// String convert to color code string
func (s *RGBStyle) String() string {
	var ss []string
	if s.fg[3] == 1 { // last value ensure is enable.
		ss = append(ss, fmt.Sprintf(TplFgRGB, s.fg[0], s.fg[1], s.fg[2]))
	}

	if s.bg[3] == 1 {
		ss = append(ss, fmt.Sprintf(TplBgRGB, s.bg[0], s.bg[1], s.bg[2]))
	}

	return strings.Join(ss, ";")
}

// IsEmpty style
func (s *RGBStyle) IsEmpty() bool {
	return s.fg[3] != 1 && s.bg[3] != 1
}

// RGBto256 value
// func RGBto256(r, g, b uint8) {
//
// }
