package test_proto

//go:generate sh -c "cd ../.. && protoc -I grpc/test_proto/ -I . --go_out=grpc/test_proto --go-grpc_out=grpc/test_proto --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative grpc/test_proto/interceptors_test.proto"
