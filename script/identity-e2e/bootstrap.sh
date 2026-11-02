#!/usr/bin/env bash
# Guided review and account bootstrap, adapted from the original multiaccount runner.
# Keep behavior aligned; prototype-specific commands live in run.sh.

log() {
  printf '\n==> %s\n' "$*"
}

step() {
  printf '\n  -> %s\n' "$*"
}

divider() {
  printf '%s\n' "------------------------------------------------------------------------"
}

label() {
  local word="$1"
  shift
  printf '%s%-5s%s %s\n' "$BOLD" "$word" "$RESET" "$*"
}

note() {
  printf '%s%s%s\n' "$DIM" "$*" "$RESET"
}

cmdline() {
  printf '  %s%s%s\n' "$YELLOW" "$*" "$RESET"
}

print_running() {
  if [[ "$interactive" != true ]]; then
    return 0
  fi
  printf '%s$ %s%s\n' "$CYAN" "$*" "$RESET"
}

run_visible() {
  local display="$1"
  shift

  print_running "$display"
  "$@"
}

run_capture() {
  local display="$1"
  shift

  print_running "$display"
  captured_output="$("$@")"
  printf '%s\n' "$captured_output"
}

run_expect_failure() {
  local display="$1"
  shift
  local status

  print_running "$display"
  set +e
  captured_output="$("$@" 2>&1)"
  status=$?
  set -e
  printf '%s\n' "$captured_output"
  if [[ "$status" -eq 0 ]]; then
    fail "$display unexpectedly succeeded"
  fi
}

pass() {
  if [[ "$interactive" != true ]]; then
    return 0
  fi
  printf '  %sPASS%s %s\n' "$GREEN" "$RESET" "$*"
}

heading() {
  printf '%s%s%s\n' "$BOLD" "$*" "$RESET"
}

checkpoint() {
  local description="$1"
  local directory="${2:-$scenario_root/work}"
  local response shell

  if [[ "$interactive" != true ]]; then
    return 0
  fi
  [[ -d "$directory" ]] || directory="$scenario_root/work"

  printf '\n'
  divider
  printf '%s%sExplore%s %s\n' "$BOLD" "$CYAN" "$RESET" "$description"
  printf '%sWorking directory: %s%s\n' "$DIM" "$directory" "$RESET"
  printf '\n%sInput%s\n' "$BOLD" "$RESET"
  printf '  Press Enter to return to the guided step.\n'
  printf '  Anything else runs as a shell command unless it starts with %s:%s\n' \
    "$YELLOW" "$RESET"
  printf '\n%sMeta-commands%s\n' "$BOLD" "$RESET"
  printf '  %s:state%s  show authentication, bindings, Git config, and repository state\n' \
    "$YELLOW" "$RESET"
  printf '  %s:shell%s  open an interactive shell; exit it to return here\n' \
    "$YELLOW" "$RESET"
  printf '  %s:help%s   show the short command summary\n' "$YELLOW" "$RESET"
  printf '  %s:quit%s   stop the runner and perform normal cleanup\n' \
    "$YELLOW" "$RESET"

  while true; do
    printf '\n%s%sgh-e2e%s %s>%s ' \
      "$BOLD" "$CYAN" "$RESET" "$GREEN" "$RESET" >/dev/tty
    if ! IFS= read -r response </dev/tty; then
      printf '\n' >/dev/tty
      return
    fi

    case "$response" in
      "")
        return
        ;;
      :state)
        show_checkpoint_state "$directory"
        ;;
      :shell)
        shell="${SHELL:-/bin/sh}"
        printf '\n%sOpening %s in %s%s\n' "$CYAN" "$shell" "$directory" "$RESET"
        note "Exit the shell to return to explore mode."
        (
          cd "$directory"
          "$shell" -i </dev/tty
        ) || printf '%sShell exited unsuccessfully; explore mode remains active.%s\n' \
          "$RED" "$RESET" >&2
        ;;
      :quit)
        printf '%sStopping; cleanup will run now.%s\n' "$YELLOW" "$RESET"
        exit 130
        ;;
      :help)
        printf '%sEnter%s returns; %s:state%s inspects; %s:shell%s opens a shell; %s:quit%s cleans up and stops.\n' \
          "$BOLD" "$RESET" "$YELLOW" "$RESET" "$YELLOW" "$RESET" "$YELLOW" "$RESET"
        ;;
      :*)
        printf '%sUnknown meta-command: %s%s (try %s:help%s)\n' \
          "$RED" "$response" "$RESET" "$YELLOW" "$RESET" >&2
        ;;
      *)
        printf '%s$ %s%s\n' "$CYAN" "$response" "$RESET"
        (
          cd "$directory"
          /bin/sh -c "$response" </dev/tty
        ) || printf '%sCommand exited unsuccessfully; explore mode remains active.%s\n' \
          "$RED" "$RESET" >&2
        ;;
    esac
  done
}

review_step() {
  local given="$1"
  local when="$2"
  local then="$3"
  local directory="$4"
  shift 4

  if [[ "$interactive" != true ]]; then
    step "$when"
    return
  fi

  printf '\n'
  divider
  label "Given" "$given"
  label "When" "$when"
  label "Then" "$then"
  printf '%sWorking directory:%s %s\n' "$DIM" "$RESET" "$directory"
  printf '\n'
  heading "Commands"
  while [[ "$#" -gt 0 ]]; do
    cmdline "$1"
    shift
  done

  while true; do
    printf '\n%sPress Enter to run%s (e to explore, q to quit): ' \
      "$BOLD" "$RESET" >/dev/tty
    local answer
    IFS= read -r answer </dev/tty || answer=q
    case "$answer" in
      "")
        return
        ;;
      e)
        checkpoint "explore before running this step" "$directory"
        ;;
      q)
        printf 'Stopping; cleanup will run now.\n'
        exit 130
        ;;
      *)
        printf 'Enter runs the step; e explores; q quits.\n' >&2
        ;;
    esac
  done
}

review_complete() {
  local directory="$1"
  shift

  if [[ "$interactive" != true ]]; then
    return 0
  fi

  printf '\n'
  label "Expected:" ""
  while [[ "$#" -gt 0 ]]; do
    pass "$1"
    shift
  done

  while true; do
    printf '\n%sPress Enter for the next step%s (e to explore, q to quit): ' \
      "$BOLD" "$RESET" >/dev/tty
    local answer
    IFS= read -r answer </dev/tty || answer=q
    case "$answer" in
      "")
        return
        ;;
      e)
        checkpoint "explore after this step" "$directory"
        ;;
      q)
        printf 'Stopping; cleanup will run now.\n'
        exit 130
        ;;
      *)
        printf 'Enter advances; e explores; q quits.\n' >&2
        ;;
    esac
  done
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

contains() {
  local expected="$1"
  shift

  local candidate
  for candidate in "$@"; do
    if [[ "$candidate" == "$expected" ]]; then
      return 0
    fi
  done
  return 1
}

configure_isolated_environment() {
  local root="$1"
  local variable

  mkdir -p "$root/home" "$root/gh-config" "$root/work"
  while IFS='=' read -r variable _; do
    case "$variable" in
      GIT_CONFIG_KEY_* | GIT_CONFIG_VALUE_*)
        unset "$variable"
        ;;
    esac
  done < <(env)
  unset GIT_CONFIG GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS GIT_CONFIG_SYSTEM
  export HOME="$root/home"
  export GH_CONFIG_DIR="$root/gh-config"
  export GH_INTERNAL_DISABLE_KEYRING=true
  export GIT_CONFIG_GLOBAL="$root/gitconfig"
  export GIT_CONFIG_NOSYSTEM=1
  export XDG_CONFIG_HOME="$root/xdg-config"
  export XDG_DATA_HOME="$root/xdg-data"
  export XDG_CACHE_HOME="$root/xdg-cache"
  export GH_PAGER=cat
  export GIT_PAGER=cat
  unset GH_TOKEN GITHUB_TOKEN GH_ENTERPRISE_TOKEN GITHUB_ENTERPRISE_TOKEN GH_HOST GH_ACCOUNT GH_REPO
  unset GIT_SSH_COMMAND

  git config --global init.defaultBranch main
}

login_in_browser() {
  local ordinal="$1"
  local -a args=(
    auth login --hostname "$host" --web --insecure-storage
    --git-protocol https --skip-ssh-key
  )
  if [[ "$api_only" != true ]]; then
    args+=(--scopes "repo,read:org,gist,delete_repo,admin:public_key")
  fi

  step "Authenticate the $ordinal account in the browser"
  note "Use the GitHub account you want to treat as the $ordinal account."
  run_visible "gh ${args[*]}" "$gh_bin" "${args[@]}"

  run_capture "gh api user --jq .login" \
    "$gh_bin" api user --jq .login
  authenticated_login="$captured_output"
  pass "browser authentication succeeded as $authenticated_login"
}

validate_bootstrap_accounts() {
  local -a saved_logins=()
  local saved_login

  step "Verify the two browser logins are distinct saved accounts"

  run_capture \
    "gh auth status --hostname $host --json hosts --jq '.hosts[\"$host\"] | map(.login) | unique | .[]'" \
    "$gh_bin" auth status --hostname "$host" --json hosts \
    --jq ".hosts[\"$host\"] | map(.login) | unique | .[]"
  if [[ "$second_login" == "$first_login" ]]; then
    fail "expected two distinct accounts, but both browser logins resolved to $first_login; rerun and choose a different account during the second login"
  fi
  while IFS= read -r saved_login; do
    [[ -n "$saved_login" ]] || continue
    saved_logins+=("$saved_login")
  done <<<"$captured_output"

  [[ "${#saved_logins[@]}" -eq 2 ]] ||
    fail "expected exactly two saved users after browser login, found ${#saved_logins[@]}: ${saved_logins[*]:-none}"
  contains "$first_login" "${saved_logins[@]}" ||
    fail "bootstrap configuration does not contain the first account $first_login"
  contains "$second_login" "${saved_logins[@]}" ||
    fail "bootstrap configuration does not contain the second account $second_login"

  pass "isolated configuration contains the two distinct accounts $first_login and $second_login"
}

bootstrap_accounts() {
  local bootstrap_root="$run_root/bootstrap"
  current_scenario="bootstrap"
  scenario_root="$bootstrap_root"
  configure_isolated_environment "$bootstrap_root"

  login_in_browser first
  first_login="$authenticated_login"
  [[ -n "$first_login" ]] || fail "could not determine the first login"

  login_in_browser second
  second_login="$authenticated_login"
  [[ -n "$second_login" ]] || fail "could not determine the second login"
  validate_bootstrap_accounts

  step "Capture tokens without printing them and clear the bootstrap accounts"

  print_running "gh auth token --hostname $host --user $first_login > <captured token>"
  first_token="$("$gh_bin" auth token --hostname "$host" --user "$first_login")"
  [[ -n "$first_token" ]] || fail "could not read the token for $first_login"
  printf '%s[output redacted: token captured]%s\n' "$DIM" "$RESET"
  print_running "gh auth token --hostname $host --user $second_login > <captured token>"
  second_token="$("$gh_bin" auth token --hostname "$host" --user "$second_login")"
  [[ -n "$second_token" ]] || fail "could not read the token for $second_login"
  printf '%s[output redacted: token captured]%s\n' "$DIM" "$RESET"

  log "Captured accounts"
  printf 'First:  %s\nSecond: %s\n' "$first_login" "$second_login"

  run_visible "gh auth logout --hostname $host --user $first_login" \
    "$gh_bin" auth logout --hostname "$host" --user "$first_login"
  run_visible "gh auth logout --hostname $host --user $second_login" \
    "$gh_bin" auth logout --hostname "$host" --user "$second_login"
  if "$gh_bin" auth status --hostname "$host" >/dev/null 2>&1; then
    fail "bootstrap configuration still contains an authenticated account"
  fi
  pass "tokens captured without printing them; bootstrap accounts removed"
}

login_saved_account() {
  local token="$1"
  local protocol="$2"
  local login="$3"

  print_running "printf '<token for $login>' | gh auth login --hostname $host --with-token --insecure-storage --git-protocol $protocol --skip-ssh-key"
  printf '%s\n' "$token" |
    "$gh_bin" auth login \
      --hostname "$host" \
      --with-token \
      --insecure-storage \
      --git-protocol "$protocol" \
      --skip-ssh-key
}

global_gh() {
  env \
    -u GH_TOKEN \
    -u GITHUB_TOKEN \
    -u GH_ENTERPRISE_TOKEN \
    -u GITHUB_ENTERPRISE_TOKEN \
    "$gh_bin" "$@"
}

missing_account_scopes() {
  [[ "$api_only" != true ]] || return 0
  local configured_scopes="${1// /}"
  local required
  local -a missing=()

  for required in repo delete_repo admin:public_key; do
    if [[ ",$configured_scopes," != *",$required,"* ]]; then
      missing+=("$required")
    fi
  done

  local IFS=,
  printf '%s' "${missing[*]}"
}

saved_account_scopes() {
  local login="$1"

  global_gh auth status --no-refresh --hostname "$host" --json hosts \
    --jq ".hosts[\"$host\"][] | select(.login == \"$login\" and .state == \"success\") | .scopes" \
    2>/dev/null
}

offer_existing_accounts() {
  local accounts_output login scopes selection first_index second_index
  local first_missing second_missing
  local -a accounts=()
  local -a account_scopes=()

  print_running "gh auth status --no-refresh --hostname $host --json hosts --jq '.hosts[\"$host\"][] | select(.state == \"success\") | [.login, .scopes] | @tsv'"
  set +e
  accounts_output="$(
    global_gh auth status --no-refresh --hostname "$host" --json hosts \
      --jq ".hosts[\"$host\"][] | select(.state == \"success\") | [.login, .scopes] | @tsv" 2>/dev/null
  )"
  set -e
  printf '%s\n' "$accounts_output"
  while IFS=$'\t' read -r login scopes; do
    [[ -n "$login" ]] || continue
    accounts+=("$login")
    account_scopes+=("$scopes")
  done <<<"$accounts_output"

  if [[ "${#accounts[@]}" -lt 2 ]]; then
    note "Fewer than two healthy saved $host accounts were found; browser login is required."
    return 1
  fi

  printf '\n'
  heading "Saved accounts"
  note "Selected tokens will be copied to private temporary plaintext files for this run."
  note "Saved credentials are read without refresh; --keep retains the temporary copies."
  local index
  for index in "${!accounts[@]}"; do
    scopes="$(missing_account_scopes "${account_scopes[$index]}")"
    if [[ -n "$scopes" ]]; then
      printf '  %d. %s %s(needs: %s)%s\n' \
        "$((index + 1))" "${accounts[$index]}" "$YELLOW" "$scopes" "$RESET"
    else
      printf '  %d. %s %s(ready)%s\n' \
        "$((index + 1))" "${accounts[$index]}" "$GREEN" "$RESET"
    fi
  done

  if [[ "${#accounts[@]}" -eq 2 ]]; then
    first_index=0
    second_index=1
  else
    while true; do
      printf '\n%sEnter two account numbers to use%s (or press Enter for browser login): ' \
        "$BOLD" "$RESET" >/dev/tty
      IFS= read -r selection </dev/tty || selection=""
      [[ -n "$selection" ]] || return 1
      if [[ "$selection" =~ ^[[:space:]]*([0-9]+)[[:space:]]+([0-9]+)[[:space:]]*$ ]]; then
        first_index=$((BASH_REMATCH[1] - 1))
        second_index=$((BASH_REMATCH[2] - 1))
        if [[ "$first_index" -ge 0 &&
          "$second_index" -ge 0 &&
          "$first_index" -lt "${#accounts[@]}" &&
          "$second_index" -lt "${#accounts[@]}" &&
          "$first_index" -ne "$second_index" ]]; then
          break
        fi
      fi
      printf 'Choose two different numbers from the list.\n' >&2
    done
  fi

  first_login="${accounts[$first_index]}"
  second_login="${accounts[$second_index]}"
  first_missing="$(missing_account_scopes "${account_scopes[$first_index]}")"
  second_missing="$(missing_account_scopes "${account_scopes[$second_index]}")"

  if [[ -n "$first_missing" || -n "$second_missing" ]]; then
    printf '\n'
    heading "Additional scopes required"
    [[ -z "$first_missing" ]] ||
      printf '  %s: %s\n' "$first_login" "$first_missing"
    [[ -z "$second_missing" ]] ||
      printf '  %s: %s\n' "$second_login" "$second_missing"
    printf '\n%sRefresh these accounts or use fresh browser login?%s [r/L]: ' \
      "$BOLD" "$RESET" >/dev/tty
    IFS= read -r selection </dev/tty || selection=l
    case "$selection" in
      r | R | refresh | REFRESH)
        if [[ -n "$first_missing" ]] &&
          ! refresh_saved_account_scopes "$first_login" "$first_missing"; then
          first_login=""
          second_login=""
          return 1
        fi
        if [[ -n "$second_missing" ]] &&
          ! refresh_saved_account_scopes "$second_login" "$second_missing"; then
          first_login=""
          second_login=""
          return 1
        fi
        ;;
      *)
        first_login=""
        second_login=""
        return 1
        ;;
    esac
  else
    printf '\n%sUse these two accounts?%s [Y/n]: ' "$BOLD" "$RESET" >/dev/tty
    IFS= read -r selection </dev/tty || selection=n
    case "$selection" in
      "" | y | Y | yes | YES)
        ;;
      *)
        first_login=""
        second_login=""
        return 1
        ;;
    esac
  fi

  print_running "gh auth token --no-refresh --hostname $host --user $first_login > <captured token>"
  if ! first_token="$(global_gh auth token --no-refresh --hostname "$host" --user "$first_login" 2>/dev/null)" ||
    [[ -z "$first_token" ]]; then
    note "The saved credential for $first_login is unavailable; browser login is required."
    first_login=""
    second_login=""
    first_token=""
    second_token=""
    return 1
  fi
  printf '%s[output redacted: token captured for %s]%s\n' \
    "$DIM" "$first_login" "$RESET"

  print_running "gh auth token --no-refresh --hostname $host --user $second_login > <captured token>"
  if ! second_token="$(global_gh auth token --no-refresh --hostname "$host" --user "$second_login" 2>/dev/null)" ||
    [[ -z "$second_token" ]]; then
    note "The saved credential for $second_login is unavailable; browser login is required."
    first_login=""
    second_login=""
    first_token=""
    second_token=""
    return 1
  fi
  printf '%s[output redacted: token captured for %s]%s\n' \
    "$DIM" "$second_login" "$RESET"

  return 0
}

prepare_scenario() {
  local scenario="$1"
  local protocol="$2"
  local active_login=""
  local restored_login
  local restored_active
  local status_query=".hosts[\"$host\"] | unique_by(.login)[] | [.login, .active] | @tsv"

  current_scenario="$scenario"
  scenario_root="$run_root/scenarios/$scenario"
  configure_isolated_environment "$scenario_root"
  created_repositories=()
  uploaded_ssh_keys=()
  step "Restore both accounts into isolated configuration for $scenario"
  login_saved_account "$first_token" "$protocol" "$first_login"
  login_saved_account "$second_token" "$protocol" "$second_login"

  local -a restored_logins=()
  run_capture \
    "gh auth status --hostname $host --json hosts --jq '$status_query'" \
    "$gh_bin" auth status --hostname "$host" --json hosts \
    --jq "$status_query"
  while IFS=$'\t' read -r restored_login restored_active; do
    [[ -n "$restored_login" ]] || continue
    restored_logins+=("$restored_login")
    if [[ "$restored_active" == true ]]; then
      [[ -z "$active_login" ]] || fail "$scenario restored more than one active account"
      active_login="$restored_login"
    fi
  done <<<"$captured_output"
  [[ "${#restored_logins[@]}" -eq 2 ]] ||
    fail "$scenario expected exactly two restored users, found ${#restored_logins[@]}"
  contains "$first_login" "${restored_logins[@]}" ||
    fail "$scenario did not restore $first_login"
  contains "$second_login" "${restored_logins[@]}" ||
    fail "$scenario did not restore $second_login"

  if [[ "$active_login" == "$first_login" ]]; then
    scenario_active_login="$first_login"
    scenario_other_login="$second_login"
  elif [[ "$active_login" == "$second_login" ]]; then
    scenario_active_login="$second_login"
    scenario_other_login="$first_login"
  else
    fail "$scenario active account $active_login is not one of the restored accounts"
  fi
  pass "$first_login and $second_login restored; active account $scenario_active_login; Git protocol $protocol"
}

assert_equal() {
  local expected="$1"
  local actual="$2"
  local description="$3"

  if [[ "$actual" != "$expected" ]]; then
    fail "$description: expected $expected, got $actual"
  fi
  pass "$description"
}

assert_contains() {
  local expected="$1"
  local actual="$2"
  local description="$3"

  if [[ "$actual" != *"$expected"* ]]; then
    fail "$description: expected output to contain $expected"
  fi
  pass "$description"
}

unique_suffix() {
  printf '%s-%s-%s' "$(date -u +%Y%m%d%H%M%S)" "$$" "$RANDOM"
}
