#!/bin/bash
# Offline account-selection scenario. No installed-gh fallback or real credentials.
# From a checkout (the fixture defaults to a new private directory under HOME):
# env -i HOME="$HOME" PATH=/usr/bin:/bin:/opt/homebrew/bin \
#   /bin/bash script/multiaccount-review/01-account.sh --gh "$PWD/bin/gh"
# For individual phases, set SCENARIO_ROOT to the same explicit absolute path.
# Setup requires a nonexistent path; later phases require its ownership marker.
set -euo pipefail
umask 077
if [[ ${1:-} == --gh ]]; then
  GH_SCENARIO_BINARY=$2
  shift 2
fi
: "${GH_SCENARIO_BINARY:?usage: 01-account.sh --gh /absolute/gh [phase]}"
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
  root=${SCENARIO_ROOT:-"${HOME:?HOME or SCENARIO_ROOT is required}/.gh-account-review-$$"}
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
    printf '%s\n' gh-account-review-v2 >"$root/.scenario-owner"
  else
    [[ -d $root && ! -L $root && -O $root &&
       $(/usr/bin/stat -f '%Lp' "$root") == 700 &&
       -f $root/.scenario-owner && ! -L $root/.scenario-owner &&
       -O $root/.scenario-owner && $(cat "$root/.scenario-owner") == gh-account-review-v2 ]] ||
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
export GIT_EXEC_PATH="$root/tools" SCENARIO_ROOT="$root"
gh=$GH_SCENARIO_BINARY
seed() {
  [[ ! -e $root/fixture-ready ]] || { echo "Fixture already initialized" >&2; exit 2; }
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
  printf '#!/bin/sh\necho "Network SSH is disabled in this fixture" >&2\nexit 97\n' >"$root/tools/ssh"
  cat >"$root/tools/git-remote-https" <<'EOF'
#!/bin/bash
set -euo pipefail
[[ $2 == https://accounts.invalid/demo/project.git ]] || exit 97
credential=$(printf 'url=%s\n\n' "$2" | git credential fill)
username=$(printf '%s\n' "$credential" | sed -n 's/^username=//p')
case "$username" in alice|bob) ;; *) exit 98 ;; esac
printf '%s\n' "$credential" | grep -qx "password=fixture-$username-not-a-real-token"
printf '%s\n' "$username" >>"$SCENARIO_ROOT/authenticated-users"
echo "Simulated HTTPS remote authenticated as: $username" >&2
while IFS= read -r line; do
  case "$line" in
    capabilities) printf 'connect\n\n' ;;
    option*) printf 'unsupported\n' ;;
    'connect git-upload-pack') printf '\n'; exec git upload-pack "$SCENARIO_ROOT/remote.git" ;;
    *) exit 99 ;;
  esac
done
EOF
  chmod 700 "$root/tools/"*
  git init -q --bare --initial-branch=main "$root/remote.git"
  git init -q --initial-branch=main "$root/repo"
  git -C "$root/repo" -c user.name=Fixture -c user.email=fixture@example.invalid \
    -c commit.gpgsign=false commit --allow-empty -qm baseline
  git -C "$root/repo" push -q "$root/remote.git" main
  git -C "$root/repo" remote add origin https://accounts.invalid/demo/project.git
  git -C "$root/repo" update-ref refs/remotes/origin/main HEAD
  touch "$root/fixture-ready"
  echo "Offline fixture: saved accounts alice and bob on accounts.invalid"
  echo "Saved default account: $(sed -n 's/^    user: //p' "$root/config/hosts.yml")"
  echo "HTTPS remote is simulated locally; native keyring and SSH are disabled."
}
case "$phase" in
  setup) seed ;;
  exercise)
    cd "$root/repo"
    exec "$gh" repo sync --source accounts.invalid/demo/project --branch main --account bob
    ;;
  validate)
    cmp "$root/hosts-before" "$root/config/hosts.yml"
    [[ $(cat "$root/authenticated-users") == bob ]]
    [[ $(git -C "$root/repo" rev-parse HEAD) == $(git --git-dir="$root/remote.git" rev-parse main) ]]
    echo "PASS: gh-launched Git authenticated as bob against the simulated remote."
    echo "PASS: local main matches the remote; saved default remains alice."
    ;;
  missing-setup)
    cmp "$root/hosts-before" "$root/config/hosts.yml"
    echo "Saved accounts: alice, bob. Account charlie is deliberately absent."
    ;;
  missing-exercise)
    exec "$gh" api user --hostname accounts.invalid --account charlie
    ;;
  missing-validate)
    cmp "$root/hosts-before" "$root/config/hosts.yml"
    [[ $(cat "$root/authenticated-users") == bob ]]
    echo "PASS: the failed selection changed no saved account or Git authentication."
    ;;
  all)
    /bin/bash "$0" setup
    /bin/bash "$0" exercise
    /bin/bash "$0" validate
    /bin/bash "$0" missing-setup
    set +e
    /bin/bash "$0" missing-exercise
    code=$?
    set -e
    [[ $code == 1 ]]
    /bin/bash "$0" missing-validate
    echo "Replay passed; isolated fixture retained in the chosen root."
    ;;
  *) echo "Unknown phase" >&2; exit 2 ;;
esac
