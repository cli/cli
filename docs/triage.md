# Triage role

The primary responsibility of the First Responder (FR) during their weekly rotation is to triage incoming issues and pull requests from the open source community. An issue is considered "triaged" when the `needs-triage` label is removed.

## Quick Guide

Pick an issue from the triage queue.

**Your goal:** Do what is needed to remove the `needs-triage` label.

1. **Can we close it?**
   - Duplicate → Comment and close as duplicate, linking the original
   - Spam → Add `invalid` or `suspected-spam` (auto-closes)
   - Abuse → Add `invalid`, remove content, report, block (see [Spam and abuse](#spam-and-abuse))
   - Off-topic → Add `off-topic` (auto-closes with comment)

2. **Is it a bug?**
   - Reproducible → Add `bug` and a priority label (`priority-1`, `priority-2`, or `priority-3`)
   - Not reproducible → Add `unable-to-reproduce` (auto-requests info, 14-day timer)

3. **Is it an enhancement?**
   - Clear value → Add `enhancement` (auto-posts backlog comment)
   - Unclear → Comment for clarification and add `more-info-needed` (14-day timer)

4. **Is it a pull request?** (see [Community pull requests](#community-pull-requests))
   - Spam or AI sludge → Add `invalid` (auto-closes)
   - Tiny fix (e.g., typo) → Review, test, and merge directly
   - Not linked to a help-wanted issue → Add `no-help-wanted-issue` (auto-closes with comment)
   - Valid → Add `ready-for-review` and run CI (auto-removes `needs-triage`, auto-posts acknowledging comment)

The `needs-triage` label is automatically removed when end-state labels (`enhancement`, `bug`, `ready-for-review`) are applied or the issue is closed.

## Bug Triage

1. Try to reproduce the issue
2. If reproducible (or strongly suspect an intermittent bug) → add `bug` and a priority label
3. If not reproducible → add `unable-to-reproduce` (auto-requests info, 14-day timer) or request clarification with `more-info-needed`

### Bug Priorities

| Priority | Description |
|----------|-------------|
| `priority-1` | Affects a large population and inhibits work. **Escalate internally via the appropriate incident channel; may require a hotfix.** |
| `priority-2` | Affects more than a few users but does not prevent core functions |
| `priority-3` | Affects a small number of users or is largely cosmetic |

## Enhancement Triage

**Do:**
- Ensure the value is clear (ask if needed) and apply `more-info-needed` while waiting for clarification
- Apply the `enhancement` label once value is clear (auto-posts backlog comment)

**Don't:**
- Deep-dive technical feasibility
- Prematurely accept or suggest the feature will be added

## Community Pull Requests

Community pull requests receive `needs-triage` (as well as `external`) just like issues do, but **are not meant to be reviewed as part of triage.**

The triager's responsibility is to do a quick pass:

1. **Spam or AI sludge** → Add `invalid` label (auto-closes). Block user if necessary.
2. **Tiny mergeable fix** (e.g., typo) → Review, test, and merge.
3. **Not related to a help-wanted issue** → Add `no-help-wanted-issue` (auto-closes with comment).
4. **Valid for review** → Add `ready-for-review` and run CI (auto-removes `needs-triage`, auto-posts acknowledging comment).

The pull request will be auto-assigned to an engineer on the team; that engineer will wait to review until `needs-triage` is removed.

## Spam and Abuse

The primary goal of triaging spam and abuse is to remove distracting and offensive content from our community.

- **Spam issues:** Add the `invalid` label (auto-closes as "won't do").
- **Spam comments:** Mark as spam using GitHub's built-in feature.
- **Abusive content:** Defined by our [Code of Conduct](../.github/CODE-OF-CONDUCT.md). Remove the content. Repeat offenses or particularly offensive abuse should be reported and the user blocked.

## Automated Workflows

| Label | Automation |
|-------|------------|
| `needs-triage` | Auto-added on open; removed when classified or closed |
| `more-info-needed` | Auto-closes after 14 days without response |
| `unable-to-reproduce` | Auto-adds `more-info-needed` + posts comment |
| `enhancement` | Auto-posts backlog comment |
| `invalid` | Auto-closes immediately |
| `suspected-spam` | Auto-closes immediately |
| `off-topic` | Auto-posts explanation comment + closes |
| `no-help-wanted-issue` | Auto-posts explanation comment + closes |
| `ready-for-review` | Auto-removes `needs-triage` + posts acknowledging comment |

## Examples

We want our project to be a safe and encouraging open-source environment. Below are some examples of how to empathetically respond to or close an issue/PR:

- [Closing a quality PR when its scope is too large](https://github.com/cli/cli/pull/1161)
- [Closing a stale PR](https://github.com/cli/cli/pull/557#issuecomment-639077269)
- [Closing a PR that doesn't follow our CONTRIBUTING policy](https://github.com/cli/cli/pull/864)
- [Responding to a bug report](https://github.com/desktop/desktop/issues/9195#issuecomment-592243129)
- [Closing an issue that is out of scope](https://github.com/cli/cli/issues/777#issuecomment-612926229)
- [Closing an issue with a feature request](https://github.com/desktop/desktop/issues/9722#issuecomment-625461766)

