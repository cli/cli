# Multiple Accounts with the CLI - v2.40.0

Since its creation, `gh` has enforced a mapping of one account per host. Functionally, this meant that when targeting a
single host (e.g. github.com) each `auth login` would replace the token being used for API requests, and for git
operations when `gh` was configured as a git credential manager. Removing this limitation has been a [long requested
feature](https://github.com/cli/cli/issues/326), with many community members offering workarounds for a variety of use cases.
A particular shoutout to @gabe565 and his long term community support for https://github.com/gabe565/gh-profile in this space.

With the release of `v2.40.0`, `gh` has begun supporting multiple accounts for some use cases on github.com and
in GitHub Enterprise. We recognise that there are a number of missing quality of life features, and we've opted
not to address the use case of automatic account switching based on some context (e.g. `pwd`, `git remote`).
However, we hope many of those using these custom solutions will now find it easier to obtain and update tokens (via the standard
OAuth flow rather than as a PAT), and to store them securely in the system keyring managed by `gh`.

We are by no means excluding these things from ever being native to `gh` but we wanted to ship this MVP and get more
feedback so that we can iterate on it with the community.

## What is in scope for this release?

The support for multiple accounts in this release is focused around `auth login` becoming additive in behaviour.
This allows for multiple accounts to be easily switched between using the new `auth switch` command. Switching the "active"
user for a host will swap the token used by `gh` for API requests, and for git operations when `gh` was configured as a
git credential manager.

We have extended the `auth logout` command to switch account where possible if the currently active user is the target
of the `logout`. Finally we have extended `auth token`, `auth switch`, and `auth logout` with a
`--user` flag. This new flag in combination with `--hostname` can be used to disambiguate accounts when running
non-interactively.

Here's an example usage. First, we can see that I have a single account `wilmartin_microsoft` logged in, and
`auth status` reports that this is the active account:

```
➜ gh auth status
github.com
  ✓ Logged in to github.com account wilmartin_microsoft (keyring)
  - Active account: true
  - Git operations protocol: https
  - Token: gho_************************************
  - Token scopes: 'gist', 'read:org', 'repo', 'workflow'
```

Running `auth login` and proceeding through the browser based OAuth flow as `williammartin`, we can see that
`auth status` now reports two accounts under `github.com`, and our new account is now marked as active.

```
➜ gh auth login
? What account do you want to log into? GitHub.com
? What is your preferred protocol for Git operations on this host? HTTPS
? How would you like to authenticate GitHub CLI? Login with a web browser

! First copy your one-time code: A1F4-3B3C
Press Enter to open github.com in your browser...
✓ Authentication complete.
- gh config set -h github.com git_protocol https
✓ Configured git protocol
✓ Logged in as williammartin

➜ gh auth status
github.com
  ✓ Logged in to github.com account williammartin (keyring)
  - Active account: true
  - Git operations protocol: https
  - Token: gho_************************************
  - Token scopes: 'gist', 'read:org', 'repo', 'workflow'

  ✓ Logged in to github.com account wilmartin_microsoft (keyring)
  - Active account: false
  - Git operations protocol: https
  - Token: gho_************************************
  - Token scopes: 'gist', 'read:org', 'repo', 'workflow'
```

Fetching our username from the API shows that our active token correctly corresponds to `williammartin`:

```
➜ gh api /user | jq .login
"williammartin"
```

Now we can easily switch accounts using `gh auth switch`, and hitting the API shows that the active token has been
changed:

```
➜ gh auth switch
✓ Switched active account for github.com to wilmartin_microsoft

➜ gh api /user | jq .login
"wilmartin_microsoft"
```

We can use `gh auth token --user` to get a specific token for a user (which should be handy for automated switching
solutions):

```
➜ GH_TOKEN=$(gh auth token --user williammartin) gh api /user | jq .login
"williammartin"
```

Finally, running `gh auth logout` presents a prompt when there are multiple choices for logout, and switches account
if there are any remaining logged into the host:

```
➜ gh auth logout
? What account do you want to log out of? wilmartin_microsoft (github.com)
✓ Logged out of github.com account wilmartin_microsoft
✓ Switched active account for github.com to williammartin
```

## What is out of scope for this release?

As mentioned above, we know that this only addresses some of the requests around supporting multiple accounts. While
these are not out of scope forever, for this release some of the big things we have intentionally not included are:
 * Automatic account switching based on some context (e.g. `pwd`, `git remote`)
 * Automatic configuration of git config such as `user.name` and `user.email` when switching
 * User level configuration e.g. `williammartin` uses `vim` but `wilmartin_microsoft` uses `emacs`

## What are some sharp edges in this release?

As in any MVP there are going to be some sharp edges that need to be smoothed out over time. Here are a list of known
sharp edges in this release.

### Data Migration

The trickiest piece of this work was that the `hosts.yml` file only supported a mapping of one-to-one in the host to
account relationship. Having persistent data on disk that required a schema change presented a compatibility challenge
both backwards for those who use [`go-gh`](https://github.com/cli/go-gh/) outside of `gh`, and forward for `gh` itself
where we try to ensure that it's possible to use older versions in case we accidentally make a breaking change for users.

As such, from this release, running any command will attempt to migrate this data into a new format, and will
additionally add a `version` field into the `config.yml` to aid in our future maintainability. While we have tried
to maintain forward compatibility (except in one edge case outlined below), and in the worst case you should be able
to remove these files and start from scratch, if you are concerned about the data in these files, we advise you to take
a backup.

#### Forward Compatibility Exclusion

There is one known case using `--insecure-storage` that we don't maintain complete forward and backward compatibility.
This occurs if you `auth login --insecure-storage`, upgrade to this release (which performs the data migration), run
`auth login --insecure-storage` again on an older release, then at some time later use `auth switch` to make this
account active. The symptom here would be usage of an older token (which may for example have different scopes).

This occurs because we will only perform the data migration once, moving the original insecure token to a place where
it would later be used by `auth switch`.

#### Immutable Config Users

Some of our users lean on tools to manage their application configuration in an immutable manner for example using
https://github.com/nix-community/home-manager. These users will hit an error when we attempt to persist the new
`version` field to the `config.yml`. They will need to ensure that the `home-manager` configuration scripts are updated
to add `version: 1`.

See https://github.com/nix-community/home-manager/issues/4744 for more details.

### Auth Refresh

Although this has always been possible, the multi account flow increases the likelihood of doing something surprising
with `auth refresh`. This command allows for a token to be updated with additional or fewer scopes. For example,
in the following example we add the `read:project` scope to the scopes for our currently active user `williammartin`,
and proceed through the OAuth browser flow as `williammartin`:

```
➜ gh auth refresh -s read:project
? What account do you want to refresh auth for? github.com

! First copy your one-time code: E79E-5FA2
Press Enter to open github.com in your browser...
✓ Authentication complete.

➜ gh auth status
github.com
  ✓ Logged in to github.com account williammartin (keyring)
  - Active account: true
  - Git operations protocol: https
  - Token: gho_************************************
  - Token scopes: 'gist', 'read:org', 'read:project', 'repo', 'workflow'

  ✓ Logged in to github.com account wilmartin_microsoft (keyring)

  ✓ Logged in to github.com account wilmartin_microsoft (keyring)
  - Active account: false
  - Git operations protocol: https
  - Token: gho_************************************
  - Token scopes: 'gist', 'read:org', 'repo', 'workflow'
```

However, what happens if I try to remove the `workflow` scope from my active user `williammartin` but proceed through
the OAuth browser flow as `wilmartin_microsoft`?

```
➜ gh auth refresh -r workflow

! First copy your one-time code: EEA3-091C
Press Enter to open github.com in your browser...
error refreshing credentials for williammartin, received credentials for wilmartin_microsoft, did you use the correct account in the browser?
```

When adding or removing scopes for a user, the CLI gets the scopes for the current token and then requests a new token with the requested amended scopes. Unfortunately, when we go through the account switcher flow as a different user, we end up getting a token for the wrong user with surprising scopes. We don't believe that starting and ending a `refresh` as different accounts is
a use case we wish to support and has the potential for misuse. As such, we have begun erroring in this case.

Note that a token has still been minted on the platform but `gh` will refuse to store it. We are investigating
alternative approaches with the platform team to put some better guardrails in place earlier in the flow.

### Account Switcher on GitHub Enterprise

When using `auth login` with github.com, if a user has multiple accounts in the browser, they should be presented
with an interstitial page that allows for proceeding as any of their accounts. However, for Device Control Flow OAuth
flows, this feature has not yet made it into GHES.

For the moment, if you have multiple accounts on GHES that you wish to log in as, you will need to ensure that you
are authenticated as the correct user in the browser before running `auth login`.
