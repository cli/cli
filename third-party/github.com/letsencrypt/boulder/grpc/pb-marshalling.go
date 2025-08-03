// Copyright 2016 ISRG.  All rights reserved
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package grpc

import (
	"fmt"
	"net/netip"
	"time"

	"github.com/go-jose/go-jose/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/probs"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	vapb "github.com/letsencrypt/boulder/va/proto"
)

var ErrMissingParameters = CodedError(codes.FailedPrecondition, "required RPC parameter was missing")
var ErrInvalidParameters = CodedError(codes.InvalidArgument, "RPC parameter was invalid")

// This file defines functions to translate between the protobuf types and the
// code types.

func ProblemDetailsToPB(prob *probs.ProblemDetails) (*corepb.ProblemDetails, error) {
	if prob == nil {
		// nil problemDetails is valid
		return nil, nil
	}
	return &corepb.ProblemDetails{
		ProblemType: string(prob.Type),
		Detail:      prob.Detail,
		HttpStatus:  int32(prob.HTTPStatus), //nolint: gosec // HTTP status codes are guaranteed to be small, no risk of overflow.
	}, nil
}

func PBToProblemDetails(in *corepb.ProblemDetails) (*probs.ProblemDetails, error) {
	if in == nil {
		// nil problemDetails is valid
		return nil, nil
	}
	if in.ProblemType == "" || in.Detail == "" {
		return nil, ErrMissingParameters
	}
	prob := &probs.ProblemDetails{
		Type:   probs.ProblemType(in.ProblemType),
		Detail: in.Detail,
	}
	if in.HttpStatus != 0 {
		prob.HTTPStatus = int(in.HttpStatus)
	}
	return prob, nil
}

func ChallengeToPB(challenge core.Challenge) (*corepb.Challenge, error) {
	prob, err := ProblemDetailsToPB(challenge.Error)
	if err != nil {
		return nil, err
	}
	recordAry := make([]*corepb.ValidationRecord, len(challenge.ValidationRecord))
	for i, v := range challenge.ValidationRecord {
		recordAry[i], err = ValidationRecordToPB(v)
		if err != nil {
			return nil, err
		}
	}

	var validated *timestamppb.Timestamp
	if challenge.Validated != nil {
		validated = timestamppb.New(challenge.Validated.UTC())
		if !validated.IsValid() {
			return nil, fmt.Errorf("error creating *timestamppb.Timestamp for *corepb.Challenge object")
		}
	}

	return &corepb.Challenge{
		Type:              string(challenge.Type),
		Status:            string(challenge.Status),
		Token:             challenge.Token,
		Error:             prob,
		Validationrecords: recordAry,
		Validated:         validated,
	}, nil
}

func PBToChallenge(in *corepb.Challenge) (challenge core.Challenge, err error) {
	if in == nil {
		return core.Challenge{}, ErrMissingParameters
	}
	if in.Type == "" || in.Status == "" || in.Token == "" {
		return core.Challenge{}, ErrMissingParameters
	}
	var recordAry []core.ValidationRecord
	if len(in.Validationrecords) > 0 {
		recordAry = make([]core.ValidationRecord, len(in.Validationrecords))
		for i, v := range in.Validationrecords {
			recordAry[i], err = PBToValidationRecord(v)
			if err != nil {
				return core.Challenge{}, err
			}
		}
	}
	prob, err := PBToProblemDetails(in.Error)
	if err != nil {
		return core.Challenge{}, err
	}
	var validated *time.Time
	if !core.IsAnyNilOrZero(in.Validated) {
		val := in.Validated.AsTime()
		validated = &val
	}
	ch := core.Challenge{
		Type:             core.AcmeChallenge(in.Type),
		Status:           core.AcmeStatus(in.Status),
		Token:            in.Token,
		Error:            prob,
		ValidationRecord: recordAry,
		Validated:        validated,
	}
	return ch, nil
}

func ValidationRecordToPB(record core.ValidationRecord) (*corepb.ValidationRecord, error) {
	addrs := make([][]byte, len(record.AddressesResolved))
	addrsTried := make([][]byte, len(record.AddressesTried))
	var err error
	for i, v := range record.AddressesResolved {
		addrs[i] = v.AsSlice()
	}
	for i, v := range record.AddressesTried {
		addrsTried[i] = v.AsSlice()
	}
	addrUsed, err := record.AddressUsed.MarshalText()
	if err != nil {
		return nil, err
	}
	return &corepb.ValidationRecord{
		Hostname:          record.Hostname,
		Port:              record.Port,
		AddressesResolved: addrs,
		AddressUsed:       addrUsed,
		Url:               record.URL,
		AddressesTried:    addrsTried,
		ResolverAddrs:     record.ResolverAddrs,
	}, nil
}

func PBToValidationRecord(in *corepb.ValidationRecord) (record core.ValidationRecord, err error) {
	if in == nil {
		return core.ValidationRecord{}, ErrMissingParameters
	}
	addrs := make([]netip.Addr, len(in.AddressesResolved))
	for i, v := range in.AddressesResolved {
		netIP, ok := netip.AddrFromSlice(v)
		if !ok {
			return core.ValidationRecord{}, ErrInvalidParameters
		}
		addrs[i] = netIP
	}
	addrsTried := make([]netip.Addr, len(in.AddressesTried))
	for i, v := range in.AddressesTried {
		netIP, ok := netip.AddrFromSlice(v)
		if !ok {
			return core.ValidationRecord{}, ErrInvalidParameters
		}
		addrsTried[i] = netIP
	}
	var addrUsed netip.Addr
	err = addrUsed.UnmarshalText(in.AddressUsed)
	if err != nil {
		return
	}
	return core.ValidationRecord{
		Hostname:          in.Hostname,
		Port:              in.Port,
		AddressesResolved: addrs,
		AddressUsed:       addrUsed,
		URL:               in.Url,
		AddressesTried:    addrsTried,
		ResolverAddrs:     in.ResolverAddrs,
	}, nil
}

func ValidationResultToPB(records []core.ValidationRecord, prob *probs.ProblemDetails, perspective, rir string) (*vapb.ValidationResult, error) {
	recordAry := make([]*corepb.ValidationRecord, len(records))
	var err error
	for i, v := range records {
		recordAry[i], err = ValidationRecordToPB(v)
		if err != nil {
			return nil, err
		}
	}
	marshalledProb, err := ProblemDetailsToPB(prob)
	if err != nil {
		return nil, err
	}
	return &vapb.ValidationResult{
		Records:     recordAry,
		Problem:     marshalledProb,
		Perspective: perspective,
		Rir:         rir,
	}, nil
}

func pbToValidationResult(in *vapb.ValidationResult) ([]core.ValidationRecord, *probs.ProblemDetails, error) {
	if in == nil {
		return nil, nil, ErrMissingParameters
	}
	recordAry := make([]core.ValidationRecord, len(in.Records))
	var err error
	for i, v := range in.Records {
		recordAry[i], err = PBToValidationRecord(v)
		if err != nil {
			return nil, nil, err
		}
	}
	prob, err := PBToProblemDetails(in.Problem)
	if err != nil {
		return nil, nil, err
	}
	return recordAry, prob, nil
}

func RegistrationToPB(reg core.Registration) (*corepb.Registration, error) {
	keyBytes, err := reg.Key.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var contacts []string
	if reg.Contact != nil {
		contacts = *reg.Contact
	}
	var createdAt *timestamppb.Timestamp
	if reg.CreatedAt != nil {
		createdAt = timestamppb.New(reg.CreatedAt.UTC())
		if !createdAt.IsValid() {
			return nil, fmt.Errorf("error creating *timestamppb.Timestamp for *corepb.Authorization object")
		}
	}

	return &corepb.Registration{
		Id:        reg.ID,
		Key:       keyBytes,
		Contact:   contacts,
		Agreement: reg.Agreement,
		CreatedAt: createdAt,
		Status:    string(reg.Status),
	}, nil
}

func PbToRegistration(pb *corepb.Registration) (core.Registration, error) {
	var key jose.JSONWebKey
	err := key.UnmarshalJSON(pb.Key)
	if err != nil {
		return core.Registration{}, err
	}
	var createdAt *time.Time
	if !core.IsAnyNilOrZero(pb.CreatedAt) {
		c := pb.CreatedAt.AsTime()
		createdAt = &c
	}
	var contacts *[]string
	if len(pb.Contact) != 0 {
		contacts = &pb.Contact
	}
	return core.Registration{
		ID:        pb.Id,
		Key:       &key,
		Contact:   contacts,
		Agreement: pb.Agreement,
		CreatedAt: createdAt,
		Status:    core.AcmeStatus(pb.Status),
	}, nil
}

func AuthzToPB(authz core.Authorization) (*corepb.Authorization, error) {
	challs := make([]*corepb.Challenge, len(authz.Challenges))
	for i, c := range authz.Challenges {
		pbChall, err := ChallengeToPB(c)
		if err != nil {
			return nil, err
		}
		challs[i] = pbChall
	}
	var expires *timestamppb.Timestamp
	if authz.Expires != nil {
		expires = timestamppb.New(authz.Expires.UTC())
		if !expires.IsValid() {
			return nil, fmt.Errorf("error creating *timestamppb.Timestamp for *corepb.Authorization object")
		}
	}

	return &corepb.Authorization{
		Id:                     authz.ID,
		Identifier:             authz.Identifier.ToProto(),
		RegistrationID:         authz.RegistrationID,
		Status:                 string(authz.Status),
		Expires:                expires,
		Challenges:             challs,
		CertificateProfileName: authz.CertificateProfileName,
	}, nil
}

func PBToAuthz(pb *corepb.Authorization) (core.Authorization, error) {
	challs := make([]core.Challenge, len(pb.Challenges))
	for i, c := range pb.Challenges {
		chall, err := PBToChallenge(c)
		if err != nil {
			return core.Authorization{}, err
		}
		challs[i] = chall
	}
	var expires *time.Time
	if !core.IsAnyNilOrZero(pb.Expires) {
		c := pb.Expires.AsTime()
		expires = &c
	}
	authz := core.Authorization{
		ID:                     pb.Id,
		Identifier:             identifier.FromProto(pb.Identifier),
		RegistrationID:         pb.RegistrationID,
		Status:                 core.AcmeStatus(pb.Status),
		Expires:                expires,
		Challenges:             challs,
		CertificateProfileName: pb.CertificateProfileName,
	}
	return authz, nil
}

// orderValid checks that a corepb.Order is valid. In addition to the checks
// from `newOrderValid` it ensures the order ID and the Created fields are not
// the zero value.
func orderValid(order *corepb.Order) bool {
	return order.Id != 0 && order.Created != nil && newOrderValid(order)
}

// newOrderValid checks that a corepb.Order is valid. It allows for a nil
// `order.Id` because the order has not been assigned an ID yet when it is being
// created initially. It allows `order.BeganProcessing` to be nil because
// `sa.NewOrder` explicitly sets it to the default value. It allows
// `order.Created` to be nil because the SA populates this. It also allows
// `order.CertificateSerial` to be nil such that it can be used in places where
// the order has not been finalized yet.
func newOrderValid(order *corepb.Order) bool {
	return !(order.RegistrationID == 0 || order.Expires == nil || len(order.Identifiers) == 0)
}

// PBToAuthzMap converts a protobuf map of identifiers mapped to protobuf
// authorizations to a golang map[string]*core.Authorization.
func PBToAuthzMap(pb *sapb.Authorizations) (map[identifier.ACMEIdentifier]*core.Authorization, error) {
	m := make(map[identifier.ACMEIdentifier]*core.Authorization, len(pb.Authzs))
	for _, v := range pb.Authzs {
		authz, err := PBToAuthz(v)
		if err != nil {
			return nil, err
		}
		m[authz.Identifier] = &authz
	}
	return m, nil
}
