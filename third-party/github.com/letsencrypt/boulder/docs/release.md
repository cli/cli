# Boulder Release Process

A description and demonstration of the full process for tagging a normal weekly
release, a "clean" hotfix release, and a "dirty" hotfix release.

Once a release is tagged, it will be generally deployed to
[staging](https://letsencrypt.org/docs/staging-environment/) and then to
[production](https://acme-v02.api.letsencrypt.org/) over the next few days.

## Goals

1. All development, including reverts and hotfixes needed to patch a broken
   release, happens on the `main` branch of this repository. Code is never
   deployed without being reviewed and merged here first, and code is never
   landed on a release branch that isn't landed on `main` first.

2. Doing a normal release requires approximately zero thought. It Just Works.

3. Doing a hotfix release differs as little as possible from the normal release
   process.

## Release Schedule

Boulder developers make a new release at the beginning of each week, typically
around 10am PST **Monday**. Operations deploys the new release to the [staging
environment](https://letsencrypt.org/docs/staging-environment/) on **Tuesday**,
typically by 2pm PST. If there have been no issues discovered with the release
from its time in staging, then on **Thursday** the operations team deploys the
release to the production environment.

Holidays, unexpected bugs, and other resource constraints may affect the above
schedule and result in staging or production updates being skipped. It should be
considered a guideline for normal releases but not a strict contract.

## Release Structure

All releases are tagged with a tag of the form `release-YYYY-MM-DD[x]`, where
the `YYYY-MM-DD` is the date that the initial release is cut (usually the Monday
of the current week), and the `[x]` is an optional lowercase letter suffix
indicating that the release is an incremental hotfix release. For example, the
second hotfix release (i.e. third release overall) in the third week of January
2022 was
[`release-2022-01-18b`](https://github.com/letsencrypt/boulder/releases/tag/release-2022-01-18b).

All release tags are signed with a key associated with a Boulder developer. Tag
signatures are automatically verified by GitHub using the public keys that
developer has uploaded, and are additionally checked before being built and
deployed to our staging and production environments. Note that, due to how Git
works, in order for a tag to be signed it must also have a message; we set the
tag message to just be a slightly more readable version of the tag name.

## Making a Release

### Prerequisites

* You must have a GPG key with signing capability:
  * [Checking for existing GPG keys](https://docs.github.com/en/free-pro-team@latest/github/authenticating-to-github/checking-for-existing-gpg-keys)

* If you don't have a GPG key with signing capability, create one:
  * [Generating a new local GPG key](https://docs.github.com/en/free-pro-team@latest/github/authenticating-to-github/generating-a-new-gpg-key)
  * [Generating a new Yubikey GPG key](https://support.yubico.com/hc/en-us/articles/360013790259-Using-Your-YubiKey-with-OpenPGP)

* The signing GPG key must be added to your GitHub account:
  * [Adding a new GPG key to your GitHub
    account](https://docs.github.com/en/free-pro-team@latest/github/authenticating-to-github/adding-a-new-gpg-key-to-your-github-account)

* `git` *may* need to be configured to call the correct GPG binary:
  * The default: `git config --global gpg.program gpg` is correct for most Linux platforms
  * On macOS and some Linux platforms: `git config --global gpg.program gpg2` is correct

* `git` must be configured to use the correct GPG key:
  * [Telling Git about your GPG key](https://docs.github.com/en/free-pro-team@latest/github/authenticating-to-github/telling-git-about-your-signing-key)

* Understand the [process for signing tags](https://docs.github.com/en/free-pro-team@latest/github/authenticating-to-github/signing-tags)

### Regular Releases

Simply create a signed tag whose name and message both include the date that the
release is being tagged (not the date that the release is expected to be
deployed):

```sh
go run github.com/letsencrypt/boulder/tools/release/tag@main
```

This will print the newly-created tag and instructions on how to push it after
you are satisfied that it is correct. Alternately you can run the command with
the `-push` flag to push the resulting tag automatically.

### Hotfix Releases

Sometimes it is necessary to create a new release which looks like a prior
release but with one or more additional commits added. This is usually the case
when we discover a critical bug in the currently-deployed version that needs to
be fixed, but we don't want to include other changes that have already been
merged to `main` since the currently-deployed release was tagged.

In this situation, we create a new hotfix release branch starting at the point
of the previous release tag. We then use the normal GitHub PR and code-review
process to merge the necessary fix(es) to the branch. Finally we create a new release tag at the tip of the release branch instead of the tip of main.

To create the new release branch, substitute the name of the release tag which you want to use as the starting point into this command:

```sh
go run github.com/letsencrypt/boulder/tools/release/branch@main v0.YYYYMMDD.0
```

This will create a release branch named `release-branch-v0.YYYYMMDD`. When all necessary PRs have been merged into that branch, create the new tag by substituting the branch name into this command:

```sh
go run github.com/letsencrypt/boulder/tools/release/tag@main release-branch-v0.YYYYMMDD
```

## Deploying Releases

When doing a release, SRE's tooling will check that:

1. GitHub shows that tests have passed for the commit at the planned release
   tag.

2. The planned release tag is an ancestor of the current `main` on GitHub, or
   the planned release tag is equal to the head of a branch named
   `release-branch-XXX`, and all commits between `main` and the head of that
   branch are cherry-picks of commits which landed on `main` following the
   normal review process.
