set -e

TOKEN="$(awk '/oauth_token/ {print $2}' ~/.config/hub | head -1)"

env \
  "GITHUB_REPOSITORY=github/gh-cli" \
  "GITHUB_REF=refs/tags/v0.0.195" \
  "INPUT_TARGET-REPO=github/homebrew-gh" \
  "GITHUB_TOKEN=$TOKEN" \
  "UPLOAD_GITHUB_TOKEN=$TOKEN" \
  node lib/index.js