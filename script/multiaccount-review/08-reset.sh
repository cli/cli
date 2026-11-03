#!/bin/bash
# Replay: bash 08-reset.sh --gh /absolute/debug-gh --old-gh /absolute/stable-gh run-all
set -euo pipefail
LAYER=08-reset.sh
source "$(cd "$(dirname "$0")" && pwd -P)/common.sh"
if [[ "${1:-}" = --_isolated ]]; then shift; else parse reset "$@"; fi
inner_parse "$@"
[[ "$CASE" = reset ]] || die "unknown case"

credential() {
  printf 'protocol=https\nhost=%s\npath=company/project.git\n\n' "$HOST" |
    "$GH" auth git-credential get
}
setup() {
  base_setup
  "$GH" version > "$F/subject-version"
  grep -Fq '[debug build]' "$F/subject-version" || die "layer8 replay requires a ghdebug build"
  grep -F 'OS keyring: disabled (isolated debug workflow)' "$F/subject-version" ||
    die "layer8 debug keyring isolation capability is not enabled"
  [[ "$SOURCE_OLD" = /* && -x "$SOURCE_OLD" ]] || die "reset requires --old-gh /absolute/stable-gh"
  cp "$SOURCE_OLD" "$F/tools/stable-gh"
  cp "$SOURCE_OLD" "$GH"
  "$GH" version | head -1
  "$GH" version > "$F/stable-version"
  grep -q 'gh version 2.102.0' "$F/stable-version" || die "expected stable 2.102.0"
  git config --global "credential.https://$HOST.helper" "!$GH auth git-credential"
  git config --global core.sshCommand 'ssh -F fixture-config'
  git config --global ssh.variant ssh
  "$GH" alias set fixture-version version
  cp "$GH_CONFIG_DIR/hosts.yml" "$F/before-hosts"
  cp "$GH_CONFIG_DIR/config.yml" "$F/before-config"
  cp "$GIT_CONFIG_GLOBAL" "$F/before-gitconfig"
  cd "$F/tree/project"
  credential > "$F/before-credential"
  cp "$SOURCE_GH" "$GH"
  identity experiment identity-fixture-alice 'Experiment Identity' experiment@example.invalid
  "$GH" auth use experiment
  expect "$(git config user.email)" experiment@example.invalid \
    'opt-in changes independent Git attribution'
  credential > "$F/experiment-credential"
  grep -q '^username=identity-fixture-alice$' "$F/experiment-credential"
  printf 'PASS opt-in selects synthetic Alice for the Git credential helper (token hidden)\n'
}
exercise() {
  "$GH" auth reset-multi-account-experiment --confirm
}
validation() {
  "$GH" auth reset-multi-account-experiment --confirm
  [[ ! -e "$GH_CONFIG_DIR/auth.gitconfig" && ! -d "$GH_CONFIG_DIR/identities" ]] ||
    die "experiment state remains"
  cmp "$F/before-hosts" "$GH_CONFIG_DIR/hosts.yml"
  cmp "$F/before-config" "$GH_CONFIG_DIR/config.yml"
  cmp "$F/before-gitconfig" "$GIT_CONFIG_GLOBAL"
  printf 'PASS reset is idempotent; original accounts, aliases and Git config are byte-identical\n'
  cp "$F/tools/stable-gh" "$GH"
  "$GH" version | head -1
  "$GH" version > "$F/after-version"
  cmp "$F/stable-version" "$F/after-version"
  "$GH" fixture-version > "$F/after-alias"
  cmp "$F/stable-version" "$F/after-alias"
  cd "$F/tree/project"
  credential > "$F/after-credential"
  cmp "$F/before-credential" "$F/after-credential"
  grep '^username=' "$F/after-credential"
  printf 'PASS stable 2.102.0 alias and saved credential response restored (token hidden)\n'
  expect "$(git config core.sshCommand)" 'ssh -F fixture-config' 'original independent SSH command restored'
  expect "$(git config ssh.variant)" ssh 'original SSH variant restored'
  expect "$(git config user.email)" ordinary@example.invalid 'original independent Git attribution restored'
  git -c commit.gpgsign=false commit --allow-empty -qm 'After reset'
  git log -1 '--format=%ae | %ce'
  expect "$(git log -1 '--format=%ae|%ce')" 'ordinary@example.invalid|ordinary@example.invalid' \
    'actual commit after downgrade has original author and committer'
  safe_network
}
run_phases
