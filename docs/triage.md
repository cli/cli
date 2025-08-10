# Triage role

As we get more issues and pull requests opened on the GitHub CLI, we've decided on a weekly rotation triage role as defined by our First Responder (FR) rotation. The primary responsibility of the FR during that week is to triage incoming issues from the Open Source community, as defined below. An issue is considered "triaged" when the `needs-triage` label is removed.

## Expectations for triaging incoming issues

Review and label [open issues missing either the `enhancement`, `bug`, or `docs` label](https://github.com/cli/cli/issues?q=is%3Aopen+is%3Aissue+-label%3Abug%2Cenhancement%2Cdocs+) and the label(s) corresponding to the command space prefixed with `gh-`, such as `gh-pr` for the `gh pr` command set, or `gh-extension` for the `gh extension` command set. 

The heuristics for triaging the different issue types are as follows:

### Bugs

For bugs, the FR should engage with the issue and community with the goal to remove the `needs-triage` label from the issue.

To be considered triaged, `bug` issues require the following:

- A severity label `p1`, `p2`, and `p3`
- Clearly defined Acceptance Criteria, added to the Issue as a standalone comment (see [example](https://github.com/cli/cli/issues/9469#issuecomment-2292315743))

#### Bug severities

| Severity | Description |
| - | - |
| `p1` | Affects a large population and inhibits work |
| `p2` | Affects more than a few users but doesn't prevent core functions |
| `p3` | Affects a small number of users or is largely cosmetic |

### Enhancements and Docs

For `enhancement` issues, the FR's role is to prepare the issue for team review and triage.

When a new issue is opened, the FR **should**:

- Acknowledge the issue
- Assign themselves to the issue
- Ensure there is enough information to understand the enhancement's scope and value
- Ask the user for more information about value and use-case, if necessary
- Leave the `needs-triage` label on the issue
- Add the `needs-user-input` and `needs-investigation` labels as needed

When the FR has enough information to be triaged, they should:
- Remove the `needs-user-input` and `needs-investigation` labels
- Remove their assignment from the issue

The FR should **avoid**:

- Thoroughly investigating the enhancement's technical feasibility
- Prematurely accepting the enhancement request
- Removing the `needs-triage` label
- Labeling issues as `help wanted`

## Additional triaging labels

The FR can consider adding any of the following labels below.

| Label | Description |
| - | - |
| `discuss` | Some issues require discussion with the internal team. Adding this label will automatically open up an internal discussion with the team to facilitate this discussion. |
| `core` |  Defines what we would like to do internally. We tend to lean towards `help wanted` by default, and adding `core` should be reserved for trickier issues or implementations we have strong opinions/preferences about. |
| `needs-user-input` | After asking any contributors for more information, add this label so it is clear that the issue has been responded to and we are waiting on the user. |
| `needs-investigation` | Used when the issue requires further investigation before it can be reviewed and triaged. This is often used for issues that are not clearly bugs or enhancements, or when the FR needs to gather more information before proceeding. |
| `invalid` | Added to spam and abusive issues. |

### Labels for team enhancement triaging

The FR should **avoid** adding these labels outside of team enhancement triage.

| Label | Description |
| - | - |
| `good first issue` | Used to denote when an issue may be a good candidate for a first-time contributor to the CLI. These are usually small and well defined issues. |
| `help wanted` | These issues are ready for community contribution. |
| `help wanted candidate` | Used to denote when an issue may be a good candidate for community contribution. Issues labelled this way are discussed internally and may be promoted to `help wanted`. |

## Expectations for community pull requests

All incoming pull requests are assigned to one of the engineers for review on a load-balanced basis.
The person in a triage role for a week could take a glance at these pull requests, mostly to see whether
the changeset is feasible and to allow the associated CI run for new contributors.

## Spam and abuse

The primary goal of triaging spam and abuse is to remove distracting and offensive content from our community.

We get a lot of spam. Whenever you determine an issue as spam, add the `invalid` label and close it as "won't do". For spammy comments, simply mark them as spam using GitHub's built-in spam feature.

Abusive contributions are defined by our [Code of Conduct](../.github/CODE-OF-CONDUCT.md). Any contribution you determine abusive should be removed. Repeat offenses or particularly offensive abuse should be reported using GitHub's reporting features and the user blocked. If an entire issue is abusive, label it as `invalid` and close as "won't do".

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
