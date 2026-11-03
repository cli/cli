#!/usr/bin/env bash

set -euo pipefail

if (( BASH_VERSINFO[0] < 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] < 4) )); then
  printf 'identity-e2e requires Bash 4.4+ (on macOS, use Homebrew bash rather than /bin/bash).\n' >&2
  exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
readonly host="github.com"
gh_bin=""
run_root=""
keep=false
interactive=true
api_only=false
list_only=false
first_login=""
second_login=""
first_token=""
second_token=""
current_scenario=""
scenario_root=""
authenticated_login=""
captured_output=""
scenario_active_login=""
scenario_other_login=""
original_path="$PATH"
real_ssh=""
declare -a requested_scenarios=()
declare -a created_repositories=()
declare -a uploaded_ssh_keys=()
declare -a kept_resources=()

BOLD="" DIM="" RESET="" GREEN="" RED="" YELLOW="" CYAN=""
if [[ -t 1 ]]; then
  BOLD=$'\033[1m' DIM=$'\033[2m' RESET=$'\033[0m'
  GREEN=$'\033[32m' RED=$'\033[31m' YELLOW=$'\033[33m' CYAN=$'\033[36m'
fi

# Account setup and guided review are adapted from the original runner.
# shellcheck source=script/identity-e2e/bootstrap.sh
source "$script_dir/bootstrap.sh"

usage() {
  cat <<'EOF'
Usage: run.sh [options]

Guided, LIVE GitHub journeys for reusable identities and auth use (requires make debug).
Offers two healthy saved github.com accounts, otherwise uses browser login.
Each journey restores those accounts into isolated gh/Git configuration.

  --scenario NAME      Select a journey (repeatable)
  --all                Run all journeys (default)
  --gh PATH            Binary to exercise (default: this worktree's bin/gh)
  --interactive        Pause at identity-behavior steps; e opens exploration
  --non-interactive    Skip journey pauses (account selection still needs a TTY)
  --keep               Preserve local state AND created GitHub resources
  --run-root PATH      Parent for a new, uniquely named temporary directory
  --list               List journeys without credentials or network access
  -h, --help           Show this help

Journeys:
  https-directory     Recursive identities; private clone, commit, push, and PR
  https-owner         Owner identities; private clone, commit, push, and PR
  ssh-directory       Same journey with isolated, account-verified SSH aliases
  ssh-owner           Owner-selected SSH aliases and independent Git
  https-worktrees     Repository-family choices from external worktrees over HTTPS
  ssh-worktrees       Same family journey with account-verified SSH aliases
  https-account-override  One-off account selection, including gh's PR push
  ssh-account-override    Same override journey with inferred SSH routes
  ssh-cooperation        Preserve an OpenSSH-compatible connection wrapper during overrides
  logout-recovery        Kept identities, missing-account warnings, and signing back in
  identity-lifecycle  API-only: nested directory explanations, live edits, shared
                      account identities, account-only metadata, unsetting, and deletion

Git journeys create private repositories and PRs; SSH journeys also upload keys.
The runner deletes its own resources unless --keep is supplied. The preflight
requires repo, delete_repo, and admin:public_key scopes for Git journeys, matching
the previous runner. No live journey is run merely by listing scenarios.
The runner verifies debug keyring-isolation support before setup. Isolated
login/logout and Git helpers cannot access the OS keyring. Normal saved-account
discovery and token capture use --no-refresh. Tokens/private keys in --keep
directories remain sensitive.
EOF
}

fail() { printf 'identity-e2e: %s\n' "$*" >&2; exit 1; }

scenario_exists() {
  case "$1" in
    https-directory | https-owner | ssh-directory | ssh-owner | https-worktrees | ssh-worktrees | https-account-override | ssh-account-override | ssh-cooperation | logout-recovery | identity-lifecycle) return 0 ;;
    *) return 1 ;;
  esac
}

list_scenarios() {
  printf '%s\n' https-directory https-owner ssh-directory ssh-owner https-worktrees ssh-worktrees https-account-override ssh-account-override ssh-cooperation logout-recovery identity-lifecycle
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --scenario)
        [[ -n "${2:-}" ]] || fail "--scenario requires a known journey"
        scenario_exists "$2" || fail "--scenario requires a known journey"
        contains "$2" "${requested_scenarios[@]}" || requested_scenarios+=("$2")
        shift 2 ;;
      --all)
        requested_scenarios=(https-directory https-owner ssh-directory ssh-owner https-worktrees ssh-worktrees https-account-override ssh-account-override ssh-cooperation logout-recovery identity-lifecycle)
        shift ;;
      --gh) [[ -n "${2:-}" ]] || fail "--gh requires a path"; gh_bin="$2"; shift 2 ;;
      --run-root) [[ -n "${2:-}" ]] || fail "--run-root requires a path"; run_root="$2"; shift 2 ;;
      --keep) keep=true; shift ;;
      --interactive) interactive=true; shift ;;
      --non-interactive) interactive=false; shift ;;
      --list) list_only=true; shift ;;
      -h | --help) usage; exit 0 ;;
      *) fail "unknown argument: $1" ;;
    esac
  done
  if [[ ${#requested_scenarios[@]} -eq 0 ]]; then
    requested_scenarios=(https-directory https-owner ssh-directory ssh-owner https-worktrees ssh-worktrees https-account-override ssh-account-override ssh-cooperation logout-recovery identity-lifecycle)
  fi
  api_only=true
  local scenario
  for scenario in "${requested_scenarios[@]}"; do
    [[ "$scenario" == identity-lifecycle || "$scenario" == logout-recovery ]] || api_only=false
  done
}

# The earlier prototype supports `--account auth refresh`; this one does not.
# Do not switch/refresh an operator account as a substitute. Retain the original
# offer's browser-login fallback when selected saved accounts need more scopes.
refresh_saved_account_scopes() {
  note "This prototype cannot refresh a named account without switching it."
  note "Using the original fallback: fresh browser login in isolated configuration."
  return 1
}

show_checkpoint_state() {
  local directory="$1"
  heading "Scenario: $current_scenario"
  printf 'Directory: %s\nRun root: %s\nBinary: %s\n' "$directory" "$scenario_root" "$gh_bin"
  "$gh_bin" auth status --hostname "$host" || true
  "$gh_bin" auth identity list
  "$gh_bin" auth use --list
  git config --global --show-origin --list
  (
    cd "$directory"
    "$gh_bin" auth use --view
    "$gh_bin" auth use --explain
    if git rev-parse --git-dir >/dev/null 2>&1; then
      git status --short --branch
      git remote -v
      git config --show-origin --get-regexp '^(user\.|gh-identity\.|credential\.|core\.sshCommand)' || true
    fi
  )
}

account_gh() {
  local login="$1"
  shift
  (
    # Resource setup/cleanup uses a separate, unbound gh config. The selected
    # token is never passed in argv or printed, and never changes the operator.
    mkdir -p "$run_root/maintenance"
    export GH_CONFIG_DIR="$run_root/maintenance"
    export GH_HOST="$host" GH_PROMPT_DISABLED=1
    if [[ "$login" == "$first_login" ]]; then
      export GH_TOKEN="$first_token"
    elif [[ "$login" == "$second_login" ]]; then
      export GH_TOKEN="$second_token"
    else
      fail "no captured account for $login"
    fi
    cd "$run_root"
    "$gh_bin" "$@"
  )
}

cleanup_scenario_resources() {
  local entry login resource key_id remainder
  local -a remaining_repos=() remaining_keys=()
  if [[ "$keep" == true ]]; then
    for entry in "${created_repositories[@]}"; do kept_resources+=("repository ${entry#*:}"); done
    for entry in "${uploaded_ssh_keys[@]}"; do kept_resources+=("SSH key $entry"); done
    created_repositories=()
    uploaded_ssh_keys=()
    return 0
  fi
  for entry in "${created_repositories[@]}"; do
    login="${entry%%:*}" resource="${entry#*:}"
    if ! run_visible "gh repo delete --yes $resource  # maintenance account: $login" \
      account_gh "$login" repo delete --yes "$resource"; then
      remaining_repos+=("$entry")
    fi
  done
  for entry in "${uploaded_ssh_keys[@]}"; do
    login="${entry%%|*}" remainder="${entry#*|}"
    resource="${remainder%%|*}" key_id="${remainder#*|}"
    if [[ -z "$key_id" ]]; then
      if ! key_id="$(account_gh "$login" api user/keys --paginate --jq ".[] | select(.title == \"$resource\") | .id")"; then
        remaining_keys+=("$entry")
        continue
      fi
    fi
    while IFS= read -r key_id; do
      [[ -n "$key_id" ]] || continue
      if ! run_visible "gh api --method DELETE user/keys/$key_id  # maintenance account: $login" \
        account_gh "$login" api --method DELETE "user/keys/$key_id"; then
        remaining_keys+=("$login|$resource|$key_id")
      fi
    done <<<"$key_id"
  done
  created_repositories=("${remaining_repos[@]}")
  uploaded_ssh_keys=("${remaining_keys[@]}")
  [[ ${#created_repositories[@]} -eq 0 && ${#uploaded_ssh_keys[@]} -eq 0 ]]
}

cleanup_on_exit() {
  local status=$?
  trap - EXIT
  trap '' INT TERM
  if ! cleanup_scenario_resources; then
    printf 'Cleanup failed. Preserving recovery files in %s\n' "$run_root" >&2
    printf '%s\n' "${created_repositories[@]}" "${uploaded_ssh_keys[@]}" >"$run_root/cleanup-needed.txt"
    keep=true
    status=1
  fi
  first_token="" second_token=""
  cd /
  if [[ -n "$run_root" && "$keep" != true ]]; then
    rm -rf -- "$run_root"
  elif [[ -n "$run_root" ]]; then
    printf 'Preserved run directory: %s\nWARNING: plaintext credentials and SSH private keys may remain.\n' "$run_root"
    if [[ ${#kept_resources[@]} -gt 0 ]]; then printf 'Preserved: %s\n' "${kept_resources[@]}"; fi
  fi
  exit "$status"
}

finish_scenario() {
  if [[ "$keep" == true ]]; then
    review_step "the journey has completed" "we retain its resources because --keep was requested" \
      "this run remains available for exploration after exit" "$scenario_root/work"
  fi
  cleanup_scenario_resources || fail "cleanup failed"
  if [[ "$keep" == true ]]; then
    review_complete "$scenario_root/work" "journey resources retained according to --keep"
  fi
  log "$current_scenario passed"
  current_scenario=""
}

json_field() {
  jq -er --arg field "$1" '.[$field]'
}

create_identity() {
  local name="$1" login="$2" alias="$3" email="$4"
  local -a args=(auth identity create "$name" --account "$login" --git-user-name "$login identity" --git-user-email "$email")
  [[ -z "$alias" ]] || args+=(--ssh-host "$alias")
  local display
  printf -v display '%q ' gh "${args[@]}"
  review_step "the saved account $login exists independently of identities" \
    "we create reusable identity $name" "its account, SSH route, and commit details are stored together" \
    "$scenario_root/work" "${display% }" "gh auth identity view $name"
  run_visible "${display% }" "$gh_bin" "${args[@]}"
  pass "identity created"
  run_visible "gh auth identity view $name" "$gh_bin" auth identity view "$name"
  pass "supported identity fields are visible"
  review_complete "$scenario_root/work" "the identity is reusable, not yet bound anywhere"
}

bind_identity() {
  local name="$1" scope="$2" value="$3"
  local directory="$value"
  local -a args=(auth use "$name")
  local -a inspect=(auth use --explain)
  if [[ "$scope" == owner ]]; then
    args+=(--owner "$value")
    inspect+=(--owner "$value")
    directory="$scenario_root/work"
  fi
  local display inspection
  printf -v display '%q ' gh "${args[@]}"
  printf -v inspection '%q ' gh "${inspect[@]}"
  review_step "identity $name exists" "we bind it to $scope $value" \
    "the choice references the reusable identity, not copies of its settings" "$directory" \
    "${display% }" "${inspection% }" "gh auth use --list"
  (
    cd "$directory"
    run_visible "${display% }" "$gh_bin" "${args[@]}"
    pass "scope choice installed"
    run_visible "${inspection% }" "$gh_bin" "${inspect[@]}"
    run_visible "gh auth use --list" "$gh_bin" auth use --list
    pass "scope choice is inspectable"
  )
  review_complete "$directory" "edits to $name will affect this choice"
}

configure_ssh() {
  local index login alias key title key_id response status
  mkdir -p "$scenario_root/ssh" "$scenario_root/tools"
  local ssh_config="$scenario_root/ssh/config"
  local known_hosts="$scenario_root/ssh/known_hosts"
  step "Fetch GitHub's published SSH host keys over authenticated HTTPS"
  captured_output="$(account_gh "$first_login" api meta --jq '.ssh_keys[]')"
  [[ -n "$captured_output" ]] || fail "GitHub returned no SSH host keys"
  while IFS= read -r response; do printf 'github.com %s\n' "$response"; done <<<"$captured_output" >"$known_hosts"
  : >"$ssh_config"
  for index in 1 2; do
    if [[ "$index" == 1 ]]; then login="$first_login"; else login="$second_login"; fi
    alias="identity-account-$index"
    key="$scenario_root/ssh/key-$index"
    title="gh-identity-e2e-$current_scenario-$(unique_suffix)"
    step "Generate and upload an isolated SSH key for $login"
    run_visible "ssh-keygen -q -t ed25519 -N '' -f $key" ssh-keygen -q -t ed25519 -N '' -f "$key"
    pass "isolated key generated"
    uploaded_ssh_keys+=("$login|$title|")
    run_visible "gh ssh-key add $key.pub --title $title  # maintenance account: $login" \
      account_gh "$login" ssh-key add "$key.pub" --title "$title"
    pass "public key uploaded"
    key_id="$(account_gh "$login" api user/keys --paginate --jq ".[] | select(.title == \"$title\") | .id")"
    [[ "$key_id" =~ ^[0-9]+$ ]] || fail "uploaded key title did not resolve to one key ID"
    uploaded_ssh_keys[${#uploaded_ssh_keys[@]}-1]="$login|$title|$key_id"
    cat >>"$ssh_config" <<EOF
Host $alias
  HostName github.com
  HostKeyAlias github.com
  User git
  IdentityFile "$key"
  IdentitiesOnly yes
  IdentityAgent none
  UserKnownHostsFile "$known_hosts"
  StrictHostKeyChecking yes
  BatchMode yes
EOF
    chmod 600 "$ssh_config" "$known_hosts"
    step "Verify SSH alias $alias authenticates as $login"
    print_running "ssh -F $ssh_config -G git@$alias"
    captured_output="$("$real_ssh" -F "$ssh_config" -G "git@$alias")"
    assert_contains "hostname github.com" "$captured_output" "alias targets github.com"
    assert_contains "identityfile $key" "$captured_output" "alias selects the isolated key"
    print_running "ssh -F $ssh_config -T git@$alias"
    status=0
    response="$("$real_ssh" -F "$ssh_config" -T "git@$alias" 2>&1)" || status=$?
    printf '%s\n' "$response"
    assert_equal 1 "$status" "GitHub's no-shell response exits 1"
    assert_contains "Hi $login!" "$response" "SSH really authenticates as $login"
  done
  # OpenSSH uses the OS account home on macOS, not HOME. The identity dispatcher
  # delegates through PATH; this wrapper supplies only this scenario's config.
  printf '#!/usr/bin/env bash\nexec %q -F %q "$@"\n' "$real_ssh" "$ssh_config" >"$scenario_root/tools/ssh"
  chmod 700 "$scenario_root/tools/ssh"
  export PATH="$scenario_root/tools:$original_path"
}

exercise_git_journey() {
  local protocol="$1" scope="$2"
  local login identity alias email context repository name url actual index create_name
  prepare_scenario "$current_scenario" "$protocol"
  cd "$scenario_root/work"
  if [[ "$protocol" == ssh ]]; then configure_ssh; fi
  for index in 1 2; do
    alias=""
    identity="account-$index-identity"
    if [[ "$index" == 1 ]]; then login="$first_login"; else login="$second_login"; fi
    if [[ "$protocol" == ssh ]]; then alias="identity-account-$index"; fi
    actual="$(account_gh "$login" api user --jq '"\(.id)+\(.login)@users.noreply.github.com"')"
    email="$actual"
    create_identity "$identity" "$login" "$alias" "$email"
    context="$scenario_root/work/$identity"
    mkdir -p "$context"
    if [[ "$scope" == owner ]]; then
      bind_identity "$identity" owner "$login"
    else
      bind_identity "$identity" directory "$context"
    fi
    cd "$context"
    name="gh-identity-$current_scenario-$(unique_suffix)"
    repository="$login/$name"
    create_name="$name"
    local create_display="gh repo create $name --private"
    if [[ "$scope" == owner ]]; then
      create_name="$repository"
      create_display="gh repo create $repository --private  # maintenance account: $login"
    fi
    review_step "we are outside a repository; identity $identity applies to $scope $login" \
      "we create a private fixture repository and clone it with gh" \
      "a directory choice infers $login as owner without an OWNER/ prefix; owner-only setup remains explicit" "$context" \
      "$create_display" "gh repo clone $repository checkout" "git -C checkout switch -c main"
    # Register before creation so interruption cannot leave an untracked resource.
    created_repositories+=("$login:$repository")
    if [[ "$scope" == owner ]]; then
      run_visible "$create_display" account_gh "$login" repo create "$create_name" --private
    else
      run_capture "$create_display" "$gh_bin" repo create "$create_name" --private
      # If selection regresses, record the unexpected resource before failing.
      for actual in "$first_login/$name" "$second_login/$name"; do
        if [[ "$captured_output" == "https://$host/$actual" && "$actual" != "$repository" ]]; then
          created_repositories+=("${actual%%/*}:$actual")
        fi
      done
      assert_equal "https://$host/$repository" "$captured_output" "unqualified create inferred the directory identity's account as owner"
    fi
    pass "private fixture repository created"
    run_visible "gh repo clone $repository checkout" "$gh_bin" repo clone "$repository" checkout
    pass "gh-invoked Git cloned the private repository"
    run_visible "git -C checkout switch -c main" git -C checkout switch -c main
    pass "empty checkout uses main regardless of the account's default branch"
    review_complete "$context" "clone succeeded using the identity's account and route"
    cd "$context/checkout"
    if [[ "$protocol" == ssh ]]; then url="git@github.com:$repository.git"; else url="https://github.com/$repository.git"; fi
    run_capture "git config --get remote.origin.url" git config --get remote.origin.url
    assert_equal "$url" "$captured_output" "saved remote URL is canonical"
    review_step "the checkout inherits $identity" "we query gh, commit, and push with independently invoked Git" \
      "both authentication paths agree, and author/committer metadata comes from the shared identity" "$PWD" \
      "gh api user --jq .login" "git config --show-origin --get user.email" \
      "printf '%s\n' 'Initial identity fixture' > identity-fixture.txt" "git add identity-fixture.txt" \
      "git -c commit.gpgsign=false commit -m 'Initial identity fixture'" \
      "git push -u origin main"
    run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
    assert_equal "$login" "$captured_output" "gh API selects the contextual account"
    run_capture "git config --show-origin --get user.email" git config --show-origin --get user.email
    assert_contains "$email" "$captured_output" "Git reads email from the identity include"
    run_visible "printf '%s\n' 'Initial identity fixture' > identity-fixture.txt" \
      bash -c "printf '%s\n' 'Initial identity fixture' > identity-fixture.txt"
    run_visible "git add identity-fixture.txt" git add identity-fixture.txt
    run_visible "git -c commit.gpgsign=false commit -m 'Initial identity fixture'" \
      git -c commit.gpgsign=false commit -m 'Initial identity fixture'
    pass "commit created with the identity metadata"
    run_capture "git log -1 --format='%an|%ae|%cn|%ce'" git log -1 '--format=%an|%ae|%cn|%ce'
    assert_equal "$login identity|$email|$login identity|$email" "$captured_output" "author and committer use name/email"
    run_visible "git push -u origin main" git push -u origin main
    pass "independently invoked Git can push to the private repository"
    review_complete "$PWD" "gh API and independent Git agree"
    review_step "main exists remotely" "we push a feature and open a PR through the implicit identity" \
      "the PR author is $login" "$PWD" "git switch -c feature" \
      "printf '%s\n' 'Feature identity fixture' > identity-fixture.txt" "git add identity-fixture.txt" \
      "git -c commit.gpgsign=false commit -m 'Feature identity fixture'" \
      "git push -u origin feature" "gh pr create --title 'Identity fixture' --body 'Temporary identity UX journey' --base main" \
      "gh pr view --json author --jq .author.login"
    run_visible "git switch -c feature" git switch -c feature
    pass "feature branch created"
    run_visible "printf '%s\n' 'Feature identity fixture' > identity-fixture.txt" \
      bash -c "printf '%s\n' 'Feature identity fixture' > identity-fixture.txt"
    run_visible "git add identity-fixture.txt" git add identity-fixture.txt
    run_visible "git -c commit.gpgsign=false commit -m 'Feature identity fixture'" \
      git -c commit.gpgsign=false commit -m 'Feature identity fixture'
    pass "feature commit created"
    run_visible "git push -u origin feature" git push -u origin feature
    pass "feature branch pushed by ordinary Git"
    run_visible "gh pr create --title 'Identity fixture' --body 'Temporary identity UX journey' --base main" \
      "$gh_bin" pr create --title 'Identity fixture' --body 'Temporary identity UX journey' --base main
    pass "PR created through contextual gh account"
    run_capture "gh pr view --json author --jq .author.login" "$gh_bin" pr view --json author --jq .author.login
    assert_equal "$login" "$captured_output" "PR author matches the Git authentication account"
    review_complete "$PWD" "private repository, independent Git push, and PR are owned by $login"
    cd "$scenario_root/work"
  done
  if [[ "$current_scenario" == https-worktrees || "$current_scenario" == ssh-worktrees ]]; then
    exercise_worktree_family "$protocol"
  fi
  finish_scenario
}

exercise_worktree_family() {
  local protocol="$1" alias="" main="$scenario_root/work/account-1-identity/checkout"
  local linked="$scenario_root/work/account-2-identity/agent"
  local future="$scenario_root/future-agent" email context
  [[ "$protocol" != ssh ]] || alias="identity-account-2"
  email="$(account_gh "$second_login" api user --jq '"\(.id)+\(.login)@users.noreply.github.com"')"
  create_identity family-identity "$second_login" "$alias" "$email"
  cd "$main"
  review_step "the main repository inherits account-1-identity from its parent directory" \
    "we create an agent worktree under the other account's directory" \
    "the main repository's location determines inheritance, not the agent checkout's location" "$PWD" \
    "git worktree add --detach $linked" "gh auth use --view --directory $linked" \
    "git -C $linked ls-remote origin"
  run_visible "git worktree add --detach $linked" git worktree add --detach "$linked"
  cd "$linked"
  use_selection account-1-identity
  run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_equal "$first_login" "$captured_output" "external worktree uses the main repository's directory choice"
  run_visible "git ls-remote origin" git ls-remote origin
  review_complete "$PWD" "API and independent Git can access the first account's private repository"

  review_step "the external worktree currently inherits the first account" \
    "we apply family-identity from here and create another worktree" \
    "main, existing, and future worktrees use the second account and its Git defaults" "$PWD" \
    "gh auth use family-identity" "git worktree add --detach $future" \
    "gh auth use --explain" "git -C $main config user.email"
  run_visible "gh auth use family-identity" "$gh_bin" auth use family-identity
  run_visible "git worktree add --detach $future" git worktree add --detach "$future"
  run_visible "gh auth use --explain" "$gh_bin" auth use --explain
  for context in "$main" "$linked" "$future"; do
    cd "$context"
    use_selection family-identity
    run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
    assert_equal "$second_login" "$captured_output" "repository choice is shared by every worktree"
    run_capture "git config user.email" git config user.email
    assert_equal "$email" "$captured_output" "native Git reads repository-family attribution"
  done
  run_expect_failure "git ls-remote origin" git ls-remote origin
  pass "Git no longer authenticates as the private repository's first-account owner"
  review_complete "$PWD" "repository family uses one identity regardless of checkout locations"

  review_step "family-identity is applied to the whole repository" \
    "we unset from a sibling, then reapply and delete the family identity" \
    "each action affects all siblings; the main directory's fallback is restored" "$PWD" \
    "gh auth use --unset" "gh auth use family-identity" "gh auth identity delete family-identity"
  run_visible "gh auth use --unset" "$gh_bin" auth use --unset
  run_capture "gh auth use --view --directory $main" "$gh_bin" auth use --view --directory "$main"
  assert_equal account-1-identity "$(printf '%s' "$captured_output" | json_field name)" "unset from a sibling removed the shared choice"
  run_visible "gh auth use family-identity" "$gh_bin" auth use family-identity
  delete_identities family-identity
  for context in "$main" "$linked" "$future"; do
    cd "$context"
    use_selection account-1-identity
    run_visible "git ls-remote origin" git ls-remote origin
  done
  review_complete "$PWD" "deletion removed the family choice without losing its directory fallback"
  cd "$scenario_root/work"
}

run_account_override() {
  local protocol="$1" current_alias="" override_alias="" email name repository base before sha url connection_command="" connection_count=0
  prepare_scenario "$current_scenario" "$protocol"
  unset GH_PROMPT_DISABLED
  cd "$scenario_root/work"
  review_step "two saved accounts exist without identities" \
    "we select another account for one API invocation" "the next invocation still uses the saved active account" "$PWD" \
    "gh api user --jq .login" "gh api --account $scenario_other_login user --jq .login" "gh api user --jq .login"
  run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_equal "$scenario_active_login" "$captured_output" "ordinary API account"
  run_capture "gh api --account $scenario_other_login user --jq .login" "$gh_bin" api --account "$scenario_other_login" user --jq .login
  assert_equal "$scenario_other_login" "$captured_output" "override needs no identity"
  run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_equal "$scenario_active_login" "$captured_output" "override did not switch accounts"
  review_complete "$PWD" "API override is invocation-local"

  if [[ "$protocol" == ssh ]]; then
    configure_ssh
    if [[ "$current_scenario" == ssh-cooperation ]]; then
      step "Install a fixture connection wrapper before applying identities"
      printf '#!/usr/bin/env bash\nprintf "connection wrapper used\\n" >> %q\nexec %q "$@"\n' \
        "$scenario_root/ssh/connections.log" "$real_ssh" >"$scenario_root/tools/enterprise-ssh"
      chmod 700 "$scenario_root/tools/enterprise-ssh"
      printf -v connection_command '%q -F %q' "$scenario_root/tools/enterprise-ssh" "$scenario_root/ssh/config"
      git config --global core.sshCommand "$connection_command"
      git config --global ssh.variant ssh
    fi
    if [[ "$scenario_active_login" == "$first_login" ]]; then
      current_alias=identity-account-1 override_alias=identity-account-2
    else
      current_alias=identity-account-2 override_alias=identity-account-1
    fi
  fi
  email="$(account_gh "$scenario_active_login" api user --jq '"\(.id)+\(.login)@users.noreply.github.com"')"
  create_identity current-identity "$scenario_active_login" "$current_alias" "$email"
  create_identity override-identity "$scenario_other_login" "$override_alias" override@example.invalid
  bind_identity current-identity directory "$scenario_root/work"
  name="gh-identity-$current_scenario-$(unique_suffix)"
  repository="$scenario_other_login/$name"
  step "Create a private repository owned only by the override account"
  created_repositories+=("$scenario_other_login:$repository")
  run_visible "gh repo create $repository --private --add-readme  # maintenance account: $scenario_other_login" \
    account_gh "$scenario_other_login" repo create "$repository" --private --add-readme

  review_step "the directory selects $scenario_active_login, which cannot access $repository" \
    "we clone with --account $scenario_other_login" "both repository lookup and Git clone use the override" "$PWD" \
    "gh repo clone $repository checkout --account $scenario_other_login"
  run_visible "gh repo clone $repository checkout --account $scenario_other_login" \
    "$gh_bin" repo clone "$repository" checkout --account "$scenario_other_login"
  review_complete "$PWD" "gh-launched Git used the override"
  cd checkout
  base="$(git branch --show-current)"
  url="$(git config --get remote.origin.url)"
  review_step "the checkout still selects current-identity" \
    "we commit an unpublished feature, then let gh push it while opening the PR" \
    "choose $repository at gh's push prompt; authentication changes, attribution does not" "$PWD" \
    "git switch -c feature" "printf '%s\n' 'Account override fixture' > override.txt" "git add override.txt" \
    "git -c commit.gpgsign=false commit -m 'Account override fixture'" \
    "gh pr create --account $scenario_other_login --title 'Account override fixture' --body 'Temporary identity UX journey' --base $base"
  run_visible "git switch -c feature" git switch -c feature
  run_visible "printf '%s\n' 'Account override fixture' > override.txt" bash -c "printf '%s\n' 'Account override fixture' > override.txt"
  run_visible "git add override.txt" git add override.txt
  run_visible "git -c commit.gpgsign=false commit -m 'Account override fixture'" \
    git -c commit.gpgsign=false commit -m 'Account override fixture'
  if [[ -n "$connection_command" ]]; then connection_count="$(wc -l <"$scenario_root/ssh/connections.log")"; fi
  before="$(git log -1 '--format=%H|%an|%ae|%cn|%ce')"
  sha="$(git rev-parse HEAD)"
  run_expect_failure "git rev-parse --verify refs/remotes/origin/feature" git rev-parse --verify refs/remotes/origin/feature
  pass "feature has not been pushed"
  run_visible "gh pr create --account $scenario_other_login --title 'Account override fixture' --body 'Temporary identity UX journey' --base $base" \
    "$gh_bin" pr create --account "$scenario_other_login" --title 'Account override fixture' --body 'Temporary identity UX journey' --base "$base"
  if [[ -n "$connection_command" ]]; then
    (( $(wc -l <"$scenario_root/ssh/connections.log") > connection_count )) || fail "gh's automatic push bypassed the user's connection wrapper"
  fi
  run_capture "gh pr view --account $scenario_other_login --json author --jq .author.login" \
    "$gh_bin" pr view --account "$scenario_other_login" --json author --jq .author.login
  assert_equal "$scenario_other_login" "$captured_output" "PR author is the overridden account"
  run_capture "gh api --account $scenario_other_login repos/$repository/git/ref/heads/feature --jq .object.sha" \
    "$gh_bin" api --account "$scenario_other_login" "repos/$repository/git/ref/heads/feature" --jq .object.sha
  assert_equal "$sha" "$captured_output" "gh pushed the actual feature commit"
  run_capture "git log -1 --format='%H|%an|%ae|%cn|%ce'" git log -1 '--format=%H|%an|%ae|%cn|%ce'
  assert_equal "$before" "$captured_output" "override did not change commit attribution"
  run_capture "git config user.email" git config user.email
  assert_equal "$email" "$captured_output" "later commits retain current-identity's attribution"
  run_capture "git config remote.origin.url" git config remote.origin.url
  assert_equal "$url" "$captured_output" "remote URL was not rewritten"
  if [[ -n "$connection_command" ]]; then
    run_capture "git config --global --no-includes core.sshCommand" git config --global --no-includes core.sshCommand
    assert_equal "$connection_command" "$captured_output" "the user's original connection command is still stored"
    [[ -s "$scenario_root/ssh/connections.log" ]] || fail "Git did not invoke the configured connection wrapper"
  fi
  review_complete "$PWD" "PR creation and its Git push used the same overridden account"

  review_step "the override command has finished" "we call gh and Git without --account" \
    "the original identity still applies; independent Git cannot access the override account's private repository" "$PWD" \
    "gh api user --jq .login" "gh auth use --view" "git ls-remote origin"
  run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_equal "$scenario_active_login" "$captured_output" "API reverted to the persistent identity"
  use_selection current-identity
  if [[ -n "$connection_command" ]]; then connection_count="$(wc -l <"$scenario_root/ssh/connections.log")"; fi
  run_expect_failure "git ls-remote origin" git ls-remote origin
  if [[ -n "$connection_command" ]]; then
    (( $(wc -l <"$scenario_root/ssh/connections.log") > connection_count )) || fail "independent Git bypassed the user's connection wrapper"
  fi
  pass "independently invoked Git did not retain the override"
  review_complete "$PWD" "persistent account, route, remote URL, and attribution are unchanged"
  if [[ -n "$connection_command" ]]; then
    review_step "the configured SSH wrapper has handled the override" "we remove the identities and their bindings" \
      "Git again selects the user's original connection command directly" "$PWD" \
      "gh auth identity delete --all" "git config core.sshCommand"
    delete_identities --all
    run_capture "git config core.sshCommand" git config core.sshCommand
    assert_equal "$connection_command" "$captured_output" "cleanup restored the original SSH command"
    review_complete "$PWD" "identity cleanup did not remove the user's SSH setup"
  fi
  cd "$scenario_root/work"
  finish_scenario
}

delete_identities() {
  local -a args=(auth identity delete "$@")
  [[ "$interactive" == true ]] || args+=(--confirm)
  local display
  printf -v display '%q ' gh "${args[@]}"
  run_visible "${display% }" "$gh_bin" "${args[@]}"
}

use_selection() {
  local expected="$1"
  run_capture "gh auth use --view" "$gh_bin" auth use --view
  assert_equal "$expected" "$(printf '%s' "$captured_output" | json_field name)" "selected identity is $expected"
}

run_identity_lifecycle() {
  prepare_scenario identity-lifecycle https
  cd "$scenario_root/work"
  create_identity work-identity "$first_login" "" "work@example.invalid"
  create_identity public-identity "$first_login" "" "public@example.invalid"
  run_visible "gh auth identity create other-identity --account $second_login" \
    "$gh_bin" auth identity create other-identity --account "$second_login"
  pass "an account-only identity needs no commit metadata"
  create_identity client-identity "$second_login" "" "client@example.invalid"
  mkdir -p tree/first tree/second tree/client
  bind_identity work-identity directory "$scenario_root/work/tree"
  cd tree/client
  review_step "the client subdirectory inherits work-identity and is not a repository" \
    "we use client-identity here and inspect the parent and child explanations" \
    "the more specific directory wins here; the parent shows it as a descendant exception" "$PWD" \
    "gh auth use client-identity" "gh auth use --explain" \
    "gh auth use --explain --json selection,attribution --jq '[.selection.identity.name,.attribution.status] | join(\":\")'" \
    "gh auth use --explain --directory .." "gh api user --jq .login" \
    "git init project" "git -C project config user.email"
  run_visible "gh auth use client-identity" "$gh_bin" auth use client-identity
  run_capture "gh auth use --explain" "$gh_bin" auth use --explain
  assert_contains "more specific directory" "$captured_output" "child explains why it overrides the broader directory"
  run_capture "gh auth use --explain --json selection,attribution --jq '[.selection.identity.name,.attribution.status] | join(\":\")'" \
    "$gh_bin" auth use --explain --json selection,attribution --jq '[.selection.identity.name,.attribution.status] | join(":")'
  assert_equal client-identity:declared "$captured_output" "JSON distinguishes the selected identity and defaults outside a repository"
  run_capture "gh auth use --explain --directory .." "$gh_bin" auth use --explain --directory ..
  assert_contains "Descendant overrides" "$captured_output" "parent explains the narrower exception without selecting it"
  run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_equal "$second_login" "$captured_output" "nested directory changes the API account before any repository exists"
  run_visible "git init project" git init project
  run_capture "git -C project config user.email" git -C project config user.email
  assert_equal client@example.invalid "$captured_output" "new descendant repository inherits the more specific Git identity"
  review_complete "$PWD" "nested directory selection and descendant Git metadata agree"
  cd "$scenario_root/work"
  for context in first second; do
    run_visible "git init tree/$context" git init "tree/$context"
  done
  cd tree/first
  review_step "both repositories inherit work-identity" "we use public-identity for this repository and all its worktrees" \
    "the same account is reused with different commit metadata" "$PWD" \
    "gh auth use public-identity" "gh auth use --view" "gh api user --jq .login" "git config user.email"
  run_visible "gh auth use public-identity" "$gh_bin" auth use public-identity
  pass "repository-family choice installed"
  use_selection public-identity
  run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_equal "$first_login" "$captured_output" "different identity still uses the shared saved account"
  run_capture "git config user.email" git config user.email
  assert_equal public@example.invalid "$captured_output" "more specific directory overrides the broader metadata"
  run_visible "gh auth use --explain" "$gh_bin" auth use --explain
  review_complete "$PWD" "the repository choice overrides its directory fallback"
  review_step "public-identity is live-referenced" "we edit its email and bind a second repository to it" \
    "both repositories pick up later edits without rebinding" "$PWD" \
    "gh auth use public-identity --directory ../second" \
    "gh auth identity edit public-identity --git-user-email edited@example.invalid" \
    "git config user.email" "git -C ../second config user.email"
  run_visible "gh auth use public-identity --directory ../second" "$gh_bin" auth use public-identity --directory ../second
  pass "second repository references the same identity"
  run_visible "gh auth identity edit public-identity --git-user-email edited@example.invalid" \
    "$gh_bin" auth identity edit public-identity --git-user-email edited@example.invalid
  pass "shared identity edited"
  run_capture "git config user.email" git config user.email
  assert_equal edited@example.invalid "$captured_output" "first repository sees edited email"
  run_capture "git -C ../second config user.email" git -C ../second config user.email
  assert_equal edited@example.invalid "$captured_output" "second repository sees edited email"
  review_complete "$PWD" "one file edit updated both references"
  review_step "another saved account has an account-only identity" "we apply it to this repository, then unset the shared choice" \
    "API account changes; optional metadata inherits the broader tree; unsetting reveals work-identity" "$PWD" \
    "gh auth use other-identity" "gh api user --jq .login" "git config user.email" \
    "gh auth use --unset" "gh auth use --view"
  run_visible "gh auth use other-identity" "$gh_bin" auth use other-identity
  pass "account-only repository identity applied"
  run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_equal "$second_login" "$captured_output" "API switches accounts without changing global active account"
  run_capture "git config user.email" git config user.email
  assert_equal work@example.invalid "$captured_output" "account-only leaves broader Git metadata alone"
  run_visible "gh auth use --unset" "$gh_bin" auth use --unset
  pass "repository-family choice removed"
  use_selection work-identity
  review_complete "$PWD" "metadata inheritance is visible rather than silently treated as profile isolation"
  local noreply public_email prior_email
  noreply="$(account_gh "$first_login" api "users/$first_login" --jq '"\(.id)+\(.login)@users.noreply.github.com"')"
  public_email="$(account_gh "$first_login" api "users/$first_login" --jq '.email // ""')"
  review_step "public-identity still supplies attribution to the second repository" \
    "we resolve its email from the account instead of copying an address manually" \
    "the concrete email is saved once; missing public email fails without changing it" "$PWD" \
    "gh auth identity edit public-identity --git-user-email @noreply" \
    "git -C ../second config user.email" \
    "gh auth identity edit public-identity --git-user-email @public"
  run_visible "gh auth identity edit public-identity --git-user-email @noreply" \
    "$gh_bin" auth identity edit public-identity --git-user-email @noreply
  run_capture "git -C ../second config user.email" git -C ../second config user.email
  assert_equal "$noreply" "$captured_output" "the CLI resolved and saved the account's ID-based noreply address"
  prior_email="$captured_output"
  if [[ -n "$public_email" ]]; then
    run_visible "gh auth identity edit public-identity --git-user-email @public" \
      "$gh_bin" auth identity edit public-identity --git-user-email @public
    run_capture "git -C ../second config user.email" git -C ../second config user.email
    assert_equal "$public_email" "$captured_output" "the CLI uses the public profile address"
  else
    run_expect_failure "gh auth identity edit public-identity --git-user-email @public" \
      "$gh_bin" auth identity edit public-identity --git-user-email @public
    assert_contains "has no public profile email" "$captured_output" "a missing public email is explicit, not a fallback"
    run_capture "git -C ../second config user.email" git -C ../second config user.email
    assert_equal "$prior_email" "$captured_output" "a failed lookup left the identity unchanged"
  fi
  review_complete "$PWD" "email selectors change attribution only when explicitly requested"
  review_step "public-identity still applies in the second repository" "we delete it and its remaining bindings" \
    "retype public-identity when prompted; the broader work identity becomes effective" "$PWD" \
    "gh auth identity delete public-identity" "gh auth use --view --directory ../second"
  delete_identities public-identity
  run_capture "gh auth use --view --directory ../second" "$gh_bin" auth use --view --directory ../second
  assert_equal work-identity "$(printf '%s' "$captured_output" | json_field name)" "deletion removed the identity's bindings, revealing the broader choice"
  review_complete "$PWD" "other identities and bindings remain intact"
  review_step "the remaining identities are no longer needed" "we delete all identities and bindings" \
    "retype all identities when prompted; saved accounts remain usable" "$PWD" \
    "gh auth identity delete --all" "gh auth identity list" "gh auth use --list" "gh api user --jq .login"
  delete_identities --all
  run_capture "gh auth identity list" "$gh_bin" auth identity list
  assert_equal '[]' "$captured_output" "all identities removed"
  run_capture "gh auth use --list" "$gh_bin" auth use --list
  assert_equal '[]' "$captured_output" "all bindings removed"
  run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_equal "$scenario_active_login" "$captured_output" "cleanup retained the original saved account"
  review_complete "$PWD" "no identity choices remain"
  cd "$scenario_root/work"
  finish_scenario
}

run_logout_recovery() {
  prepare_scenario logout-recovery https
  cd "$scenario_root/work"
  create_identity work-identity "$first_login" "" work@example.invalid
  bind_identity work-identity directory "$PWD"
  git init -q project
  cd project
  review_step "work-identity selects a signed-in account" "we log out of that account without deleting its identity" \
    "logout explains which choices remain; inspection explains how to recover" "$PWD" \
    "gh auth logout --hostname $host --user $first_login" "gh auth use --view" \
    "gh auth use --explain --json authentication,warnings" "git config user.email"
  run_visible "gh auth logout --hostname $host --user $first_login" "$gh_bin" auth logout --hostname "$host" --user "$first_login"
  use_selection work-identity
  run_visible "gh auth use --explain --json authentication,warnings" "$gh_bin" auth use --explain --json authentication,warnings
  run_capture "gh auth use --explain --json authentication --jq .authentication.api.status" \
    "$gh_bin" auth use --explain --json authentication --jq .authentication.api.status
  assert_equal blocked "$captured_output" "API cannot use the logged-out account"
  run_expect_failure "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_contains "is not signed in" "$captured_output" "does not silently use the other saved account"
  run_capture "git config user.email" git config user.email
  assert_equal work@example.invalid "$captured_output" "logout retained Git attribution"
  review_complete "$PWD" "account credentials and identity preferences have separate lifecycles"
  step "Restore the captured account without changing its identity"
  login_saved_account "$first_token" https "$first_login"
  review_step "the same account is signed back in" "we use the retained identity and then remove it" \
    "no rebinding is needed; deleting the identity does not log out the account" "$PWD" \
    "gh api user --jq .login" "gh auth use --explain --json warnings --jq '.warnings | length'" \
    "gh auth identity delete work-identity" "gh api user --jq .login"
  run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_equal "$first_login" "$captured_output" "signing back in repaired the retained choice"
  run_capture "gh auth use --explain --json warnings --jq '.warnings | length'" \
    "$gh_bin" auth use --explain --json warnings --jq '.warnings | length'
  assert_equal 0 "$captured_output" "missing-account warning cleared"
  delete_identities work-identity
  run_capture "gh api user --jq .login" "$gh_bin" api user --jq .login
  assert_equal "$first_login" "$captured_output" "identity deletion did not delete credentials"
  review_complete "$PWD" "logout and identity deletion have distinct, recoverable effects"
  cd "$scenario_root/work"
  finish_scenario
}

main() {
  parse_args "$@"
  if [[ "$list_only" == true ]]; then list_scenarios; return; fi
  [[ -t 0 && -t 1 ]] || fail "live journeys require a terminal for account setup"
  if contains https-account-override "${requested_scenarios[@]}" || contains ssh-account-override "${requested_scenarios[@]}" || contains ssh-cooperation "${requested_scenarios[@]}"; then
    [[ -t 0 && -t 1 ]] || fail "account-override journeys require a terminal for gh pr create's push selection"
  fi
  local command scenario parent
  for command in git jq grep head wc; do require_command "$command"; done
  if contains ssh-directory "${requested_scenarios[@]}" || contains ssh-owner "${requested_scenarios[@]}" || contains ssh-worktrees "${requested_scenarios[@]}" || contains ssh-account-override "${requested_scenarios[@]}" || contains ssh-cooperation "${requested_scenarios[@]}"; then
    require_command ssh
    require_command ssh-keygen
    real_ssh="$(command -v ssh)"
  fi
  if [[ -z "$gh_bin" ]]; then gh_bin="$script_dir/../../bin/gh"; fi
  [[ -x "$gh_bin" ]] || fail "build this worktree's bin/gh first, or pass --gh"
  gh_bin="$(cd "$(dirname "$gh_bin")" && pwd -P)/$(basename "$gh_bin")"
  if ! GH_INTERNAL_DISABLE_KEYRING=true "$gh_bin" version | grep -q '^OS keyring: disabled (isolated debug workflow)$'; then
    fail "this binary cannot isolate the OS keyring; build this worktree with 'make debug' before running live journeys"
  fi
  "$gh_bin" auth identity create --help >/dev/null
  "$gh_bin" auth use --help >/dev/null
  parent="${run_root:-${TMPDIR:-/tmp}}"
  mkdir -p "$parent"
  umask 077
  run_root="$(mktemp -d "$parent/gh-identity-e2e.XXXXXX")"
  run_root="$(cd "$run_root" && pwd -P)"
  export GIT_CEILING_DIRECTORIES="$run_root"
  trap cleanup_on_exit EXIT
  trap 'printf "\nInterrupted; cleaning up.\n" >&2; exit 130' INT
  trap 'printf "\nTerminated; cleaning up.\n" >&2; exit 143' TERM
  mkdir -p "$run_root/tools"
  printf '#!/usr/bin/env bash\nexec %q "$@"\n' "$gh_bin" >"$run_root/tools/gh"
  chmod 700 "$run_root/tools/gh"
  original_path="$run_root/tools:$original_path"
  export PATH="$original_path"
  log "LIVE journey: $gh_bin"
  "$gh_bin" version | head -n 1
  unset GIT_DIR GIT_WORK_TREE GIT_COMMON_DIR GIT_INDEX_FILE GIT_OBJECT_DIRECTORY
  unset GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_SSH GIT_SSH_COMMAND SSH_AUTH_SOCK
  unset GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL GIT_COMMITTER_NAME GIT_COMMITTER_EMAIL
  unset GH_DEBUG GH_FORCE_TTY GIT_TRACE GIT_TRACE_CURL GIT_TRACE_PACKET
  unset GH_IDENTITY_ACCOUNT GH_IDENTITY_HOST GH_IDENTITY_SSH_COMMAND GH_IDENTITY_SSH_VARIANT
  # Inspect normal saved accounts before changing this process's configuration.
  if ! offer_existing_accounts; then bootstrap_accounts; fi
  for scenario in "${requested_scenarios[@]}"; do
    export PATH="$original_path"
    current_scenario="$scenario"
    case "$scenario" in
      https-directory) exercise_git_journey https directory ;;
      https-owner) exercise_git_journey https owner ;;
      ssh-directory) exercise_git_journey ssh directory ;;
      ssh-owner) exercise_git_journey ssh owner ;;
      https-worktrees) exercise_git_journey https directory ;;
      ssh-worktrees) exercise_git_journey ssh directory ;;
      https-account-override) run_account_override https ;;
      ssh-account-override) run_account_override ssh ;;
      ssh-cooperation) run_account_override ssh ;;
      logout-recovery) run_logout_recovery ;;
      identity-lifecycle) run_identity_lifecycle ;;
    esac
  done
  log "All selected journeys passed"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
