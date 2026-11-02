#!/bin/bash
# Shared offline fixture support. Introduced with 03-directories.sh.
set -euo pipefail
OFFLINE_POLICY='(version 1)(allow default)(deny network*)(deny process-exec (literal "/usr/bin/security"))(deny file-read* file-write* (regex #"/Library/Keychains(/|$)"))(deny mach-lookup (global-name "com.apple.securityd") (global-name "com.apple.secd"))'

review_bootstrap() {
  GH= FIXTURE= CASE=all PHASE=run-all
  local isolated=no
  while (($#)); do
    case "$1" in
      --gh) GH=$2; shift 2 ;;
      --fixture) FIXTURE=$2; shift 2 ;;
      --case) CASE=$2; shift 2 ;;
      --phase) PHASE=$2; shift 2 ;;
      --isolated) isolated=yes; shift ;;
      *) echo "Unknown argument: $1" >&2; exit 2 ;;
    esac
  done
  [[ $GH = /* && -x $GH ]] || { echo 'Supply --gh /absolute/path/to/built/gh' >&2; exit 2; }
  case "$PHASE" in setup|exercise|validate|cleanup|run-all) ;; *) exit 2 ;; esac
  if [[ -z $FIXTURE ]]; then
    [[ $PHASE = run-all ]] || { echo 'Separate phases require --fixture' >&2; exit 2; }
    [[ ${HOME:-} = /* && -d $HOME ]] || { echo 'Set an absolute HOME or provide --fixture' >&2; exit 2; }
    # mkdir in fixture_setup atomically claims this private, non-reused location.
    FIXTURE="$HOME/.multiaccount-review-$LAYER-$$-$RANDOM"
  fi
  [[ $FIXTURE = /* && $FIXTURE != / && ! -L $FIXTURE ]] || exit 2
  if [[ $isolated = no ]]; then
    [[ -x /usr/bin/sandbox-exec ]] || { echo 'This offline replay requires macOS sandbox-exec; refusing uncontained keyring access.' >&2; exit 2; }
    exec /usr/bin/sandbox-exec -p "$OFFLINE_POLICY" /usr/bin/env -i PATH=/usr/bin:/bin:/usr/sbin:/sbin \
      HOME="$FIXTURE/home" GH_CONFIG_DIR="$FIXTURE/home/gh" \
      XDG_CONFIG_HOME="$FIXTURE/home/.config" XDG_CACHE_HOME="$FIXTURE/cache" \
      GIT_CONFIG_GLOBAL="$FIXTURE/home/.gitconfig" GIT_CONFIG_NOSYSTEM=1 \
      GIT_TERMINAL_PROMPT=0 GIT_TEMPLATE_DIR="$FIXTURE/templates" \
      GIT_CEILING_DIRECTORIES="$FIXTURE" TMPDIR="$FIXTURE/scratch" \
      GH_HOST=identities-fixture.invalid GH_PROMPT_DISABLED=1 \
      GH_NO_UPDATE_NOTIFIER=1 GH_NO_EXTENSION_UPDATE_NOTIFIER=1 GH_TELEMETRY=0 \
      LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8 TERM=xterm-truecolor \
      /bin/bash "$SELF" --isolated --gh "$GH" --fixture "$FIXTURE" --case "$CASE" --phase "$PHASE"
  fi
  export PATH="$FIXTURE/tools:/usr/bin:/bin:/usr/sbin:/sbin"
  export REVIEW_SSH_LOG="$FIXTURE/ssh.log"
  HOST=identities-fixture.invalid
}

fixture_setup() {
  umask 077
  [[ ! -e $FIXTURE ]] || { echo 'Refusing to overwrite an existing fixture' >&2; exit 2; }
  mkdir -m 700 "$FIXTURE"
  printf 'multiaccount-review-%s\n' "$LAYER" > "$FIXTURE/.review-owned"
  printf '%s\n' "$OFFLINE_POLICY" > "$FIXTURE/offline.sb"
  mkdir -p "$HOME/gh" "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME" "$TMPDIR" "$FIXTURE/tools" "$FIXTURE/templates" "$FIXTURE/elsewhere"
  cat > "$GH_CONFIG_DIR/hosts.yml" <<'YAML'
identities-fixture.invalid:
    user: fixture-bob
    oauth_token: fixture-bob-not-a-secret
    git_protocol: https
    users:
        fixture-alice:
            oauth_token: fixture-alice-not-a-secret
        fixture-bob:
            oauth_token: fixture-bob-not-a-secret
YAML
  printf 'telemetry: disabled\n' > "$GH_CONFIG_DIR/config.yml"
  printf '[user]\n name = Fixture User\n email = fixture@example.invalid\n[init]\n defaultBranch = main\n' > "$GIT_CONFIG_GLOBAL"
  printf '#!/bin/sh\nexit 44\n' > "$FIXTURE/tools/security"
  printf '#!/bin/sh\nexit 1\n' > "$FIXTURE/tools/secret-tool"
  cat > "$FIXTURE/tools/ssh" <<'SH'
#!/bin/sh
for arg do
  if [ "$arg" = -G ]; then printf 'hostname offline.invalid\n'; exit 0; fi
done
printf '%s\n' "$@" >> "$REVIEW_SSH_LOG"
printf '0000'
SH
  chmod 700 "$FIXTURE/tools/"*
  cd "$FIXTURE"
  "$GH" auth identity create work --hostname "$HOST" --account fixture-alice --ssh-host work-ssh.invalid > setup.log 2>&1
  "$GH" auth identity create personal --hostname "$HOST" --account fixture-bob --ssh-host personal-ssh.invalid >> setup.log 2>&1
  "$GH" auth identity create project --hostname "$HOST" --account fixture-alice --ssh-host project-ssh.invalid >> setup.log 2>&1
  echo 'SIMULATED authentication: fake saved accounts; .invalid host; no network.'
  echo 'OS sandbox denies network, /usr/bin/security and Keychains access.'
  echo 'Private HOME, Git config, keyring stubs and SSH transport are isolated.'
}

fixture_open() {
  [[ -f $FIXTURE/.review-owned && ! -L $FIXTURE ]] || exit 2
  [[ $(cat "$FIXTURE/.review-owned") = "multiaccount-review-$LAYER" ]] || exit 2
  cd "$FIXTURE"
}

repo() {
  git init -q "$1"
  git -C "$1" remote add origin "git@$HOST:$2/project.git"
}

bind() {
  (cd "$1" && "$GH" auth use "$2") >> "$FIXTURE/setup.log" 2>&1
}

selection() {
  local actual
  actual=$(cd "$1" && "$GH" auth use --view --hostname "$HOST")
  printf '%s\n' "$actual"
  printf '%s\n' "$actual" | grep -F "\"name\": \"$2\"" >/dev/null
}

credential() {
  local actual
  actual=$(cd "$1" && printf 'protocol=https\nhost=%s\npath=%s/project.git\n\n' "$HOST" "$2" | git credential fill)
  printf '%s\n' "$actual" | grep -Fx "username=$3"
}

transport() {
  : > "$REVIEW_SSH_LOG"
  git -C "$1" ls-remote origin
  grep -Fx "git@$2" "$REVIEW_SSH_LOG"
}

active_unchanged() {
  grep -Fx '    user: fixture-bob' "$GH_CONFIG_DIR/hosts.yml" >/dev/null
  echo 'PASS: saved active account remains fixture-bob.'
}

fixture_cleanup() {
  fixture_open
  cd /
  rm -rf -- "$FIXTURE"
}

review_main() {
  if [[ $CASE = all ]]; then
    [[ $PHASE = run-all ]] || exit 2
    for next in $CASES; do
      /bin/bash "$SELF" --gh "$GH" --fixture "$FIXTURE-$next" --case "$next" --phase run-all
    done
    return
  fi
  case " $CASES " in *" $CASE "*) ;; *) echo "Unknown case: $CASE" >&2; exit 2 ;; esac
  case "$PHASE" in
    setup) setup_case ;;
    exercise) fixture_open; exercise_case ;;
    validate) fixture_open; validate_case ;;
    cleanup) fixture_cleanup ;;
    run-all)
      setup_case
      trap fixture_cleanup EXIT
      exercise_case
      fixture_open
      validate_case
      ;;
  esac
}
