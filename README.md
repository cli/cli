# gh - The GitHub CLI tool

The #ce-cli team is working on a publicly available CLI tool to reduce the friction between GitHub and one's local machine for people who use the command line primarily to interact with Git and GitHub. https://github.com/github/releases/issues/659

This tool is an endeavor separate from [github/hub](https://github.com/github/hub), which acts as a proxy to `git`, since our aim is to reimagine from scratch the kind of command line interface to GitHub that would serve our users' interests best.

# Installation

_warning, gh is in a very alpha phase_

## OSX

`brew install github/gh/gh`

## Debian/Ubuntu Linux

1. Download the latest `.deb` file from the [releases page](https://github.com/github/gh-cli/releases)
2. Install it with `sudo dpkg -i gh_0.2.2_linux_amd64.deb`, changing version number accordingly

_(Uninstall with `sudo apt remove gh`)_

## Fedora/Centos Linux

1. Download the latest `.rpm` file from the [releases page](https://github.com/github/gh-cli/releases)
2. Install it with `sudo yum localinstall gh_0.2.2_linux_amd64.rpm`, changing version number accordingly

_(Uninstall with `sudo yum remove gh`)_

## Other Linux

1. Download the latest `_linux_amd64.tar.gz` file from the [releases page](https://github.com/github/gh-cli/releases)
2. `tar -xvf gh_0.2.2_linux_amd64.tar.gz`, changing version number accordingly
3. Copy the uncompressed `gh` somewhere on your `$PATH` (e.g. `sudo cp gh /usr/local/bin/`)

_(Uninstall with `rm`)_

# Process

- [Demo planning doc](https://docs.google.com/document/d/18ym-_xjFTSXe0-xzgaBn13Su7MEhWfLE5qSNPJV4M0A/edit)
- [Weekly tracking issue](https://github.com/github/gh-cli/labels/tracking%20issue)
- [Weekly sync notes](https://docs.google.com/document/d/1eUo9nIzXbC1DG26Y3dk9hOceLua2yFlwlvFPZ82MwHg/edit)

# How to create a release

This can all be done from your local terminal.

1. `git tag 'vVERSION_NUMBER' # example git tag 'v0.0.1'`
2. `git push origin vVERSION_NUMBER`
3. Wait a few minutes for the build to run and CI to pass. Look at the [actions tab](https://github.com/github/gh-cli/actions) to check the progress.
4. Go to <https://github.com/github/homebrew-gh/releases> and look at the release

# Test a release

A local release can be created for testing without creating anything official on the release page.

1. `git tag 'v6.6.6' # some throwaway version number`
2. `env GH_OAUTH_CLIENT_SECRET=foobar GH_OAUTH_CLIENT_ID=1234 goreleaser --skip-publish --rm-dist`
3. Check and test files in `dist/`
4. `git tag -d v6.6.6 # delete the throwaway tag`
