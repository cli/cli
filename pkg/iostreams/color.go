package iostreams

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mgutz/ansi"
)

const (
	NoTheme        = "none"
	DarkTheme      = "dark"
	LightTheme     = "light"
	highlightStyle = "black:yellow"
)

// Special cases like darkThemeTableHeader / lightThemeTableHeader are necessary when using color and modifiers
// (bold, underline, dim) because ansi.ColorFunc requires a foreground color and resets formats.
var (
	magenta               = ansi.ColorFunc("magenta")
	cyan                  = ansi.ColorFunc("cyan")
	red                   = ansi.ColorFunc("red")
	yellow                = ansi.ColorFunc("yellow")
	blue                  = ansi.ColorFunc("blue")
	green                 = ansi.ColorFunc("green")
	gray                  = ansi.ColorFunc("black+h")
	bold                  = ansi.ColorFunc("default+b")
	cyanBold              = ansi.ColorFunc("cyan+b")
	greenBold             = ansi.ColorFunc("green+b")
	highlightStart        = ansi.ColorCode(highlightStyle)
	highlight             = ansi.ColorFunc(highlightStyle)
	darkThemeMuted        = ansi.ColorFunc("white+d")
	darkThemeTableHeader  = ansi.ColorFunc("white+du")
	lightThemeMuted       = ansi.ColorFunc("black+h")
	lightThemeTableHeader = ansi.ColorFunc("black+hu")
	noThemeTableHeader    = ansi.ColorFunc("default+u")

	gray256 = func(t string) string {
		return fmt.Sprintf("\x1b[%d;5;%dm%s\x1b[0m", 38, 242, t)
	}
)

// ColorScheme controls how text is colored based upon terminal capabilities and user preferences.
type ColorScheme struct {
	// Enabled is whether color is used at all.
	Enabled bool
	// EightBitColor is whether the terminal supports 8-bit, 256 colors.
	EightBitColor bool
	// TrueColor is whether the terminal supports 24-bit, 16 million colors.
	TrueColor bool
	// Accessible is whether colors must be base 16 colors that users can customize in terminal preferences.
	Accessible bool
	// ColorLabels is whether labels are colored based on their truecolor RGB hex color.
	ColorLabels bool
	// linkEnabled is whether terminal hyperlinks should be rendered.
	linkEnabled bool
	// Theme is the terminal background color theme used to contextually color text for light, dark, or none at all.
	Theme string
}

func (c *ColorScheme) Bold(t string) string {
	if !c.Enabled {
		return t
	}
	return bold(t)
}

func (c *ColorScheme) Boldf(t string, args ...interface{}) string {
	return c.Bold(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) Muted(t string) string {
	// Fallback to previous logic if accessible colors preview is disabled.
	if !c.Accessible {
		return c.Gray(t)
	}

	// Muted text is only stylized if color is enabled.
	if !c.Enabled {
		return t
	}

	switch c.Theme {
	case LightTheme:
		return lightThemeMuted(t)
	case DarkTheme:
		return darkThemeMuted(t)
	default:
		return t
	}
}

func (c *ColorScheme) Mutedf(t string, args ...interface{}) string {
	return c.Muted(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) Red(t string) string {
	if !c.Enabled {
		return t
	}
	return red(t)
}

func (c *ColorScheme) Redf(t string, args ...interface{}) string {
	return c.Red(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) Yellow(t string) string {
	if !c.Enabled {
		return t
	}
	return yellow(t)
}

func (c *ColorScheme) Yellowf(t string, args ...interface{}) string {
	return c.Yellow(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) Green(t string) string {
	if !c.Enabled {
		return t
	}
	return green(t)
}

func (c *ColorScheme) Greenf(t string, args ...interface{}) string {
	return c.Green(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) GreenBold(t string) string {
	if !c.Enabled {
		return t
	}
	return greenBold(t)
}

// Deprecated: Use Muted instead for thematically contrasting color.
func (c *ColorScheme) Gray(t string) string {
	if !c.Enabled {
		return t
	}
	if c.EightBitColor {
		return gray256(t)
	}
	return gray(t)
}

// Deprecated: Use Mutedf instead for thematically contrasting color.
func (c *ColorScheme) Grayf(t string, args ...interface{}) string {
	return c.Gray(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) Magenta(t string) string {
	if !c.Enabled {
		return t
	}
	return magenta(t)
}

func (c *ColorScheme) Magentaf(t string, args ...interface{}) string {
	return c.Magenta(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) Cyan(t string) string {
	if !c.Enabled {
		return t
	}
	return cyan(t)
}

func (c *ColorScheme) Cyanf(t string, args ...interface{}) string {
	return c.Cyan(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) CyanBold(t string) string {
	if !c.Enabled {
		return t
	}
	return cyanBold(t)
}

func (c *ColorScheme) Blue(t string) string {
	if !c.Enabled {
		return t
	}
	return blue(t)
}

func (c *ColorScheme) Bluef(t string, args ...interface{}) string {
	return c.Blue(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) SuccessIcon() string {
	return c.SuccessIconWithColor(c.Green)
}

func (c *ColorScheme) SuccessIconWithColor(colo func(string) string) string {
	return colo("✓")
}

func (c *ColorScheme) WarningIcon() string {
	return c.Yellow("!")
}

func (c *ColorScheme) FailureIcon() string {
	return c.FailureIconWithColor(c.Red)
}

func (c *ColorScheme) FailureIconWithColor(colo func(string) string) string {
	return colo("X")
}

func (c *ColorScheme) HighlightStart() string {
	if !c.Enabled {
		return ""
	}

	return highlightStart
}

func (c *ColorScheme) Highlight(t string) string {
	if !c.Enabled {
		return t
	}

	return highlight(t)
}

func (c *ColorScheme) Reset() string {
	if !c.Enabled {
		return ""
	}

	return ansi.Reset
}

func (c *ColorScheme) ColorFromString(s string) func(string) string {
	s = strings.ToLower(s)
	var fn func(string) string
	switch s {
	case "bold":
		fn = c.Bold
	case "red":
		fn = c.Red
	case "yellow":
		fn = c.Yellow
	case "green":
		fn = c.Green
	case "gray":
		fn = c.Muted
	case "magenta":
		fn = c.Magenta
	case "cyan":
		fn = c.Cyan
	case "blue":
		fn = c.Blue
	default:
		fn = func(s string) string {
			return s
		}
	}

	return fn
}

func (c *ColorScheme) Hyperlink(text, url string) string {
	if !c.linkEnabled {
		return text
	}

	// Make trailing spaces not to be part of the link as it looks ugly, ...
	link_text := strings.TrimRight(text, " ")
	trailing_spaces := text[len(link_text):]
	if link_text == "" {
		// ... but still allow spaces-only text to be clickable.
		link_text = text
		trailing_spaces = ""
	}

	// https://gist.github.com/egmontkob/eb114294efbcd5adb1944c9f3cb5feda
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\%s", url, link_text, trailing_spaces)
}

func (c *ColorScheme) WithHyperlink(url string, colorize func(string) string) func(string) string {
	if colorize == nil {
		colorize = func(s string) string { return s }
	}
	if !c.linkEnabled {
		return colorize
	}
	return func(text string) string {
		// Call c.Hyperlink first, then colorize.
		// Otherwise space-trimming logic in c.Hyperlink wouldn't work.
		return colorize(c.Hyperlink(text, url))
	}
}

// Label stylizes text based on label's RGB hex color.
func (c *ColorScheme) Label(hex string, x string) string {
	if !c.Enabled || !c.TrueColor || !c.ColorLabels || len(hex) != 6 {
		return x
	}

	r, _ := strconv.ParseInt(hex[0:2], 16, 64)
	g, _ := strconv.ParseInt(hex[2:4], 16, 64)
	b, _ := strconv.ParseInt(hex[4:6], 16, 64)
	return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", r, g, b, x)
}

func (c *ColorScheme) TableHeader(t string) string {
	// Table headers are only stylized if color is enabled including underline modifier.
	if !c.Enabled {
		return t
	}

	switch c.Theme {
	case DarkTheme:
		return darkThemeTableHeader(t)
	case LightTheme:
		return lightThemeTableHeader(t)
	default:
		return noThemeTableHeader(t)
	}
}
