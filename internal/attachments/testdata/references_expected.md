# Images

An embed alone in its paragraph:

![the login screen](https://example.com/login)

![the login screen without dot-slash](https://example.com/login)

An embed inline in ![the login screen](https://example.com/login) a sentence.

A link to [the screenshot](https://example.com/login) instead of an embed.

An embed and a link to ![one file](https://example.com/login) and [the same file](https://example.com/login).

Alt text written here wins over the flag: ![the auth error page](https://example.com/login)

Formatting inside the ![*emphasised* alt](https://example.com/login) alt text survives.

A title survives: ![the login screen](https://example.com/login "Login")

A single quoted title: ![the login screen](https://example.com/login 'Login')

A parenthesised title: ![the login screen](https://example.com/login (Login))

A title with escaped quotes: ![the login screen](https://example.com/login "he said \"hi\"")

# Videos

A video alone in its paragraph plays:

https://example.com/repro

A video inline in [the recording](https://example.com/repro) a sentence cannot play.

A video written as a link stays [a link](https://example.com/repro).

A video with no alt text inline [repro.mp4](https://example.com/repro) falls back to the file name.

A bang before a video embed\![the recording](https://example.com/repro) must not re-form an embed.

A bang already escaped\![the recording](https://example.com/repro) is left alone.

A bracket before a video embed][the recording](https://example.com/repro) needs no escaping.

# Paths that need escaping

Angle brackets around spaces: ![the screenshot](https://example.com/screenshot)

Percent encoding for spaces: ![the screenshot](https://example.com/screenshot)

Balanced parentheses: ![a](https://example.com/parens)

Nested balanced parentheses: ![a](https://example.com/nested-parens)

An escaped closing parenthesis: ![a](https://example.com/escaped-paren)

An escaped backslash: ![a](https://example.com/backslash)

Escaped parentheses: ![a](https://example.com/escaped-parens)

# Paths markdown does not parse as a link

An unbalanced closing parenthesis ends the destination: ![a](https://example.com/truncated).png)

An unbalanced opening parenthesis is literal text: ![a](./f(1.png)

An unbracketed space is literal text: ![a](./my file.png)

An unclosed angle bracket is literal text: ![a](<./login.png)

# Nesting

An image nested in a link: [![the badge](https://example.com/login)](https://example.com/repro)

A thumbnail linking to its own file: [![the login screen](https://example.com/login)](https://example.com/login)

A video embedded inside a link to itself: [[the recording](https://example.com/repro)](https://example.com/repro)

A link inside the alt text of a degraded video: here [see [the screenshot](https://example.com/login)](https://example.com/repro) inline.

An image inside the alt text of a degraded video: here [see ![the screenshot](https://example.com/login)](https://example.com/repro) inline.

# Code is never touched

```
![not a reference](./login.png)
[not a definition]: ./login.png
```

Inline `![not a reference](./login.png)` code span.

A code span that could pair with an earlier bracket: [x `](./login.png)` y ![the real one](https://example.com/login)

A bracket inside a code span in the alt text: ![before `[` after](https://example.com/login)

    ![an indented code block](./login.png)

# Malformed tails are skipped

An unterminated title earlier in the block: [x](./login.png "oops) then ![the login screen](https://example.com/login)

A tail that never closes: [x](./login.png b) and [the login screen](https://example.com/login)

# Structure

- a tight list item ![the login screen](https://example.com/login)
- [the recording](https://example.com/repro)

* one loose item

* https://example.com/repro

> a blockquote ![the login screen](https://example.com/login)

> https://example.com/repro

> > https://example.com/repro

- > https://example.com/repro

# ![the login screen](https://example.com/login)

# [the recording](https://example.com/repro)

Watch this:
[the recording](https://example.com/repro)
and then read on.

An image and a video in one paragraph: ![the login screen](https://example.com/login) and [the recording](https://example.com/repro).

# Reference style

An image written as a reference-style image: ![the login screen][shot]

An image written as a reference-style link: see [the screenshot][shot] for detail.

A video written as a reference-style link: [the recording][clip]

The collapsed form: ![shot][]

The shortcut form: ![shot]

An inline usage and a reference usage of one file: ![inline](https://example.com/login) and [reference][shot].

[shot]: https://example.com/login
[clip]: https://example.com/repro
[spare]: https://example.com/login
[titled]: https://example.com/login "Login"
[angled]: https://example.com/screenshot
[escaped\]: label]: https://example.com/login
[angle-escape]: https://example.com/escaped-angle
[unused]: ./unused.png
[unattached]: ./other.png

An unclosed angle bracket is not a definition, so it and everything after it
in this paragraph stays text:

[bad angle]: <./login.png

The titled reference: ![titled][titled]

The angled reference: ![angled][angled]

The escaped label reference: ![escaped][escaped\]: label]

The escaped angle bracket definition: ![escaped angle][angle-escape]

The unattached reference: ![unattached][unattached]

The unclosed angle definition: ![bad][bad angle]

# Left exactly as written

A remote URL: ![hosted](https://example.com/login.png)

An anchor: [jump](#login.png)

A local path nobody attached: ![unattached](./other.png)

A file whose upload produced no URL: ![no url](./nourl.png)
