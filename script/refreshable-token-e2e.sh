#!/usr/bin/env bash
#
# Guided, interactive end-to-end verification for refreshable (short-lived) OAuth tokens.
#
# This script is a review aid, not a merged test. It walks a reviewer through the
# scenarios in files/e2e-verification-scenarios.md one command at a time, using a
# Given / When / Then framing. For each step it:
#
#   1. Prints Given / When / Then and the exact command it is about to run.
#   2. Waits for the reviewer to confirm the run.
#   3. Runs the command with a forced TTY so gh behaves as in a real terminal,
#      while capturing stdout and stderr to temp files for assertions.
#   4. Prints the Expected list and auto-checks what it can (PASS/FAIL), leaving
#      anything visual for the reviewer to eyeball.
#   5. Prints a divider and moves on.
#
# Refresh detection: with GH_DEBUG=api set, gh logs a head line for every HTTP
# request. A token refresh is a POST to the OAuth token endpoint, so the presence
# of a "login/oauth/access_token" line in stderr means a refresh happened.
#
# Usage:
#   script/refreshable-token-e2e.sh [--keyring | --config]
#
# Environment:
#   GH_BIN     Path to the gh binary to test. Defaults to ./bin/gh, else gh on PATH.
#   GH_E2E_HOST  Host to authenticate against. Defaults to github.com.

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

pass() { printf "  %sPASS%s %s\n" "$GREEN" "$RESET" "$1"; }
fail() { printf "  %sFAIL%s %s\n" "$RED" "$RESET" "$1"; FAILURES=$((FAILURES + 1)); }

FAILURES=0

# ---------------------------------------------------------------------------
# Capture harness
# ---------------------------------------------------------------------------
#
# OUT and ERR hold the captured stdout and stderr of the most recent command.
# GH_FORCE_TTY=1 makes gh render as if attached to a terminal even though its
# stdout is captured, so the reviewer sees the real interactive presentation.

TMP="$(mktemp -d)"
OUT="$TMP/out.log"
ERR="$TMP/err.log"
trap 'rm -rf "$TMP"' EXIT

# run_batch runs a non-interactive command. It prints the clean display form
# ($1), runs the real command with its test env ($2), and captures both streams.
# Safe from tee flush races because it redirects directly.
run_batch() { # run_batch <display> <exec>
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

# run_interactive runs a command the reviewer must interact with (login, refresh).
# It tees both streams live so prompts stay visible while still being captured.
run_interactive() { # run_interactive <display> <exec>
  : >"$OUT"; : >"$ERR"
  print_running "$1"
  eval "GH_FORCE_TTY=1 $2" > >(tee "$OUT") 2> >(tee "$ERR" >&2)
  RC=$?
  sleep 0.15
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
assert_out_absent() { # assert_out_absent <desc> <substr>
  if grep -qiF -- "$2" "$OUT"; then fail "$1"; else pass "$1"; fi
}
assert_out_nonempty() { # assert_out_nonempty <desc>
  if [ -s "$OUT" ]; then pass "$1"; else fail "$1"; fi
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

# ---------------------------------------------------------------------------
# Theme selection
# ---------------------------------------------------------------------------

THEME=""
case "${1:-}" in
  --keyring) THEME="keyring" ;;
  --config)  THEME="config" ;;
  "" ) : ;;
  * ) echo "Unknown option: $1"; echo "Usage: $0 [--keyring | --config]"; exit 2 ;;
esac

if [ -z "$THEME" ]; then
  printf "Select storage theme: [1] keyring (secure)  [2] config (insecure): "
  read -r t
  case "$t" in
    1) THEME="keyring" ;;
    2) THEME="config" ;;
    *) echo "Invalid selection."; exit 2 ;;
  esac
fi

if [ "$THEME" = "config" ]; then
  STORE_FLAG="--insecure-storage"
else
  STORE_FLAG=""
fi

# ---------------------------------------------------------------------------
# Intro
# ---------------------------------------------------------------------------

require_gh

clear 2>/dev/null || true
divider
label "Refreshable token e2e" "- theme: $THEME, host: $HOST"
note "gh binary: $GH"
divider
echo
note "This drives real gh auth commands and will clobber stored credentials for $HOST."
note "To trigger refreshes on demand, expiry is forced with a test-only env hack"
note "(GH_AT_EXPIRES_IN); whether a refresh happened is detected from gh's debug"
note "logs, and for auth status from its own output."
echo
printf "%sReady? Press Enter to begin%s (q to quit): " "$BOLD" "$RESET"
read -r ans; [ "$ans" = "q" ] && exit 0
echo; divider; echo

# ===========================================================================
# NEW SYSTEM
# ===========================================================================

# --- Step: login with --short-lived -------------------------------------------
label "Given" "a clean auth state for $HOST"
label "When" "we log in requesting a short-lived credential"
label "Then" "login succeeds and reports it received a short-lived refreshable token"
cmdline "$GH_DISPLAY auth login --hostname $HOST --short-lived --clipboard $STORE_FLAG"
note   "  (--clipboard copies the one-time code, so you can just paste it into the browser.)"
confirm_run
run_interactive \
  "$GH_DISPLAY auth login --hostname $HOST --short-lived --clipboard $STORE_FLAG" \
  "\"$GH\" auth login --hostname \"$HOST\" --short-lived --clipboard $STORE_FLAG"

label "Expected:"
manual "login succeeds with no error"
assert_err_contains "confirms a short-lived refreshable token was received" "Received short-lived refreshable token"
next_step

# --- Step: an ordinary command refreshes an expired token ---------------------
label "Given" "a signed-in short-lived credential"
label "When" "an ordinary command runs while the access token is expired or near expiry"
label "Then" "the command succeeds and gh refreshes the token first"
cmdline "$GH_DISPLAY api repos/cli/cli/contents/.gitattributes"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "$GH_DISPLAY api repos/cli/cli/contents/.gitattributes" \
  "GH_AT_EXPIRES_IN=1 GH_DEBUG=api \"$GH\" api repos/cli/cli/contents/.gitattributes"

label "Expected:"
assert_out_contains "the file contents are returned" "\"name\""
assert_refresh      "a refresh happened"
next_step

# --- Step: a valid token is not refreshed -------------------------------------
label "Given" "the same short-lived credential with a still-valid access token"
label "When" "the same command runs"
label "Then" "the command succeeds without any refresh"
cmdline "$GH_DISPLAY api repos/cli/cli/contents/.gitattributes"
confirm_run
run_batch \
  "$GH_DISPLAY api repos/cli/cli/contents/.gitattributes" \
  "GH_DEBUG=api \"$GH\" api repos/cli/cli/contents/.gitattributes"

label "Expected:"
assert_out_contains "the file contents are returned" "\"name\""
assert_no_refresh   "no refresh happened"
next_step

# --- Step: an environment token is never refreshed ----------------------------
label "Given" "a token supplied through the GH_TOKEN environment variable"
label "When" "a command runs while the token in storage is expired or near expiry"
label "Then" "the command succeeds and no refresh happens, because an environment token is never refreshed"
cmdline "GH_TOKEN=\$($GH_DISPLAY auth token --no-refresh) $GH_DISPLAY api repos/cli/cli/contents/.gitattributes"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "GH_TOKEN=\$($GH_DISPLAY auth token --no-refresh) $GH_DISPLAY api repos/cli/cli/contents/.gitattributes" \
  "GH_TOKEN=\$(\"$GH\" auth token --no-refresh) GH_AT_EXPIRES_IN=1 GH_DEBUG=api \"$GH\" api repos/cli/cli/contents/.gitattributes"

label "Expected:"
assert_out_contains "the file contents are returned" "\"name\""
assert_no_refresh   "no refresh happened (an environment token is never refreshed)"
next_step

# ---------------------------------------------------------------------------
# AUTH STATUS
# ---------------------------------------------------------------------------
#
# auth status refreshes the active credential on its own, so for the plain
# (human) form we detect a refresh from its output: it prints "Token refreshed
# just now" when it renewed the token. The --json form has no such line, so
# those steps fall back to the GH_DEBUG=api log detection used elsewhere.

# --- Step: status refreshes an expired token ----------------------------------
label "Given" "a signed-in short-lived credential"
label "When" "auth status runs while the access token is expired or near expiry"
label "Then" "status refreshes the token and shows it as a short-lived refreshable credential"
cmdline "$GH_DISPLAY auth status"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "$GH_DISPLAY auth status" \
  "GH_AT_EXPIRES_IN=1 \"$GH\" auth status"

label "Expected:"
assert_out_contains "the account is shown as a short-lived refreshable credential" "Short-lived token that gh refreshes automatically"
assert_out_contains "status reports it refreshed the token just now" "Token refreshed just now"
next_step

# --- Step: status --json also refreshes ---------------------------------------
label "Given" "the same short-lived credential"
label "When" "auth status --json hosts runs while the access token is expired or near expiry"
label "Then" "status refreshes the token and the JSON carries the refreshable fields"
cmdline "$GH_DISPLAY auth status --json hosts"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "$GH_DISPLAY auth status --json hosts" \
  "GH_AT_EXPIRES_IN=1 GH_DEBUG=api \"$GH\" auth status --json hosts"

label "Expected:"
assert_out_contains "the JSON includes the refreshable fields" "\"refreshable\""
assert_refresh      "a refresh happened"
next_step

# --- Step: status does not refresh a valid token ------------------------------
label "Given" "the same short-lived credential with a still-valid access token"
label "When" "auth status runs"
label "Then" "status shows the refreshable credential without refreshing it"
cmdline "$GH_DISPLAY auth status"
confirm_run
run_batch \
  "$GH_DISPLAY auth status" \
  "\"$GH\" auth status"

label "Expected:"
assert_out_contains "the account is shown as a short-lived refreshable credential" "Short-lived token that gh refreshes automatically"
assert_out_absent   "no refresh happened" "Token refreshed just now"
next_step

# --- Step: status --json does not refresh a valid token -----------------------
label "Given" "the same short-lived credential with a still-valid access token"
label "When" "auth status --json hosts runs"
label "Then" "the JSON carries the refreshable fields and no refresh happens"
cmdline "$GH_DISPLAY auth status --json hosts"
confirm_run
run_batch \
  "$GH_DISPLAY auth status --json hosts" \
  "GH_DEBUG=api \"$GH\" auth status --json hosts"

label "Expected:"
assert_out_contains "the JSON includes the refreshable fields" "\"refreshable\""
assert_no_refresh   "no refresh happened"
next_step

# --- Step: status --no-refresh never refreshes --------------------------------
label "Given" "a signed-in short-lived credential"
label "When" "auth status --no-refresh runs while the access token is expired or near expiry"
label "Then" "status still shows the refreshable credential but does not refresh it"
cmdline "$GH_DISPLAY auth status --no-refresh"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "$GH_DISPLAY auth status --no-refresh" \
  "GH_AT_EXPIRES_IN=1 \"$GH\" auth status --no-refresh"

label "Expected:"
assert_out_contains "the account is shown as a short-lived refreshable credential" "Short-lived token that gh refreshes automatically"
assert_out_absent   "no refresh happened despite forced expiry" "Token refreshed just now"
next_step

# --- Step: status --json --no-refresh never refreshes -------------------------
label "Given" "a signed-in short-lived credential"
label "When" "auth status --json hosts --no-refresh runs while the access token is expired or near expiry"
label "Then" "the JSON carries the refreshable fields and no refresh happens"
cmdline "$GH_DISPLAY auth status --json hosts --no-refresh"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "$GH_DISPLAY auth status --json hosts --no-refresh" \
  "GH_AT_EXPIRES_IN=1 GH_DEBUG=api \"$GH\" auth status --json hosts --no-refresh"

label "Expected:"
assert_out_contains "the JSON includes the refreshable fields" "\"refreshable\""
assert_no_refresh   "no refresh happened despite forced expiry"
next_step

# --- Step: an environment token is reported as such, never refreshed ----------
label "Given" "an unrelated token supplied through the GH_TOKEN environment variable"
label "When" "auth status runs"
label "Then" "the active entry is reported as sourced from GH_TOKEN and used verbatim, never refreshed"
cmdline "GH_TOKEN=gho_notarealtoken $GH_DISPLAY auth status"
note   "  (GH_TOKEN is a placeholder value here to show it is used as-is; an environment token is never refreshed.)"
note   "  (A non-zero exit is expected, since the placeholder is not a valid credential.)"
confirm_run
run_batch \
  "GH_TOKEN=gho_notarealtoken $GH_DISPLAY auth status" \
  "GH_TOKEN=gho_notarealtoken \"$GH\" auth status"

label "Expected:"
assert_err_contains "the active entry is reported as sourced from GH_TOKEN" "GH_TOKEN"
assert_err_contains "the placeholder token is used verbatim, not refreshed into a valid one" "invalid"
next_step

# --- Step: an environment token in --json is never refreshed ------------------
label "Given" "an unrelated token supplied through the GH_TOKEN environment variable"
label "When" "auth status --json hosts runs"
label "Then" "the active entry is reported as sourced from GH_TOKEN with no refreshable fields"
cmdline "GH_TOKEN=gho_notarealtoken $GH_DISPLAY auth status --json hosts"
note   "  (GH_TOKEN is a placeholder value here to show it is used as-is; an environment token is never refreshed.)"
confirm_run
run_batch \
  "GH_TOKEN=gho_notarealtoken $GH_DISPLAY auth status --json hosts" \
  "GH_TOKEN=gho_notarealtoken \"$GH\" auth status --json hosts"

label "Expected:"
assert_out_contains "the active entry is sourced from GH_TOKEN" "GH_TOKEN"
manual "the GH_TOKEN entry carries no refreshable or tokenExpiresAt fields (any refresh you see belongs to the separate stored account, not the environment token)"
next_step

# ---------------------------------------------------------------------------
# AUTH TOKEN
# ---------------------------------------------------------------------------
#
# auth token prints only the token, so a refresh is detected from the
# GH_DEBUG=api log (a POST to the OAuth token endpoint).

# --- Step: token refreshes an expired token -----------------------------------
label "Given" "a signed-in short-lived credential"
label "When" "auth token runs while the access token is expired or near expiry"
label "Then" "gh refreshes the token and prints the refreshed value"
cmdline "$GH_DISPLAY auth token"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "$GH_DISPLAY auth token" \
  "GH_AT_EXPIRES_IN=1 GH_DEBUG=api \"$GH\" auth token"

label "Expected:"
assert_out_nonempty "a token is printed"
assert_refresh      "a refresh happened"
next_step

# --- Step: token does not refresh a valid token -------------------------------
label "Given" "the same short-lived credential with a still-valid access token"
label "When" "auth token runs"
label "Then" "gh prints the current token without refreshing it"
cmdline "$GH_DISPLAY auth token"
confirm_run
run_batch \
  "$GH_DISPLAY auth token" \
  "GH_DEBUG=api \"$GH\" auth token"

label "Expected:"
assert_out_nonempty "a token is printed"
assert_no_refresh   "no refresh happened"
next_step

# --- Step: token --no-refresh never refreshes ---------------------------------
label "Given" "a signed-in short-lived credential"
label "When" "auth token --no-refresh runs while the access token is expired or near expiry"
label "Then" "gh prints the stored token as-is without refreshing it"
cmdline "$GH_DISPLAY auth token --no-refresh"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "$GH_DISPLAY auth token --no-refresh" \
  "GH_AT_EXPIRES_IN=1 GH_DEBUG=api \"$GH\" auth token --no-refresh"

label "Expected:"
assert_out_nonempty "the stored token is printed as-is"
assert_no_refresh   "no refresh happened despite forced expiry"
next_step

# --- Step: an environment token is printed, never refreshed -------------------
label "Given" "a token supplied through the GH_TOKEN environment variable"
label "When" "auth token runs while the token in storage is expired or near expiry"
label "Then" "gh prints the environment token without refreshing it"
cmdline "GH_TOKEN=\$($GH_DISPLAY auth token --no-refresh) $GH_DISPLAY auth token"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "GH_TOKEN=\$($GH_DISPLAY auth token --no-refresh) $GH_DISPLAY auth token" \
  "GH_TOKEN=\$(\"$GH\" auth token --no-refresh) GH_AT_EXPIRES_IN=1 GH_DEBUG=api \"$GH\" auth token"

label "Expected:"
assert_out_nonempty "the environment token is printed"
assert_no_refresh   "no refresh happened (an environment token is never refreshed)"
next_step

# --- Step: token --secure-storage refreshes an expired token ------------------
label "Given" "a signed-in short-lived credential"
label "When" "auth token --secure-storage runs while the access token is expired or near expiry"
label "Then" "gh refreshes the token and prints the refreshed value"
cmdline "$GH_DISPLAY auth token --secure-storage"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "$GH_DISPLAY auth token --secure-storage" \
  "GH_AT_EXPIRES_IN=1 GH_DEBUG=api \"$GH\" auth token --secure-storage"

label "Expected:"
assert_out_nonempty "a token is printed"
assert_refresh      "a refresh happened"
next_step

# --- Step: token --secure-storage does not refresh a valid token --------------
label "Given" "the same short-lived credential with a still-valid access token"
label "When" "auth token --secure-storage runs"
label "Then" "gh prints the current token without refreshing it"
cmdline "$GH_DISPLAY auth token --secure-storage"
confirm_run
run_batch \
  "$GH_DISPLAY auth token --secure-storage" \
  "GH_DEBUG=api \"$GH\" auth token --secure-storage"

label "Expected:"
assert_out_nonempty "a token is printed"
assert_no_refresh   "no refresh happened"
next_step

# --- Step: token --secure-storage --no-refresh never refreshes ----------------
label "Given" "a signed-in short-lived credential"
label "When" "auth token --secure-storage --no-refresh runs while the access token is expired or near expiry"
if [ "$THEME" = "config" ]; then
  label "Then" "no refresh happens and gh reports no token, because secure storage is keyring-only and the token lives in config"
else
  label "Then" "no refresh happens and gh prints the token straight from the keyring"
fi
cmdline "$GH_DISPLAY auth token --secure-storage --no-refresh"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
confirm_run
run_batch \
  "$GH_DISPLAY auth token --secure-storage --no-refresh" \
  "GH_AT_EXPIRES_IN=1 GH_DEBUG=api \"$GH\" auth token --secure-storage --no-refresh"

label "Expected:"
assert_no_refresh "no refresh happened despite forced expiry"
if [ "$THEME" = "config" ]; then
  assert_out_empty    "no token is printed (keyring-only lookup, token is in config)"
  assert_err_contains "gh reports no token found" "no oauth token found"
else
  assert_out_nonempty "the token is printed from the keyring"
fi
next_step

# --- Step: --secure-storage ignores GH_TOKEN and resolves stored credential ---
label "Given" "a token supplied through the GH_TOKEN environment variable"
label "When" "auth token --secure-storage --hostname $HOST runs while the access token is expired or near expiry"
label "Then" "gh ignores GH_TOKEN, refreshes the stored credential, and prints that value"
cmdline "GH_TOKEN=\$($GH_DISPLAY auth token --no-refresh) $GH_DISPLAY auth token --secure-storage --hostname $HOST"
note   "  (Expiry is forced here via the GH_AT_EXPIRES_IN test hack.)"
note   "  (Dropping --hostname behaves identically, since the default host resolves to $HOST.)"
confirm_run
run_batch \
  "GH_TOKEN=\$($GH_DISPLAY auth token --no-refresh) $GH_DISPLAY auth token --secure-storage --hostname $HOST" \
  "GH_TOKEN=\$(\"$GH\" auth token --no-refresh) GH_AT_EXPIRES_IN=1 GH_DEBUG=api \"$GH\" auth token --secure-storage --hostname \"$HOST\""

label "Expected:"
assert_out_nonempty "the stored token is printed, not the environment token"
assert_refresh      "a refresh happened (the environment token is ignored for secure storage)"
next_step

# ---------------------------------------------------------------------------
# AUTH REFRESH
# ---------------------------------------------------------------------------

# --- Step: refresh --short-lived keeps the credential refreshable -------------
label "Given" "a signed-in short-lived credential"
label "When" "we run auth refresh --short-lived"
label "Then" "the flow completes and the credential stays short-lived and refreshable"
cmdline "$GH_DISPLAY auth refresh --hostname $HOST --short-lived --clipboard $STORE_FLAG"
note   "  (--clipboard copies the one-time code, so you can just paste it into the browser.)"
confirm_run
run_interactive \
  "$GH_DISPLAY auth refresh --hostname $HOST --short-lived --clipboard $STORE_FLAG" \
  "\"$GH\" auth refresh --hostname \"$HOST\" --short-lived --clipboard $STORE_FLAG"

label "Expected:"
manual "the refresh flow completes without error"
next_step

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

echo
printf "%sCleanup%s\n" "$BOLD" "$RESET"
note "You are still signed in with a short-lived refreshable credential for $HOST."
note "You can keep it to experiment further, or log out now to return to a clean state."
printf "\n%sLog out now?%s [y/N]: " "$BOLD" "$RESET"
read -r ans
case "$ans" in
  y|Y|yes|YES)
    echo
    run_interactive \
      "$GH_DISPLAY auth logout --hostname $HOST" \
      "\"$GH\" auth logout --hostname \"$HOST\""
    note "Logged out. State is clean."
    ;;
  *)
    note "Keeping the credential. Run 'gh auth logout --hostname $HOST' when you are done."
    ;;
esac

echo
divider
if [ "$FAILURES" -eq 0 ]; then
  printf "%sAll auto-checked expectations passed.%s\n" "$GREEN" "$RESET"
else
  printf "%s%d auto-checked expectation(s) failed.%s\n" "$RED" "$FAILURES" "$RESET"
fi
divider
exit 0
