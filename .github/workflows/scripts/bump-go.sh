#!/usr/bin/env bash
#
# bump-go.sh — Update go.mod `go` directive and toolchain to latest stable Go release.
#
# Usage:
#   ./bump-go.sh [--apply|-a] <path/to/go.mod>
#
# By default the script runs in *dry‑run* mode: it creates a local branch,
# commits the version bump, shows the exact patch, **checks for an existing PR**
# with the same title, and exits. Nothing is pushed. The temporary branch is
# deleted automatically on exit, so your working tree stays clean. Pass
# --apply (or -a) to push the branch and open a new PR *only if one doesn’t
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

# ---- Discover latest stable Go release --------------------------------------
echo "Fetching latest stable Go version…"
LATEST_JSON=$(curl -fsSL https://go.dev/dl/?mode=json | jq -c '[.[] | select(.stable==true)][0]')
FULL_VERSION=$(jq -r '.version' <<< "$LATEST_JSON")        # e.g. go1.23.4
TOOLCHAIN_VERSION="${FULL_VERSION#go}"                     # e.g. 1.23.4
# `go mod tidy` will always add `.0` if there is no minor version
# so let's just ensure .0 is suffixed to the go directive.
GO_DIRECTIVE_VERSION="$(cut -d. -f1-2 <<< "$TOOLCHAIN_VERSION").0"

echo "  → go : $GO_DIRECTIVE_VERSION"
echo "  → toolchain : $TOOLCHAIN_VERSION"

# ---- Prepare Git branch ---------------------------------------------------
CURRENT_GO_DIRECTIVE=$(grep -E '^go ' "$GO_MOD" | cut -d ' ' -f2)
CURRENT_TOOLCHAIN_DIRECTIVE=$(grep -E '^toolchain ' "$GO_MOD" | cut -d ' ' -f2)

if [[ "$CURRENT_GO_DIRECTIVE" = "$GO_DIRECTIVE_VERSION" && \
      "$CURRENT_TOOLCHAIN_DIRECTIVE" = "go$TOOLCHAIN_VERSION" ]]; then
  echo "Already on latest Go version: $CURRENT_GO_DIRECTIVE (toolchain: $CURRENT_TOOLCHAIN_DIRECTIVE)"
  exit 0
fi

BRANCH="bump-go-$TOOLCHAIN_VERSION"
cleanup() {
  git checkout - >/dev/null 2>&1 || true
  git branch -D "$BRANCH" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "Creating branch $BRANCH"
git switch -c "$BRANCH" >/dev/null 2>&1

# ---- Patch go.mod -----------------------------------------------------------
if [[ "$CURRENT_GO_DIRECTIVE" != "$GO_DIRECTIVE_VERSION" ]]; then
  sed -Ei.bak "s/^go [0-9]+\.[0-9]+.*$/go $GO_DIRECTIVE_VERSION/" "$GO_MOD"
  echo "  • go directive $CURRENT_GO_DIRECTIVE → $GO_DIRECTIVE_VERSION"
fi

if [[ "$CURRENT_TOOLCHAIN_DIRECTIVE" != "go$TOOLCHAIN_VERSION" ]]; then
  sed -Ei.bak "s/^toolchain go[0-9]+\.[0-9]+\.[0-9]+.*$/toolchain go$TOOLCHAIN_VERSION/" "$GO_MOD"
  echo "  • toolchain $CURRENT_TOOLCHAIN_DIRECTIVE → go$TOOLCHAIN_VERSION"
fi

rm -f "$GO_MOD.bak"

git add "$GO_MOD"

# ---- Commit -----------------------------------------------------------------
COMMIT_MSG="Bump Go to $TOOLCHAIN_VERSION"
git commit -m "$COMMIT_MSG" >/dev/null
COMMIT_HASH=$(git rev-parse --short HEAD)

PR_TITLE="$COMMIT_MSG"

# ---- Check for existing PR --------------------------------------------------
existing_pr=$(gh search prs --repo cli/cli --match title "$PR_TITLE" --json title --jq "map(select(.title == \"$PR_TITLE\") | .title) | length > 0")

if [[ "$existing_pr" == "true" ]]; then
  echo "Found an existing open PR titled '$PR_TITLE'. Skipping push/PR creation."
  if [[ $APPLY -eq 0 ]]; then
    echo -e "\n=== DRY‑RUN DIFF (commit $COMMIT_HASH):\n"
    git --no-pager show --color "$COMMIT_HASH"
  fi
  exit 0
fi

# ---- Dry‑run handling -------------------------------------------------------
if [[ $APPLY -eq 0 ]]; then
  echo -e "\n=== DRY‑RUN DIFF (commit $COMMIT_HASH):\n"
  git --no-pager show --color "$COMMIT_HASH"
  echo -e "\nIf --apply were provided, script would continue with:\n  git push -u origin $BRANCH\n  gh pr create --title \"$PR_TITLE\" --body <body>\n"
  exit 0
fi

# ---- Push & PR --------------------------------------------------------------
PR_BODY=$(cat <<EOF
This PR updates Go to the latest stable release.

* **go directive:** \`$GO_DIRECTIVE_VERSION\`
* **toolchain:** \`$TOOLCHAIN_VERSION\`
EOF
)

git push -u origin "$BRANCH"

gh pr create --title "$PR_TITLE" --body "$PR_BODY" --fill

echo "Done!"
