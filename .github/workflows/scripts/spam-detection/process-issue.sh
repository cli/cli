#!/bin/bash

# Performs spam detection on an issue and labels it if it's spam.
#
# Regardless of the spam detection result, the script always exits with a zero
# exit code, unless there's a runtime error.
#
# This script must be run from the root directory of the repository.

set -euo pipefail

_issue_url="$1"
if [[ -z "$_issue_url" ]]; then
    echo "error: issue URL is empty" >&2
    exit 1
fi

_suspected_spam_label="suspected-spam"
_check_issue_script=".github/workflows/scripts/spam-detection/check-issue.sh"

_result="$($_check_issue_script "$_issue_url")"

if [[ "$_result" == "PASS" ]]; then
    echo "detected as not-spam: $_issue_url"
    exit 0
fi

echo "detected as spam: $_issue_url"

gh issue edit --add-label "$_suspected_spam_label" "$_issue_url"

echo "issue labelled as suspected spam"
