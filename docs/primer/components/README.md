# Components

Components are consistent, reusable patterns that we use throughout the command line tool.

## Syntax

We show meaning or objects through syntax such as angled brackets, square brackets, curly brackets, parenthesis, and color.

### Branches

Display branch names in brackets and/or cyan

![A branch name in brackets and cyan](images/Syntax-Branch.png)

### Labels

Display labels in parenthesis and/or gray

![A label name in parenthesis and gray](images/Syntax-Label.png)

### Repository

Display repository names in bold where appropriate

![A repository name in bold](images/Syntax-Repo.png)

### Help

Use consistent syntax in [help pages](/docs/command-line-syntax.md) to explain command usage.

#### Literal text

Use plain text for parts of the command that cannot be changed

```shell
gh help
```

The argument help is required in this command.

#### Placeholder values

Use angled brackets to represent a value the user must replace. No other expressions can be contained within the angled brackets.

```shell
gh pr view <issue-number>
```

Replace "issue-number" with an issue number.

#### Optional arguments

Place optional arguments in square brackets. Mutually exclusive arguments can be included inside square brackets if they are separated with vertical bars.


```shell
gh pr checkout [--web]
```

The argument `--web` is optional.

```shell
gh pr view [<number> | <url>]
```

The "number" and "url" arguments are optional.

#### Required mutually exclusive arguments

Place required mutually exclusive arguments inside braces, separate arguments with vertical bars.

```shell
gh pr {view | create}
```

#### Repeatable arguments

Ellipsis represent arguments that can appear multiple times

```shell
gh pr close <pr-number>...
```

#### Variable naming

For multi-word variables use dash-case (all lower case with words separated by dashes)


```shell
gh pr checkout <issue-number>
```

#### Additional examples

Optional argument with placeholder:

```shell
<command> <subcommand> [<arg>]
```

Required argument with mutually exclusive options:

```shell
<command> <subcommand> {<path> | <string> | literal}
```

Optional argument with mutually exclusive options:

```shell
<command> <subcommand> [<path> | <string>]
```

## Prompts

Generally speaking, prompts are the CLI’s version of forms.

- Use prompts for entering information
- Use a prompt when user intent is unclear
- Make sure to provide flags for all prompts

### Yes/No

Use for yes/no questions, usually a confirmation. The default (what will happen if you enter nothing and hit enter) is in caps.

![An example of a yes/no prompt](images/Prompt-YesNo.png)

### Short text

Use to enter short strings of text. Enter will accept the auto fill if available

![An example of a short text prompt](images/Prompt-ShortText.png)

### Long text

Use to enter large bodies of text. E key will open the user’s preferred editor, and Enter will skip.

![An example of a long text prompt](images/Prompt-LongText.png)

### Radio select

Use to select one option

![An example of a radio select prompt](images/Prompt-RadioSelect.png)

### Multi select

Use to select multiple options

![An example of a multi select prompt](images/Prompt-MultiSelect.png)

## State

The CLI reflects how GitHub.com displays state through [color](/docs/primer/foundations#color) and [iconography](/docs/primer/foundations#iconography).

![A collection of examples of state from various command outputs](images/States.png)

## Progress indicators

For processes that might take a while, include a progress indicator with context on what’s happening.

![An example of a loading spinner when forking a repository](images/Progress-Spinner.png)

## Headers

When viewing output that could be unclear, headers can quickly set context for what the user is seeing and where they are.

### Examples

![An example of the header of the `gh pr create` command](images/Headers-Examples.png)

The header of the `gh pr create` command reassures the user that they're creating the correct pull request.

![An example of the header of the `gh pr list` command](images/Headers-gh-pr-list.png)

The header of the `gh pr list` command sets context for what list the user is seeing.

## Lists

Lists use tables to show information.

- State is shown in color.
- A header is used for context.
- Information shown may be branch names, dates, or what is most relevant in context.

![An example of gh pr list](images/Lists-gh-pr-list.png)

## Detail views

Single item views show more detail than list views. The body of the item is rendered indented. The item’s URL is shown at the bottom.

![An example of gh issue view](images/Detail-gh-issue-view.png)

## Empty states

Make sure to include empty messages in command outputs when appropriate.

![The empty state of the gh pr status command](images/Empty-states-1.png)

The empty state of `gh pr status`

![The empty state of the gh issue list command](images/Empty-states-2.png)

The empty state of `gh issue list`

## Help pages

Help commands can exist at any level:

- Top level (`gh`)
- Second level (`gh [command]`)
- Third level (`gh [command] [subcommand]`)

Each can be accessed using the `--help` flag, or using `gh help [command]`.

Each help page includes a combination of different sections.

### Required sections

- Usage
- Core commands
- Flags
- Learn more
- Inherited flags

### Other available sections

- Additional commands
- Examples
- Arguments
- Feedback

### Example

![The output of gh help](images/Help.png)
