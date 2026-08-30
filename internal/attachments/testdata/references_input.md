# Images

An embed alone in its paragraph:

![the login screen](./login.png)

![the login screen without dot-slash](login.png)

An embed inline in ![the login screen](./login.png) a sentence.

A link to [the screenshot](./login.png) instead of an embed.

An embed and a link to ![one file](./login.png) and [the same file](./login.png).

Alt text written here wins over the flag: ![the auth error page](./login.png)

Formatting inside the ![*emphasised* alt](./login.png) alt text survives.

A title survives: ![the login screen](./login.png "Login")

A single quoted title: ![the login screen](./login.png 'Login')

A parenthesised title: ![the login screen](./login.png (Login))

A title with escaped quotes: ![the login screen](./login.png "he said \"hi\"")

# Videos

A video alone in its paragraph plays:

![the recording](./repro.mp4)

A video inline in ![the recording](./repro.mp4) a sentence cannot play.

A video written as a link stays [a link](./repro.mp4).

A video with no alt text inline ![](./repro.mp4) falls back to the file name.

A bang before a video embed!![the recording](./repro.mp4) must not re-form an embed.

A bang already escaped\!![the recording](./repro.mp4) is left alone.

A bracket before a video embed]![the recording](./repro.mp4) needs no escaping.

# Paths that need escaping

Angle brackets around spaces: ![the screenshot](<./Screenshot 2026-08-10 at 5.38.10 PM.png>)

Percent encoding for spaces: ![the screenshot](./Screenshot%202026-08-10%20at%205.38.10%20PM.png)

Balanced parentheses: ![a](./f(1).png)

Nested balanced parentheses: ![a](./f((1)(2)).png)

An escaped closing parenthesis: ![a](./f\).png)

An escaped backslash: ![a](./a\\b.png)

Escaped parentheses: ![a](./login\(1\).png)

# Paths markdown does not parse as a link

An unbalanced closing parenthesis ends the destination: ![a](./f).png)

An unbalanced opening parenthesis is literal text: ![a](./f(1.png)

An unbracketed space is literal text: ![a](./my file.png)

An unclosed angle bracket is literal text: ![a](<./login.png)

# Nesting

An image nested in a link: [![the badge](./login.png)](./repro.mp4)

A thumbnail linking to its own file: [![the login screen](./login.png)](./login.png)

A video embedded inside a link to itself: [![the recording](./repro.mp4)](./repro.mp4)

A link inside the alt text of a degraded video: here ![see [the screenshot](./login.png)](./repro.mp4) inline.

An image inside the alt text of a degraded video: here ![see ![the screenshot](./login.png)](./repro.mp4) inline.

# Code is never touched

```
![not a reference](./login.png)
[not a definition]: ./login.png
```

Inline `![not a reference](./login.png)` code span.

A code span that could pair with an earlier bracket: [x `](./login.png)` y ![the real one](./login.png)

A bracket inside a code span in the alt text: ![before `[` after](./login.png)

    ![an indented code block](./login.png)

# Malformed tails are skipped

An unterminated title earlier in the block: [x](./login.png "oops) then ![the login screen](./login.png)

A tail that never closes: [x](./login.png b) and [the login screen](./login.png)

# Structure

- a tight list item ![the login screen](./login.png)
- ![the recording](./repro.mp4)

* one loose item

* ![the recording](./repro.mp4)

> a blockquote ![the login screen](./login.png)

> ![the recording](./repro.mp4)

> > ![the recording](./repro.mp4)

- > ![the recording](./repro.mp4)

# ![the login screen](./login.png)

# ![the recording](./repro.mp4)

Watch this:
![the recording](./repro.mp4)
and then read on.

An image and a video in one paragraph: ![the login screen](./login.png) and ![the recording](./repro.mp4).

# Reference style

An image written as a reference-style image: ![the login screen][shot]

An image written as a reference-style link: see [the screenshot][shot] for detail.

A video written as a reference-style link: [the recording][clip]

The collapsed form: ![shot][]

The shortcut form: ![shot]

An inline usage and a reference usage of one file: ![inline](./login.png) and [reference][shot].

[shot]: ./login.png
[clip]: ./repro.mp4
[spare]: ./login.png
[titled]: ./login.png "Login"
[angled]: <./Screenshot 2026-08-10 at 5.38.10 PM.png>
[escaped\]: label]: ./login.png
[angle-escape]: <./a\>b.png>
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
