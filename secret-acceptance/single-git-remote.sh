#!/usr/bin/env sh

_cli=/Users/williammartin/workspace/cli/
_tmp=/tmp
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
    exit $ARG
} 
trap cleanup EXIT

cd $_tmp
$_gh repo create --private --add-readme $_repo
$_gh repo clone $_repo
cd $_repo

$_gh secret set ACCEPTANCE_KEY --body ACCEPTANCE_VALUE

if [[ -z $($_gh secret list | grep ACCEPTANCE_KEY) ]]; then
    echo 'FAIL: expected secret list to contain ACCEPTANCE_KEY'
    exit 1
fi

$_gh secret delete ACCEPTANCE_KEY

if [[ -n $($_gh secret list | grep ACCEPTANCE_KEY) ]]; then
    echo 'FAIL: expected secret list not to contain ACCEPTANCE_KEY'
    exit 1
fi

echo 'PASS'
