#!/bin/bash
set -eu

binary="${1?}"
KEY_CHAIN="${2:-}"

cert="$(security find-identity -v $KEY_CHAIN | awk '/Developer ID Application/ {print $2}')"
codesign -s "$cert" "$binary"
codesign -v "$binary"
