package sa

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/db"
	berrors "github.com/letsencrypt/boulder/errors"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/identifier"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/revocation"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/unpause"
)

var (
	errIncompleteRequest = errors.New("incomplete gRPC request message")
)

// SQLStorageAuthority defines a Storage Authority.
//
// Note that although SQLStorageAuthority does have methods wrapping all of the
// read-only methods provided by the SQLStorageAuthorityRO, those wrapper
// implementations are in saro.go, next to the real implementations.
type SQLStorageAuthority struct {
	sapb.UnsafeStorageAuthorityServer

	*SQLStorageAuthorityRO

	dbMap *db.WrappedMap

	// rateLimitWriteErrors is a Counter for the number of times
	// a ratelimit update transaction failed during AddCertificate request
	// processing. We do not fail the overall AddCertificate call when ratelimit
	// transactions fail and so use this stat to maintain visibility into the rate
	// this occurs.
	rateLimitWriteErrors prometheus.Counter
}

var _ sapb.StorageAuthorityServer = (*SQLStorageAuthority)(nil)

// NewSQLStorageAuthorityWrapping provides persistence using a SQL backend for
// Boulder. It takes a read-only storage authority to wrap, which is useful if
// you are constructing both types of implementations and want to share
// read-only database connections between them.
func NewSQLStorageAuthorityWrapping(
	ssaro *SQLStorageAuthorityRO,
	dbMap *db.WrappedMap,
	stats prometheus.Registerer,
) (*SQLStorageAuthority, error) {
	rateLimitWriteErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "rate_limit_write_errors",
		Help: "number of failed ratelimit update transactions during AddCertificate",
	})
	stats.MustRegister(rateLimitWriteErrors)

	ssa := &SQLStorageAuthority{
		SQLStorageAuthorityRO: ssaro,
		dbMap:                 dbMap,
		rateLimitWriteErrors:  rateLimitWriteErrors,
	}

	return ssa, nil
}

// NewSQLStorageAuthority provides persistence using a SQL backend for
// Boulder. It constructs its own read-only storage authority to wrap.
func NewSQLStorageAuthority(
	dbMap *db.WrappedMap,
	dbReadOnlyMap *db.WrappedMap,
	dbIncidentsMap *db.WrappedMap,
	parallelismPerRPC int,
	lagFactor time.Duration,
	clk clock.Clock,
	logger blog.Logger,
	stats prometheus.Registerer,
) (*SQLStorageAuthority, error) {
	ssaro, err := NewSQLStorageAuthorityRO(
		dbReadOnlyMap, dbIncidentsMap, stats, parallelismPerRPC, lagFactor, clk, logger)
	if err != nil {
		return nil, err
	}

	return NewSQLStorageAuthorityWrapping(ssaro, dbMap, stats)
}

// NewRegistration stores a new Registration
func (ssa *SQLStorageAuthority) NewRegistration(ctx context.Context, req *corepb.Registration) (*corepb.Registration, error) {
	if len(req.Key) == 0 {
		return nil, errIncompleteRequest
	}

	reg, err := registrationPbToModel(req)
	if err != nil {
		return nil, err
	}

	reg.CreatedAt = ssa.clk.Now()

	err = ssa.dbMap.Insert(ctx, reg)
	if err != nil {
		if db.IsDuplicate(err) {
			// duplicate entry error can only happen when jwk_sha256 collides, indicate
			// to caller that the provided key is already in use
			return nil, berrors.DuplicateError("key is already in use for a different account")
		}
		return nil, err
	}
	return registrationModelToPb(reg)
}

// UpdateRegistrationContact makes no changes, and simply returns the account
// as it exists in the database.
//
// Deprecated: See https://github.com/letsencrypt/boulder/issues/8199 for removal.
func (ssa *SQLStorageAuthority) UpdateRegistrationContact(ctx context.Context, req *sapb.UpdateRegistrationContactRequest) (*corepb.Registration, error) {
	return ssa.GetRegistration(ctx, &sapb.RegistrationID{Id: req.RegistrationID})
}

// UpdateRegistrationKey stores an updated key in a Registration.
func (ssa *SQLStorageAuthority) UpdateRegistrationKey(ctx context.Context, req *sapb.UpdateRegistrationKeyRequest) (*corepb.Registration, error) {
	if core.IsAnyNilOrZero(req.RegistrationID, req.Jwk) {
		return nil, errIncompleteRequest
	}

	// Even though we don't need to convert from JSON to an in-memory JSONWebKey
	// for the sake of the `Key` field, we do need to do the conversion in order
	// to compute the SHA256 key digest.
	var jwk jose.JSONWebKey
	err := jwk.UnmarshalJSON(req.Jwk)
	if err != nil {
		return nil, fmt.Errorf("parsing JWK: %w", err)
	}
	sha, err := core.KeyDigestB64(jwk.Key)
	if err != nil {
		return nil, fmt.Errorf("computing key digest: %w", err)
	}

	result, overallError := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		result, err := tx.ExecContext(ctx,
			"UPDATE registrations SET jwk = ?, jwk_sha256 = ? WHERE id = ? LIMIT 1",
			req.Jwk,
			sha,
			req.RegistrationID,
		)
		if err != nil {
			if db.IsDuplicate(err) {
				// duplicate entry error can only happen when jwk_sha256 collides, indicate
				// to caller that the provided key is already in use
				return nil, berrors.DuplicateError("key is already in use for a different account")
			}
			return nil, err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil || rowsAffected != 1 {
			return nil, berrors.InternalServerError("no registration ID '%d' updated with new jwk", req.RegistrationID)
		}

		updatedRegistrationModel, err := selectRegistration(ctx, tx, "id", req.RegistrationID)
		if err != nil {
			if db.IsNoRows(err) {
				return nil, berrors.NotFoundError("registration with ID '%d' not found", req.RegistrationID)
			}
			return nil, err
		}
		updatedRegistration, err := registrationModelToPb(updatedRegistrationModel)
		if err != nil {
			return nil, err
		}

		return updatedRegistration, nil
	})
	if overallError != nil {
		return nil, overallError
	}

	return result.(*corepb.Registration), nil
}

// AddSerial writes a record of a serial number generation to the DB.
func (ssa *SQLStorageAuthority) AddSerial(ctx context.Context, req *sapb.AddSerialRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(req.Serial, req.RegID, req.Created, req.Expires) {
		return nil, errIncompleteRequest
	}
	err := ssa.dbMap.Insert(ctx, &recordedSerialModel{
		Serial:         req.Serial,
		RegistrationID: req.RegID,
		Created:        req.Created.AsTime(),
		Expires:        req.Expires.AsTime(),
	})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// SetCertificateStatusReady changes a serial's OCSP status from core.OCSPStatusNotReady to core.OCSPStatusGood.
// Called when precertificate issuance succeeds. returns an error if the serial doesn't have status core.OCSPStatusNotReady.
func (ssa *SQLStorageAuthority) SetCertificateStatusReady(ctx context.Context, req *sapb.Serial) (*emptypb.Empty, error) {
	res, err := ssa.dbMap.ExecContext(ctx,
		`UPDATE certificateStatus
		 SET status = ?
		 WHERE status = ? AND
		       serial = ?`,
		string(core.OCSPStatusGood),
		string(core.OCSPStatusNotReady),
		req.Serial,
	)
	if err != nil {
		return nil, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, errors.New("failed to set certificate status to ready")
	}

	return &emptypb.Empty{}, nil
}

// AddPrecertificate writes a record of a linting certificate to the database.
//
// Note: The name "AddPrecertificate" is a historical artifact, and this is now
// always called with a linting certificate. See #6807.
//
// Note: this is not idempotent: it does not protect against inserting the same
// certificate multiple times. Calling code needs to first insert the cert's
// serial into the Serials table to ensure uniqueness.
func (ssa *SQLStorageAuthority) AddPrecertificate(ctx context.Context, req *sapb.AddCertificateRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(req.Der, req.RegID, req.IssuerNameID, req.Issued) {
		return nil, errIncompleteRequest
	}
	parsed, err := x509.ParseCertificate(req.Der)
	if err != nil {
		return nil, err
	}
	serialHex := core.SerialToString(parsed.SerialNumber)

	preCertModel := &lintingCertModel{
		Serial:         serialHex,
		RegistrationID: req.RegID,
		DER:            req.Der,
		Issued:         req.Issued.AsTime(),
		Expires:        parsed.NotAfter,
	}

	_, overallError := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		// Select to see if precert exists
		var row struct {
			Count int64
		}
		err := tx.SelectOne(ctx, &row, "SELECT COUNT(*) as count FROM precertificates WHERE serial=?", serialHex)
		if err != nil {
			return nil, err
		}
		if row.Count > 0 {
			return nil, berrors.DuplicateError("cannot add a duplicate cert")
		}

		err = tx.Insert(ctx, preCertModel)
		if err != nil {
			return nil, err
		}

		status := core.OCSPStatusGood
		if req.OcspNotReady {
			status = core.OCSPStatusNotReady
		}
		cs := &certificateStatusModel{
			Serial:                serialHex,
			Status:                status,
			OCSPLastUpdated:       ssa.clk.Now(),
			RevokedDate:           time.Time{},
			RevokedReason:         0,
			LastExpirationNagSent: time.Time{},
			NotAfter:              parsed.NotAfter,
			IsExpired:             false,
			IssuerID:              req.IssuerNameID,
		}
		err = ssa.dbMap.Insert(ctx, cs)
		if err != nil {
			return nil, err
		}

		idents := identifier.FromCert(parsed)

		isRenewal, err := ssa.checkFQDNSetExists(
			ctx,
			tx.SelectOne,
			idents)
		if err != nil {
			return nil, err
		}

		err = addIssuedNames(ctx, tx, parsed, isRenewal)
		if err != nil {
			return nil, err
		}

		err = addKeyHash(ctx, tx, parsed)
		if err != nil {
			return nil, err
		}

		return nil, nil
	})
	if overallError != nil {
		return nil, overallError
	}

	return &emptypb.Empty{}, nil
}

// AddCertificate stores an issued certificate, returning an error if it is a
// duplicate or if any other failure occurs.
func (ssa *SQLStorageAuthority) AddCertificate(ctx context.Context, req *sapb.AddCertificateRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(req.Der, req.RegID, req.Issued) {
		return nil, errIncompleteRequest
	}
	parsedCertificate, err := x509.ParseCertificate(req.Der)
	if err != nil {
		return nil, err
	}
	digest := core.Fingerprint256(req.Der)
	serial := core.SerialToString(parsedCertificate.SerialNumber)

	cert := &core.Certificate{
		RegistrationID: req.RegID,
		Serial:         serial,
		Digest:         digest,
		DER:            req.Der,
		Issued:         req.Issued.AsTime(),
		Expires:        parsedCertificate.NotAfter,
	}

	_, overallError := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		// Select to see if cert exists
		var row struct {
			Count int64
		}
		err := tx.SelectOne(ctx, &row, "SELECT COUNT(*) as count FROM certificates WHERE serial=?", serial)
		if err != nil {
			return nil, err
		}
		if row.Count > 0 {
			return nil, berrors.DuplicateError("cannot add a duplicate cert")
		}

		// Save the final certificate
		err = tx.Insert(ctx, cert)
		if err != nil {
			return nil, err
		}

		return nil, err
	})
	if overallError != nil {
		return nil, overallError
	}

	// In a separate transaction, perform the work required to update the table
	// used for order reuse. Since the effect of failing the write is just a
	// missed opportunity to reuse an order, we choose to not fail the
	// AddCertificate operation if this update transaction fails.
	_, fqdnTransactionErr := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		// Update the FQDN sets now that there is a final certificate to ensure
		// reuse is determined correctly.
		err = addFQDNSet(
			ctx,
			tx,
			identifier.FromCert(parsedCertificate),
			core.SerialToString(parsedCertificate.SerialNumber),
			parsedCertificate.NotBefore,
			parsedCertificate.NotAfter,
		)
		if err != nil {
			return nil, err
		}

		return nil, nil
	})
	// If the FQDN sets transaction failed, increment a stat and log a warning
	// but don't return an error from AddCertificate.
	if fqdnTransactionErr != nil {
		ssa.rateLimitWriteErrors.Inc()
		ssa.log.AuditErrf("failed AddCertificate FQDN sets insert transaction: %v", fqdnTransactionErr)
	}

	return &emptypb.Empty{}, nil
}

// DeactivateRegistration deactivates a currently valid registration and removes its contact field
func (ssa *SQLStorageAuthority) DeactivateRegistration(ctx context.Context, req *sapb.RegistrationID) (*corepb.Registration, error) {
	if req == nil || req.Id == 0 {
		return nil, errIncompleteRequest
	}

	result, overallError := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (any, error) {
		result, err := tx.ExecContext(ctx,
			"UPDATE registrations SET status = ? WHERE status = ? AND id = ? LIMIT 1",
			string(core.StatusDeactivated),
			string(core.StatusValid),
			req.Id,
		)
		if err != nil {
			return nil, fmt.Errorf("deactivating account %d: %w", req.Id, err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("deactivating account %d: %w", req.Id, err)
		}
		if rowsAffected == 0 {
			return nil, berrors.NotFoundError("no active account with id %d", req.Id)
		} else if rowsAffected > 1 {
			return nil, berrors.InternalServerError("unexpectedly deactivated multiple accounts with id %d", req.Id)
		}

		updatedRegistrationModel, err := selectRegistration(ctx, tx, "id", req.Id)
		if err != nil {
			if db.IsNoRows(err) {
				return nil, berrors.NotFoundError("fetching account %d: no rows found", req.Id)
			}
			return nil, fmt.Errorf("fetching account %d: %w", req.Id, err)
		}

		updatedRegistration, err := registrationModelToPb(updatedRegistrationModel)
		if err != nil {
			return nil, err
		}

		return updatedRegistration, nil
	})
	if overallError != nil {
		return nil, overallError
	}

	res, ok := result.(*corepb.Registration)
	if !ok {
		return nil, fmt.Errorf("unexpected casting failure in DeactivateRegistration")
	}

	return res, nil
}

// DeactivateAuthorization2 deactivates a currently valid or pending authorization.
func (ssa *SQLStorageAuthority) DeactivateAuthorization2(ctx context.Context, req *sapb.AuthorizationID2) (*emptypb.Empty, error) {
	if req.Id == 0 {
		return nil, errIncompleteRequest
	}

	_, err := ssa.dbMap.ExecContext(ctx,
		`UPDATE authz2 SET status = :deactivated WHERE id = :id and status IN (:valid,:pending)`,
		map[string]interface{}{
			"deactivated": statusUint(core.StatusDeactivated),
			"id":          req.Id,
			"valid":       statusUint(core.StatusValid),
			"pending":     statusUint(core.StatusPending),
		},
	)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// NewOrderAndAuthzs adds the given authorizations to the database, adds their
// autogenerated IDs to the given order, and then adds the order to the db.
// This is done inside a single transaction to prevent situations where new
// authorizations are created, but then their corresponding order is never
// created, leading to "invisible" pending authorizations.
func (ssa *SQLStorageAuthority) NewOrderAndAuthzs(ctx context.Context, req *sapb.NewOrderAndAuthzsRequest) (*corepb.Order, error) {
	if req.NewOrder == nil {
		return nil, errIncompleteRequest
	}

	for _, authz := range req.NewAuthzs {
		if authz.RegistrationID != req.NewOrder.RegistrationID {
			// This is a belt-and-suspenders check. These were just created by the RA,
			// so their RegIDs should match. But if they don't, the consequences would
			// be very bad, so we do an extra check here.
			return nil, errors.New("new order and authzs must all be associated with same account")
		}
	}

	output, err := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		// First, insert all of the new authorizations and record their IDs.
		newAuthzIDs := make([]int64, 0, len(req.NewAuthzs))
		for _, authz := range req.NewAuthzs {
			am, err := newAuthzReqToModel(authz, req.NewOrder.CertificateProfileName)
			if err != nil {
				return nil, err
			}
			err = tx.Insert(ctx, am)
			if err != nil {
				return nil, err
			}
			newAuthzIDs = append(newAuthzIDs, am.ID)
		}

		// Second, insert the new order.
		created := ssa.clk.Now()
		om := orderModel{
			RegistrationID:         req.NewOrder.RegistrationID,
			Expires:                req.NewOrder.Expires.AsTime(),
			Created:                created,
			CertificateProfileName: &req.NewOrder.CertificateProfileName,
			Replaces:               &req.NewOrder.Replaces,
		}
		err := tx.Insert(ctx, &om)
		if err != nil {
			return nil, err
		}
		orderID := om.ID

		// Third, insert all of the orderToAuthz relations.
		// Have to combine the already-associated and newly-created authzs.
		allAuthzIds := append(req.NewOrder.V2Authorizations, newAuthzIDs...)
		inserter, err := db.NewMultiInserter("orderToAuthz2", []string{"orderID", "authzID"})
		if err != nil {
			return nil, err
		}
		for _, id := range allAuthzIds {
			err := inserter.Add([]interface{}{orderID, id})
			if err != nil {
				return nil, err
			}
		}
		err = inserter.Insert(ctx, tx)
		if err != nil {
			return nil, err
		}

		// Fourth, insert the FQDNSet entry for the order.
		err = addOrderFQDNSet(ctx, tx, identifier.FromProtoSlice(req.NewOrder.Identifiers), orderID, req.NewOrder.RegistrationID, req.NewOrder.Expires.AsTime())
		if err != nil {
			return nil, err
		}

		if req.NewOrder.ReplacesSerial != "" {
			// Update the replacementOrders table to indicate that this order
			// replaces the provided certificate serial.
			err := addReplacementOrder(ctx, tx, req.NewOrder.ReplacesSerial, orderID, req.NewOrder.Expires.AsTime())
			if err != nil {
				return nil, err
			}
		}

		// Get the partial Authorization objects for the order
		authzValidityInfo, err := getAuthorizationStatuses(ctx, tx, allAuthzIds)
		// If there was an error getting the authorizations, return it immediately
		if err != nil {
			return nil, err
		}

		// Finally, build the overall Order PB.
		res := &corepb.Order{
			// ID and Created were auto-populated on the order model when it was inserted.
			Id:      orderID,
			Created: timestamppb.New(created),
			// These are carried over from the original request unchanged.
			RegistrationID: req.NewOrder.RegistrationID,
			Expires:        req.NewOrder.Expires,
			Identifiers:    req.NewOrder.Identifiers,
			// This includes both reused and newly created authz IDs.
			V2Authorizations: allAuthzIds,
			// A new order is never processing because it can't be finalized yet.
			BeganProcessing: false,
			// An empty string is allowed. When the RA retrieves the order and
			// transmits it to the CA, the empty string will take the value of
			// DefaultCertProfileName from the //issuance package.
			CertificateProfileName: req.NewOrder.CertificateProfileName,
			Replaces:               req.NewOrder.Replaces,
		}

		// Calculate the order status before returning it. Since it may have reused
		// all valid authorizations the order may be "born" in a ready status.
		status, err := statusForOrder(res, authzValidityInfo, ssa.clk.Now())
		if err != nil {
			return nil, err
		}
		res.Status = status

		return res, nil
	})
	if err != nil {
		return nil, err
	}

	order, ok := output.(*corepb.Order)
	if !ok {
		return nil, fmt.Errorf("casting error in NewOrderAndAuthzs")
	}

	return order, nil
}

// SetOrderProcessing updates an order from pending status to processing
// status by updating the `beganProcessing` field of the corresponding
// Order table row in the DB.
func (ssa *SQLStorageAuthority) SetOrderProcessing(ctx context.Context, req *sapb.OrderRequest) (*emptypb.Empty, error) {
	if req.Id == 0 {
		return nil, errIncompleteRequest
	}
	_, overallError := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		result, err := tx.ExecContext(ctx, `
		UPDATE orders
		SET beganProcessing = ?
		WHERE id = ?
		AND beganProcessing = ?`,
			true,
			req.Id,
			false)
		if err != nil {
			return nil, berrors.InternalServerError("error updating order to beganProcessing status")
		}

		n, err := result.RowsAffected()
		if err != nil || n == 0 {
			return nil, berrors.OrderNotReadyError("Order was already processing. This may indicate your client finalized the same order multiple times, possibly due to a client bug.")
		}

		return nil, nil
	})
	if overallError != nil {
		return nil, overallError
	}
	return &emptypb.Empty{}, nil
}

// SetOrderError updates a provided Order's error field.
func (ssa *SQLStorageAuthority) SetOrderError(ctx context.Context, req *sapb.SetOrderErrorRequest) (*emptypb.Empty, error) {
	if req.Id == 0 || req.Error == nil {
		return nil, errIncompleteRequest
	}
	_, overallError := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		om, err := orderToModel(&corepb.Order{
			Id:    req.Id,
			Error: req.Error,
		})
		if err != nil {
			return nil, err
		}

		result, err := tx.ExecContext(ctx, `
		UPDATE orders
		SET error = ?
		WHERE id = ?`,
			om.Error,
			om.ID)
		if err != nil {
			return nil, berrors.InternalServerError("error updating order error field")
		}

		n, err := result.RowsAffected()
		if err != nil || n == 0 {
			return nil, berrors.InternalServerError("no order updated with new error field")
		}

		return nil, nil
	})
	if overallError != nil {
		return nil, overallError
	}
	return &emptypb.Empty{}, nil
}

// FinalizeOrder finalizes a provided *corepb.Order by persisting the
// CertificateSerial and a valid status to the database. No fields other than
// CertificateSerial and the order ID on the provided order are processed (e.g.
// this is not a generic update RPC).
func (ssa *SQLStorageAuthority) FinalizeOrder(ctx context.Context, req *sapb.FinalizeOrderRequest) (*emptypb.Empty, error) {
	if req.Id == 0 || req.CertificateSerial == "" {
		return nil, errIncompleteRequest
	}
	_, overallError := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		result, err := tx.ExecContext(ctx, `
		UPDATE orders
		SET certificateSerial = ?
		WHERE id = ? AND
		beganProcessing = true`,
			req.CertificateSerial,
			req.Id)
		if err != nil {
			return nil, berrors.InternalServerError("error updating order for finalization")
		}

		n, err := result.RowsAffected()
		if err != nil || n == 0 {
			return nil, berrors.InternalServerError("no order updated for finalization")
		}

		// Delete the orderFQDNSet row for the order now that it has been finalized.
		// We use this table for order reuse and should not reuse a finalized order.
		err = deleteOrderFQDNSet(ctx, tx, req.Id)
		if err != nil {
			return nil, err
		}

		err = setReplacementOrderFinalized(ctx, tx, req.Id)
		if err != nil {
			return nil, err
		}

		return nil, nil
	})
	if overallError != nil {
		return nil, overallError
	}
	return &emptypb.Empty{}, nil
}

// FinalizeAuthorization2 moves a pending authorization to either the valid or invalid status. If
// the authorization is being moved to invalid the validationError field must be set. If the
// authorization is being moved to valid the validationRecord and expires fields must be set.
func (ssa *SQLStorageAuthority) FinalizeAuthorization2(ctx context.Context, req *sapb.FinalizeAuthorizationRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(req.Status, req.Attempted, req.Id, req.Expires) {
		return nil, errIncompleteRequest
	}

	if req.Status != string(core.StatusValid) && req.Status != string(core.StatusInvalid) {
		return nil, berrors.InternalServerError("authorization must have status valid or invalid")
	}
	query := `UPDATE authz2 SET
		status = :status,
		attempted = :attempted,
		attemptedAt = :attemptedAt,
		validationRecord = :validationRecord,
		validationError = :validationError,
		expires = :expires
		WHERE id = :id AND status = :pending`
	var validationRecords []core.ValidationRecord
	for _, recordPB := range req.ValidationRecords {
		record, err := bgrpc.PBToValidationRecord(recordPB)
		if err != nil {
			return nil, err
		}
		if req.Attempted == string(core.ChallengeTypeHTTP01) {
			// Remove these fields because they can be rehydrated later
			// on from the URL field.
			record.Hostname = ""
			record.Port = ""
		}
		validationRecords = append(validationRecords, record)
	}
	vrJSON, err := json.Marshal(validationRecords)
	if err != nil {
		return nil, err
	}
	var veJSON []byte
	if req.ValidationError != nil {
		validationError, err := bgrpc.PBToProblemDetails(req.ValidationError)
		if err != nil {
			return nil, err
		}
		j, err := json.Marshal(validationError)
		if err != nil {
			return nil, err
		}
		veJSON = j
	}
	// Check to see if the AttemptedAt time is non zero and convert to
	// *time.Time if so. If it is zero, leave nil and don't convert. Keep the
	// database attemptedAt field Null instead of 1970-01-01 00:00:00.
	var attemptedTime *time.Time
	if !core.IsAnyNilOrZero(req.AttemptedAt) {
		val := req.AttemptedAt.AsTime()
		attemptedTime = &val
	}
	params := map[string]interface{}{
		"status":           statusToUint[core.AcmeStatus(req.Status)],
		"attempted":        challTypeToUint[req.Attempted],
		"attemptedAt":      attemptedTime,
		"validationRecord": vrJSON,
		"id":               req.Id,
		"pending":          statusUint(core.StatusPending),
		"expires":          req.Expires.AsTime(),
		// if req.ValidationError is nil veJSON should also be nil
		// which should result in a NULL field
		"validationError": veJSON,
	}

	res, err := ssa.dbMap.ExecContext(ctx, query, params)
	if err != nil {
		return nil, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, berrors.NotFoundError("no pending authorization with id %d", req.Id)
	} else if rows > 1 {
		return nil, berrors.InternalServerError("multiple rows updated for authorization id %d", req.Id)
	}
	return &emptypb.Empty{}, nil
}

// addRevokedCertificate is a helper used by both RevokeCertificate and
// UpdateRevokedCertificate. It inserts a new row into the revokedCertificates
// table based on the contents of the input request. The second argument must be
// a transaction object so that it is safe to conduct multiple queries with a
// consistent view of the database. It must only be called when the request
// specifies a non-zero ShardIdx.
func addRevokedCertificate(ctx context.Context, tx db.Executor, req *sapb.RevokeCertificateRequest, revokedDate time.Time) error {
	if req.ShardIdx == 0 {
		return errors.New("cannot add revoked certificate with shard index 0")
	}

	var serial struct {
		Expires time.Time
	}
	err := tx.SelectOne(
		ctx, &serial, `SELECT expires FROM serials WHERE serial = ?`, req.Serial)
	if err != nil {
		return fmt.Errorf("retrieving revoked certificate expiration: %w", err)
	}

	err = tx.Insert(ctx, &revokedCertModel{
		IssuerID:      req.IssuerID,
		Serial:        req.Serial,
		ShardIdx:      req.ShardIdx,
		RevokedDate:   revokedDate,
		RevokedReason: revocation.Reason(req.Reason),
		// Round the notAfter up to the next hour, to reduce index size while still
		// ensuring we correctly serve revocation info past the actual expiration.
		NotAfterHour: serial.Expires.Add(time.Hour).Truncate(time.Hour),
	})
	if err != nil {
		return fmt.Errorf("inserting revoked certificate row: %w", err)
	}

	return nil
}

// RevokeCertificate stores revocation information about a certificate. It will only store this
// information if the certificate is not already marked as revoked.
//
// If ShardIdx is non-zero, RevokeCertificate also writes an entry for this certificate to
// the revokedCertificates table, with the provided shard number.
func (ssa *SQLStorageAuthority) RevokeCertificate(ctx context.Context, req *sapb.RevokeCertificateRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(req.Serial, req.IssuerID, req.Date) {
		return nil, errIncompleteRequest
	}

	_, overallError := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		revokedDate := req.Date.AsTime()

		res, err := tx.ExecContext(ctx,
			`UPDATE certificateStatus SET
				status = ?,
				revokedReason = ?,
				revokedDate = ?,
				ocspLastUpdated = ?
			WHERE serial = ? AND status != ?`,
			string(core.OCSPStatusRevoked),
			revocation.Reason(req.Reason),
			revokedDate,
			revokedDate,
			req.Serial,
			string(core.OCSPStatusRevoked),
		)
		if err != nil {
			return nil, err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rows == 0 {
			return nil, berrors.AlreadyRevokedError("no certificate with serial %s and status other than %s", req.Serial, string(core.OCSPStatusRevoked))
		}

		if req.ShardIdx != 0 {
			err = addRevokedCertificate(ctx, tx, req, revokedDate)
			if err != nil {
				return nil, err
			}
		}

		return nil, nil
	})
	if overallError != nil {
		return nil, overallError
	}

	return &emptypb.Empty{}, nil
}

// UpdateRevokedCertificate stores new revocation information about an
// already-revoked certificate. It will only store this information if the
// cert is already revoked, if the new revocation reason is `KeyCompromise`,
// and if the revokedDate is identical to the current revokedDate.
func (ssa *SQLStorageAuthority) UpdateRevokedCertificate(ctx context.Context, req *sapb.RevokeCertificateRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(req.Serial, req.IssuerID, req.Date, req.Backdate) {
		return nil, errIncompleteRequest
	}
	if req.Reason != ocsp.KeyCompromise {
		return nil, fmt.Errorf("cannot update revocation for any reason other than keyCompromise (1); got: %d", req.Reason)
	}

	_, overallError := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		thisUpdate := req.Date.AsTime()
		revokedDate := req.Backdate.AsTime()

		res, err := tx.ExecContext(ctx,
			`UPDATE certificateStatus SET
					revokedReason = ?,
					ocspLastUpdated = ?
				WHERE serial = ? AND status = ? AND revokedReason != ? AND revokedDate = ?`,
			revocation.Reason(ocsp.KeyCompromise),
			thisUpdate,
			req.Serial,
			string(core.OCSPStatusRevoked),
			revocation.Reason(ocsp.KeyCompromise),
			revokedDate,
		)
		if err != nil {
			return nil, err
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rows == 0 {
			// InternalServerError because we expected this certificate status to exist,
			// to already be revoked for a different reason, and to have a matching date.
			return nil, berrors.InternalServerError("no certificate with serial %s and revoked reason other than keyCompromise", req.Serial)
		}

		// Only update the revokedCertificates table if the revocation request
		// specifies the CRL shard that this certificate belongs in. Our shards are
		// one-indexed, so a ShardIdx of zero means no value was set.
		if req.ShardIdx != 0 {
			var rcm revokedCertModel
			// Note: this query MUST be updated to enforce the same preconditions as
			// the "UPDATE certificateStatus SET revokedReason..." above if this
			// query ever becomes the first or only query in this transaction. We are
			// currently relying on the query above to exit early if the certificate
			// does not have an appropriate status and revocation reason.
			err = tx.SelectOne(
				ctx, &rcm, `SELECT * FROM revokedCertificates WHERE serial = ?`, req.Serial)
			if db.IsNoRows(err) {
				// TODO: Remove this fallback codepath once we know that all unexpired
				// certs marked as revoked in the certificateStatus table have
				// corresponding rows in the revokedCertificates table. That should be
				// 90+ days after the RA starts sending ShardIdx in its
				// RevokeCertificateRequest messages.
				err = addRevokedCertificate(ctx, tx, req, revokedDate)
				if err != nil {
					return nil, err
				}
				return nil, nil
			} else if err != nil {
				return nil, fmt.Errorf("retrieving revoked certificate row: %w", err)
			}

			rcm.RevokedReason = revocation.Reason(ocsp.KeyCompromise)
			_, err = tx.Update(ctx, &rcm)
			if err != nil {
				return nil, fmt.Errorf("updating revoked certificate row: %w", err)
			}
		}

		return nil, nil
	})
	if overallError != nil {
		return nil, overallError
	}

	return &emptypb.Empty{}, nil
}

// AddBlockedKey adds a key hash to the blockedKeys table
func (ssa *SQLStorageAuthority) AddBlockedKey(ctx context.Context, req *sapb.AddBlockedKeyRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(req.KeyHash, req.Added, req.Source) {
		return nil, errIncompleteRequest
	}
	sourceInt, ok := stringToSourceInt[req.Source]
	if !ok {
		return nil, errors.New("unknown source")
	}
	cols, qs := blockedKeysColumns, "?, ?, ?, ?"
	vals := []interface{}{
		req.KeyHash,
		req.Added.AsTime(),
		sourceInt,
		req.Comment,
	}
	if req.RevokedBy != 0 {
		cols += ", revokedBy"
		qs += ", ?"
		vals = append(vals, req.RevokedBy)
	}
	_, err := ssa.dbMap.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO blockedKeys (%s) VALUES (%s)", cols, qs),
		vals...,
	)
	if err != nil {
		if db.IsDuplicate(err) {
			// Ignore duplicate inserts so multiple certs with the same key can
			// be revoked.
			return &emptypb.Empty{}, nil
		}
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Health implements the grpc.checker interface.
func (ssa *SQLStorageAuthority) Health(ctx context.Context) error {
	err := ssa.dbMap.SelectOne(ctx, new(int), "SELECT 1")
	if err != nil {
		return err
	}

	err = ssa.SQLStorageAuthorityRO.Health(ctx)
	if err != nil {
		return err
	}
	return nil
}

// LeaseCRLShard marks a single crlShards row as leased until the given time.
// If the request names a specific shard, this function will return an error
// if that shard is already leased. Otherwise, this function will return the
// index of the oldest shard for the given issuer.
func (ssa *SQLStorageAuthority) LeaseCRLShard(ctx context.Context, req *sapb.LeaseCRLShardRequest) (*sapb.LeaseCRLShardResponse, error) {
	if core.IsAnyNilOrZero(req.Until, req.IssuerNameID) {
		return nil, errIncompleteRequest
	}
	if req.Until.AsTime().Before(ssa.clk.Now()) {
		return nil, fmt.Errorf("lease timestamp must be in the future, got %q", req.Until.AsTime())
	}

	if req.MinShardIdx == req.MaxShardIdx {
		return ssa.leaseSpecificCRLShard(ctx, req)
	}

	return ssa.leaseOldestCRLShard(ctx, req)
}

// leaseOldestCRLShard finds the oldest unleased crl shard for the given issuer
// and then leases it. Shards within the requested range which have never been
// leased or are previously-unknown indices are considered older than any other
// shard. It returns an error if all shards for the issuer are already leased.
func (ssa *SQLStorageAuthority) leaseOldestCRLShard(ctx context.Context, req *sapb.LeaseCRLShardRequest) (*sapb.LeaseCRLShardResponse, error) {
	shardIdx, err := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		var shards []*crlShardModel
		_, err := tx.Select(
			ctx,
			&shards,
			`SELECT id, issuerID, idx, thisUpdate, nextUpdate, leasedUntil
				FROM crlShards
				WHERE issuerID = ?
				AND idx BETWEEN ? AND ?`,
			req.IssuerNameID, req.MinShardIdx, req.MaxShardIdx,
		)
		if err != nil {
			return -1, fmt.Errorf("selecting candidate shards: %w", err)
		}

		// Determine which shard index we want to lease.
		var shardIdx int
		if len(shards) < (int(req.MaxShardIdx + 1 - req.MinShardIdx)) {
			// Some expected shards are missing (i.e. never-before-produced), so we
			// pick one at random.
			missing := make(map[int]struct{}, req.MaxShardIdx+1-req.MinShardIdx)
			for i := req.MinShardIdx; i <= req.MaxShardIdx; i++ {
				missing[int(i)] = struct{}{}
			}
			for _, shard := range shards {
				delete(missing, shard.Idx)
			}
			for idx := range missing {
				// Go map iteration is guaranteed to be in randomized key order.
				shardIdx = idx
				break
			}

			_, err = tx.ExecContext(ctx,
				`INSERT INTO crlShards (issuerID, idx, leasedUntil)
					VALUES (?, ?, ?)`,
				req.IssuerNameID,
				shardIdx,
				req.Until.AsTime(),
			)
			if err != nil {
				return -1, fmt.Errorf("inserting selected shard: %w", err)
			}
		} else {
			// We got all the shards we expect, so we pick the oldest unleased shard.
			var oldest *crlShardModel
			for _, shard := range shards {
				if shard.LeasedUntil.After(ssa.clk.Now()) {
					continue
				}
				if oldest == nil ||
					(oldest.ThisUpdate != nil && shard.ThisUpdate == nil) ||
					(oldest.ThisUpdate != nil && shard.ThisUpdate.Before(*oldest.ThisUpdate)) {
					oldest = shard
				}
			}
			if oldest == nil {
				return -1, fmt.Errorf("issuer %d has no unleased shards in range %d-%d", req.IssuerNameID, req.MinShardIdx, req.MaxShardIdx)
			}
			shardIdx = oldest.Idx

			res, err := tx.ExecContext(ctx,
				`UPDATE crlShards
					SET leasedUntil = ?
					WHERE issuerID = ?
					AND idx = ?
					AND leasedUntil = ?
					LIMIT 1`,
				req.Until.AsTime(),
				req.IssuerNameID,
				shardIdx,
				oldest.LeasedUntil,
			)
			if err != nil {
				return -1, fmt.Errorf("updating selected shard: %w", err)
			}
			rowsAffected, err := res.RowsAffected()
			if err != nil {
				return -1, fmt.Errorf("confirming update of selected shard: %w", err)
			}
			if rowsAffected != 1 {
				return -1, errors.New("failed to lease shard")
			}
		}

		return shardIdx, err
	})
	if err != nil {
		return nil, fmt.Errorf("leasing oldest shard: %w", err)
	}

	return &sapb.LeaseCRLShardResponse{
		IssuerNameID: req.IssuerNameID,
		ShardIdx:     int64(shardIdx.(int)),
	}, nil
}

// leaseSpecificCRLShard attempts to lease the crl shard for the given issuer
// and shard index. It returns an error if the specified shard is already
// leased.
func (ssa *SQLStorageAuthority) leaseSpecificCRLShard(ctx context.Context, req *sapb.LeaseCRLShardRequest) (*sapb.LeaseCRLShardResponse, error) {
	if req.MinShardIdx != req.MaxShardIdx {
		return nil, fmt.Errorf("request must identify a single shard index: %d != %d", req.MinShardIdx, req.MaxShardIdx)
	}

	_, err := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		needToInsert := false
		var shardModel crlShardModel
		err := tx.SelectOne(ctx,
			&shardModel,
			`SELECT leasedUntil
			  FROM crlShards
				WHERE issuerID = ?
				AND idx = ?
				LIMIT 1`,
			req.IssuerNameID,
			req.MinShardIdx,
		)
		if db.IsNoRows(err) {
			needToInsert = true
		} else if err != nil {
			return nil, fmt.Errorf("selecting requested shard: %w", err)
		} else if shardModel.LeasedUntil.After(ssa.clk.Now()) {
			return nil, fmt.Errorf("shard %d for issuer %d already leased", req.MinShardIdx, req.IssuerNameID)
		}

		if needToInsert {
			_, err = tx.ExecContext(ctx,
				`INSERT INTO crlShards (issuerID, idx, leasedUntil)
					VALUES (?, ?, ?)`,
				req.IssuerNameID,
				req.MinShardIdx,
				req.Until.AsTime(),
			)
			if err != nil {
				return nil, fmt.Errorf("inserting selected shard: %w", err)
			}
		} else {
			res, err := tx.ExecContext(ctx,
				`UPDATE crlShards
					SET leasedUntil = ?
					WHERE issuerID = ?
					AND idx = ?
					AND leasedUntil = ?
					LIMIT 1`,
				req.Until.AsTime(),
				req.IssuerNameID,
				req.MinShardIdx,
				shardModel.LeasedUntil,
			)
			if err != nil {
				return nil, fmt.Errorf("updating selected shard: %w", err)
			}
			rowsAffected, err := res.RowsAffected()
			if err != nil {
				return -1, fmt.Errorf("confirming update of selected shard: %w", err)
			}
			if rowsAffected != 1 {
				return -1, errors.New("failed to lease shard")
			}
		}

		return nil, nil
	})
	if err != nil {
		return nil, fmt.Errorf("leasing specific shard: %w", err)
	}

	return &sapb.LeaseCRLShardResponse{
		IssuerNameID: req.IssuerNameID,
		ShardIdx:     req.MinShardIdx,
	}, nil
}

// UpdateCRLShard updates the thisUpdate and nextUpdate timestamps of a CRL
// shard. It rejects the update if it would cause the thisUpdate timestamp to
// move backwards, but if thisUpdate would stay the same (for instance, multiple
// CRL generations within a single second), it will succeed.
//
// It does *not* reject the update if the shard is no longer
// leased: although this would be unexpected (because the lease timestamp should
// be the same as the crl-updater's context expiration), it's not inherently a
// sign of an update that should be skipped. It does reject the update if the
// identified CRL shard does not exist in the database (it should exist, as
// rows are created if necessary when leased). It also sets the leasedUntil time
// to be equal to thisUpdate, to indicate that the shard is no longer leased.
func (ssa *SQLStorageAuthority) UpdateCRLShard(ctx context.Context, req *sapb.UpdateCRLShardRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(req.IssuerNameID, req.ThisUpdate) {
		return nil, errIncompleteRequest
	}

	// Only set the nextUpdate if it's actually present in the request message.
	var nextUpdate *time.Time
	if req.NextUpdate != nil {
		nut := req.NextUpdate.AsTime()
		nextUpdate = &nut
	}

	_, err := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		res, err := tx.ExecContext(ctx,
			`UPDATE crlShards
				SET thisUpdate = ?, nextUpdate = ?, leasedUntil = ?
				WHERE issuerID = ?
				AND idx = ?
				AND (thisUpdate is NULL OR thisUpdate <= ?)
				LIMIT 1`,
			req.ThisUpdate.AsTime(),
			nextUpdate,
			req.ThisUpdate.AsTime(),
			req.IssuerNameID,
			req.ShardIdx,
			req.ThisUpdate.AsTime(),
		)
		if err != nil {
			return nil, err
		}

		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rowsAffected == 0 {
			return nil, fmt.Errorf("unable to update shard %d for issuer %d; possibly because shard exists", req.ShardIdx, req.IssuerNameID)
		}
		if rowsAffected != 1 {
			return nil, errors.New("update affected unexpected number of rows")
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// PauseIdentifiers pauses a set of identifiers for the provided account. If an
// identifier is currently paused, this is a no-op. If an identifier was
// previously paused and unpaused, it will be repaused unless it was unpaused
// less than two weeks ago. The response will indicate how many identifiers were
// paused and how many were repaused. All work is accomplished in a transaction
// to limit possible race conditions.
func (ssa *SQLStorageAuthority) PauseIdentifiers(ctx context.Context, req *sapb.PauseRequest) (*sapb.PauseIdentifiersResponse, error) {
	if core.IsAnyNilOrZero(req.RegistrationID, req.Identifiers) {
		return nil, errIncompleteRequest
	}

	// Marshal the identifier now that we've crossed the RPC boundary.
	idents, err := newIdentifierModelsFromPB(req.Identifiers)
	if err != nil {
		return nil, err
	}

	response := &sapb.PauseIdentifiersResponse{}
	_, err = db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (interface{}, error) {
		for _, ident := range idents {
			pauseError := func(op string, err error) error {
				return fmt.Errorf("while %s identifier %s for registration ID %d: %w",
					op, ident.Value, req.RegistrationID, err,
				)
			}

			var entry pausedModel
			err := tx.SelectOne(ctx, &entry, `
			SELECT pausedAt, unpausedAt
			FROM paused
			WHERE
				registrationID = ? AND
				identifierType = ? AND
				identifierValue = ?`,
				req.RegistrationID,
				ident.Type,
				ident.Value,
			)

			switch {
			case err != nil && !errors.Is(err, sql.ErrNoRows):
				// Error querying the database.
				return nil, pauseError("querying pause status for", err)

			case err != nil && errors.Is(err, sql.ErrNoRows):
				// Not currently or previously paused, insert a new pause record.
				err = tx.Insert(ctx, &pausedModel{
					RegistrationID: req.RegistrationID,
					PausedAt:       ssa.clk.Now().Truncate(time.Second),
					identifierModel: identifierModel{
						Type:  ident.Type,
						Value: ident.Value,
					},
				})
				if err != nil && !db.IsDuplicate(err) {
					return nil, pauseError("pausing", err)
				}

				// Identifier successfully paused.
				response.Paused++
				continue

			case entry.UnpausedAt == nil || entry.PausedAt.After(*entry.UnpausedAt):
				// Identifier is already paused.
				continue

			case entry.UnpausedAt.After(ssa.clk.Now().Add(-14 * 24 * time.Hour)):
				// Previously unpaused less than two weeks ago, skip this identifier.
				continue

			case entry.UnpausedAt.After(entry.PausedAt):
				// Previously paused (and unpaused), repause the identifier.
				_, err := tx.ExecContext(ctx, `
				UPDATE paused
				SET pausedAt = ?,
				    unpausedAt = NULL
				WHERE
					registrationID = ? AND
					identifierType = ? AND
					identifierValue = ? AND
					unpausedAt IS NOT NULL`,
					ssa.clk.Now().Truncate(time.Second),
					req.RegistrationID,
					ident.Type,
					ident.Value,
				)
				if err != nil {
					return nil, pauseError("repausing", err)
				}

				// Identifier successfully repaused.
				response.Repaused++
				continue

			default:
				// This indicates a database state which should never occur.
				return nil, fmt.Errorf("impossible database state encountered while pausing identifier %s",
					ident.Value,
				)
			}
		}
		return nil, nil
	})
	if err != nil {
		// Error occurred during transaction.
		return nil, err
	}
	return response, nil
}

// UnpauseAccount uses up to 5 iterations of UPDATE queries each with a LIMIT of
// 10,000 to unpause up to 50,000 identifiers and returns a count of identifiers
// unpaused. If the returned count is 50,000 there may be more paused identifiers.
func (ssa *SQLStorageAuthority) UnpauseAccount(ctx context.Context, req *sapb.RegistrationID) (*sapb.Count, error) {
	if core.IsAnyNilOrZero(req.Id) {
		return nil, errIncompleteRequest
	}

	total := &sapb.Count{}

	for i := 0; i < unpause.MaxBatches; i++ {
		result, err := ssa.dbMap.ExecContext(ctx, `
			UPDATE paused
			SET unpausedAt = ?
			WHERE
				registrationID = ? AND
				unpausedAt IS NULL
			LIMIT ?`,
			ssa.clk.Now(),
			req.Id,
			unpause.BatchSize,
		)
		if err != nil {
			return nil, err
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}

		total.Count += rowsAffected
		if rowsAffected < unpause.BatchSize {
			// Fewer than batchSize rows were updated, so we're done.
			break
		}
	}

	return total, nil
}

// AddRateLimitOverride adds a rate limit override to the database. If the
// override already exists, it will be updated. If the override does not exist,
// it will be inserted and enabled. If the override exists but has been
// disabled, it will be updated but not be re-enabled. The status of the
// override is returned in Enabled field of the response. To re-enable an
// override, use the EnableRateLimitOverride method.
func (ssa *SQLStorageAuthority) AddRateLimitOverride(ctx context.Context, req *sapb.AddRateLimitOverrideRequest) (*sapb.AddRateLimitOverrideResponse, error) {
	if core.IsAnyNilOrZero(req, req.Override, req.Override.LimitEnum, req.Override.BucketKey, req.Override.Count, req.Override.Burst, req.Override.Period, req.Override.Comment) {
		return nil, errIncompleteRequest
	}

	var inserted bool
	var enabled bool
	now := ssa.clk.Now()

	_, err := db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (any, error) {
		var alreadyEnabled bool
		err := tx.SelectOne(ctx, &alreadyEnabled, `
			SELECT enabled
			  FROM overrides
			 WHERE limitEnum = ? AND
			       bucketKey = ?`,
			req.Override.LimitEnum,
			req.Override.BucketKey,
		)

		switch {
		case err != nil && !db.IsNoRows(err):
			// Error querying the database.
			return nil, fmt.Errorf("querying override for rate limit %d and bucket key %s: %w",
				req.Override.LimitEnum,
				req.Override.BucketKey,
				err,
			)

		case db.IsNoRows(err):
			// Insert a new overrides row.
			new := overrideModelForPB(req.Override, now, true)
			err = tx.Insert(ctx, &new)
			if err != nil {
				return nil, fmt.Errorf("inserting override for rate limit %d and bucket key %s: %w",
					req.Override.LimitEnum,
					req.Override.BucketKey,
					err,
				)
			}
			inserted = true
			enabled = true

		default:
			// Update the existing overrides row.
			updated := overrideModelForPB(req.Override, now, alreadyEnabled)
			_, err = tx.Update(ctx, &updated)
			if err != nil {
				return nil, fmt.Errorf("updating override for rate limit %d and bucket key %s override: %w",
					req.Override.LimitEnum,
					req.Override.BucketKey,
					err,
				)
			}
			inserted = false
			enabled = alreadyEnabled
		}
		return nil, nil
	})
	if err != nil {
		// Error occurred during transaction.
		return nil, err
	}
	return &sapb.AddRateLimitOverrideResponse{Inserted: inserted, Enabled: enabled}, nil
}

// setRateLimitOverride sets the enabled field of a rate limit override to the
// provided value and updates the updatedAt column. If the override does not
// exist, a NotFoundError is returned. If the override exists but is already in
// the requested state, this is a no-op.
func (ssa *SQLStorageAuthority) setRateLimitOverride(ctx context.Context, limitEnum int64, bucketKey string, enabled bool) (*emptypb.Empty, error) {
	overrideColumnsList, err := ssa.dbMap.ColumnsForModel(overrideModel{})
	if err != nil {
		// This should never happen, the model is registered at init time.
		return nil, fmt.Errorf("getting columns for override model: %w", err)
	}
	overrideColumns := strings.Join(overrideColumnsList, ", ")
	_, err = db.WithTransaction(ctx, ssa.dbMap, func(tx db.Executor) (any, error) {
		var existing overrideModel
		err := tx.SelectOne(ctx, &existing,
			// Use SELECT FOR UPDATE to both verify the row exists and lock it
			// for the duration of the transaction.
			`SELECT `+overrideColumns+` FROM overrides
			 WHERE limitEnum = ? AND
			       bucketKey = ?
			 FOR UPDATE`,
			limitEnum,
			bucketKey,
		)
		if err != nil {
			if db.IsNoRows(err) {
				return nil, berrors.NotFoundError(
					"no rate limit override found for limit %d and bucket key %s",
					limitEnum,
					bucketKey,
				)
			}
			return nil, fmt.Errorf("querying status of override for rate limit %d and bucket key %s: %w",
				limitEnum,
				bucketKey,
				err,
			)
		}

		if existing.Enabled == enabled {
			// No-op
			return nil, nil
		}

		// Update the existing overrides row.
		updated := existing
		updated.Enabled = enabled
		updated.UpdatedAt = ssa.clk.Now()

		_, err = tx.Update(ctx, &updated)
		if err != nil {
			return nil, fmt.Errorf("updating status of override for rate limit %d and bucket key %s to %t: %w",
				limitEnum,
				bucketKey,
				enabled,
				err,
			)
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DisableRateLimitOverride disables a rate limit override. If the override does
// not exist, a NotFoundError is returned. If the override exists but is already
// disabled, this is a no-op.
func (ssa *SQLStorageAuthority) DisableRateLimitOverride(ctx context.Context, req *sapb.DisableRateLimitOverrideRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(req, req.LimitEnum, req.BucketKey) {
		return nil, errIncompleteRequest
	}
	return ssa.setRateLimitOverride(ctx, req.LimitEnum, req.BucketKey, false)
}

// EnableRateLimitOverride enables a rate limit override. If the override does
// not exist, a NotFoundError is returned. If the override exists but is already
// enabled, this is a no-op.
func (ssa *SQLStorageAuthority) EnableRateLimitOverride(ctx context.Context, req *sapb.EnableRateLimitOverrideRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(req, req.LimitEnum, req.BucketKey) {
		return nil, errIncompleteRequest
	}
	return ssa.setRateLimitOverride(ctx, req.LimitEnum, req.BucketKey, true)
}
