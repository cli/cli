#!/bin/bash
# Offline repository-family identity scenario. No installed-gh fallback.
# From a checkout (the fixture defaults to a new private directory under HOME):
# env -i HOME="$HOME" PATH=/usr/bin:/bin:/opt/homebrew/bin \
#   /bin/bash script/multiaccount-review/02-repository.sh --gh "$PWD/bin/gh"
# For individual phases, set SCENARIO_ROOT to the same explicit absolute path.
# Setup requires a nonexistent path; later phases require its ownership marker.
set -euo pipefail
umask 077
if [[ ${1:-} == --gh ]]; then GH_SCENARIO_BINARY=$2; shift 2; fi
: "${GH_SCENARIO_BINARY:?usage: 02-repository.sh --gh /absolute/gh [phase]}"
[[ $GH_SCENARIO_BINARY = /* && -x $GH_SCENARIO_BINARY ]] || exit 2
phase=${1:-all}
if [[ ${SCENARIO_ISOLATED:-} != yes ]]; then
  [[ -z ${GH_TOKEN:-}${GITHUB_TOKEN:-}${GH_ENTERPRISE_TOKEN:-}${GITHUB_ENTERPRISE_TOKEN:-} ]] ||
    { echo "Refusing an inherited credential environment" >&2; exit 2; }
  [[ -x /usr/bin/sandbox-exec ]] || { echo "macOS sandbox-exec is required for keyring isolation" >&2; exit 2; }
  if [[ -z ${SCENARIO_ROOT:-} && $phase != setup && $phase != all ]]; then
    echo "Individual phases require an explicit SCENARIO_ROOT" >&2
    exit 2
  fi
  root=${SCENARIO_ROOT:-"${HOME:?HOME or SCENARIO_ROOT is required}/.gh-repository-review-$$"}
  [[ $root = /* ]] || exit 2
  parent=$(cd -P "$(dirname "$root")" && pwd)
  leaf=$(basename "$root")
  [[ $leaf != . && $leaf != .. && $leaf != / ]] || exit 2
  root="$parent/$leaf"
  checkout=$(/usr/bin/env -i PATH=/usr/bin:/bin GIT_CONFIG_NOSYSTEM=1 \
    GIT_CONFIG_GLOBAL=/dev/null /usr/bin/git rev-parse --show-toplevel 2>/dev/null || true)
  if [[ -n $checkout && ( $root == "$checkout" || $root == "$checkout/"* ) ]]; then
    echo "Refusing a fixture root inside the current checkout" >&2
    exit 2
  fi
  if [[ $phase == setup || $phase == all ]]; then
    [[ ! -e $root && ! -L $root ]] || { echo "Setup requires a new fixture root" >&2; exit 2; }
    mkdir -m 700 "$root"
    printf '%s\n' gh-repository-review-v2 >"$root/.scenario-owner"
  else
    [[ -d $root && ! -L $root && -O $root &&
       $(/usr/bin/stat -f '%Lp' "$root") == 700 &&
       -f $root/.scenario-owner && ! -L $root/.scenario-owner &&
       -O $root/.scenario-owner && $(cat "$root/.scenario-owner") == gh-repository-review-v2 ]] ||
      { echo "Refusing a fixture root without private ownership and its scenario marker" >&2; exit 2; }
  fi
  policy='(version 1)(allow default)(deny network*)(deny process-exec (literal "/usr/bin/security"))(deny file-read* file-write* (regex #"/Library/Keychains(/|$)"))(deny mach-lookup (global-name "com.apple.securityd") (global-name "com.apple.secd"))'
  exec /usr/bin/sandbox-exec -p "$policy" /usr/bin/env -i PATH=/usr/bin:/bin:/opt/homebrew/bin HOME="$root/home" \
    SCENARIO_ISOLATED=yes SCENARIO_ROOT="$root" GH_SCENARIO_BINARY="$GH_SCENARIO_BINARY" \
    /bin/bash "$0" "$phase"
fi
root=$SCENARIO_ROOT
mkdir -p "$root"/{home,config,cache,tmp,tools,empty-templates}
export HOME="$root/home" GH_CONFIG_DIR="$root/config" XDG_CONFIG_HOME="$root/config"
export XDG_CACHE_HOME="$root/cache" TMPDIR="$root/tmp" GIT_CONFIG_GLOBAL="$root/gitconfig"
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_SYSTEM=/dev/null GIT_TEMPLATE_DIR="$root/empty-templates"
export GH_HOST=accounts.invalid GH_PROMPT_DISABLED=1 GH_NO_UPDATE_NOTIFIER=1
export GH_NO_EXTENSION_UPDATE_NOTIFIER=1 GH_PAGER=cat PAGER=cat GIT_TERMINAL_PROMPT=0
export PATH="$root/tools:/usr/bin:/bin:/opt/homebrew/bin"
gh=$GH_SCENARIO_BINARY
inspect() {
  local directory=$1 label=$2 expected=$3 result
  result=$(cd "$directory" && "$gh" auth use --view --hostname accounts.invalid)
  printf '%s: %s\n' "$label" "$result"
  if [[ $expected == bob ]]; then
    [[ $result == *'"name": "work"'* && $result == *'"account": "bob"'* ]]
  else
    [[ $result == null ]]
  fi
}
case "$phase" in
  setup)
    [[ ! -e $root/fixture-ready ]] || exit 2
    cat >"$root/config/hosts.yml" <<'EOF'
accounts.invalid:
    user: alice
    oauth_token: fixture-alice-not-a-real-token
    git_protocol: https
    users:
        alice:
            oauth_token: fixture-alice-not-a-real-token
        bob:
            oauth_token: fixture-bob-not-a-real-token
EOF
    cp "$root/config/hosts.yml" "$root/hosts-before"
    : >"$root/gitconfig"
    printf '#!/bin/sh\nexit 1\n' >"$root/tools/security"
    cp "$root/tools/security" "$root/tools/secret-tool"
    printf '#!/bin/sh\necho "Network disabled in this fixture" >&2\nexit 97\n' >"$root/tools/ssh"
    chmod 700 "$root/tools/"*
    git init -q --initial-branch=main "$root/repo"
    git -C "$root/repo" -c user.name=Fixture -c user.email=fixture@example.invalid \
      -c commit.gpgsign=false commit --allow-empty -qm baseline
    git -C "$root/repo" worktree add -q -b review "$root/linked"
    git init -q --initial-branch=main "$root/repo/nested"
    # Creation reports an absolute storage path. Keep that real output off-screen
    # and display the product's structured readback instead.
    "$gh" auth identity create work --account bob --hostname accounts.invalid \
      >"$root/create.stdout" 2>"$root/create.stderr"
    echo "Created a reusable identity (fake saved accounts; no network):"
    "$gh" auth identity view work
    inspect "$root/repo" "Main checkout before selection" null
    echo "Prepared a linked worktree and an unrelated nested repository."
    touch "$root/fixture-ready"
    ;;
  exercise)
    cd "$root/repo"
    exec "$gh" auth use work
    ;;
  validate)
    inspect "$root/repo" "Main checkout" bob
    inspect "$root/linked" "Linked worktree" bob
    inspect "$root/repo/nested" "Unrelated nested repository" null
    cmp "$root/hosts-before" "$root/config/hosts.yml"
    echo "PASS: both worktrees select bob; nested repository inherits nothing."
    echo "PASS: saved active account and credentials remain unchanged."
    ;;
  cleanup-setup)
    inspect "$root/linked" "Existing linked-worktree selection" bob
    echo "The final binding will be removed from the linked worktree."
    ;;
  cleanup-exercise)
    cd "$root/linked"
    exec "$gh" auth use --unset --hostname accounts.invalid
    ;;
  cleanup-validate)
    inspect "$root/repo" "Main checkout after removal" null
    inspect "$root/linked" "Linked worktree after removal" null
    [[ -z $(git config --global --list) ]]
    cmp "$root/hosts-before" "$root/config/hosts.yml"
    "$gh" auth identity view work
    echo "PASS: global Git wiring is gone; reusable identity and accounts remain."
    ;;
  all)
    for step in setup exercise validate cleanup-setup cleanup-exercise cleanup-validate; do
      /bin/bash "$0" "$step"
    done
    echo "Replay passed; isolated fixture retained in the chosen root."
    ;;
  *) echo "Unknown phase" >&2; exit 2 ;;
esac
