# Accessibility

GitHub CLI (`gh`) brings GitHub to your terminal. We want everyone to be able to use it, including people who:

- Use a screen reader, braille display, or screen magnification
- Need high contrast or their own colors
- Are sensitive to motion

This document covers the accessibility settings in `gh`, known barriers and workarounds, how to report a barrier, and how we work on accessibility. To read about the settings in your terminal, run `gh accessibility` or `gh a11y`.

## Accessibility settings

These settings are off by default. Most can be turned on with `gh config set` or with an environment variable. To check what you've set with `gh config set`, run `gh config list`. Settings marked preview may change as we learn from feedback.

### Prompts

The default prompts redraw parts of the screen as you move through choices. Screen readers and braille displays can have trouble following these changes.

- **Accessible prompter (preview):** This prompter doesn't redraw the screen. It asks each question as plain text, shows choices as a numbered list, and waits for you to type your answer. Run `gh config set accessible_prompter enabled` or set `GH_ACCESSIBLE_PROMPTER=enabled`.
- **No prompts:** Run `gh config set prompt disabled` or set the `GH_PROMPT_DISABLED` environment variable. Commands then take their input from flags. Each command's `--help` lists its flags.

### Text progress indicators

While some commands run, `gh` shows an animated spinner. The animation can cause discomfort for people who are sensitive to motion, and it can confuse screen readers.

To show a text message such as `Working...` instead, run `gh config set spinner disabled` or set `GH_SPINNER_DISABLED=yes`.

### Colors

- **Customizable colors (preview):** Run `gh config set accessible_colors enabled` or set `GH_ACCESSIBLE_COLORS=enabled`. `gh` then limits its colors to your terminal's 16-color palette, which you can customize. It also picks colors to suit a light or dark background, including when it renders Markdown.
- **No color:** Set `NO_COLOR` to any value, or set `CLICOLOR=0`.
- **Label colors:** To show labels in the same colors as on GitHub, run `gh config set color_labels enabled` or set `GH_COLOR_LABELS=enabled`. This needs a terminal that supports truecolor.

### Other options

- Add `--json` to many commands to get structured output, and use `--jq` or `--template` to pick fields and format them. Run `gh help formatting` to learn more.
- Add `--web` to many commands to use GitHub in your web browser instead.
- Add `--editor` to `gh issue create` or `gh pr create` to write the title and body in your text editor instead of answering prompts. To make this the default, run `gh config set prefer_editor_prompt enabled`.
- To print long output straight to the terminal instead of through a pager, run `gh config set pager cat`.
- Run `gh help environment` for more settings, such as `GLAMOUR_STYLE` for the Markdown style and `GH_MDWIDTH` for the Markdown line width.

## Known limitations

These are known barriers in `gh`, with workarounds where we have them. For the full list, see [open issues with the `accessibility` label](https://github.com/cli/cli/issues?q=is%3Aissue%20state%3Aopen%20label%3Aaccessibility).

- **Tables can be hard to follow with a screen reader.** Commands like `gh pr list` print tables, and `gh status` prints columns. Matching a value to its column can take a lot of moving around. For many list commands, you can use `--json` with `--jq` or `--template` to choose the fields and how they print. For example, this prints one pull request per line:

  ```shell
  gh pr list --json number,title --jq '.[] | "\(.number): \(.title)"'
  ```

  `gh status` doesn't support `--json` yet. See [#13029](https://github.com/cli/cli/issues/13029) and [#6496](https://github.com/cli/cli/issues/6496).
- **Moving by word in default text prompts doesn't work as expected.** Ctrl+Left and Ctrl+Right move one character at a time ([#6014](https://github.com/cli/cli/issues/6014)). Key mappings that send escape sequences to move by word can make the prompt fail ([#544](https://github.com/cli/cli/issues/544)). To avoid these prompts, pass text with flags such as `--title` and `--body`, read the body from a file with `--body-file`, or write in your text editor with `--editor`.
- **`gh extension browse` shows the selected row only with color.** This includes its `--single-column` mode, which is meant for screen readers and high zoom. Use `gh extension search` to find extensions and `gh repo view` to read about one. See [#7068](https://github.com/cli/cli/issues/7068).
- **Some terminals show stray escape codes.** In a terminal that can't move the cursor, such as a shell inside Emacs or one with `TERM=dumb`, menus print raw escape codes. Turning off prompts and passing input as flags avoids the menus, and `NO_COLOR` turns off color codes. See [#5721](https://github.com/cli/cli/issues/5721). In GNU screen, stray escape codes can appear in the output. People in [#11008](https://github.com/cli/cli/issues/11008) report that setting `TERM=dumb` avoids them.
- **`CLICOLOR_FORCE` overrides `NO_COLOR`.** If `CLICOLOR_FORCE` is set to anything other than `0`, `gh` prints color even when `NO_COLOR` or `CLICOLOR=0` is set. Unset `CLICOLOR_FORCE` to turn color off. See [#13335](https://github.com/cli/cli/issues/13335).
- **Search matches can be hard to read on light themes.** `gh search code` highlights matches with black text on a yellow background, using the colors from your terminal theme. Some light themes make that combination hard to read. Changing those colors in your terminal theme changes the highlight. See [#8955](https://github.com/cli/cli/issues/8955).
- **`gh` is only available in English.** See [#961](https://github.com/cli/cli/issues/961).

## Reporting a barrier

If something in `gh` is hard or impossible for you to use, please tell us. You don't need to tell us about any disability.

[Open a bug report](https://github.com/cli/cli/issues/new?template=bug_report.md) and include as much of this as you can:

- What you were trying to do, and what happened instead
- The command you ran
- The output of `gh version`
- Your operating system, terminal, and shell
- Any assistive technology you use, such as a screen reader or magnifier, and its version
- Any settings from this document that you've turned on
- A recording or screenshot, if you're comfortable sharing one

To share ideas or talk with others about accessibility across GitHub, join the [accessibility discussions in GitHub Community](https://github.com/orgs/community/discussions/categories/accessibility).

### What happens next

- A member of our team triages new issues and sets bug priority as described in our [triage process](../docs/triage.md).
- We add the `accessibility` label so these issues are easy to find.
- If we need more details, we'll ask in the issue. If we know a workaround, we'll share it.
- We can't promise when a fix or feature will ship. Pull requests that fix an issue link to it, so you can follow progress there. For feature requests, thumbs-up reactions help us prioritize.

## Supported environments

`gh` is supported on macOS, Windows, and Linux.

GitHub publishes an [accessibility conformance report for `gh`](https://accessibility.github.com/conformance/cli/), which you can also open with `gh accessibility --web`. The report is based on an audit against the Level A and AA criteria of the Web Content Accessibility Guidelines (WCAG) 2.2, and it covers a listed set of commands. Testing for the report used keyboard-only interaction, the JAWS and NVDA screen readers with Terminal and Command Prompt, a color contrast analyzer, and platform features such as high contrast and zoom. See the report for its date and full scope.

The report doesn't cover other screen readers, such as VoiceOver on macOS or Orca on Linux. If you use one, we'd like to hear what works and what doesn't.

Prompts have known problems in MinTTY, the default terminal for Git Bash on Windows. Run `gh help mintty` for workarounds.

Extensions are separate programs maintained in their own repositories, and they may not support the settings in this document. Report barriers in an extension to its repository.

## How we work on accessibility

The terminal has no accessibility standard as complete as WCAG is for the web. We base our work on our [CLI design guidelines](../docs/primer/README.md), research into how assistive technology works with terminals, and feedback from people who use `gh`.

Our priorities:

- **Screen readers and braille displays:** Output should make sense as plain text, without screen redraws. Punctuation such as periods, commas, and colons helps screen readers pause in the right places.
- **Contrast and color choice:** Colors should be readable on light and dark backgrounds, and you should be able to change them in your terminal.
- **Meaning without color or symbols:** Color and symbols add meaning but should never be the only way to show it.
- **Less motion:** Animation should be optional, with a text alternative.
- **Working without prompts:** Every prompt should have a matching flag, so you can skip prompts.

### What we expect from contributors

Contributions should keep these settings working and avoid new barriers:

- Follow the [CLI design guidelines](../docs/primer/foundations/README.md), especially the sections on [color](../docs/primer/foundations/README.md#color), [iconography](../docs/primer/foundations/README.md#iconography), and [scriptability](../docs/primer/foundations/README.md#scriptability).
- Use `IOStreams` for color and progress indicators, and the factory's `Prompter` for prompts, so user settings apply. See [output and scriptability](../docs/command-development.md#output-and-scriptability).
- Give every prompt a matching flag.
- For changes to prompts or output, show the before and after in the pull request. If an accessibility setting changes that output, also show it with the setting turned on.

### Ownership

The GitHub CLI team maintains `gh` and this document. We update it when we add accessibility settings or learn about new barriers.

## Feedback on this document

If something here is wrong, unclear, or missing, [open an issue](https://github.com/cli/cli/issues/new/choose). To report a barrier in `gh` itself, see [Reporting a barrier](#reporting-a-barrier).
