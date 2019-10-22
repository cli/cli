# ansi

Package ansi is a small, fast library to create ANSI colored strings and codes.

## Install

Get it

```sh
go get -u github.com/mgutz/ansi
```

## Example

```go
import "github.com/mgutz/ansi"

// colorize a string, SLOW
msg := ansi.Color("foo", "red+b:white")

// create a FAST closure function to avoid computation of ANSI code
phosphorize := ansi.ColorFunc("green+h:black")
msg = phosphorize("Bring back the 80s!")
msg2 := phospohorize("Look, I'm a CRT!")

// cache escape codes and build strings manually
lime := ansi.ColorCode("green+h:black")
reset := ansi.ColorCode("reset")

fmt.Println(lime, "Bring back the 80s!", reset)
```

Other examples

```go
Color(s, "red")            // red
Color(s, "red+b")          // red bold
Color(s, "red+B")          // red blinking
Color(s, "red+u")          // red underline
Color(s, "red+bh")         // red bold bright
Color(s, "red:white")      // red on white
Color(s, "red+b:white+h")  // red bold on white bright
Color(s, "red+B:white+h")  // red blink on white bright
Color(s, "off")            // turn off ansi codes
```

To view color combinations, from project directory in terminal.

```sh
go test
```

## Style format

```go
"foregroundColor+attributes:backgroundColor+attributes"
```

Colors

* black
* red
* green
* yellow
* blue
* magenta
* cyan
* white
* 0...255 (256 colors)

Foreground Attributes

* B = Blink
* b = bold
* h = high intensity (bright)
* i = inverse
* s = strikethrough
* u = underline

Background Attributes

* h = high intensity (bright)

## Constants

* ansi.Reset
* ansi.DefaultBG
* ansi.DefaultFG
* ansi.Black
* ansi.Red
* ansi.Green
* ansi.Yellow
* ansi.Blue
* ansi.Magenta
* ansi.Cyan
* ansi.White
* ansi.LightBlack
* ansi.LightRed
* ansi.LightGreen
* ansi.LightYellow
* ansi.LightBlue
* ansi.LightMagenta
* ansi.LightCyan
* ansi.LightWhite

## References

Wikipedia ANSI escape codes [Colors](http://en.wikipedia.org/wiki/ANSI_escape_code#Colors)

General [tips and formatting](http://misc.flogisoft.com/bash/tip_colors_and_formatting)

What about support on Windows? Use [colorable by mattn](https://github.com/mattn/go-colorable).
Ansi and colorable are used by [logxi](https://github.com/mgutz/logxi) to support logging in
color on Windows.

## MIT License

Copyright (c) 2013 Mario Gutierrez mario@mgutz.com

See the file LICENSE for copying permission.

