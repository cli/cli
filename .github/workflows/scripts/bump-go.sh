#!/usr/bin/env bash
#
# bump-go.sh -- Update go.mod `go` directive and toolchain to latest stable Go release.
#
# Usage:
#   ./bump-go.sh [--apply|-a] <path/to/go.mod>
#
# By default the script runs in *dry-run* mode: it creates a local branch,
# commits the version bump, shows the exact patch, **checks for an existing PR**
# with the same title, and exits. Nothing is pushed. The temporary branch is
# deleted automatically on exit, so your working tree stays clean. Pass
# --apply (or -a) to push the branch and open a new PR *only if one doesn't
# already exist*.
# -----------------------------------------------------------------------------
set -euo pipefail

usage() {
  echo "Usage: $0 [--apply|-a] <path/to/go.mod>" >&2
  exit 1
}

# ---- Argument parsing -------------------------------------------------------
APPLY=0
GO_MOD=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply|-a) APPLY=1 ;;
    -h|--help)  usage ;;
    *)          [[ -z "$GO_MOD" ]] && GO_MOD="$1" || usage ;;
  esac
  shift
done

[[ -z "$GO_MOD" ]] && usage
[[ -f "$GO_MOD" ]] || { echo "Error: '$GO_MOD' not found" >&2; exit 1; }

REPO="cli/cli"
MODULE_DIR=$(dirname "$GO_MOD")
GO_SUM="$MODULE_DIR/go.sum"

# ---- Discover latest stable Go release --------------------------------------
echo "Fetching latest stable Go version..."
LATEST_JSON=$(curl -fsSL https://go.dev/dl/?mode=json | jq -c '[.[] | select(.stable==true)][0]')
FULL_VERSION=$(jq -r '.version' <<< "$LATEST_JSON")        # e.g. go1.23.4
TOOLCHAIN_VERSION="${FULL_VERSION#go}"                     # e.g. 1.23.4
GO_DIRECTIVE_VERSION="$(cut -d. -f1-2 <<< "$TOOLCHAIN_VERSION").0"

echo "  → go directive : $GO_DIRECTIVE_VERSION"
echo "  → toolchain    : go$TOOLCHAIN_VERSION"

# ---- Read current go.mod state using go mod edit ----------------------------
GO_MOD_JSON=$(go mod edit -json "$GO_MOD")
CURRENT_GO_DIRECTIVE=$(jq -r '.Go // ""' <<< "$GO_MOD_JSON")
CURRENT_TOOLCHAIN=$(jq -r '.Toolchain // ""' <<< "$GO_MOD_JSON")

echo "  → current go    : $CURRENT_GO_DIRECTIVE"
echo "  → current tc    : ${CURRENT_TOOLCHAIN:-(none)}"

# ---- Prepare Git branch -----------------------------------------------------
BRANCH="bump-go-$TOOLCHAIN_VERSION"
BRANCH_CREATED=0

cleanup() {
  if [[ $BRANCH_CREATED -eq 1 ]]; then
    git checkout - >/dev/null 2>&1 || true
    git branch -D "$BRANCH" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "Creating branch $BRANCH"
git switch -c "$BRANCH" >/dev/null 2>&1
BRANCH_CREATED=1

# ---- Patch go.mod -----------------------------------------------------------
# Always set both directives and let `go mod tidy` normalize.
# When the go directive version matches the toolchain version, tidy will remove
# the toolchain line because it is redundant -- this is expected Go behavior.
go mod edit -go="$GO_DIRECTIVE_VERSION" -toolchain="go$TOOLCHAIN_VERSION" "$GO_MOD"
echo "  • set go directive → $GO_DIRECTIVE_VERSION"
echo "  • set toolchain    → go$TOOLCHAIN_VERSION"

# Let go mod tidy reconcile dependencies and normalize directives.
echo "  • running go mod tidy..."
pushd "$MODULE_DIR" > /dev/null
go mod tidy
popd > /dev/null

# ---- Check if anything actually changed -------------------------------------
if git diff --quiet -- "$GO_MOD" "$GO_SUM" 2>/dev/null; then
  echo "Already on latest Go version -- no changes needed."
  exit 0
fi

git add "$GO_MOD"
[[ -f "$GO_SUM" ]] && git add "$GO_SUM"

# ---- Commit -----------------------------------------------------------------
COMMIT_MSG="Bump Go to $TOOLCHAIN_VERSION"
git commit -m "$COMMIT_MSG" >/dev/null
COMMIT_HASH=$(git rev-parse --short HEAD)

PR_TITLE="$COMMIT_MSG"

# ---- Check for existing PR --------------------------------------------------
existing_pr=$(gh search prs --repo "$REPO" --state open --match title "$PR_TITLE" \
  --json title --jq "map(select(.title == \"$PR_TITLE\") | .title) | length > 0")

if [[ "$existing_pr" == "true" ]]; then
  echo "Found an existing open PR titled '$PR_TITLE'. Skipping push/PR creation."
  if [[ $APPLY -eq 0 ]]; then
    echo -e "\n=== DRY-RUN DIFF (commit $COMMIT_HASH):\n"
    git --no-pager show --color "$COMMIT_HASH"
  fi
  exit 0
fi

# ---- Dry-run handling -------------------------------------------------------
if [[ $APPLY -eq 0 ]]; then
  echo -e "\n=== DRY-RUN DIFF (commit $COMMIT_HASH):\n"
  git --no-pager show --color "$COMMIT_HASH"
  echo -e "\nIf --apply were provided, script would continue with:\n  git push -u origin $BRANCH\n  gh pr create --title \"$PR_TITLE\" --body <body>\n"
  exit 0
fi

# ---- Push & PR --------------------------------------------------------------
FINAL_GO_MOD_JSON=$(go mod edit -json "$GO_MOD")
FINAL_GO=$(jq -r '.Go // ""' <<< "$FINAL_GO_MOD_JSON")
FINAL_TC=$(jq -r '.Toolchain // ""' <<< "$FINAL_GO_MOD_JSON")

# Build PR body reflecting final state after tidy
if [[ -n "$FINAL_TC" ]]; then
  TC_LINE="* **toolchain:** \`$FINAL_TC\`"
else
  TC_LINE="* **toolchain:** _(none -- \`go mod tidy\` removed it because the go directive already implies go$TOOLCHAIN_VERSION)_"
fi

PR_BODY=$(cat <<EOF
This PR updates Go to the latest stable release.

* **go directive:** \`$FINAL_GO\`
$TC_LINE
EOF
)

git push -u origin "$BRANCH"

gh pr create --title "$PR_TITLE" --body "$PR_BODY" --fill

echo "Done!"
