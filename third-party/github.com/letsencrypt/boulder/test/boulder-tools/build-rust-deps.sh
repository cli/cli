#!/bin/bash -ex

PROTO_ARCH=x86_64
if [ "${TARGETPLATFORM}" = linux/arm64 ]; then
  # For our Mac using friends on Apple Silicon and other 64bit ARM chips.
  PROTO_ARCH=aarch64
fi

cargo install typos-cli --target "${PROTO_ARCH}-unknown-linux-gnu"
