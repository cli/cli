# Triage role

As we get more issues and pull requests opened on the GitHub CLI, we've decided on a weekly rotation
triage role. The initial expectation is that the person in the role for the week spends no more than
2 hours a day on this work; we can refine that as needed.

## Expectations for incoming issues

All incoming issues need either an `enhancement`, `bug`, or `docs` label.

To be considered triaged, `enhancement` issues require at least one of the following additional labels:

- `core`: reserved for the core CLI team
- `help wanted`: signal that we are accepting contributions for this
- `discuss`: add to our team's queue to discuss during a sync
- `needs-investigation`: work that requires a mystery be solved by the core team before it can move forward
- `needs-user-input`: we need more information from our users before the task can move forward

To be considered triaged, `bug` issues require a severity label: one of `p1`, `p2`, or `p3`

## Expectations for community pull requests

All incoming pull requests are assigned to one of the engineers for review on a round-robin basis.
The person in a triage role for a week could take a glance at these pull requests, mostly to see whether
the changeset is feasible and to allow the associated CI run for new contributors.

## Issue triage flowchart

- can this be closed outright?
  - e.g. spam/junk
  - close without comment
- do we not want to do it?
  - e.g. have already discussed not wanting to do or duplicate issue
  - comment and close
- are we ok with outside contribution for this?
  - e.g. the task is relatively straightforward, but no people on our team have the bandwidth to take it on at the moment
  - ensure that the thread contains all the context necessary for someone new to pick this up
  - add `help wanted` label
  - consider adding `good first issue` label
- do we want to do it?
  - comment acknowledging that
  - add `core` label
  - add to the project “TODO” column if this is something that should ship soon
- is it intriguing, but requires discussion?
  - label `discuss`
  - label `needs-investigation` if engineering research is required before action can be taken
- does it need more info from the issue author?
  - ask the user for details
  - add `needs-user-input` label
- is it a usage/support question?
  - consider converting the Issue to a Discussion

## Weekly PR audit

In the interest of not letting our open PR list get out of hand (20+ total PRs _or_ multiple PRs
over a few months old), try to audit open PRs each week with the goal of getting them merged and/or
closed. It's likely too much work to deal with every PR, but even getting a few closer to done is
helpful.

For each PR, ask:

- is this too stale (more than two months old or too many conflicts)? close with comment
- is this really close but author is absent? push commits to finish, request review
- is this waiting on triage? go through the PR triage flow

## Useful aliases

This gist has some useful aliases for first responders:

https://gist.github.com/vilmibm/ee6ed8a783e4fef5b69b2ed42d743b1a

## Examples

We want our project to be a safe and encouraging open-source environment. Below are some examples
of how to empathetically respond to or close an issue/PR:

- [Closing a quality PR its scope is too large](https://github.com/cli/cli/pull/1161)
- [Closing a stale PR](https://github.com/cli/cli/pull/557#issuecomment-639077269)
- [Closing a PR that doesn't follow our CONTRIBUTING policy](https://github.com/cli/cli/pull/864)
- [Responding to a bug report](https://github.com/desktop/desktop/issues/9195#issuecomment-592243129)
- [Closing an issue that out of scope](https://github.com/cli/cli/issues/777#issuecomment-612926229)
- [Closing an issue with a feature request](https://github.com/desktop/desktop/issues/9722#issuecomment-625461766)
