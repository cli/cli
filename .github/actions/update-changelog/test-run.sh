set -e

TOKEN="$(awk '/oauth_token/ {print $2}' ~/.config/gh/config.yml | head -1)"

env "GITHUB_REPOSITORY=github/gh-cli" "GITHUB_TOKEN=$TOKEN" node lib/index.js
