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
git tag -s -m "Boulder release $(date +%F)" -s "release-$(date +%F)"
git push origin "release-$(date +%F)"
```

### Clean Hotfix Releases

If a hotfix release is necessary, and the desired hotfix commits are the **only** commits which have landed on `main` since the initial release was cut (i.e. there are not any commits on `main` which we want to exclude from the hotfix release), then the hotfix tag can be created much like a normal release tag.

If it is still the same day as an already-tagged release, increment the letter suffix of the tag:

```sh
git tag -s -m "Boulder hotfix release $(date +%F)a" -s "release-$(date +%F)a"
git push origin "release-$(date +%F)a"
```

If it is a new day, simply follow the regular release process above.

### Dirty Hotfix Release

If a hotfix release is necessary, but `main` already contains both commits that
we do and commits that we do not want to include in the hotfix release, then we
must go back and create a release branch for just the desired commits to be
cherry-picked to. Then, all subsequent hotfix releases will be tagged on this
branch.

The commands below assume that it is still the same day as the original release
tag was created (hence the use of "`date +%F`"), but this may not always be the
case. The rule is that the date in the release branch name should be identical
to the date in the original release tag. Similarly, this may not be the first
hotfix release; the rule is that the letter suffix should increment (e.g. "b",
"c", etc.) for each hotfix release with the same date.

```sh
git checkout -b "release-branch-$(date +%F)" "release-$(date +%F)"
git cherry-pick baddecaf
git tag -s -m "Boulder hotfix release $(date +%F)a" "release-$(date +%F)a"
git push origin "release-branch-$(date +%F)" "release-$(date +%F)a"
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
