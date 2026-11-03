#!/bin/bash
# Invoked by the env -i entrypoints, never source this into an operator shell.
set -euo pipefail
umask 077
layer=$1; shift
gh= root= phase=run-all case_name=
while (($#)); do
  case "$1" in
    --gh) gh=$2; shift 2 ;;
    --root) root=$2; shift 2 ;;
    --case) case_name=$2; shift 2 ;;
    --phase) phase=$2; shift 2 ;;
    run-all) phase=run-all; shift ;;
    *) printf 'Unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done
case "$gh" in /*) test -x "$gh" ;; *) echo '--gh requires an absolute executable path' >&2; exit 2 ;; esac
if [[ -z "$root" ]]; then
  [[ "$phase" == run-all ]] || { echo '--root is required for separate phases' >&2; exit 2; }
  case "${REVIEW_ALLOCATION_HOME:-}" in
    /*) ;;
    *) echo 'Default allocation requires an absolute HOME; otherwise supply --root outside a repository.' >&2; exit 2 ;;
  esac
  root="$REVIEW_ALLOCATION_HOME/.multiaccount-review-$layer-$$-$RANDOM-$RANDOM"
fi
case "$root" in /*) ;; *) echo '--root must be absolute' >&2; exit 2 ;; esac
root_parent=$(cd -P -- "$(dirname -- "$root")" && pwd)
root_name=$(basename -- "$root")
[[ "$root_name" != . && "$root_name" != .. && "$root_name" != / ]] || {
  echo 'Refusing a filesystem root or dot-component fixture root' >&2; exit 2;
}
root="$root_parent/$root_name"
if /usr/bin/env -i PATH=/usr/bin:/bin GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 \
  /usr/bin/git -C "$root_parent" rev-parse --git-dir >/dev/null 2>&1; then
  echo 'Fixture roots must be outside Git repositories' >&2
  exit 2
fi
unset REVIEW_ALLOCATION_HOME
export HOME="$root/home" GH_CONFIG_DIR="$root/home/gh" XDG_CONFIG_HOME="$root/home/.config"
export XDG_CACHE_HOME="$root/cache" XDG_DATA_HOME="$root/data" TMPDIR="$root/scratch"
export GIT_CONFIG_GLOBAL="$root/home/.gitconfig" GIT_CONFIG_NOSYSTEM=1 GIT_TERMINAL_PROMPT=0
export GIT_PAGER=cat GH_PAGER=cat PAGER=cat TERM=xterm-truecolor
export GIT_CEILING_DIRECTORIES="$root" GIT_TEMPLATE_DIR="$root/templates"
export GH_HOST=review-fixture.invalid GH_PROMPT_DISABLED=1 GH_NO_UPDATE_NOTIFIER=1 GH_TELEMETRY=0
export GIT_AUTHOR_DATE=2026-01-01T00:00:00Z GIT_COMMITTER_DATE=2026-01-01T00:00:00Z
export PATH="$root/tools:/usr/bin:/bin" GIT_SSH_VARIANT=ssh
export REVIEW_SSH_LOG="$root/ssh-args.log" REVIEW_GH="$gh"
host=review-fixture.invalid
run() { printf '$'; printf ' %q' "$@"; printf '\n'; "$@"; sleep 0.2; }
git_run() { run git "$@"; }
gh_run() { printf '$ gh'; printf ' %q' "$@"; printf '\n'; "$gh" "$@"; }
equal() { [[ "$1" == "$2" ]] || { printf 'FAIL: expected <%s>, got <%s>\n' "$2" "$1" >&2; exit 1; }; }
initialize() {
  mkdir -m 700 "$root" || { echo 'Refusing to reuse an existing fixture root' >&2; exit 1; }
  mkdir -p "$HOME/gh" "$root"/{tools,templates,cache,data,scratch}
  printf '%s\n' "$gh" > "$root/binary"
  printf '%s\n' "$layer" > "$root/layer"
  cat > "$GIT_CONFIG_GLOBAL" <<'EOF'
[user]
	name = Ordinary User
	email = ordinary@example.invalid
[init]
	defaultBranch = main
[commit]
	gpgsign = false
[credential]
	interactive = false
EOF
  cat > "$GH_CONFIG_DIR/config.yml" <<'EOF'
telemetry: disabled
EOF
  cat > "$GH_CONFIG_DIR/hosts.yml" <<'EOF'
review-fixture.invalid:
    user: bob-fixture
    oauth_token: fixture-bob-not-a-secret
    git_protocol: ssh
    users:
        alice-fixture:
            oauth_token: fixture-alice-not-a-secret
        bob-fixture:
            oauth_token: fixture-bob-not-a-secret
EOF
  printf '#!/bin/sh\nexit 44\n' > "$root/tools/security"
  printf '#!/bin/sh\nexit 1\n' > "$root/tools/secret-tool"
  printf '#!/bin/sh\nexec "$REVIEW_GH" "$@"\n' > "$root/tools/gh"
  cat > "$root/tools/ssh" <<'EOF'
#!/bin/sh
echo 'Unexpected SSH transport blocked' >&2
exit 99
EOF
  cat > "$root/tools/git-remote-https" <<'EOF'
#!/bin/sh
echo 'Unexpected HTTPS transport blocked' >&2
exit 99
EOF
  cp "$root/tools/git-remote-https" "$root/tools/git-remote-http"
  export GIT_EXEC_PATH="$root/tools"
  chmod 700 "$root/tools/"*
  printf 'Isolated synthetic accounts; no network or keychain access.\n'
}
enter() {
  [[ -d "$root" && ! -L "$root" && -O "$root" ]] || {
    echo 'Fixture root must be an owned directory, not a symlink' >&2; exit 2;
  }
  equal "$(/usr/bin/stat -f %Lp "$root")" 700
  equal "$(cat "$root/binary")" "$gh"
  equal "$(cat "$root/layer")" "$layer"
  export GIT_EXEC_PATH="$root/tools"
  cd "$root"
}
identity() {
  gh_run auth identity create "$1" --hostname "$host" --account "$2" \
    --ssh-host "$3" --git-user-name "$4" --git-user-email "$5"
}
repo() {
  git_run init -q "$root/main"
  cd "$root/main"
  git_run remote add origin "git@$host:company/project.git"
  git_run commit --allow-empty -qm 'Fixture baseline'
}
commit_check() {
  local directory=$1 name=$2 email=$3
  cd "$root/$directory"
  git_run commit --allow-empty -qm 'Attribution readback'
  git_run log -1 '--format=author: %an <%ae>%ncommitter: %cn <%ce>'
  equal "$(git log -1 --format='%an|%ae|%cn|%ce')" "$name|$email|$name|$email"
}
attribution() {
  case "$case_name/$phase" in
    family/setup)
      initialize; enter
      identity work alice-fixture alice-route 'Alice Work' alice@work.invalid
      repo
      git_run worktree add --quiet --detach "$root/linked"
      git_run var GIT_AUTHOR_IDENT
      cd "$root/linked"; git_run var GIT_AUTHOR_IDENT
      ;;
    family/exercise)
      enter; cd "$root/linked"; gh_run auth use work
      ;;
    family/validate)
      enter
      commit_check main 'Alice Work' alice@work.invalid
      commit_check linked 'Alice Work' alice@work.invalid
      echo 'PASS: author and committer follow the family choice in both worktrees.'
      ;;
    edit/setup)
      enter; cd "$root/main"
      gh_run auth identity view work
      git_run var GIT_AUTHOR_IDENT
      ;;
    edit/exercise)
      enter
      gh_run auth identity edit work --git-user-name 'Alice Revised' --git-user-email revised@work.invalid
      ;;
    edit/validate)
      enter
      commit_check main 'Alice Revised' revised@work.invalid
      commit_check linked 'Alice Revised' revised@work.invalid
      echo 'PASS: both worktrees immediately use the edited literal name and email.'
      ;;
    edit/cleanup)
      enter; gh_run auth identity delete --all --confirm
      cd "$root/main"
      equal "$(git config user.email)" ordinary@example.invalid
      cd "$root/linked"
      equal "$(git config user.email)" ordinary@example.invalid
      echo 'PASS: cleanup restores ordinary attribution in both worktrees.'
      echo 'Not exercised: @public and @noreply require API lookups.'
      ;;
    *) echo 'Use --case family|edit --phase setup|exercise|validate (edit also cleanup)' >&2; exit 2 ;;
  esac
}
ssh_args_check() {
  local alias=$1
  cat "$REVIEW_SSH_LOG"
  grep -Fx -- '<-F>' "$REVIEW_SSH_LOG" >/dev/null
  grep -Fx -- '<company config>' "$REVIEW_SSH_LOG" >/dev/null
  grep -Fx -- '<-o>' "$REVIEW_SSH_LOG" >/dev/null
  grep -Fx -- '<ConnectTimeout=7>' "$REVIEW_SSH_LOG" >/dev/null
  grep -Fx -- "<git@$alias>" "$REVIEW_SSH_LOG" >/dev/null
  grep -Fx -- "<git-upload-pack 'company/project.git'>" "$REVIEW_SSH_LOG" >/dev/null
}
ssh_scenario() {
  case "$case_name/$phase" in
    wrapper/setup)
      initialize; enter
      cat > "$root/tools/enterprise-ssh" <<'EOF'
#!/bin/sh
{
  printf 'wrapper: enterprise-ssh\n'
  for arg do printf '<%s>\n' "$arg"; done
} >> "$REVIEW_SSH_LOG"
# A local empty Git protocol advertisement. No SSH process or connection.
printf '0000'
EOF
      chmod 700 "$root/tools/enterprise-ssh"
      cat > "$root/main-ssh-config" <<'EOF'
Host alice-route bob-route
    HostName review-fixture.invalid
    Port 2222
    ProxyJump bastion.invalid
EOF
      git_run config --global core.sshCommand "'$root/tools/enterprise-ssh' -F 'company config' -o ConnectTimeout=7"
      git config --global core.sshCommand > "$root/original-ssh-command"
      cp "$root/main-ssh-config" "$root/original-ssh-config"
      identity work alice-fixture alice-route 'Alice Work' alice@work.invalid
      identity personal bob-fixture bob-route 'Bob Personal' bob@personal.invalid
      repo
      cp "$root/main-ssh-config" "$root/main/company config"
      git_run config --global --get core.sshCommand
      cat "$root/main/company config"
      ;;
    wrapper/exercise)
      enter; cd "$root/main"; gh_run auth use personal
      ;;
    wrapper/validate)
      enter; cd "$root/main"; : > "$REVIEW_SSH_LOG"
      git_run ls-remote origin
      ssh_args_check bob-route
      cmp "$root/original-ssh-config" "$root/main/company config"
      equal "$(git config remote.origin.url)" "git@$host:company/project.git"
      echo 'PASS: the inherited wrapper, arguments and connection config survive routing.'
      ;;
    override/setup)
      enter; cd "$root/main"; gh_run auth use --hostname "$host"
      git_run config --get user.email
      cp "$GH_CONFIG_DIR/hosts.yml" "$root/hosts-before"
      : > "$REVIEW_SSH_LOG"
      ;;
    override/exercise)
      enter; cd "$root/main"
      set +e
      gh_run repo sync --source "$host/company/project" --branch main --account alice-fixture > "$root/sync-output" 2>&1
      status=$?
      set -e
      cat "$root/sync-output"
      printf 'Observed exit: %s (empty fake remote has no main branch)\n' "$status"
      [[ $status -ne 0 ]]
      grep -F "couldn't find remote ref refs/heads/main" "$root/sync-output" >/dev/null
      ;;
    override/validate)
      enter; cd "$root/main"
      ssh_args_check alice-route
      : > "$REVIEW_SSH_LOG"
      git_run ls-remote origin
      ssh_args_check bob-route
      gh_run auth use --hostname "$host"
      equal "$(git config user.email)" bob@personal.invalid
      equal "$(git config remote.origin.url)" "git@$host:company/project.git"
      cmp "$GH_CONFIG_DIR/hosts.yml" "$root/hosts-before"
      cmp "$root/original-ssh-config" "$root/main/company config"
      echo 'PASS: only the gh invocation uses Alice; Git still uses Bob afterward.'
      ;;
    override/cleanup)
      enter; gh_run auth identity delete --all --confirm
      cd "$root/main"
      equal "$(git config --global core.sshCommand)" "$(cat "$root/original-ssh-command")"
      git_run config --global --get core.sshCommand
      : > "$REVIEW_SSH_LOG"; git_run ls-remote origin
      ssh_args_check "$host"
      cmp "$root/original-ssh-config" "$root/main/company config"
      echo 'PASS: cleanup restores the original wrapper and unmodified destination.'
      echo 'Simulated transport only: no SSH keys, connection or account verification.'
      ;;
    *) echo 'Use --case wrapper|override --phase setup|exercise|validate (override also cleanup)' >&2; exit 2 ;;
  esac
}
dispatch() {
  if [[ "$layer" == 05 ]]; then attribution; else ssh_scenario; fi
}
if [[ "$phase" == run-all ]]; then
  if [[ "$layer" == 05 ]]; then cases=(family edit); else cases=(wrapper override); fi
  for case_name in "${cases[@]}"; do
    for phase in setup exercise validate; do dispatch; done
  done
  phase=cleanup; dispatch
  printf 'Retained isolated fixture: %s\n' "$root"
else
  dispatch
fi
