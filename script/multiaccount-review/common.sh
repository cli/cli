#!/bin/bash
# Shared offline fixture support. Source only from the two review scripts.
set -euo pipefail
umask 077

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
parse() {
  GH= OLD_GH= FIXTURE= PHASE= CASE="$1"
  shift
  while (($#)); do
    case "$1" in
      --gh) GH="${2:?}"; shift 2 ;;
      --old-gh) OLD_GH="${2:?}"; shift 2 ;;
      --fixture) FIXTURE="${2:?}"; shift 2 ;;
      --case) CASE="${2:?}"; shift 2 ;;
      setup|exercise|validation|run-all) PHASE="$1"; shift ;;
      *) die "unknown argument: $1" ;;
    esac
  done
  [[ "$GH" = /* && -f "$GH" && -x "$GH" ]] || die "--gh must name an absolute executable; no PATH fallback"
  [[ -n "$PHASE" ]] || die "choose setup, exercise, validation, or run-all"
  if [[ -z "$FIXTURE" ]]; then
    [[ "$PHASE" = run-all ]] || die "--fixture is required for separate phases"
    # Exclusive mkdir below avoids the caller's repository and never reuses a root.
    FIXTURE="/Users/Shared/gh-review-${LAYER%.sh}-$$-$RANDOM"
  fi
  [[ "$FIXTURE" = /* && "$FIXTURE" != *" "* ]] || die "--fixture must be an absolute path without spaces"
  [[ "$FIXTURE" != /tmp/* && "$FIXTURE" != /var/tmp/* ]] || die "use a private persistent workspace, not /tmp"
  if [[ "$PHASE" = setup || "$PHASE" = run-all ]]; then
    [[ ! -e "$FIXTURE" && ! -L "$FIXTURE" ]] || die "fixture already exists; choose a fresh path"
    mkdir -m 700 "$FIXTURE"
    printf '%s\n' "$LAYER:$CASE" > "$FIXTURE/.owner"
  else
    [[ -f "$FIXTURE/.owner" && ! -L "$FIXTURE" ]] || die "not an owned fixture"
    [[ "$(cat "$FIXTURE/.owner")" = "$LAYER:$CASE" ]] || die "fixture belongs to a different case"
  fi
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[1]}")" && pwd -P)"
  # A whitelist removes all operator GH, Git, SSH, DBUS and loader variables.
  [[ "$(uname -s)" = Darwin && -x /usr/bin/sandbox-exec ]] ||
    die "this replay requires macOS sandbox-exec for hardcoded keychain isolation"
  # go-keyring invokes /usr/bin/security directly; PATH shims alone are insufficient.
  local policy='(version 1)(allow default)(deny network*)(deny process-exec (literal "/usr/bin/security"))(deny file-read* file-write* (regex #"/Library/Keychains(/|$)"))(deny mach-lookup (global-name "com.apple.securityd") (global-name "com.apple.secd"))'
  exec /usr/bin/sandbox-exec -p "$policy" /usr/bin/env -i PATH=/usr/bin:/bin HOME="$FIXTURE/home" \
    /bin/bash "$SCRIPT_DIR/$LAYER" --_isolated \
    --gh "$GH" --old-gh "$OLD_GH" --fixture "$FIXTURE" --case "$CASE" "$PHASE"
}

inner_parse() {
  while (($#)); do
    case "$1" in
      --gh) SOURCE_GH="$2"; shift 2 ;;
      --old-gh) SOURCE_OLD="$2"; shift 2 ;;
      --fixture) F="$2"; shift 2 ;;
      --case) CASE="$2"; shift 2 ;;
      *) PHASE="$1"; shift ;;
    esac
  done
  export HOME="$F/home" GH_CONFIG_DIR="$F/home/gh" XDG_CONFIG_HOME="$F/home/.config"
  export XDG_CACHE_HOME="$F/cache" XDG_DATA_HOME="$F/data" XDG_STATE_HOME="$F/state"
  export TMPDIR="$F/work" GH_HOST=identities-fixture.invalid GH_PROMPT_DISABLED=1
  export GH_NO_UPDATE_NOTIFIER=1 GH_NO_EXTENSION_UPDATE_NOTIFIER=1 GH_TELEMETRY=0
  export GIT_CONFIG_GLOBAL="$F/home/.gitconfig" GIT_CONFIG_NOSYSTEM=1
  export GIT_TERMINAL_PROMPT=0 GIT_EXEC_PATH="$F/tools" GIT_TEMPLATE_DIR="$F/templates"
  export GIT_CEILING_DIRECTORIES="$F" PATH="$F/tools:/usr/bin:/bin"
  export GIT_AUTHOR_DATE=2026-01-01T00:00:00Z GIT_COMMITTER_DATE=2026-01-01T00:00:00Z
  export GH_PAGER=cat PAGER=cat NO_COLOR=1 TERM=xterm-256color LC_ALL=C
  export GH_INTERNAL_DISABLE_KEYRING=true
  export REVIEW_KEYRING_LOG="$F/keyring-attempts" REVIEW_NETWORK_LOG="$F/network-attempts"
  GH="$F/tools/gh"
  HOST=identities-fixture.invalid
  cd "$F"
}

base_setup() {
  mkdir -p "$HOME/gh" "$HOME/.config" "$F"/{tools,templates,work,cache,data,state,tree}
  cp "$SOURCE_GH" "$GH"
  chmod 700 "$GH"
  : > "$REVIEW_KEYRING_LOG"
  : > "$REVIEW_NETWORK_LOG"
  cat > "$F/tools/security" <<'SH'
#!/bin/sh
printf 'blocked-keyring-access\n' >> "$REVIEW_KEYRING_LOG"
exit 44
SH
  cp "$F/tools/security" "$F/tools/secret-tool"
  cat > "$F/tools/ssh" <<'SH'
#!/bin/sh
printf 'blocked-network-access\n' >> "$REVIEW_NETWORK_LOG"
exit 97
SH
  cp "$F/tools/ssh" "$F/tools/git-remote-https"
  cp "$F/tools/ssh" "$F/tools/git-remote-http"
  chmod 700 "$F/tools/"*
  local denied=0
  /usr/bin/security -h > "$F/keychain-denial" 2>&1 || denied=$?
  [[ "$denied" != 0 ]] && grep -q 'Operation not permitted' "$F/keychain-denial" ||
    die "OS containment did not deny the absolute keychain executable"
  printf 'PASS OS sandbox denies the absolute macOS keychain executable (status %s)\n' "$denied"
  printf 'telemetry: disabled\n' > "$GH_CONFIG_DIR/config.yml"
  accounts both
  git config --global user.name 'Ordinary User'
  git config --global user.email ordinary@example.invalid
  git config --global init.defaultBranch main
  git config --global credential.interactive false
  git init -q "$F/tree/project"
  git -C "$F/tree/project" remote add origin "https://$HOST/company/project.git"
  printf 'Private fixture: synthetic accounts; OS sandbox denies keychain and network.\n'
  "$GH" version | head -1
}

accounts() {
  cat > "$GH_CONFIG_DIR/hosts.yml" <<'YAML'
identities-fixture.invalid:
    user: identity-fixture-bob
    oauth_token: fixture-token-bob-not-a-secret
    git_protocol: https
    users:
        identity-fixture-bob:
            oauth_token: fixture-token-bob-not-a-secret
YAML
  if [[ "$1" = both ]]; then
    cat >> "$GH_CONFIG_DIR/hosts.yml" <<'YAML'
        identity-fixture-alice:
            oauth_token: fixture-token-alice-not-a-secret
YAML
  fi
}

identity() {
  "$GH" auth identity create "$1" --hostname "$HOST" --account "$2" \
    --ssh-host "ssh-$1.invalid" --git-user-name "$3" --git-user-email "$4"
}
expect() {
  [[ "$1" = "$2" ]] || die "$3: unexpected value"
  printf 'PASS %s\n' "$3"
}
safe_network() {
  [[ ! -s "$REVIEW_NETWORK_LOG" ]] || die "an unexpected transport was attempted"
  printf 'PASS no Git transport or SSH invocation\n'
}
run_phases() {
  case "$PHASE" in
    setup) setup ;;
    exercise) exercise ;;
    validation) validation ;;
    run-all) setup; exercise; validation ;;
  esac
}
