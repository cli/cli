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
  local dir="$1"
  local proto="$2"

  local contract="$dir/$proto"

  protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative "$contract" --experimental_allow_proto3_optional
  echo "Generated protocol buffers for $contract"

  services=$(grep -Eo "service .+ {" <$contract | awk '{print $2 "Server"}')
  moq -out "$contract.mock.go" "$dir" "$services"
  echo "Generated mock protocols for $contract"
}

generate jupyter jupyter_server_host_service.v1.proto
generate codespace codespace_host_service.v1.proto
generate ssh ssh_server_host_service.v1.proto

echo 'Done!'
