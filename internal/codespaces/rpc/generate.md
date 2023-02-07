# Protocol Buffers for Codespaces

Instructions for generating and adding gRPC protocol buffers.

## Generate Protocol Buffers

1. [Download `protoc`](https://grpc.io/docs/protoc-installation/)
2. [Download protocol compiler plugins for Go](https://grpc.io/docs/languages/go/quickstart/)
3. Install moq: `go install github.com/matryer/moq@latest`
4. Run `./generate.sh` from the `internal/codespaces/rpc` directory

## Add New Protocol Buffers

1. Download a `.proto` contract from the service repo
2. Create a new directory and copy the `.proto` to it
3. Update `generate.sh` to include the include the new `.proto`
4. Follow the instructions to [Generate Protocol Buffers](#generate-protocol-buffers)
