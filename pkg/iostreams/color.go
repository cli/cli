package iostreams

import (
	"fmt"
	"os"
	"strings"

	"github.com/mgutz/ansi"
)

var (
	magenta  = ansi.ColorFunc("magenta")
	cyan     = ansi.ColorFunc("cyan")
	red      = ansi.ColorFunc("red")
	yellow   = ansi.ColorFunc("yellow")
	blue     = ansi.ColorFunc("blue")
	green    = ansi.ColorFunc("green")
	gray     = ansi.ColorFunc("black+h")
	bold     = ansi.ColorFunc("default+b")
	cyanBold = ansi.ColorFunc("cyan+b")

	gray256 = func(t string) string {
		return fmt.Sprintf("\x1b[%d;5;%dm%s\x1b[m", 38, 242, t)
	}
)

func EnvColorDisabled() bool {
	return os.Getenv("NO_COLOR") != "" || os.Getenv("CLICOLOR") == "0"
}

func EnvColorForced() bool {
	return os.Getenv("CLICOLOR_FORCE") != "" && os.Getenv("CLICOLOR_FORCE") != "0"
}

func Is256ColorSupported() bool {
	term := os.Getenv("TERM")
	colorterm := os.Getenv("COLORTERM")

	return strings.Contains(term, "256") ||
		strings.Contains(term, "24bit") ||
		strings.Contains(term, "truecolor") ||
		strings.Contains(colorterm, "256") ||
		strings.Contains(colorterm, "24bit") ||
		strings.Contains(colorterm, "truecolor")
}

func NewColorScheme(enabled, is256enabled bool) *ColorScheme {
	return &ColorScheme{
		enabled:      enabled,
		is256enabled: is256enabled,
	}
}

type ColorScheme struct {
	enabled      bool
	is256enabled bool
}

func (c *ColorScheme) Bold(t string) string {
	if !c.enabled {
		return t
	}
	return bold(t)
}

func (c *ColorScheme) Red(t string) string {
	if !c.enabled {
		return t
	}
	return red(t)
}

func (c *ColorScheme) Yellow(t string) string {
	if !c.enabled {
		return t
	}
	return yellow(t)
}

func (c *ColorScheme) Green(t string) string {
	if !c.enabled {
		return t
	}
	return green(t)
}

func (c *ColorScheme) Gray(t string) string {
	if !c.enabled {
		return t
	}
	if c.is256enabled {
		return gray256(t)
	}
	return gray(t)
}

func (c *ColorScheme) Grayf(t string, args ...interface{}) string {
	return c.Gray(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) Magenta(t string) string {
	if !c.enabled {
		return t
	}
	return magenta(t)
}

func (c *ColorScheme) Cyan(t string) string {
	if !c.enabled {
		return t
	}
	return cyan(t)
}

func (c *ColorScheme) Cyanf(t string, args ...interface{}) string {
	return c.Cyan(fmt.Sprintf(t, args...))
}

func (c *ColorScheme) CyanBold(t string) string {
	if !c.enabled {
		return t
	}
	return cyanBold(t)
}

func (c *ColorScheme) Blue(t string) string {
	if !c.enabled {
		return t
	}
	return blue(t)
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
	return c.Red("X")
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
		fn = c.Gray
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
