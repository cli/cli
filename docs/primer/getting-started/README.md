# Getting Started

## Principles

### Reasonable defaults, easy overrides

Optimize for what most people will need to do most of the time, but make it easy for people to adjust it to their needs. Often this means considering the default behavior of each command, and how it might need to be adjusted with flags.

### Make it feel like GitHub

Using this tool, it should be obvious that it’s GitHub and not anything else. Use details that are specific to GitHub, such as language or color. When designing output, reflect the GitHub.com interface as much as possible and appropriate.

<table>
  <tr>
    <td>
      Do: Use language accurate to GitHub.com.
      <img alt="`gh pr close` command" src="images/Principle2-05.png" />
    </td>
    <td>
      Don't: Don't use language that GitHub.com doesn't use.
      <img alt="`gh pr delete` command" src="images/Principle2-02.png" />
    </td>
  </tr>
</table>

<table>
  <tr>
    <td>
      Do: Use sentence case.
      <img alt="Pull request with request being a lowercase r" src="images/Principle2-04.png" />
    </td>
    <td>
      Don't: Don't use title case.
      <img alt="Pull Request with Request being an uppercase R" src="images/Principle2-01.png" />
    </td>
  </tr>
</table>

**Resources**

- [GitHub Brand Content Guide](https://brand.github.com)

### Reduce cognitive load

Command line interfaces are not as visually intuitive as graphical interfaces. They have very few affordances (indicators of use), rely on memory, and are often unforgiving of mistakes. We do our best to design our commands to mitigate this.

Reducing cognitive load is necessary for [making an accessible product](https://www.w3.org/TR/coga-usable/#summary) .

**Ways to reduce cognitive load**

- Include confirm steps, especially for riskier commands
- Include headers to help set context for output
- Ensure consistent command language to make memorizing easier
- Ensure similar commands are visually and behaviorally parallel. \* For example, any create command should behave the same
- Anticipate what people might want to do next. \* For example, we ask if you want to delete your branch after you merge.
- Anticipate what mistakes people might make

### Bias towards terminal, but make it easy to get to the browser

We want to help people stay in the terminal wherever they might want to maintain focus and reduce context switching, but when it’s necessary to jump to GitHub.com make it obvious, fast, and easy. Certain actions are probably better to do in a visual interface.

![A prompt asking 'What's next?' with the choice 'Preview in browser' selected.](images/Principle4-01.png)

A preview in browser step helps users create issues and pull requests more smoothly.

![The `gh pr create command` with `--title` and `--body` flags outputting a pull request URL.](images/Principle4-02.png)

Many commands output the relevant URL at the end.

![The `gh issue view` command with the `--web` flag. The output is opening a URL in the browser.](images/Principle4-03.png)

Web flags help users jump to the browser quickly

## Process

When designing for the command line, consider:

### 1. What the command does

- What makes sense to do from a terminal? What doesn’t?
- What might people want to automate?
- What is the default behavior? What flags might you need to change that behavior?
- What might people try and fail to do and how can you anticipate that?

### 2. What the command is called

- What should the  [command language system](/docs/primer/foundations#language)  be?
- What should be a command vs a flag?
- How can you align the language of the new command with the existing commands?

### 3. What the command outputs

- What can you do to make the CLI version [feel like the GitHub.com version](#make-it-feel-like-github), using [color](/docs/primer/foundations#color), [language](/docs/primer/foundations#language), [spacing](/docs/primer/foundations#spacing), info shown, etc?
- How should the [machine output](/docs/primer/foundations#scriptability) differ from the interactive behavior?

### 4. How you explain your command

- You will need to provide a short and long description of the command for the [help pages](/docs/primer/components#help).

### 5. How people discover your command

- Are there ways to integrate CLI into the feature where it exists on other platforms?

## Prototyping

When designing for GitHub CLI, there are several ways you can go about prototyping your ideas.

### Google Docs

![A screenshot of the Google Docs template](images/Prototyping-GoogleDocs.png)

Best for simple quick illustrations of most ideas

Use [this template](https://docs.google.com/document/d/1JIRErIUuJ6fTgabiFYfCH3x91pyHuytbfa0QLnTfXKM/edit?usp=sharing), or format your document with these steps:

1. Choose a dark background (File > Page Setup > Page Color)
1. Choose a light text color
1. Choose a monospace font

**Tips**

- Mix it up since people’s setups change so much. Not everyone uses dark background!
- Make use of the document outline and headers to help communicate your ideas

### Figma

![A screenshot of the Figma library](images/Prototyping-Figma.png)

If you need to show a process unfolding over time, or need to show a prototype that feels more real to users, Figma or code prototypes are best.

[**Figma library**](https://www.figma.com/file/zYsBk5KFoMlovE4g2f4Wkg/Primer-Command-Line) (accessible to GitHub staff only)
