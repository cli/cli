# Terminal mockup canvas

A [GitHub Copilot app](https://github.com/github/app) canvas extension
that renders mock-up `gh` output as VSCode-styled terminal screenshots. Built
for producing marketing imagery (blog posts, changelogs, social) where real
terminal recordings are impractical.

## Using it

Open the canvas from a Copilot app session. Pick a starting mockup from the
library dropdown, edit the content and toolbar options, and export a PNG via
the download button. Files download through the browser/runtime, which
typically lands them in the configured downloads directory.

The toolbar controls font, font size, width, window chrome (macOS or none),
backdrop (subtle blue glow / grid / none), and an "auto-style" toggle that
colorizes common `gh` patterns without requiring inline tags.

## Content markup

Content can be authored as raw ANSI escapes, or with a more readable bracket
syntax that the renderer maps to the VSCode Dark+ palette:

- Named colors: `[red]`, `[green]`, `[yellow]`, `[blue]`, `[magenta]`,
  `[cyan]`, `[white]`, `[black]` (bright variants prefixed `br`, e.g.
  `[brblue]`), plus `[muted]` for grayed-out text and `[link]` for blue
  underlined link styling.
- Modifiers: `[bold]` (or `[b]`), `[italic]` (or `[i]`), `[underline]`
  (or `[u]`), `[dim]`.
- Each tag closes with its matching `[/name]`, e.g. `[red]error[/red]`.

When auto-style is on, the renderer also colorizes PR/issue states, labels,
checkboxes, timestamps, and similar conventional output without explicit tags.

## Library

Mockups live in two locations:

- **Project library** at `./library/*.json`: committed to the repo, the
  shared starting set.
- **User library** at `$COPILOT_HOME/extensions/terminal-mockup/artifacts/*.json`:
  local-only, for personal experiments.

Saving a new mockup writes to the user library by default; renaming an
existing one preserves its scope. The dropdown shows both, prefixed by scope.

## Vendored dependencies

[`assets/html2canvas.min.js`](./assets/html2canvas.min.js) is the unmodified
[html2canvas](https://github.com/niklasvh/html2canvas) 1.4.1 distribution
(MIT). Used to rasterize the rendered DOM into a PNG in-browser.
