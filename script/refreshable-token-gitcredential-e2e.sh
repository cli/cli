#!/usr/bin/env bash
#
# Guided, interactive end-to-end verification for the git credential helper with
# refreshable (short-lived) OAuth tokens.
#
# This script is a review aid, not a merged test. It runs on your own host (no
# container) so you exercise the gh binary under test against your real git.
# It walks you, one command at a time, through what happens when gh is set as
# your git credential helper and hands git a short-lived token: resolving a
# credential, refreshing an expired one on demand, marking the credential
# non-cacheable on git 2.46 or newer, and warning on older git.
#
# Two things matter for this flow:
#
#   1. gh must be YOUR git credential helper. You run this build's
#      `gh auth setup-git --hostname github.com` so git calls the binary under
#      test, not whatever gh is already on your PATH.
#   2. Your git version. gh can only mark a short-lived token as non-cacheable
#      (the git 2.46 "ephemeral" attribute) on git 2.46 or newer. On older git
#      gh instead warns, on stderr and surfaced by git, that a caching helper
#      chained in front of gh could serve a stale token.
#
# Refresh detection: with GH_DEBUG=api set, gh logs a head line for every HTTP
# request. A token refresh is a POST to the OAuth token endpoint, so a
# "login/oauth/access_token" line in stderr means a refresh happened. Because
# git runs the helper as a child process and does not capture its stderr, these
# lines and gh's warnings reach your terminal through git.
#
# Usage:
#   script/refreshable-token-gitcredential-e2e.sh
#
# Run this from a standalone terminal, not VS Code's integrated terminal: VS Code
# injects its own GIT_ASKPASS and credential helper that intercept git credential
# requests before gh, so gh's helper would never be exercised. The script refuses
# to run under VS Code unless GH_E2E_ALLOW_VSCODE=1 is set.
#
# Environment:
#   GH_BIN              Path to the gh binary to test. Defaults to ./bin/gh, else gh on PATH.
#   GH_E2E_HOST         Host to authenticate against. Defaults to github.com.
#   GH_E2E_ALLOW_VSCODE Set to bypass the VS Code integrated-terminal guard.

set -uo pipefail

# ---------------------------------------------------------------------------
# Presentation helpers
# ---------------------------------------------------------------------------

if [ -t 1 ]; then
  BOLD=$'\033[1m'; DIM=$'\033[2m'; RESET=$'\033[0m'
  GREEN=$'\033[32m'; RED=$'\033[31m'; YELLOW=$'\033[33m'; CYAN=$'\033[36m'
else
  BOLD=""; DIM=""; RESET=""; GREEN=""; RED=""; YELLOW=""; CYAN=""
fi

divider() {
  printf '%s\n' "------------------------------------------------------------------------"
}

label() { # label <word> <rest...>
  local word=$1; shift
  printf "%s%-5s%s %s\n" "$BOLD" "$word" "$RESET" "$*"
}

note() { printf "%s%s%s\n" "$DIM" "$*" "$RESET"; }

# cmdline prints the exact command that is about to run, highlighted and set
# apart from the dim explanatory notes.
cmdline() { printf "  %s%s%s\n" "$YELLOW" "$*" "$RESET"; }

# print_running echoes the clean command line right before it runs.
print_running() { printf "  %s\$ %s%s\n" "$CYAN" "$1" "$RESET"; }

# heading prints a bold section heading, used to label the captured streams.
heading() { printf "%s%s%s\n" "$BOLD" "$1" "$RESET"; }

# warn_red prints a bold red warning block, one line per argument.
warn_red() { local line; for line in "$@"; do printf "%s%s%s%s\n" "$BOLD" "$RED" "$line" "$RESET"; done; }

pass() { printf "  %sPASS%s %s\n" "$GREEN" "$RESET" "$1"; }
fail() { printf "  %sFAIL%s %s\n" "$RED" "$RESET" "$1"; FAILURES=$((FAILURES + 1)); }

FAILURES=0

# ---------------------------------------------------------------------------
# Capture harness
# ---------------------------------------------------------------------------
#
# OUT and ERR hold the captured stdout and stderr of the most recent command.

TMP="$(mktemp -d)"
OUT="$TMP/out.log"
ERR="$TMP/err.log"
trap 'rm -rf "$TMP"' EXIT

# GIT_NEUTRAL runs git from the scratch dir, which is not a git repository, so
# only system and global git config apply. This keeps repo-local settings from
# shadowing the global gh credential helper when the script is run from inside a
# repo. In particular, a blank credential.helper in a repo's local config resets
# git's helper chain, and because local config is applied after global config it
# would wipe out the gh helper set globally by gh auth setup-git. mktemp -d
# yields a path outside any repo on both Linux and macOS.
GIT_NEUTRAL="git -C \"$TMP\""

# run_gh runs a non-interactive gh command with a forced TTY so gh renders as in
# a real terminal, capturing both streams.
run_gh() { # run_gh <display> <exec>
  : >"$OUT"; : >"$ERR"
  print_running "$1"
  eval "GH_FORCE_TTY=1 $2" >"$OUT" 2>"$ERR"
  RC=$?
  heading "stdout:"
  cat "$OUT"
  heading "stderr:" >&2
  cat "$ERR" >&2
  return $RC
}

# run_interactive runs a command you must interact with (login). It tees both
# streams live so prompts stay visible while still being captured.
run_interactive() { # run_interactive <display> <exec>
  : >"$OUT"; : >"$ERR"
  print_running "$1"
  eval "GH_FORCE_TTY=1 $2" > >(tee "$OUT") 2> >(tee "$ERR" >&2)
  RC=$?
  sleep 0.15
  return $RC
}

# run_git runs a plain git command (no forced gh TTY). The gh credential helper
# git spawns inherits the environment, so any test env vars set in the exec form
# reach gh, and gh's stderr reaches us through git.
run_git() { # run_git <display> <exec>
  : >"$OUT"; : >"$ERR"
  print_running "$1"
  eval "$2" >"$OUT" 2>"$ERR"
  RC=$?
  heading "stdout:"
  cat "$OUT"
  heading "stderr:" >&2
  cat "$ERR" >&2
  return $RC
}

# ---------------------------------------------------------------------------
# Assertions
# ---------------------------------------------------------------------------

refresh_happened() { grep -q 'login/oauth/access_token' "$ERR"; }

assert_refresh()    { if refresh_happened; then pass "$1"; else fail "$1"; fi; }
assert_no_refresh() { if refresh_happened; then fail "$1"; else pass "$1"; fi; }

assert_out_contains() { # assert_out_contains <desc> <substr>
  if grep -qiF -- "$2" "$OUT"; then pass "$1"; else fail "$1"; fi
}
assert_err_contains() { # assert_err_contains <desc> <substr>
  if grep -qiF -- "$2" "$ERR"; then pass "$1"; else fail "$1"; fi
}
assert_err_absent() { # assert_err_absent <desc> <substr>
  if grep -qiF -- "$2" "$ERR"; then fail "$1"; else pass "$1"; fi
}
assert_out_empty() { # assert_out_empty <desc>
  if [ -s "$OUT" ]; then fail "$1"; else pass "$1"; fi
}

manual() { # manual <thing to eyeball>
  printf "  %s??  %s%s (confirm visually)\n" "$YELLOW" "$RESET" "$1"
}

# ---------------------------------------------------------------------------
# Flow control
# ---------------------------------------------------------------------------

confirm_run() {
  printf "\n%sPress Enter to run%s (q to quit): " "$BOLD" "$RESET"
  local ans; read -r ans
  [ "$ans" = "q" ] && { echo "Aborted."; exit 0; }
}

next_step() {
  printf "\n%sPress Enter for the next step%s (q to quit): " "$BOLD" "$RESET"
  local ans; read -r ans
  [ "$ans" = "q" ] && { echo "Stopped."; exit 0; }
  echo; divider; echo
}

# ---------------------------------------------------------------------------
# Prerequisites
# ---------------------------------------------------------------------------

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GH="${GH_BIN:-$ROOT/bin/gh}"
if [ ! -x "$GH" ]; then
  GH="$(command -v gh || true)"
fi
HOST="${GH_E2E_HOST:-github.com}"
HELPER_KEY="credential.https://$HOST.helper"
NONCACHEABLE_WARNING="cannot mark the short-lived token as non-cacheable"

# GH_DISPLAY is the name shown in the command echoes. Commands still execute
# with the resolved GH binary; the echoes just read as plain `gh`.
GH_DISPLAY="gh"

require_gh() {
  if [ -z "$GH" ]; then
    echo "No gh binary found. Build the stack (make) or set GH_BIN." >&2
    exit 1
  fi
  # The scenarios rely on testing-only env hacks compiled into this build.
  if ! grep -aq 'GH_AT_EXPIRES_IN' "$GH"; then
    echo "This gh binary lacks the testing hacks (GH_AT_EXPIRES_IN)." >&2
    echo "Check out and build the refreshable-token stack, then retry." >&2
    exit 1
  fi
}

require_git() {
  if ! command -v git >/dev/null 2>&1; then
    echo "git is not installed or not on PATH." >&2
    exit 1
  fi
}

# in_vscode reports whether we are running inside VS Code's integrated terminal.
in_vscode() {
  [ "${TERM_PROGRAM:-}" = "vscode" ] && return 0
  [ -n "${VSCODE_GIT_ASKPASS_MAIN:-}" ] && return 0
  [ -n "${VSCODE_GIT_IPC_HANDLE:-}" ] && return 0
  [ -n "${VSCODE_PID:-}" ] && return 0
  [ -n "${VSCODE_INJECTION:-}" ] && return 0
  case "${GIT_ASKPASS:-}" in
    *[Vv]scode*|*"Visual Studio Code"*) return 0 ;;
  esac
  return 1
}

# require_external_terminal refuses to run inside VS Code's integrated terminal.
# VS Code injects its own GIT_ASKPASS and credential helper, which intercept git
# credential requests before gh is consulted, so git never exercises the gh
# helper and instead serves or prompts for a credential VS Code manages. Run this
# script from a standalone terminal instead. Set GH_E2E_ALLOW_VSCODE=1 to bypass
# this guard if you are certain your VS Code is not intercepting credentials.
require_external_terminal() {
  if [ -n "${GH_E2E_ALLOW_VSCODE:-}" ]; then
    return
  fi
  if in_vscode; then
    warn_red \
      "This looks like VS Code's integrated terminal." \
      "VS Code injects its own GIT_ASKPASS and credential helper, which intercept" \
      "git credential requests before gh is consulted. git would never exercise the" \
      "gh credential helper, and you would see a credential VS Code manages instead" \
      "of gh's, exactly the confusing 'weird username/password' this flow avoids."
    echo >&2
    note "Run this script from a standalone terminal (for example Terminal.app, iTerm2,"
    note "GNOME Terminal, Konsole, or Windows Terminal), not the VS Code panel."
    note "If you are certain VS Code is not intercepting credentials, rerun with"
    note "GH_E2E_ALLOW_VSCODE=1 to bypass this guard."
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# git version detection
# ---------------------------------------------------------------------------
#
# The authtype capability that lets gh mark a credential ephemeral arrived in
# git 2.46. GIT_NEW is 1 on git 2.46 or newer, 0 otherwise (including when the
# version cannot be parsed).

GIT_RAW="$(git --version 2>/dev/null)"
GIT_VER="$(printf '%s' "$GIT_RAW" | sed -nE 's/^git version ([0-9]+\.[0-9]+).*/\1/p')"
GIT_NEW=0
if [ -n "$GIT_VER" ]; then
  GIT_MAJ="${GIT_VER%%.*}"
  GIT_MIN="${GIT_VER#*.}"
  if [ "$GIT_MAJ" -gt 2 ] || { [ "$GIT_MAJ" -eq 2 ] && [ "$GIT_MIN" -ge 46 ]; }; then
    GIT_NEW=1
  fi
fi

# ---------------------------------------------------------------------------
# Intro
# ---------------------------------------------------------------------------

require_external_terminal
require_git
require_gh

clear 2>/dev/null || true
divider
label "Refreshable token git credential helper e2e" "- host: $HOST"
note "gh binary: $GH"
note "git:       ${GIT_RAW:-unknown}"
divider
echo

if [ "$GIT_NEW" -ne 1 ]; then
  warn_red \
    "WARNING: your git (${GIT_RAW:-unknown}) is older than 2.46." \
    "Older git does not support the authtype capability, so gh cannot mark a" \
    "short-lived token as non-cacheable (the git 2.46 ephemeral attribute)." \
    "If a credential caching helper is chained in front of gh, it may store the" \
    "short-lived token and keep serving it after gh has refreshed or replaced it," \
    "causing stale-token failures. Upgrade to git 2.46 or newer, or avoid a" \
    "credential caching helper for $HOST."
  echo
  note "You will see this same non-cacheable warning at runtime whenever the gh"
  note "credential helper resolves a short-lived token on this git: gh prints it to"
  note "stderr and git surfaces it. This script exercises that path below."
else
  note "Your git is 2.46 or newer, so gh marks short-lived credentials ephemeral"
  note "and a caching helper chained in front of gh will refuse to store them."
fi
echo
note "For reference, two related warnings live in the auth commands themselves:"
note "  - gh auth login and gh auth refresh recommend git 2.46+ in their --help text."
note "  - if a non-gh credential helper is configured for a host, gh auth login and"
note "    gh auth refresh warn at runtime that it cannot refresh short-lived tokens"
note "    and point you at gh auth setup-git. That is a separate warning from the"
note "    git-version one above."
echo
note "This drives real gh auth commands and will clobber stored credentials for $HOST,"
note "and it repoints your global git credential helper for https://$HOST at the gh"
note "binary under test. The final step tells you how to restore your normal setup."
echo
printf "%sReady? Press Enter to begin%s (q to quit): " "$BOLD" "$RESET"
read -r ans; [ "$ans" = "q" ] && exit 0
echo; divider; echo

# ===========================================================================
# Scenarios
# ===========================================================================

# --- Step: log in over HTTPS requesting a short-lived credential --------------
label "Given" "a clean auth state for $HOST, using default storage"
label "When" "we log in over HTTPS requesting a short-lived credential"
label "Then" "login succeeds, reports a short-lived refreshable token, and sets git protocol to https"
cmdline "$GH_DISPLAY auth login --hostname $HOST --git-protocol https --short-lived --clipboard"
note   "  (HTTPS is required: the git credential helper only applies to https remotes.)"
note   "  (--clipboard copies the one-time code, so you can just paste it into the browser.)"
note   "  (Storage is left at the default; this flow does not depend on the storage type.)"
confirm_run
run_interactive \
  "$GH_DISPLAY auth login --hostname $HOST --git-protocol https --short-lived --clipboard" \
  "\"$GH\" auth login --hostname \"$HOST\" --git-protocol https --short-lived --clipboard"

label "Expected:"
assert_err_contains "login reports a short-lived refreshable token" "refreshable token"
manual "you selected HTTPS as the git protocol"
next_step

# --- Step: make this build gh's git credential helper -------------------------
label "Given" "a signed-in account on $HOST"
label "When" "we run THIS build's auth setup-git for $HOST"
label "Then" "git is configured to call this gh as its credential helper for https://$HOST"
cmdline "$GH_DISPLAY auth setup-git --hostname $HOST"
note   "  (Run THIS build's setup-git so git uses the gh under test. Otherwise git"
note   "   would keep calling whatever gh is already on your PATH, and you would not"
note   "   be exercising this binary.)"
confirm_run
run_gh \
  "$GH_DISPLAY auth setup-git --hostname $HOST" \
  "\"$GH\" auth setup-git --hostname \"$HOST\""

label "Expected:"
if [ "$RC" -eq 0 ]; then pass "setup-git completed without error"; else fail "setup-git completed without error"; fi
next_step

# --- Step: confirm git now points at the build under test ---------------------
label "Given" "setup-git has run"
label "When" "we read git's configured credential helper for https://$HOST"
label "Then" "it points at this gh via 'auth git-credential'"
cmdline "git config --get-all $HELPER_KEY"
note   "  (git commands below run from a scratch directory so repo-local git config,"
note   "   for example a blank credential.helper that resets the helper chain, cannot"
note   "   shadow your global gh credential helper.)"
confirm_run
run_git \
  "git config --get-all $HELPER_KEY" \
  "$GIT_NEUTRAL config --get-all \"$HELPER_KEY\""

label "Expected:"
assert_out_contains "the helper is gh's 'auth git-credential'" "auth git-credential"
manual "the path in the helper points at the gh binary under test ($GH)"
next_step

# --- Step: the helper resolves a credential for git ---------------------------
label "Given" "gh is the credential helper and the access token is still valid"
label "When" "git asks gh for credentials for https://$HOST"
label "Then" "gh returns a working credential without refreshing it"
cmdline "printf 'protocol=https\\nhost=$HOST\\n' | git credential fill"
note   "  (git prints the resolved credential, including the token.)"
confirm_run
run_git \
  "printf 'protocol=https\\nhost=$HOST\\n' | git credential fill" \
  "printf 'protocol=https\\nhost=$HOST\\n' | GH_DEBUG=api GIT_TERMINAL_PROMPT=0 $GIT_NEUTRAL credential fill"

label "Expected:"
assert_out_contains "git received a username from the helper" "username="
assert_no_refresh "no refresh happened for a still-valid token"
next_step

# --- Step: the helper refreshes an expired token before git uses it -----------
label "Given" "gh is the credential helper and the access token is expired or near expiry"
label "When" "git asks gh for credentials"
label "Then" "gh refreshes the token first, then hands git a working credential"
cmdline "printf 'protocol=https\\nhost=$HOST\\n' | git credential fill"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_git \
  "printf 'protocol=https\\nhost=$HOST\\n' | git credential fill" \
  "printf 'protocol=https\\nhost=$HOST\\n' | GH_AT_EXPIRES_IN=1 GH_DEBUG=api GIT_TERMINAL_PROMPT=0 $GIT_NEUTRAL credential fill"

label "Expected:"
assert_refresh "gh refreshed the token before handing it to git"
assert_out_contains "git still received a username from the helper" "username="
next_step

# --- Step: version-dependent non-cacheable marking ----------------------------
label "Given" "a refreshable short-lived credential and gh as the helper"
label "When" "git ${GIT_RAW:-unknown} asks gh for credentials"
if [ "$GIT_NEW" -eq 1 ]; then
  label "Then" "gh marks the credential ephemeral so a caching helper will not store it, and prints no warning"
else
  label "Then" "gh warns on stderr that it cannot mark the token non-cacheable, and git surfaces the warning"
fi
cmdline "printf 'protocol=https\\nhost=$HOST\\n' | git credential fill"
confirm_run
run_git \
  "printf 'protocol=https\\nhost=$HOST\\n' | git credential fill" \
  "printf 'protocol=https\\nhost=$HOST\\n' | GIT_TERMINAL_PROMPT=0 $GIT_NEUTRAL credential fill"

label "Expected:"
if [ "$GIT_NEW" -eq 1 ]; then
  assert_err_absent "no non-cacheable warning on git 2.46 or newer" "$NONCACHEABLE_WARNING"
  manual "git shows authtype/ephemeral fields (gh marked the credential non-cacheable)"
else
  assert_err_contains "gh warns it cannot mark the token non-cacheable" "$NONCACHEABLE_WARNING"
  manual "git surfaced gh's warning above"
fi
next_step

# --- Step: an expired refresh token stops the helper --------------------------
label "Given" "a refreshable credential whose refresh token the server rejects"
label "When" "git asks gh for credentials and gh's refresh attempt is rejected"
label "Then" "gh clears its now-dead stored credential, hands git nothing, and tells you (through git) to run gh auth login"
cmdline "printf 'protocol=https\\nhost=$HOST\\n' | git credential fill"
heading "  This step logs you out of $HOST in gh."
note   "  (Access-token and refresh-token expiry are both forced via the GH_AT_EXPIRES_IN"
note   "   and GH_RT_EXPIRES_IN test hacks, so gh attempts a refresh and the refresh is"
note   "   rejected. On rejection gh removes its locally stored credential for the host,"
note   "   just as it would when the real server rejects a refresh token. The token is"
note   "   not revoked server-side; only gh's local copy is cleared, so you must run"
note   "   gh auth login again to keep using gh for $HOST.)"
note   "  (A non-zero exit is expected here: git gets no credential.)"
confirm_run
run_git \
  "printf 'protocol=https\\nhost=$HOST\\n' | git credential fill" \
  "printf 'protocol=https\\nhost=$HOST\\n' | GH_AT_EXPIRES_IN=1 GH_RT_EXPIRES_IN=1 GIT_TERMINAL_PROMPT=0 $GIT_NEUTRAL credential fill"

label "Expected:"
assert_err_contains "gh reports the token has expired" "has expired"
assert_err_contains "gh points you at gh auth login" "gh auth login"
assert_out_empty "git received no credential"
next_step

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

heading "Cleanup"
echo
note "Your global git credential helper for https://$HOST currently points at the"
note "gh binary under test. To restore your normal setup, run YOUR INSTALLED gh's"
note "setup-git (the gh already on your PATH, not this build):"
echo
cmdline "gh auth setup-git --hostname $HOST"
echo
note "If you ran the final scenario above, gh already cleared its stored credential"
note "for $HOST and you are logged out. If you skipped it and want to log out of a"
note "throwaway account, run it with the gh under test:"
echo
cmdline "$GH auth logout --hostname $HOST"
echo
divider
if [ "$FAILURES" -eq 0 ]; then
  printf "%s%sAll automatic checks passed.%s\n" "$BOLD" "$GREEN" "$RESET"
else
  printf "%s%s%d automatic check(s) failed.%s\n" "$BOLD" "$RED" "$FAILURES" "$RESET"
fi
divider
exit $((FAILURES > 0 ? 1 : 0))
