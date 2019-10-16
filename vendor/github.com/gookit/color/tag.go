package color

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

// output colored text like use html tag. (not support windows cmd)
const (
	// MatchExpr regex to match color tags
	// Notice: golang 不支持反向引用.  即不支持使用 \1 引用第一个匹配 ([a-z=;]+)
	// MatchExpr = `<([a-z=;]+)>(.*?)<\/\1>`
	// 所以调整一下 统一使用 `</>` 来结束标签，例如 "<info>some text</>"
	// 支持自定义颜色属性的tag "<fg=white;bg=blue;op=bold>content</>"
	// (?s:...) s - 让 "." 匹配换行
	MatchExpr = `<([a-zA-Z_=,;]+)>(?s:(.*?))<\/>`

	// AttrExpr regex to match color attributes
	AttrExpr = `(fg|bg|op)[\s]*=[\s]*([a-zA-Z,]+);?`

	// StripExpr regex used for removing color tags
	// StripExpr = `<[\/]?[a-zA-Z=;]+>`
	// 随着上面的做一些调整
	StripExpr = `<[\/]?[a-zA-Z_=,;]*>`
)

var (
	attrRegex  = regexp.MustCompile(AttrExpr)
	matchRegex = regexp.MustCompile(MatchExpr)
	stripRegex = regexp.MustCompile(StripExpr)
)

/*************************************************************
 * internal defined color tags
 *************************************************************/

// Some internal defined color tags
// Usage: <tag>content text</>
// @notice 加 0 在前面是为了防止之前的影响到现在的设置
var colorTags = map[string]string{
	// basic tags,
	"red":      "0;31",
	"blue":     "0;34",
	"cyan":     "0;36",
	"black":    "0;30",
	"green":    "0;32",
	"white":    "1;37",
	"default":  "0;39", // no color
	"normal":   "0;39", // no color
	"brown":    "0;33",
	"yellow":   "1;33",
	"mga":      "0;35", // short name
	"magenta":  "0;35",
	"mgb":      "1;35", // short name
	"magentaB": "1;35", // add bold

	// alert tags, like bootstrap's alert
	"suc":     "1;32", // same "green" and "bold"
	"success": "1;32",
	"info":    "0;32", // same "green",
	"comment": "0;33", // same "brown"
	"note":    "36;1",
	"notice":  "36;4",
	"warn":    "0;1;33",
	"warning": "0;30;43",
	"primary": "0;34",
	"danger":  "1;31", // same "red" but add bold
	"err":     "97;41",
	"error":   "97;41", // fg light white; bg red

	// more tags
	"lightRed":      "1;31",
	"light_red":     "1;31",
	"lightGreen":    "1;32",
	"light_green":   "1;32",
	"lightBlue":     "1;34",
	"light_blue":    "1;34",
	"lightCyan":     "1;36",
	"light_cyan":    "1;36",
	"lightDray":     "0;37",
	"light_gray":    "0;37",
	"gray":          "0;90",
	"darkGray":      "0;90",
	"dark_gray":     "0;90",
	"lightYellow":   "0;93",
	"light_yellow":  "0;93",
	"lightMagenta":  "0;95",
	"light_magenta": "0;95",

	// extra
	"lightRedEx":     "0;91",
	"light_red_ex":   "0;91",
	"lightGreenEx":   "0;92",
	"light_green_ex": "0;92",
	"lightBlueEx":    "0;94",
	"light_blue_ex":  "0;94",
	"lightCyanEx":    "0;96",
	"light_cyan_ex":  "0;96",
	"whiteEx":        "0;97;40",
	"white_ex":       "0;97;40",

	// option
	"bold":       "1",
	"underscore": "4",
	"reverse":    "7",
}

/*************************************************************
 * print methods(will auto parse color tags)
 *************************************************************/

// Print render color tag and print messages
func Print(a ...interface{}) {
	Fprint(output, a...)
}

// Printf format and print messages
func Printf(format string, a ...interface{}) {
	Fprintf(output, format, a...)
}

// Println messages with new line
func Println(a ...interface{}) {
	Fprintln(output, a...)
}

// Fprint print rendered messages to writer
// Notice: will ignore print error
func Fprint(w io.Writer, a ...interface{}) {
	if isLikeInCmd {
		renderColorCodeOnCmd(func() {
			_, _ = fmt.Fprint(w, Render(a...))
		})
	} else {
		_, _ = fmt.Fprint(w, Render(a...))
	}
}

// Fprintf print format and rendered messages to writer.
// Notice: will ignore print error
func Fprintf(w io.Writer, format string, a ...interface{}) {
	str := fmt.Sprintf(format, a...)
	if isLikeInCmd {
		renderColorCodeOnCmd(func() {
			_, _ = fmt.Fprint(w, ReplaceTag(str))
		})
	} else {
		_, _ = fmt.Fprint(w, ReplaceTag(str))
	}
}

// Fprintln print rendered messages line to writer
// Notice: will ignore print error
func Fprintln(w io.Writer, a ...interface{}) {
	str := formatArgsForPrintln(a)
	if isLikeInCmd {
		renderColorCodeOnCmd(func() {
			_, _ = fmt.Fprintln(w, ReplaceTag(str))
		})
	} else {
		_, _ = fmt.Fprintln(w, ReplaceTag(str))
	}
}

// Render parse color tags, return rendered string.
// Usage:
//	text := Render("<info>hello</> <cyan>world</>!")
//	fmt.Println(text)
func Render(a ...interface{}) string {
	if len(a) == 0 {
		return ""
	}

	return ReplaceTag(fmt.Sprint(a...))
}

// Sprint parse color tags, return rendered string
func Sprint(args ...interface{}) string {
	return Render(args...)
}

// Sprintf format and return rendered string
func Sprintf(format string, a ...interface{}) string {
	return ReplaceTag(fmt.Sprintf(format, a...))
}

// String alias of the ReplaceTag
func String(s string) string {
	return ReplaceTag(s)
}

// Text alias of the ReplaceTag
func Text(s string) string {
	return ReplaceTag(s)
}

/*************************************************************
 * parse color tags
 *************************************************************/

// ReplaceTag parse string, replace color tag and return rendered string
func ReplaceTag(str string) string {
	// not contains color tag
	if !strings.Contains(str, "</>") {
		return str
	}

	// disabled OR not support color
	if !Enable || !isSupportColor {
		return ClearTag(str)
	}

	// find color tags by regex
	matched := matchRegex.FindAllStringSubmatch(str, -1)

	// item: 0 full text 1 tag name 2 tag content
	for _, item := range matched {
		full, tag, content := item[0], item[1], item[2]

		// custom color in tag: "<fg=white;bg=blue;op=bold>content</>"
		if code := ParseCodeFromAttr(tag); len(code) > 0 {
			now := RenderString(code, content)
			str = strings.Replace(str, full, now, 1)
			continue
		}

		// use defined tag: "<tag>content</>"
		if code := GetTagCode(tag); len(code) > 0 {
			now := RenderString(code, content)
			// old := WrapTag(content, tag) is equals to var 'full'
			str = strings.Replace(str, full, now, 1)
		}
	}

	return str
}

// ParseCodeFromAttr parse color attributes.
// attr like:
// 		"fg=VALUE;bg=VALUE;op=VALUE" // VALUE please see var: FgColors, BgColors, Options
// eg:
// 		"fg=yellow"
// 		"bg=red"
// 		"op=bold,underscore" option is allow multi value
// 		"fg=white;bg=blue;op=bold"
// 		"fg=white;op=bold,underscore"
func ParseCodeFromAttr(attr string) (code string) {
	if !strings.Contains(attr, "=") {
		return
	}

	attr = strings.Trim(attr, ";=,")
	if len(attr) == 0 {
		return
	}

	var colors []Color

	matched := attrRegex.FindAllStringSubmatch(attr, -1)
	for _, item := range matched {
		pos, val := item[1], item[2]
		switch pos {
		case "fg":
			if c, ok := FgColors[val]; ok { // basic fg
				colors = append(colors, c)
			} else if c, ok := ExFgColors[val]; ok { // extra fg
				colors = append(colors, c)
			}
		case "bg":
			if c, ok := BgColors[val]; ok { // basic bg
				colors = append(colors, c)
			} else if c, ok := ExBgColors[val]; ok { // extra bg
				colors = append(colors, c)
			}
		case "op": // options allow multi value
			if strings.Contains(val, ",") {
				ns := strings.Split(val, ",")
				for _, n := range ns {
					if c, ok := Options[n]; ok {
						colors = append(colors, c)
					}
				}
			} else if c, ok := Options[val]; ok {
				colors = append(colors, c)
			}
		}
	}

	return colors2code(colors...)
}

// ClearTag clear all tag for a string
func ClearTag(s string) string {
	if !strings.Contains(s, "</>") {
		return s
	}

	return stripRegex.ReplaceAllString(s, "")
}

/*************************************************************
 * helper methods
 *************************************************************/

// GetTagCode get color code by tag name
func GetTagCode(name string) string {
	if code, ok := colorTags[name]; ok {
		return code
	}

	return ""
}

// ApplyTag for messages
func ApplyTag(tag string, a ...interface{}) string {
	return RenderCode(GetTagCode(tag), a...)
}

// WrapTag wrap a tag for a string "<tag>content</>"
func WrapTag(s string, tag string) string {
	if s == "" || tag == "" {
		return s
	}

	return fmt.Sprintf("<%s>%s</>", tag, s)
}

// GetColorTags get all internal color tags
func GetColorTags() map[string]string {
	return colorTags
}

// IsDefinedTag is defined tag name
func IsDefinedTag(name string) bool {
	_, ok := colorTags[name]
	return ok
}

/*************************************************************
 * Tag extra
 *************************************************************/

// Tag value is a defined style name
// Usage:
// 	Tag("info").Println("message")
type Tag string

// Print messages
func (tg Tag) Print(a ...interface{}) {
	name := string(tg)
	str := fmt.Sprint(a...)

	if stl := GetStyle(name); !stl.IsEmpty() {
		stl.Print(str)
	} else {
		doPrintV2(GetTagCode(name), str)
	}
}

// Printf format and print messages
func (tg Tag) Printf(format string, a ...interface{}) {
	name := string(tg)
	str := fmt.Sprintf(format, a...)

	if stl := GetStyle(name); !stl.IsEmpty() {
		stl.Print(str)
	} else {
		doPrintV2(GetTagCode(name), str)
	}
}

// Println messages line
func (tg Tag) Println(a ...interface{}) {
	name := string(tg)
	if stl := GetStyle(name); !stl.IsEmpty() {
		stl.Println(a...)
	} else {
		doPrintlnV2(GetTagCode(name), a)
	}
}

// Sprint render messages
func (tg Tag) Sprint(a ...interface{}) string {
	name := string(tg)
	// if stl := GetStyle(name); !stl.IsEmpty() {
	// 	return stl.Render(args...)
	// }

	return RenderCode(GetTagCode(name), a...)
}

// Sprintf format and render messages
func (tg Tag) Sprintf(format string, a ...interface{}) string {
	tag := string(tg)
	str := fmt.Sprintf(format, a...)

	return RenderString(GetTagCode(tag), str)
}
