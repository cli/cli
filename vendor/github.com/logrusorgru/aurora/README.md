Aurora
======

[![GoDoc](https://godoc.org/github.com/logrusorgru/aurora?status.svg)](https://godoc.org/github.com/logrusorgru/aurora)
[![WTFPL License](https://img.shields.io/badge/license-wtfpl-blue.svg)](http://www.wtfpl.net/about/)
[![Build Status](https://travis-ci.org/logrusorgru/aurora.svg)](https://travis-ci.org/logrusorgru/aurora)
[![Coverage Status](https://coveralls.io/repos/logrusorgru/aurora/badge.svg?branch=master)](https://coveralls.io/r/logrusorgru/aurora?branch=master)
[![GoReportCard](https://goreportcard.com/badge/logrusorgru/aurora)](https://goreportcard.com/report/logrusorgru/aurora)
[![Gitter](https://img.shields.io/badge/chat-on_gitter-46bc99.svg?logo=data:image%2Fsvg%2Bxml%3Bbase64%2CPHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIGhlaWdodD0iMTQiIHdpZHRoPSIxNCI%2BPGcgZmlsbD0iI2ZmZiI%2BPHJlY3QgeD0iMCIgeT0iMyIgd2lkdGg9IjEiIGhlaWdodD0iNSIvPjxyZWN0IHg9IjIiIHk9IjQiIHdpZHRoPSIxIiBoZWlnaHQ9IjciLz48cmVjdCB4PSI0IiB5PSI0IiB3aWR0aD0iMSIgaGVpZ2h0PSI3Ii8%2BPHJlY3QgeD0iNiIgeT0iNCIgd2lkdGg9IjEiIGhlaWdodD0iNCIvPjwvZz48L3N2Zz4%3D&logoWidth=10)](https://gitter.im/logrusorgru/aurora)

Ultimate ANSI colors for Golang. The package supports Printf/Sprintf etc.


![aurora logo](https://github.com/logrusorgru/aurora/blob/master/gopher_aurora.png)

# TOC

- [Insallation](#installation)
- [Usage](#usage)
  + [Simple](#simple)
  + [Printf](#printf)
  + [aurora.Sprintf](#aurorasprintf)
  + [Enable/Disable colors](#enabledisable-colors)
- [Chains](#chains)
- [Colorize](#colorize)
- [Grayscale](#grayscale)
- [8-bit colors](#8-bit-colors)
- [Supported Colors & Formats](#supported-colors--formats)
  + [All colors](#all-colors)
  + [Standard and bright colors](#standard-and-bright-colors)
  + [Formats are likely supported](#formats-are-likely-supported)
  + [Formats are likely unsupported](#formats-are-likely-unsupported)
- [Limitations](#limitations)
  + [Windows](#windows)
  + [TTY](#tty)
- [Licensing](#licensing)

# Installation

Get
```
go get -u github.com/logrusorgru/aurora
```
Test
```
go test -cover github.com/logrusorgru/aurora
```

# Usage

### Simple

```go
package main

import (
	"fmt"

	. "github.com/logrusorgru/aurora"
)

func main() {
	fmt.Println("Hello,", Magenta("Aurora"))
	fmt.Println(Bold(Cyan("Cya!")))
}

```

![simple png](https://github.com/logrusorgru/aurora/blob/master/simple.png)

### Printf

```go
package main

import (
	"fmt"

	. "github.com/logrusorgru/aurora"
)

func main() {
	fmt.Printf("Got it %d times\n", Green(1240))
	fmt.Printf("PI is %+1.2e\n", Cyan(3.14))
}

```

![printf png](https://github.com/logrusorgru/aurora/blob/master/printf.png)

### aurora.Sprintf

```go
package main

import (
	"fmt"

	. "github.com/logrusorgru/aurora"
)

func main() {
	fmt.Println(Sprintf(Magenta("Got it %d times"), Green(1240)))
}

```

![sprintf png](https://github.com/logrusorgru/aurora/blob/master/sprintf.png)

### Enable/Disable colors

```go
package main

import (
	"fmt"
	"flag"

	"github.com/logrusorgru/aurora"
)

// colorizer
var au aurora.Aurora

var colors = flag.Bool("colors", false, "enable or disable colors")

func init() {
	flag.Parse()
	au = aurora.NewAurora(*colors)
}

func main() {
	// use colorizer
	fmt.Println(au.Green("Hello"))
}

```
Without flags: 
![disable png](https://github.com/logrusorgru/aurora/blob/master/disable.png)
  
With `-colors` flag:
![enable png](https://github.com/logrusorgru/aurora/blob/master/enable.png)

# Chains

The following samples are equal

```go
x := BgMagenta(Bold(Red("x")))
```

```go
x := Red("x").Bold().BgMagenta()
```

The second is more readable

# Colorize

There is `Colorize` function that allows to choose some colors and
format from a side

```go

func getColors() Color {
	// some stuff that returns appropriate colors and format
}

// [...]

func main() {
	fmt.Println(Colorize("Greeting", getColors()))
}

```
Less complicated example

```go
x := Colorize("Greeting", GreenFg|GrayBg|BoldFm)
```

Unlike other color functions and methods (such as Red/BgBlue etc)
a `Colorize` clears previous colors

```go
x := Red("x").Colorize(BgGreen) // will be with green background only
```

# Grayscale

```go
fmt.Println("  ",
	Gray(1-1, " 00-23 ").BgGray(24-1),
	Gray(4-1, " 03-19 ").BgGray(20-1),
	Gray(8-1, " 07-15 ").BgGray(16-1),
	Gray(12-1, " 11-11 ").BgGray(12-1),
	Gray(16-1, " 15-07 ").BgGray(8-1),
	Gray(20-1, " 19-03 ").BgGray(4-1),
	Gray(24-1, " 23-00 ").BgGray(1-1),
)
```

![grayscale png](https://github.com/logrusorgru/aurora/blob/master/aurora_grayscale.png)  

# 8-bit colors

Methods `Index` and `BgIndex` implements 8-bit colors.

| Index/BgIndex  |    Meaning      | Foreground | Background |
| -------------- | --------------- | ---------- | ---------- |
|      0-  7     | standard colors |   30- 37   |   40- 47   |
|      8- 15     | bright colors   |   90- 97   |  100-107   |
|     16-231     | 216 colors      |   38;5;n   |   48;5;n   |
|    232-255     | 24 grayscale    |   38;5;n   |   48;5;n   |


# Supported colors & formats

- formats
  + bold (1)
  + faint (2)
  + doubly-underline (21)
  + fraktur (20)
  + italic (3)
  + underline (4)
  + slow blink (5)
  + rapid blink (6)
  + reverse video (7)
  + conceal (8)
  + crossed out (9)
  + framed (51)
  + encircled (52)
  + overlined (53)
- background and foreground colors, including bright
  + black
  + red
  + green
  + yellow (brown)
  +  blue
  + magenta
  + cyan
  + white
  + 24 grayscale colors
  + 216 8-bit colors

### All colors

![linux png](https://github.com/logrusorgru/aurora/blob/master/aurora_colors_black.png)  
![white png](https://github.com/logrusorgru/aurora/blob/master/aurora_colors_white.png)  

### Standard and bright colors

![linux black standard png](https://github.com/logrusorgru/aurora/blob/master/aurora_black_standard.png)
![linux white standard png](https://github.com/logrusorgru/aurora/blob/master/aurora_white_standard.png)

### Formats are likely supported

![formats supported gif](https://github.com/logrusorgru/aurora/blob/master/aurora_formats.gif)

### Formats are likely unsupported

![formats rarely supported png](https://github.com/logrusorgru/aurora/blob/master/aurora_rarely_supported.png)

# Limitations

There is no way to represent `%T` and `%p` with colors using
a standard approach

```go
package main

import (
	"fmt"

	. "github.com/logrusorgru/aurora"
)

func main() {
	r := Red("red")
	var i int
	fmt.Printf("%T %p\n", r, Green(&i))
}
```

Output will be without colors

```
aurora.value %!p(aurora.value={0xc42000a310 768 0})
```

The obvious workaround is `Red(fmt.Sprintf("%T", some))`

### Windows

The Aurora provides ANSI colors only. So, there are not supports and workarounds for OS Windows.
Check out this commetns to find a way
- [Using go-colrable](https://github.com/logrusorgru/aurora/issues/2#issuecomment-299014211).
- [Using registry for Windows 10](https://github.com/logrusorgru/aurora/issues/10#issue-476361247).

### TTY

The Aurora has no internal TTY detectors by design. Take a look
 [this comment](https://github.com/logrusorgru/aurora/issues/2#issuecomment-299030108) if you want turn
on colors for a terminal only, and turn them off for a file.

### Licensing

Copyright &copy; 2016-2019 The Aurora Authors. This work is free.
It comes without any warranty, to the extent permitted by applicable
law. You can redistribute it and/or modify it under the terms of the
Do What The Fuck You Want To Public License, Version 2, as published
by Sam Hocevar. See the LICENSE file for more details.


