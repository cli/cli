package iostreams

import "github.com/mgutz/ansi"

var (
	magenta = ansi.ColorFunc("magenta")
	cyan    = ansi.ColorFunc("cyan")
	red     = ansi.ColorFunc("red")
	yellow  = ansi.ColorFunc("yellow")
	blue    = ansi.ColorFunc("blue")
	green   = ansi.ColorFunc("green")
	gray    = ansi.ColorFunc("black+h")
	bold    = ansi.ColorFunc("default+b")
)

func NewColorScheme(enabled bool) *ColorScheme {
	return &ColorScheme{enabled: enabled}
}

type ColorScheme struct {
	enabled bool
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
	return gray(t)
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

func (c *ColorScheme) Blue(t string) string {
	if !c.enabled {
		return t
	}
	return blue(t)
}

func (c *ColorScheme) SuccessIcon() string {
	return c.Green("✓")
}

func (c *ColorScheme) WarningIcon() string {
	return c.Yellow("!")
}
