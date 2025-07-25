#!/bin/bash

# Run the eval tests for the spam detection AI model.
#
# This script must be run from the root directory of the repository.

set -euo pipefail

# Determine absolute path to script directory based on where it is called from.
# This allows the script to be run from any directory.
SPAM_DIR="$(dirname "$(realpath "$0")")"

# Generate dynamic prompts for inference
_system_prompt="$($SPAM_DIR/generate-sys-prompt.sh)"
_final_prompt="$(_value="$_system_prompt" yq eval '.messages[0].content = strenv(_value)' $SPAM_DIR/eval-prompts.yml)"

# The following `gh models eval` command will fail after 20 requests due to rate limits.
# We are going to open up an issue in `github/gh-models` to address this.
#
# TODO: break up `eval-prompts.yml` file into smaller batches to avoid hitting the rate limit.
gh models eval <(echo "$_final_prompt")
