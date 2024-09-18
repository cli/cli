#!/usr/bin/env sh

_cli=/Users/williammartin/workspace/cli/
_tmp=/tmp
_org=williammartin-test-org
_repo=gh-some-repo
_gh="$_cli/bin/gh"
_pwd=$(pwd)

set -e
# Uncomment this to echo expanded commands:
#   set -x

cleanup () {
    ARG=$?
    cd $_pwd
    rm -rf "$_tmp/$_repo"
    $_gh repo delete --yes $_repo
    $_gh repo delete --yes "$_org/$_repo"
    exit $ARG
} 
trap cleanup EXIT

cd $_tmp
_upstream=$($_gh repo create --private --add-readme $_repo)
$_gh repo fork --clone --org $_org $_upstream
cd $_repo

$_gh secret set ACCEPTANCE_KEY --body ACCEPTANCE_VAL --repo "$_org/$_repo"

if [[ -z $($_gh secret list --repo "$_org/$_repo" | grep ACCEPTANCE_KEY) ]]; then
    echo 'FAIL: expected secret list to contain ACCEPTANCE_KEY'
    exit 1
fi

$_gh secret delete ACCEPTANCE_KEY --repo "$_org/$_repo"

if [[ -n $($_gh secret list --repo "$_org/$_repo" | grep ACCEPTANCE_KEY) ]]; then
    echo 'FAIL: expected secret list not to contain ACCEPTANCE_KEY'
    exit 1
fi

echo 'PASS'
