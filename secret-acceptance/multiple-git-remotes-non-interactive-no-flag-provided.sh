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

set +e

_secret_set_io=$($_gh secret set ACCEPTANCE_KEY --body ACCEPTANCE_VAL 2>&1)
if [[ $_secret_set_io != *"multiple remotes detected"* ]]; then
    echo 'FAIL: expected to secret set to fail with an error that multiple remotes are detected'
    echo "GOT: $_secret_set_io"
    exit 1 
fi

_secret_list_io=$($_gh secret list 2>&1)
if [[ $_secret_list_io != *"multiple remotes detected"* ]]; then
    echo 'FAIL: expected secret list to fail with an error that multiple remotes are detected'
    echo "GOT: $_secret_list_io"
    exit 1 
fi

_secret_delete_io=$($_gh secret delete ACCEPTANCE_KEY 2>&1)
if [[ $_secret_delete_io != *"multiple remotes detected"* ]]; then
    echo 'FAIL: expected secret delete to fail with an error that multiple remotes are detected'
    echo "GOT: $_secret_delete_io"
    exit 1 
fi

echo 'PASS'
