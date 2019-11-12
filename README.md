# gh - The GitHub CLI tool

The #ce-cli team is working on a publicly available CLI tool to reduce the friction between GitHub and one's local machine for people who use the command line primarily to interact with Git and GitHub. https://github.com/github/releases/issues/659

This tool is an endeavor separate from [github/hub](https://github.com/github/hub), which acts as a proxy to `git`, since our aim is to reimagine from scratch the kind of command line interface to GitHub that would serve our users' interests best.

# Process

- [Demo planning doc](https://docs.google.com/document/d/18ym-_xjFTSXe0-xzgaBn13Su7MEhWfLE5qSNPJV4M0A/edit)
- [Weekly tracking issue](https://github.com/github/gh-cli/labels/tracking%20issue)
- [Weekly sync notes](https://docs.google.com/document/d/1eUo9nIzXbC1DG26Y3dk9hOceLua2yFlwlvFPZ82MwHg/edit)

# How to create a release

This can all be done from your local terminal.

1. `git tag -a vYOUR_VERSION_NUMBER -m 'vYOUR_VERSION_NUMBER'`
2. `git push origin vYOUR_VERSION_NUMBER`
3. Go to https://github.com/github/homebrew-gh/releases and look at the release (read https://github.com/github/homebrew-gh for how to install the release via homebrew)