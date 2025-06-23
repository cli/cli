#!/usr/bin/env bash

# Should point to /path/to/boulder, given that this script
# lives in the //grpc subdirectory of the boulder repo.
root_dir=$(dirname $(dirname $(readlink -f "$0")))

# Find each file below root_dir whose name matches *.proto and whose
# path does not include the "vendor" directory. Emit them null-delimited
# (to allow for spaces and newlines in filenames), and assign each to the
# local variable `file`.
find "${root_dir}" -name "*.proto" -not -path "*/vendor/*" -print0 | while read -d $'\0' file
do
  # Have to use absolute paths to make protoc happy.
  proto_file=$(realpath "${file}")
  proto_dir=$(dirname "${proto_file}")
  # -I "${proto_dir}" makes imports search the current directory first
  # -I "${root_dir}" ensures that our proto files can import each other
  # --go_out="${proto_dir}" writes the .pb.go file adjacent to the proto file
  # --go-grpc_out="${proto_dir}" does the same for _grpc.pb.go
  # --go_opt=paths=source_relative derives output filenames from input filenames
  # --go-grpc_opt=paths=source_relative does the same for _grpc.pb.go
  # --go-grpc_opt=use_generic_streams=true causes protoc-gen-go-grpc to use generics for its stream objects, rather than generating a new impl for each one
  protoc -I "${proto_dir}" -I "${root_dir}" --go_out="${proto_dir}" --go-grpc_out="${proto_dir}" --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative,use_generic_streams_experimental=true "${proto_file}"
done
