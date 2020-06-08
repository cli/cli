# Triage role

As we get more issues and pull requests opened on the GitHub CLI, we've decided on a weekly rotation
triage role. The initial expectation is that the person in the role for the week spends no more than
1-2 hours a day on this work; we can refine that as needed.

# Incoming issues

just imagine a flowchart

- can this be closed outright?
  - e.g. spam/junk
  - close without comment
- do we not want to do it?
  - e.g. have already discussed not wanting to do or duplicate issue
  - comment acknowledging receipt
  - close
- do we want to do it? 
  - e.g. bugs or things we have discussed before
  - comment acknowledging it
  - label appropriately
  - add to project TODO column if appropriate, otherwise just leave it labeled
- is it intriguing but needs discussion?
  - label design-needed if amanda is needed, ping
  - ping engineers if eng needed
  - ping billy if producty
- does it need more info?
  - ask the user for that
  - add user input label
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

In the interest of not letting our open PR list get out of hand (20+ total PRs _or_  multiple PRs
over a few months old), try to audit open PRs each week with the goal of getting them merged and/or
closed. It's likely too much work to deal with every PR, but even getting a few closer to done is
helpful.

For each PR, ask:

- is this too stale? close with comment
- is this really close but author is absent? push commits to finish, request review
- is this waiting on triage? go through the PR triage flow
