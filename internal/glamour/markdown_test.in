# Gold

Render markdown on the CLI, with _pizzazz_!

## What is it?

Gold is a Golang library that allows you to use JSON based stylesheets to
render Markdown files in the terminal. Just like CSS, you can define color and
style attributes on Markdown elements. The difference is that you use ANSI
color and terminal codes instead of CSS properties and hex colors.

## Usage

See [cmd/gold](cmd/gold/).

## Example Output

![Gold Dark Style](https://github.com/charmbracelet/gold/raw/master/styles/gallery/dark.png)

Check out the [Gold Style Gallery](https://github.com/charmbracelet/gold/blob/master/styles/gallery/README.md)!

## Colors

Currently `gold` uses the [Aurora ANSI colors](https://godoc.org/github.com/logrusorgru/aurora#Index).

## Development

Style definitions located in `styles/` can be embedded into the binary by
running [statik](https://github.com/rakyll/statik):

```console
statik -f -src styles -include "*.json"
```

You can re-generate screenshots of all available styles by running `gallery.sh`.
This requires `termshot` and `pngcrush` installed on your system!
