package main

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/emptypb"

	blog "github.com/letsencrypt/boulder/log"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

type dryRunRAC struct {
	rapb.RegistrationAuthorityClient
	log blog.Logger
}

func (d dryRunRAC) AdministrativelyRevokeCertificate(_ context.Context, req *rapb.AdministrativelyRevokeCertificateRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	b, err := prototext.Marshal(req)
	if err != nil {
		return nil, err
	}
	d.log.Infof("dry-run: %#v", string(b))
	return &emptypb.Empty{}, nil
}

type dryRunSAC struct {
	sapb.StorageAuthorityClient
	log blog.Logger
}

func (d dryRunSAC) AddBlockedKey(_ context.Context, req *sapb.AddBlockedKeyRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	d.log.Infof("dry-run: Block SPKI hash %x by %s %s", req.KeyHash, req.Comment, req.Source)
	return &emptypb.Empty{}, nil
}
