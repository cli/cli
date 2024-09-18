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

$_gh secret set ACCEPTANCE_KEY

$_gh secret list

$_gh secret delete ACCEPTANCE_KEY

echo 'PASS'
