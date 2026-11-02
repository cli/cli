#!/bin/bash
# Replay: bash 04-owners.sh --gh /absolute/path/to/gh
# Select --case precedence|ambiguous and --phase setup|exercise|validate|cleanup
# with the same absolute --fixture path to inspect individual phases.
# Default fixtures are private, uniquely allocated under HOME and removed on exit.
# Inherited authentication variables are discarded with env -i before any gh/Git use.
set -euo pipefail
SELF=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
source "$(dirname "$SELF")/fixture.sh"
LAYER=04 CASES='precedence ambiguous'
review_bootstrap "$@"

setup_case() {
  fixture_setup
  if [[ $CASE = precedence ]]; then
    mkdir -p team
    bind "$FIXTURE/team" personal
    repo elsewhere/project company
    repo team/default company
    repo team/override company
    bind "$FIXTURE/team/override" project
    echo 'Prepared an unbound repository, directory=personal, and repository=project.'
    echo 'Owner company is not yet bound.'
  else
    "$GH" auth use work --owner company >> setup.log 2>&1
    "$GH" auth use personal --owner personal >> setup.log 2>&1
    repo ambiguous company
    git -C ambiguous remote add fork "git@$HOST:personal/project.git"
    "$GH" auth use --list > before-bindings.json
    echo 'Prepared two remotes: company -> work; personal -> personal.'
    echo 'No explicit directory or repository choice resolves the conflict.'
  fi
}

exercise_case() {
  if [[ $CASE = precedence ]]; then
    cd "$FIXTURE/elsewhere"
    "$GH" auth use work --owner company
  else
    cd "$FIXTURE/ambiguous"
    set +e
    "$GH" auth use --view --hostname "$HOST" > "$FIXTURE/refusal.out" 2>&1
    local status=$?
    set -e
    cat "$FIXTURE/refusal.out"
    [[ $status = 1 ]] || { echo "Expected refusal (exit 1), got $status" >&2; exit 1; }
    grep -F 'multiple owners select different identities' "$FIXTURE/refusal.out" >/dev/null
  fi
}

validate_case() {
  if [[ $CASE = precedence ]]; then
    echo 'Owner fallback works outside any repository:'
    (cd "$FIXTURE/elsewhere" && "$GH" auth use --view --owner company --hostname "$HOST") | grep -E '"(name|account)"'
    credential "$FIXTURE/elsewhere" company fixture-alice
    echo 'Unbound repository uses owner work:'
    selection "$FIXTURE/elsewhere/project" work
    echo 'Explicit directory beats owner:'
    selection "$FIXTURE/team/default" personal
    credential "$FIXTURE/team/default" company fixture-bob
    echo 'Explicit repository beats both:'
    selection "$FIXTURE/team/override" project
    transport "$FIXTURE/team/override" project-ssh.invalid
    echo 'PASS: repository > directory > owner, including Git outside a checkout.'
  else
    "$GH" auth use --list > after-bindings.json
    cmp before-bindings.json after-bindings.json
    echo 'PASS: refusal did not change saved bindings.'
    echo 'Configured remote owners remain distinct:'
    git -C ambiguous remote -v
    echo 'PASS: neither owner was silently chosen for repository inspection.'
  fi
  active_unchanged
}

review_main
