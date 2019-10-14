Changes
=======

---
15:39:40
Wednesday, April 17, 2019

- Bright background and foreground colors
- 8-bit indexed colors `Index`, `BgIndex`
- 24 grayscale colors `Gray`, `BgGray`
- `Yellow` and `BgYellow` methods, mark Brow and BgBrown as deprecated
  Following specifications, correct name of the colors are yellow, but
  by historical reason they are called brown. Both, the `Yellow` and the
  `Brown` methods (including `Bg+`) represents the same colors. The Brown
  are leaved for backward compatibility until Go modules production release.
- Additional formats
  + `Faint` that is opposite to the `Bold`
  + `DoublyUnderline`
  + `Fraktur`
  + `Italic`
  + `Underline`
  + `SlowBlink` with `Blink` alias
  + `RapidBlink`
  + `Reverse` that is alias for the `Inverse`
  + `Conceal` with `Hidden` alias
  + `CrossedOut` with `StrikeThrough` alias
  + `Framed`
  + `Encircled`
  + `Overlined`
- Add AUTHORS.md file and change all copyright notices.
- `Reset` method to create clear value. `Reset` method that replaces
  `Bleach` method. The `Bleach` method was marked as deprecated.

---

14:25:49
Friday, August 18, 2017

- LICENSE.md changed to LICENSE
- fix email in README.md
- add "no warranty" to README.md
- set proper copyright date

---

16:59:28
Tuesday, November 8, 2016

- Rid out off sync.Pool
- Little optimizations (very little)
- Improved benchmarks

---
