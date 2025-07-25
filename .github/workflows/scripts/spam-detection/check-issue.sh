#!/bin/bash

# Check if an issue is spam or not and output "PASS" (not spam) or "FAIL" (spam).
#
# Regardless of the spam detection result, the script always exits with a zero
# exit code, unless there's a runtime error.
#
# This script must be run from the root directory of the repository.

set -euo pipefail

# Determine absolute path to script directory based on where it is called from.
# This allows the script to be run from any directory.
SPAM_DIR="$(dirname "$(realpath "$0")")"

# Retrieve and prepare information about issue for detection
_issue_url="$1"
if [[ -z "$_issue_url" ]]; then
    echo "error: issue URL is empty" >&2
    exit 1
fi

_user_prompt_template='
<TITLE>
{{ .title }}
</TITLE>

<BODY>
{{ .body }}
</BODY>
'

_user_prompt="$(gh issue view --json title,body --template "$_user_prompt_template" "$_issue_url")"

# Generate dynamic prompts for inference
_system_prompt="$($SPAM_DIR/generate-sys-prompt.sh)"
_final_prompt="$(_system="$_system_prompt" _user="$_user_prompt" yq eval ".messages[0].content = strenv(_system) | .messages[1].content = strenv(_user)" "$SPAM_DIR/check-issue-prompts.yml")"

gh extension install github/gh-models 2>/dev/null

_result="$(gh models run --file <(echo "$_final_prompt") | cat)"

if [[ "$_result" != "PASS" && "$_result" != "FAIL" ]]; then
    echo "error: expected PASS or FAIL but got an unexpected result: $_result" >&2
    exit 1
fi

echo "$_result"
