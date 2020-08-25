# Triage role

As we get more issues and pull requests opened on the GitHub CLI, we've decided on a weekly rotation
triage role. The initial expectation is that the person in the role for the week spends no more than
1-2 hours a day on this work; we can refine that as needed. Below is a basic timeline for a typical
triage day.

1. Note the time
2. Open every new [issue](https://github.com/cli/cli/issues?q=is%3Aopen+is%3Aissue)/[pr](https://github.com/cli/cli/pulls?q=is%3Apr+is%3Aopen+draft%3Afalse) in a tab
3. Go through each one and look for things that should be closed outright (See the PR and Issue section below for more details.)
4. Go through again and look for issues that are worth keeping around, update each one with labels/pings
5. Go through again and look for PRs that solve a useful problem but lack obvious things like tests or passing builds; request changes on those
6. Mark any remaining PRs (i.e. ones that look worth merging with a cursory glance) as `community` PRs and move to Needs Review
7. Look for [issues](https://github.com/cli/cli/issues?q=is%3Aopen+is%3Aissue) and [PRs](https://github.com/cli/cli/pulls?q=is%3Apr+is%3Aopen+draft%3Afalse+sort%3Aupdated-desc) updated in the last day and see if they need a response.
8. Check the clock at each step and just bail out when an hour passes

# Incoming issues

just imagine a flowchart

- can this be closed outright?
  - e.g. spam/junk
  - close without comment
- do we not want to do it?
  - e.g. have already discussed not wanting to do or duplicate issue
  - comment acknowledging receipt
  - close
- do we want someone in the community to do it?
  - e.g. the task is relatively straightforward, but no people on our team have the bandwidth to take it on at the moment
  - ensure that the thread contains all the context necessary for someone new to pick this up
  - add `help-wanted` label
- do we want to do it, but not in the next year?
  - comment acknowledging it, but that we don't plan on working on it this year.
  - add `future` label
  - add additional labels as needed(examples include `enhancement` or `bug`)
  - close
- do we want to do it?
  - e.g. bugs or things we have discussed before
  - comment acknowledging it
  - label appropriately
  - add to project TODO column if appropriate, otherwise just leave it labeled
- is it intriguing but needs discussion?
  - label `needs-design` if design input is needed, ping
  - label `needs-investigation` if engineering research is required before action can be taken
  - ping engineers if eng needed
  - ping product if it's about future directions/roadmap/big changes
- does it need more info from the issue author?
  - ask the user for that
  - add `needs-user-input` label
- is it a user asking for help and you have all the info you need to help?
  - try and help

# Incoming PRs

just imagine a flowchart

- can it be closed outright?
  - ie spam/junk
- do we not want to do it?
  - ie have already discussed not wanting to do, duplicate issue
  - comment acknowledging receipt
  - close
- is it intriguing but needs discussion?
  - request an issue
  - close
- is it something we want to include?
  - add `community` label
  - add to `needs review` column

# Weekly PR audit

In the interest of not letting our open PR list get out of hand (20+ total PRs _or_ multiple PRs
over a few months old), try to audit open PRs each week with the goal of getting them merged and/or
closed. It's likely too much work to deal with every PR, but even getting a few closer to done is
helpful.

For each PR, ask:

- is this too stale (more than two months old or too many conflicts)? close with comment
- is this really close but author is absent? push commits to finish, request review
- is this waiting on triage? go through the PR triage flow

## Examples

We want the cli/cli repo to be a safe and encouraging open-source environment. Below are some examples
of how to empathetically respond to or close an issue/PR:

- [Closing a quality PR its scope is too large](https://github.com/cli/cli/pull/1161)
- [Closing a stale PR](https://github.com/cli/cli/pull/557#issuecomment-639077269)
- [Closing a PR that doesn't follow our CONTRIBUTING policy](https://github.com/cli/cli/pull/864)
- [Responding to a bug report](https://github.com/desktop/desktop/issues/9195#issuecomment-592243129)
- [Closing an issue that out of scope](https://github.com/cli/cli/issues/777#issuecomment-612926229)
- [Closing an issue with a feature request](https://github.com/desktop/desktop/issues/9722#issuecomment-625461766)
