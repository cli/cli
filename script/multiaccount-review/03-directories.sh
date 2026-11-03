#!/bin/bash
# Replay: bash 03-directories.sh --gh /absolute/path/to/gh
# Select --case nested|worktree and --phase setup|exercise|validate|cleanup
# with the same absolute --fixture path to inspect individual phases.
# Default fixtures are private, uniquely allocated under HOME and removed on exit.
# Inherited authentication variables are discarded with env -i before any gh/Git use.
set -euo pipefail
SELF=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
source "$(dirname "$SELF")/fixture.sh"
LAYER=03 CASES='nested worktree'
review_bootstrap "$@"

setup_case() {
  fixture_setup
  mkdir -p work/team agents
  bind "$FIXTURE/work" work
  if [[ $CASE = nested ]]; then
    repo work/project company
    repo work/team/project company
    echo 'Prepared parent work identity, nested team directory, and two repositories.'
  else
    bind "$FIXTURE/agents" personal
    repo work/project company
    git -C work/project -c commit.gpgsign=false commit --allow-empty -qm 'Offline fixture'
    git -C work/project worktree add --quiet --detach "$FIXTURE/agents/linked"
    echo 'Prepared external worktree under a conflicting personal directory binding.'
    echo 'Main checkout is under work; linked checkout is under agents.'
  fi
}

exercise_case() {
  if [[ $CASE = nested ]]; then
    cd "$FIXTURE/work/team"
    "$GH" auth use personal
  else
    cd "$FIXTURE/agents/linked"
    "$GH" auth use --view --hostname "$HOST"
  fi
}

validate_case() {
  echo 'Independent readbacks and Git credential/SSH routing (simulated authentication):'
  if [[ $CASE = nested ]]; then
    echo 'Parent repository keeps work:'
    selection "$FIXTURE/work/project" work
    credential "$FIXTURE/work/project" company fixture-alice
    echo 'Nested repository selects the deeper personal directory:'
    selection "$FIXTURE/work/team/project" personal
    credential "$FIXTURE/work/team/project" company fixture-bob
    transport "$FIXTURE/work/team/project" personal-ssh.invalid
    echo 'PASS: recursive defaults and the deepest directory win.'
  else
    echo 'External worktree inherits the main checkout location, not agents:'
    selection "$FIXTURE/agents/linked" work
    credential "$FIXTURE/agents/linked" company fixture-alice
    transport "$FIXTURE/agents/linked" work-ssh.invalid
    echo 'Main checkout agrees:'
    selection "$FIXTURE/work/project" work
    echo 'PASS: linked and main checkouts inherit work despite agents=personal.'
  fi
  active_unchanged
}

review_main
