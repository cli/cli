/*
Package ansi is a small, fast library to create ANSI colored strings and codes.

Installation

    # this installs the color viewer and the package
    go get -u github.com/mgutz/ansi/cmd/ansi-mgutz

Example

	// colorize a string, SLOW
	msg := ansi.Color("foo", "red+b:white")

	// create a closure to avoid recalculating ANSI code compilation
	phosphorize := ansi.ColorFunc("green+h:black")
	msg = phosphorize("Bring back the 80s!")
	msg2 := phospohorize("Look, I'm a CRT!")

	// cache escape codes and build strings manually
	lime := ansi.ColorCode("green+h:black")
	reset := ansi.ColorCode("reset")

	fmt.Println(lime, "Bring back the 80s!", reset)

Other examples

	Color(s, "red")            // red
	Color(s, "red+b")          // red bold
	Color(s, "red+B")          // red blinking
	Color(s, "red+u")          // red underline
	Color(s, "red+bh")         // red bold bright
	Color(s, "red:white")      // red on white
	Color(s, "red+b:white+h")  // red bold on white bright
	Color(s, "red+B:white+h")  // red blink on white bright

To view color combinations, from terminal

	ansi-mgutz

Style format

	"foregroundColor+attributes:backgroundColor+attributes"

Colors

	black
	red
	green
	yellow
	blue
	magenta
	cyan
	white

Attributes

	b = bold foreground
	B = Blink foreground
	u = underline foreground
	h = high intensity (bright) foreground, background
	i = inverse

Wikipedia ANSI escape codes [Colors](http://en.wikipedia.org/wiki/ANSI_escape_code#Colors)
*/
package ansi
