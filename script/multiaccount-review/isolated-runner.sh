#!/bin/sh
set -eu
# On macOS go-keyring invokes /usr/bin/security directly, bypassing PATH stubs.
if [ ! -x /usr/bin/sandbox-exec ]; then
  echo 'Blocked: this offline replay requires macOS sandbox-exec to deny keychain and network access.' >&2
  exit 2
fi
exec /usr/bin/sandbox-exec -p '(version 1)(allow default)(deny network*)(deny process-exec (literal "/usr/bin/security"))(deny file-read* file-write* (regex #"/Library/Keychains(/|$)"))(deny mach-lookup (global-name "com.apple.securityd") (global-name "com.apple.secd"))' /bin/bash --noprofile --norc "${0%/*}/identity-scenario.sh" "$@"
