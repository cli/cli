#!/bin/bash

set -e

if ! protoc --version; then
  echo 'ERROR: protoc is not on your PATH'
  exit 1
fi
if ! protoc-gen-go --version; then
  echo 'ERROR: protoc-gen-go is not on your PATH'
  exit 1
fi
if ! protoc-gen-go-grpc --version; then
  echo 'ERROR: protoc-gen-go-grpc is not on your PATH'
fi

function generate {
  local contract="$1"

  protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative "$contract"
  echo "Generated protocol buffers for $contract"
}

generate jupyter/JupyterServerHostService.v1.proto

echo 'Done!'
