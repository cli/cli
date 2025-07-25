package sa

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/db"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/identifier"
	blog "github.com/letsencrypt/boulder/log"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

var (
	validIncidentTableRegexp = regexp.MustCompile(`^incident_[0-9a-zA-Z_]{1,100}$`)
)

// SQLStorageAuthorityRO defines a read-only subset of a Storage Authority
type SQLStorageAuthorityRO struct {
	sapb.UnsafeStorageAuthorityReadOnlyServer

	dbReadOnlyMap  *db.WrappedMap
	dbIncidentsMap *db.WrappedMap

	// For RPCs that generate multiple, parallelizable SQL queries, this is the
	// max parallelism they will use (to avoid consuming too many MariaDB
	// threads).
	parallelismPerRPC int

	// lagFactor is the amount of time we're willing to delay before retrying a
	// request that may have failed due to replication lag. For example, a user
	// might create a new account and then immediately create a new order, but
	// validating that new-order request requires reading their account info from
	// a read-only database replica... which may not have their brand new data
	// yet. This value should be less than, but about the same order of magnitude
	// as, the observed database replication lag.
	lagFactor time.Duration

	clk clock.Clock
	log blog.Logger

	// lagFactorCounter is a Prometheus counter that tracks the number of times
	// we've retried a query inside of GetRegistration, GetOrder, and
	// GetAuthorization2 due to replication lag. It is labeled by method name
	// and whether data from the retry attempt was found, notfound, or some
	// other error was encountered.
	lagFactorCounter *prometheus.CounterVec
}

var _ sapb.StorageAuthorityReadOnlyServer = (*SQLStorageAuthorityRO)(nil)

// NewSQLStorageAuthorityRO provides persistence using a SQL backend for
// Boulder. It will modify the given borp.DbMap by adding relevant tables.
func NewSQLStorageAuthorityRO(
	dbReadOnlyMap *db.WrappedMap,
	dbIncidentsMap *db.WrappedMap,
	stats prometheus.Registerer,
	parallelismPerRPC int,
	lagFactor time.Duration,
	clk clock.Clock,
	logger blog.Logger,
) (*SQLStorageAuthorityRO, error) {
	lagFactorCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "sa_lag_factor",
		Help: "A counter of SA lagFactor checks labelled by method and pass/fail",
	}, []string{"method", "result"})
	stats.MustRegister(lagFactorCounter)

	ssaro := &SQLStorageAuthorityRO{
		dbReadOnlyMap:     dbReadOnlyMap,
		dbIncidentsMap:    dbIncidentsMap,
		parallelismPerRPC: parallelismPerRPC,
		lagFactor:         lagFactor,
		clk:               clk,
		log:               logger,
		lagFactorCounter:  lagFactorCounter,
	}

	return ssaro, nil
}

// GetRegistration obtains a Registration by ID
func (ssa *SQLStorageAuthorityRO) GetRegistration(ctx context.Context, req *sapb.RegistrationID) (*corepb.Registration, error) {
	if req == nil || req.Id == 0 {
		return nil, errIncompleteRequest
	}

	model, err := selectRegistration(ctx, ssa.dbReadOnlyMap, "id", req.Id)
	if db.IsNoRows(err) && ssa.lagFactor != 0 {
		// GetRegistration is often called to validate a JWK belonging to a brand
		// new account whose registrations table row hasn't propagated to the read
		// replica yet. If we get a NoRows, wait a little bit and retry, once.
		ssa.clk.Sleep(ssa.lagFactor)
		model, err = selectRegistration(ctx, ssa.dbReadOnlyMap, "id", req.Id)
		if err != nil {
			if db.IsNoRows(err) {
				ssa.lagFactorCounter.WithLabelValues("GetRegistration", "notfound").Inc()
			} else {
				ssa.lagFactorCounter.WithLabelValues("GetRegistration", "other").Inc()
			}
		} else {
			ssa.lagFactorCounter.WithLabelValues("GetRegistration", "found").Inc()
		}
	}
	if err != nil {
		if db.IsNoRows(err) {
			return nil, berrors.NotFoundError("registration with ID '%d' not found", req.Id)
		}
		return nil, err
	}

	return registrationModelToPb(model)
}

// GetRegistrationByKey obtains a Registration by JWK
func (ssa *SQLStorageAuthorityRO) GetRegistrationByKey(ctx context.Context, req *sapb.JSONWebKey) (*corepb.Registration, error) {
	if req == nil || len(req.Jwk) == 0 {
		return nil, errIncompleteRequest
	}

	var jwk jose.JSONWebKey
	err := jwk.UnmarshalJSON(req.Jwk)
	if err != nil {
		return nil, err
	}

	sha, err := core.KeyDigestB64(jwk.Key)
	if err != nil {
		return nil, err
	}
	model, err := selectRegistration(ctx, ssa.dbReadOnlyMap, "jwk_sha256", sha)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, berrors.NotFoundError("no registrations with public key sha256 %q", sha)
		}
		return nil, err
	}

	return registrationModelToPb(model)
}

// GetSerialMetadata returns metadata stored alongside the serial number,
// such as the RegID whose certificate request created that serial, and when
// the certificate with that serial will expire.
func (ssa *SQLStorageAuthorityRO) GetSerialMetadata(ctx context.Context, req *sapb.Serial) (*sapb.SerialMetadata, error) {
	if req == nil || req.Serial == "" {
		return nil, errIncompleteRequest
	}

	if !core.ValidSerial(req.Serial) {
		return nil, fmt.Errorf("invalid serial %q", req.Serial)
	}

	recordedSerial := recordedSerialModel{}
	err := ssa.dbReadOnlyMap.SelectOne(
		ctx,
		&recordedSerial,
		"SELECT * FROM serials WHERE serial = ?",
		req.Serial,
	)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, berrors.NotFoundError("serial %q not found", req.Serial)
		}
		return nil, err
	}

	return &sapb.SerialMetadata{
		Serial:         recordedSerial.Serial,
		RegistrationID: recordedSerial.RegistrationID,
		Created:        timestamppb.New(recordedSerial.Created),
		Expires:        timestamppb.New(recordedSerial.Expires),
	}, nil
}

// GetCertificate takes a serial number and returns the corresponding
// certificate, or error if it does not exist.
func (ssa *SQLStorageAuthorityRO) GetCertificate(ctx context.Context, req *sapb.Serial) (*corepb.Certificate, error) {
	if req == nil || req.Serial == "" {
		return nil, errIncompleteRequest
	}
	if !core.ValidSerial(req.Serial) {
		return nil, fmt.Errorf("invalid certificate serial %s", req.Serial)
	}

	cert, err := SelectCertificate(ctx, ssa.dbReadOnlyMap, req.Serial)
	if db.IsNoRows(err) {
		return nil, berrors.NotFoundError("certificate with serial %q not found", req.Serial)
	}
	if err != nil {
		return nil, err
	}
	return cert, nil
}

// GetLintPrecertificate takes a serial number and returns the corresponding
// linting precertificate, or error if it does not exist. The returned precert
// is identical to the actual submitted-to-CT-logs precertificate, except for
// its signature.
func (ssa *SQLStorageAuthorityRO) GetLintPrecertificate(ctx context.Context, req *sapb.Serial) (*corepb.Certificate, error) {
	if req == nil || req.Serial == "" {
		return nil, errIncompleteRequest
	}
	if !core.ValidSerial(req.Serial) {
		return nil, fmt.Errorf("invalid precertificate serial %s", req.Serial)
	}

	cert, err := SelectPrecertificate(ctx, ssa.dbReadOnlyMap, req.Serial)
	if db.IsNoRows(err) {
		return nil, berrors.NotFoundError("precertificate with serial %q not found", req.Serial)
	}
	if err != nil {
		return nil, err
	}
	return cert, nil
}

// GetCertificateStatus takes a hexadecimal string representing the full 128-bit serial
// number of a certificate and returns data about that certificate's current
// validity.
func (ssa *SQLStorageAuthorityRO) GetCertificateStatus(ctx context.Context, req *sapb.Serial) (*corepb.CertificateStatus, error) {
	if req.Serial == "" {
		return nil, errIncompleteRequest
	}
	if !core.ValidSerial(req.Serial) {
		err := fmt.Errorf("invalid certificate serial %s", req.Serial)
		return nil, err
	}

	certStatus, err := SelectCertificateStatus(ctx, ssa.dbReadOnlyMap, req.Serial)
	if db.IsNoRows(err) {
		return nil, berrors.NotFoundError("certificate status with serial %q not found", req.Serial)
	}
	if err != nil {
		return nil, err
	}

	return certStatus, nil
}

// GetRevocationStatus takes a hexadecimal string representing the full serial
// number of a certificate and returns a minimal set of data about that cert's
// current validity.
func (ssa *SQLStorageAuthorityRO) GetRevocationStatus(ctx context.Context, req *sapb.Serial) (*sapb.RevocationStatus, error) {
	if req.Serial == "" {
		return nil, errIncompleteRequest
	}
	if !core.ValidSerial(req.Serial) {
		return nil, fmt.Errorf("invalid certificate serial %s", req.Serial)
	}

	status, err := SelectRevocationStatus(ctx, ssa.dbReadOnlyMap, req.Serial)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, berrors.NotFoundError("certificate status with serial %q not found", req.Serial)
		}
		return nil, err
	}

	return status, nil
}

// FQDNSetTimestampsForWindow returns the issuance timestamps for each
// certificate, issued for a set of identifiers, during a given window of time,
// starting from the most recent issuance.
//
// If req.Limit is nonzero, it returns only the most recent `Limit` results
func (ssa *SQLStorageAuthorityRO) FQDNSetTimestampsForWindow(ctx context.Context, req *sapb.CountFQDNSetsRequest) (*sapb.Timestamps, error) {
	idents := identifier.FromProtoSlice(req.Identifiers)

	if core.IsAnyNilOrZero(req.Window) || len(idents) == 0 {
		return nil, errIncompleteRequest
	}
	limit := req.Limit
	if limit == 0 {
		limit = math.MaxInt64
	}
	type row struct {
		Issued time.Time
	}
	var rows []row
	_, err := ssa.dbReadOnlyMap.Select(
		ctx,
		&rows,
		`SELECT issued FROM fqdnSets
		WHERE setHash = ?
		AND issued > ?
		ORDER BY issued DESC
		LIMIT ?`,
		core.HashIdentifiers(idents),
		ssa.clk.Now().Add(-req.Window.AsDuration()),
		limit,
	)
	if err != nil {
		return nil, err
	}

	var results []*timestamppb.Timestamp
	for _, i := range rows {
		results = append(results, timestamppb.New(i.Issued))
	}
	return &sapb.Timestamps{Timestamps: results}, nil
}

// FQDNSetExists returns a bool indicating if one or more FQDN sets |names|
// exists in the database
func (ssa *SQLStorageAuthorityRO) FQDNSetExists(ctx context.Context, req *sapb.FQDNSetExistsRequest) (*sapb.Exists, error) {
	idents := identifier.FromProtoSlice(req.Identifiers)
	if len(idents) == 0 {
		return nil, errIncompleteRequest
	}
	exists, err := ssa.checkFQDNSetExists(ctx, ssa.dbReadOnlyMap.SelectOne, idents)
	if err != nil {
		return nil, err
	}
	return &sapb.Exists{Exists: exists}, nil
}

// oneSelectorFunc is a func type that matches both borp.Transaction.SelectOne
// and borp.DbMap.SelectOne.
type oneSelectorFunc func(ctx context.Context, holder interface{}, query string, args ...interface{}) error

// checkFQDNSetExists uses the given oneSelectorFunc to check whether an fqdnSet
// for the given names exists.
func (ssa *SQLStorageAuthorityRO) checkFQDNSetExists(ctx context.Context, selector oneSelectorFunc, idents identifier.ACMEIdentifiers) (bool, error) {
	namehash := core.HashIdentifiers(idents)
	var exists bool
	err := selector(
		ctx,
		&exists,
		`SELECT EXISTS (SELECT id FROM fqdnSets WHERE setHash = ? LIMIT 1)`,
		namehash,
	)
	return exists, err
}

// GetOrder is used to retrieve an already existing order object
func (ssa *SQLStorageAuthorityRO) GetOrder(ctx context.Context, req *sapb.OrderRequest) (*corepb.Order, error) {
	if req == nil || req.Id == 0 {
		return nil, errIncompleteRequest
	}

	txn := func(tx db.Executor) (interface{}, error) {
		omObj, err := tx.Get(ctx, orderModel{}, req.Id)
		if err != nil {
			if db.IsNoRows(err) {
				return nil, berrors.NotFoundError("no order found for ID %d", req.Id)
			}
			return nil, err
		}
		if omObj == nil {
			return nil, berrors.NotFoundError("no order found for ID %d", req.Id)
		}

		order, err := modelToOrder(omObj.(*orderModel))
		if err != nil {
			return nil, err
		}

		orderExp := order.Expires.AsTime()
		if orderExp.Before(ssa.clk.Now()) {
			return nil, berrors.NotFoundError("no order found for ID %d", req.Id)
		}

		v2AuthzIDs, err := authzForOrder(ctx, tx, order.Id)
		if err != nil {
			return nil, err
		}
		order.V2Authorizations = v2AuthzIDs

		// Get the partial Authorization objects for the order
		authzValidityInfo, err := getAuthorizationStatuses(ctx, tx, order.V2Authorizations)
		// If there was an error getting the authorizations, return it immediately
		if err != nil {
			return nil, err
		}

		var idents identifier.ACMEIdentifiers
		for _, a := range authzValidityInfo {
			idents = append(idents, identifier.ACMEIdentifier{Type: uintToIdentifierType[a.IdentifierType], Value: a.IdentifierValue})
		}
		order.Identifiers = idents.ToProtoSlice()

		// Calculate the status for the order
		status, err := statusForOrder(order, authzValidityInfo, ssa.clk.Now())
		if err != nil {
			return nil, err
		}
		order.Status = status

		return order, nil
	}

	output, err := db.WithTransaction(ctx, ssa.dbReadOnlyMap, txn)
	if (db.IsNoRows(err) || errors.Is(err, berrors.NotFound)) && ssa.lagFactor != 0 {
		// GetOrder is often called shortly after a new order is created, sometimes
		// before the order or its associated rows have propagated to the read
		// replica yet. If we get a NoRows, wait a little bit and retry, once.
		ssa.clk.Sleep(ssa.lagFactor)
		output, err = db.WithTransaction(ctx, ssa.dbReadOnlyMap, txn)
		if err != nil {
			if db.IsNoRows(err) || errors.Is(err, berrors.NotFound) {
				ssa.lagFactorCounter.WithLabelValues("GetOrder", "notfound").Inc()
			} else {
				ssa.lagFactorCounter.WithLabelValues("GetOrder", "other").Inc()
			}
		} else {
			ssa.lagFactorCounter.WithLabelValues("GetOrder", "found").Inc()
		}
	}
	if err != nil {
		return nil, err
	}

	order, ok := output.(*corepb.Order)
	if !ok {
		return nil, fmt.Errorf("casting error in GetOrder")
	}

	return order, nil
}

// GetOrderForNames tries to find a **pending** or **ready** order with the
// exact set of names requested, associated with the given accountID. Only
// unexpired orders are considered. If no order meeting these requirements is
// found a nil corepb.Order pointer is returned.
func (ssa *SQLStorageAuthorityRO) GetOrderForNames(ctx context.Context, req *sapb.GetOrderForNamesRequest) (*corepb.Order, error) {
	idents := identifier.FromProtoSlice(req.Identifiers)

	if req.AcctID == 0 || len(idents) == 0 {
		return nil, errIncompleteRequest
	}

	// Hash the names requested for lookup in the orderFqdnSets table
	fqdnHash := core.HashIdentifiers(idents)

	// Find a possibly-suitable order. We don't include the account ID or order
	// status in this query because there's no index that includes those, so
	// including them could require the DB to scan extra rows.
	// Instead, we select one unexpired order that matches the fqdnSet. If
	// that order doesn't match the account ID or status we need, just return
	// nothing. We use `ORDER BY expires ASC` because the index on
	// (setHash, expires) is in ASC order. DESC would be slightly nicer from a
	// user experience perspective but would be slow when there are many entries
	// to sort.
	// This approach works fine because in most cases there's only one account
	// issuing for a given name. If there are other accounts issuing for the same
	// name, it just means order reuse happens less often.
	var result struct {
		OrderID        int64
		RegistrationID int64
	}
	var err error
	err = ssa.dbReadOnlyMap.SelectOne(ctx, &result, `
					SELECT orderID, registrationID
					FROM orderFqdnSets
					WHERE setHash = ?
					AND expires > ?
					ORDER BY expires ASC
					LIMIT 1`,
		fqdnHash, ssa.clk.Now())

	if db.IsNoRows(err) {
		return nil, berrors.NotFoundError("no order matching request found")
	} else if err != nil {
		return nil, err
	}

	if result.RegistrationID != req.AcctID {
		return nil, berrors.NotFoundError("no order matching request found")
	}

	// Get the order
	order, err := ssa.GetOrder(ctx, &sapb.OrderRequest{Id: result.OrderID})
	if err != nil {
		return nil, err
	}
	// Only return a pending or ready order
	if order.Status != string(core.StatusPending) &&
		order.Status != string(core.StatusReady) {
		return nil, berrors.NotFoundError("no order matching request found")
	}
	return order, nil
}

// GetAuthorization2 returns the authz2 style authorization identified by the provided ID or an error.
// If no authorization is found matching the ID a berrors.NotFound type error is returned.
func (ssa *SQLStorageAuthorityRO) GetAuthorization2(ctx context.Context, req *sapb.AuthorizationID2) (*corepb.Authorization, error) {
	if req.Id == 0 {
		return nil, errIncompleteRequest
	}
	obj, err := ssa.dbReadOnlyMap.Get(ctx, authzModel{}, req.Id)
	if db.IsNoRows(err) && ssa.lagFactor != 0 {
		// GetAuthorization2 is often called shortly after a new order is created,
		// sometimes before the order's associated authz rows have propagated to the
		// read replica yet. If we get a NoRows, wait a little bit and retry, once.
		ssa.clk.Sleep(ssa.lagFactor)
		obj, err = ssa.dbReadOnlyMap.Get(ctx, authzModel{}, req.Id)
		if err != nil {
			if db.IsNoRows(err) {
				ssa.lagFactorCounter.WithLabelValues("GetAuthorization2", "notfound").Inc()
			} else {
				ssa.lagFactorCounter.WithLabelValues("GetAuthorization2", "other").Inc()
			}
		} else {
			ssa.lagFactorCounter.WithLabelValues("GetAuthorization2", "found").Inc()
		}
	}
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, berrors.NotFoundError("authorization %d not found", req.Id)
	}
	return modelToAuthzPB(*(obj.(*authzModel)))
}

// authzModelMapToPB converts a mapping of identifiers to authzModels into a
// protobuf authorizations map
func authzModelMapToPB(m map[identifier.ACMEIdentifier]authzModel) (*sapb.Authorizations, error) {
	resp := &sapb.Authorizations{}
	for _, v := range m {
		authzPB, err := modelToAuthzPB(v)
		if err != nil {
			return nil, err
		}
		resp.Authzs = append(resp.Authzs, authzPB)
	}
	return resp, nil
}

// GetAuthorizations2 returns a single pending or valid authorization owned by
// the given account for all given identifiers. If both a valid and pending
// authorization exist only the valid one will be returned.
//
// Deprecated: Use GetValidAuthorizations2, as we stop pending authz reuse.
func (ssa *SQLStorageAuthorityRO) GetAuthorizations2(ctx context.Context, req *sapb.GetAuthorizationsRequest) (*sapb.Authorizations, error) {
	idents := identifier.FromProtoSlice(req.Identifiers)

	if core.IsAnyNilOrZero(req, req.RegistrationID, idents, req.ValidUntil) {
		return nil, errIncompleteRequest
	}

	// The WHERE clause returned by this function does not contain any
	// user-controlled strings; all user-controlled input ends up in the
	// returned placeholder args.
	identConditions, identArgs := buildIdentifierQueryConditions(idents)
	query := fmt.Sprintf(
		`SELECT %s FROM authz2
			USE INDEX (regID_identifier_status_expires_idx)
			WHERE registrationID = ? AND
			status IN (?,?) AND
			expires > ? AND
			(%s)`,
		authzFields,
		identConditions,
	)

	params := []interface{}{
		req.RegistrationID,
		statusUint(core.StatusValid), statusUint(core.StatusPending),
		req.ValidUntil.AsTime(),
	}
	params = append(params, identArgs...)

	var authzModels []authzModel
	_, err := ssa.dbReadOnlyMap.Select(
		ctx,
		&authzModels,
		query,
		params...,
	)
	if err != nil {
		return nil, err
	}

	if len(authzModels) == 0 {
		return &sapb.Authorizations{}, nil
	}

	// TODO(#8111): Consider reducing the volume of data in this map.
	authzModelMap := make(map[identifier.ACMEIdentifier]authzModel, len(authzModels))
	for _, am := range authzModels {
		if req.Profile != "" {
			// Don't return authzs whose profile doesn't match that requested.
			if am.CertificateProfileName == nil || *am.CertificateProfileName != req.Profile {
				continue
			}
		}
		// If there is an existing authorization in the map, only replace it with
		// one which has a "better" validation state (valid instead of pending).
		identType, ok := uintToIdentifierType[am.IdentifierType]
		if !ok {
			return nil, fmt.Errorf("unrecognized identifier type encoding %d on authz id %d", am.IdentifierType, am.ID)
		}
		ident := identifier.ACMEIdentifier{Type: identType, Value: am.IdentifierValue}
		existing, present := authzModelMap[ident]
		if !present || uintToStatus[existing.Status] == core.StatusPending && uintToStatus[am.Status] == core.StatusValid {
			authzModelMap[ident] = am
		}
	}

	return authzModelMapToPB(authzModelMap)
}

// CountPendingAuthorizations2 returns the number of pending, unexpired authorizations
// for the given registration.
func (ssa *SQLStorageAuthorityRO) CountPendingAuthorizations2(ctx context.Context, req *sapb.RegistrationID) (*sapb.Count, error) {
	if req.Id == 0 {
		return nil, errIncompleteRequest
	}

	var count int64
	err := ssa.dbReadOnlyMap.SelectOne(ctx, &count,
		`SELECT COUNT(*) FROM authz2 WHERE
		registrationID = :regID AND
		expires > :expires AND
		status = :status`,
		map[string]interface{}{
			"regID":   req.Id,
			"expires": ssa.clk.Now(),
			"status":  statusUint(core.StatusPending),
		},
	)
	if err != nil {
		return nil, err
	}
	return &sapb.Count{Count: count}, nil
}

// GetValidOrderAuthorizations2 is used to get all authorizations
// associated with the given Order ID.
// NOTE: The name is outdated. It does *not* filter out invalid or expired
// authorizations; that it left to the caller. It also ignores the RegID field
// of the input: ensuring that the returned authorizations match the same RegID
// as the Order is also left to the caller. This is because the caller is
// generally in a better position to provide insightful error messages, whereas
// simply omitting an authz from this method's response would leave the caller
// wondering why that authz was omitted.
func (ssa *SQLStorageAuthorityRO) GetValidOrderAuthorizations2(ctx context.Context, req *sapb.GetValidOrderAuthorizationsRequest) (*sapb.Authorizations, error) {
	if core.IsAnyNilOrZero(req.Id) {
		return nil, errIncompleteRequest
	}

	// The authz2 and orderToAuthz2 tables both have a column named "id", so we
	// need to be explicit about which table's "id" column we want to select.
	qualifiedAuthzFields := strings.Split(authzFields, " ")
	for i, field := range qualifiedAuthzFields {
		if field == "id," {
			qualifiedAuthzFields[i] = "authz2.id,"
			break
		}
	}

	var ams []authzModel
	_, err := ssa.dbReadOnlyMap.Select(
		ctx,
		&ams,
		fmt.Sprintf(`SELECT %s FROM authz2
			LEFT JOIN orderToAuthz2 ON authz2.ID = orderToAuthz2.authzID
			WHERE orderToAuthz2.orderID = :orderID`,
			strings.Join(qualifiedAuthzFields, " "),
		),
		map[string]interface{}{
			"orderID": req.Id,
		},
	)
	if err != nil {
		return nil, err
	}

	// TODO(#8111): Consider reducing the volume of data in this map.
	byIdent := make(map[identifier.ACMEIdentifier]authzModel)
	for _, am := range ams {
		identType, ok := uintToIdentifierType[am.IdentifierType]
		if !ok {
			return nil, fmt.Errorf("unrecognized identifier type encoding %d on authz id %d", am.IdentifierType, am.ID)
		}
		ident := identifier.ACMEIdentifier{Type: identType, Value: am.IdentifierValue}
		_, present := byIdent[ident]
		if present {
			return nil, fmt.Errorf("identifier %q appears twice in authzs for order %d", am.IdentifierValue, req.Id)
		}
		byIdent[ident] = am
	}

	return authzModelMapToPB(byIdent)
}

// CountInvalidAuthorizations2 counts invalid authorizations for a user expiring
// in a given time range.
func (ssa *SQLStorageAuthorityRO) CountInvalidAuthorizations2(ctx context.Context, req *sapb.CountInvalidAuthorizationsRequest) (*sapb.Count, error) {
	ident := identifier.FromProto(req.Identifier)

	if core.IsAnyNilOrZero(req.RegistrationID, ident, req.Range.Earliest, req.Range.Latest) {
		return nil, errIncompleteRequest
	}

	idType, ok := identifierTypeToUint[ident.ToProto().Type]
	if !ok {
		return nil, fmt.Errorf("unsupported identifier type %q", ident.ToProto().Type)
	}

	var count int64
	err := ssa.dbReadOnlyMap.SelectOne(
		ctx,
		&count,
		`SELECT COUNT(*) FROM authz2 WHERE
		registrationID = :regID AND
		status = :status AND
		expires > :expiresEarliest AND
		expires <= :expiresLatest AND
		identifierType = :identType AND
		identifierValue = :identValue`,
		map[string]interface{}{
			"regID":           req.RegistrationID,
			"identType":       idType,
			"identValue":      ident.Value,
			"expiresEarliest": req.Range.Earliest.AsTime(),
			"expiresLatest":   req.Range.Latest.AsTime(),
			"status":          statusUint(core.StatusInvalid),
		},
	)
	if err != nil {
		return nil, err
	}
	return &sapb.Count{Count: count}, nil
}

// GetValidAuthorizations2 returns a single valid authorization owned by the
// given account for all given identifiers. If more than one valid authorization
// exists, only the one with the latest expiry will be returned.
func (ssa *SQLStorageAuthorityRO) GetValidAuthorizations2(ctx context.Context, req *sapb.GetValidAuthorizationsRequest) (*sapb.Authorizations, error) {
	idents := identifier.FromProtoSlice(req.Identifiers)

	if core.IsAnyNilOrZero(req, req.RegistrationID, idents, req.ValidUntil) {
		return nil, errIncompleteRequest
	}

	// The WHERE clause returned by this function does not contain any
	// user-controlled strings; all user-controlled input ends up in the
	// returned placeholder args.
	identConditions, identArgs := buildIdentifierQueryConditions(idents)
	query := fmt.Sprintf(
		`SELECT %s FROM authz2
			USE INDEX (regID_identifier_status_expires_idx)
			WHERE registrationID = ? AND
			status = ? AND
			expires > ? AND
			(%s)`,
		authzFields,
		identConditions,
	)

	params := []interface{}{
		req.RegistrationID,
		statusUint(core.StatusValid),
		req.ValidUntil.AsTime(),
	}
	params = append(params, identArgs...)

	var authzModels []authzModel
	_, err := ssa.dbReadOnlyMap.Select(
		ctx,
		&authzModels,
		query,
		params...,
	)
	if err != nil {
		return nil, err
	}

	if len(authzModels) == 0 {
		return &sapb.Authorizations{}, nil
	}

	// TODO(#8111): Consider reducing the volume of data in this map.
	authzMap := make(map[identifier.ACMEIdentifier]authzModel, len(authzModels))
	for _, am := range authzModels {
		if req.Profile != "" {
			// Don't return authzs whose profile doesn't match that requested.
			if am.CertificateProfileName == nil || *am.CertificateProfileName != req.Profile {
				continue
			}
		}
		// If there is an existing authorization in the map only replace it with one
		// which has a later expiry.
		identType, ok := uintToIdentifierType[am.IdentifierType]
		if !ok {
			return nil, fmt.Errorf("unrecognized identifier type encoding %d on authz id %d", am.IdentifierType, am.ID)
		}
		ident := identifier.ACMEIdentifier{Type: identType, Value: am.IdentifierValue}
		existing, present := authzMap[ident]
		if present && am.Expires.Before(existing.Expires) {
			continue
		}
		authzMap[ident] = am
	}

	return authzModelMapToPB(authzMap)
}

// KeyBlocked checks if a key, indicated by a hash, is present in the blockedKeys table
func (ssa *SQLStorageAuthorityRO) KeyBlocked(ctx context.Context, req *sapb.SPKIHash) (*sapb.Exists, error) {
	if req == nil || req.KeyHash == nil {
		return nil, errIncompleteRequest
	}

	var id int64
	err := ssa.dbReadOnlyMap.SelectOne(ctx, &id, `SELECT ID FROM blockedKeys WHERE keyHash = ?`, req.KeyHash)
	if err != nil {
		if db.IsNoRows(err) {
			return &sapb.Exists{Exists: false}, nil
		}
		return nil, err
	}

	return &sapb.Exists{Exists: true}, nil
}

// IncidentsForSerial queries each active incident table and returns every
// incident that currently impacts `req.Serial`.
func (ssa *SQLStorageAuthorityRO) IncidentsForSerial(ctx context.Context, req *sapb.Serial) (*sapb.Incidents, error) {
	if req == nil {
		return nil, errIncompleteRequest
	}

	var activeIncidents []incidentModel
	_, err := ssa.dbReadOnlyMap.Select(ctx, &activeIncidents, `SELECT * FROM incidents WHERE enabled = 1`)
	if err != nil {
		if db.IsNoRows(err) {
			return &sapb.Incidents{}, nil
		}
		return nil, err
	}

	var incidentsForSerial []*sapb.Incident
	for _, i := range activeIncidents {
		var count int
		err := ssa.dbIncidentsMap.SelectOne(ctx, &count, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE serial = ?",
			i.SerialTable), req.Serial)
		if err != nil {
			if db.IsNoRows(err) {
				continue
			}
			return nil, err
		}
		if count > 0 {
			incident := incidentModelToPB(i)
			incidentsForSerial = append(incidentsForSerial, &incident)
		}

	}
	if len(incidentsForSerial) == 0 {
		return &sapb.Incidents{}, nil
	}
	return &sapb.Incidents{Incidents: incidentsForSerial}, nil
}

// SerialsForIncident queries the provided incident table and returns the
// resulting rows as a stream of `*sapb.IncidentSerial`s. An `io.EOF` error
// signals that there are no more serials to send. If the incident table in
// question contains zero rows, only an `io.EOF` error is returned. The
// IncidentSerial messages returned may have the zero-value for their OrderID,
// RegistrationID, and LastNoticeSent fields, if those are NULL in the database.
func (ssa *SQLStorageAuthorityRO) SerialsForIncident(req *sapb.SerialsForIncidentRequest, stream grpc.ServerStreamingServer[sapb.IncidentSerial]) error {
	if req.IncidentTable == "" {
		return errIncompleteRequest
	}

	// Check that `req.IncidentTable` is a valid incident table name.
	if !validIncidentTableRegexp.MatchString(req.IncidentTable) {
		return fmt.Errorf("malformed table name %q", req.IncidentTable)
	}

	selector, err := db.NewMappedSelector[incidentSerialModel](ssa.dbIncidentsMap)
	if err != nil {
		return fmt.Errorf("initializing db map: %w", err)
	}

	rows, err := selector.QueryFrom(stream.Context(), req.IncidentTable, "")
	if err != nil {
		return fmt.Errorf("starting db query: %w", err)
	}

	return rows.ForEach(func(row *incidentSerialModel) error {
		// Scan the row into the model. Note: the fields must be passed in the
		// same order as the columns returned by the query above.
		ism, err := rows.Get()
		if err != nil {
			return err
		}

		ispb := &sapb.IncidentSerial{
			Serial: ism.Serial,
		}
		if ism.RegistrationID != nil {
			ispb.RegistrationID = *ism.RegistrationID
		}
		if ism.OrderID != nil {
			ispb.OrderID = *ism.OrderID
		}
		if ism.LastNoticeSent != nil {
			ispb.LastNoticeSent = timestamppb.New(*ism.LastNoticeSent)
		}

		return stream.Send(ispb)
	})
}

// GetRevokedCertsByShard returns revoked certificates by explicit sharding.
//
// It returns all unexpired certificates from the revokedCertificates table with the given
// shardIdx. It limits the results those revoked before req.RevokedBefore.
func (ssa *SQLStorageAuthorityRO) GetRevokedCertsByShard(req *sapb.GetRevokedCertsByShardRequest, stream grpc.ServerStreamingServer[corepb.CRLEntry]) error {
	if core.IsAnyNilOrZero(req.ShardIdx, req.IssuerNameID, req.RevokedBefore, req.ExpiresAfter) {
		return errIncompleteRequest
	}

	atTime := req.RevokedBefore.AsTime()

	clauses := `
		WHERE issuerID = ?
		AND shardIdx = ?
		AND notAfterHour >= ?`
	params := []interface{}{
		req.IssuerNameID,
		req.ShardIdx,
		// Round the expiry down to the nearest hour, to take advantage of our
		// smaller index while still capturing at least as many certs as intended.
		req.ExpiresAfter.AsTime().Truncate(time.Hour),
	}

	selector, err := db.NewMappedSelector[revokedCertModel](ssa.dbReadOnlyMap)
	if err != nil {
		return fmt.Errorf("initializing db map: %w", err)
	}

	rows, err := selector.QueryContext(stream.Context(), clauses, params...)
	if err != nil {
		return fmt.Errorf("reading db: %w", err)
	}

	return rows.ForEach(func(row *revokedCertModel) error {
		// Double-check that the cert wasn't revoked between the time at which we're
		// constructing this snapshot CRL and right now. If the cert was revoked
		// at-or-after the "atTime", we'll just include it in the next generation
		// of CRLs.
		if row.RevokedDate.After(atTime) || row.RevokedDate.Equal(atTime) {
			return nil
		}

		return stream.Send(&corepb.CRLEntry{
			Serial:    row.Serial,
			Reason:    int32(row.RevokedReason), //nolint: gosec // Revocation reasons are guaranteed to be small, no risk of overflow.
			RevokedAt: timestamppb.New(row.RevokedDate),
		})
	})
}

// GetRevokedCerts returns revoked certificates based on temporal sharding.
//
// Based on a request specifying an issuer and a period of time,
// it writes to the output stream the set of all certificates issued by that
// issuer which expire during that period of time and which have been revoked.
// The starting timestamp is treated as inclusive (certs with exactly that
// notAfter date are included), but the ending timestamp is exclusive (certs
// with exactly that notAfter date are *not* included).
func (ssa *SQLStorageAuthorityRO) GetRevokedCerts(req *sapb.GetRevokedCertsRequest, stream grpc.ServerStreamingServer[corepb.CRLEntry]) error {
	if core.IsAnyNilOrZero(req.IssuerNameID, req.RevokedBefore, req.ExpiresAfter, req.ExpiresBefore) {
		return errIncompleteRequest
	}
	atTime := req.RevokedBefore.AsTime()

	clauses := `
		WHERE notAfter >= ?
		AND notAfter < ?
		AND issuerID = ?
		AND status = ?`
	params := []interface{}{
		req.ExpiresAfter.AsTime(),
		req.ExpiresBefore.AsTime(),
		req.IssuerNameID,
		core.OCSPStatusRevoked,
	}

	selector, err := db.NewMappedSelector[crlEntryModel](ssa.dbReadOnlyMap)
	if err != nil {
		return fmt.Errorf("initializing db map: %w", err)
	}

	rows, err := selector.QueryContext(stream.Context(), clauses, params...)
	if err != nil {
		return fmt.Errorf("reading db: %w", err)
	}

	return rows.ForEach(func(row *crlEntryModel) error {
		// Double-check that the cert wasn't revoked between the time at which we're
		// constructing this snapshot CRL and right now. If the cert was revoked
		// at-or-after the "atTime", we'll just include it in the next generation
		// of CRLs.
		if row.RevokedDate.After(atTime) || row.RevokedDate.Equal(atTime) {
			return nil
		}

		return stream.Send(&corepb.CRLEntry{
			Serial:    row.Serial,
			Reason:    int32(row.RevokedReason), //nolint: gosec // Revocation reasons are guaranteed to be small, no risk of overflow.
			RevokedAt: timestamppb.New(row.RevokedDate),
		})
	})
}

// GetMaxExpiration returns the timestamp of the farthest-future notAfter date
// found in the certificateStatus table. This provides an upper bound on how far
// forward operations that need to cover all currently-unexpired certificates
// have to look.
func (ssa *SQLStorageAuthorityRO) GetMaxExpiration(ctx context.Context, req *emptypb.Empty) (*timestamppb.Timestamp, error) {
	var model struct {
		MaxNotAfter *time.Time `db:"maxNotAfter"`
	}
	err := ssa.dbReadOnlyMap.SelectOne(
		ctx,
		&model,
		"SELECT MAX(notAfter) AS maxNotAfter FROM certificateStatus",
	)
	if err != nil {
		return nil, fmt.Errorf("selecting max notAfter: %w", err)
	}
	if model.MaxNotAfter == nil {
		return nil, errors.New("certificateStatus table notAfter column is empty")
	}
	return timestamppb.New(*model.MaxNotAfter), err
}

// Health implements the grpc.checker interface.
func (ssa *SQLStorageAuthorityRO) Health(ctx context.Context) error {
	err := ssa.dbReadOnlyMap.SelectOne(ctx, new(int), "SELECT 1")
	if err != nil {
		return err
	}
	return nil
}

// ReplacementOrderExists returns whether a valid replacement order exists for
// the given certificate serial number. An existing but expired or otherwise
// invalid replacement order is not considered to exist.
func (ssa *SQLStorageAuthorityRO) ReplacementOrderExists(ctx context.Context, req *sapb.Serial) (*sapb.Exists, error) {
	if req == nil || req.Serial == "" {
		return nil, errIncompleteRequest
	}

	var replacement replacementOrderModel
	err := ssa.dbReadOnlyMap.SelectOne(
		ctx,
		&replacement,
		"SELECT * FROM replacementOrders WHERE serial = ? LIMIT 1",
		req.Serial,
	)
	if err != nil {
		if db.IsNoRows(err) {
			// No replacement order exists.
			return &sapb.Exists{Exists: false}, nil
		}
		return nil, err
	}
	if replacement.Replaced {
		// Certificate has already been replaced.
		return &sapb.Exists{Exists: true}, nil
	}
	if replacement.OrderExpires.Before(ssa.clk.Now()) {
		// The existing replacement order has expired.
		return &sapb.Exists{Exists: false}, nil
	}

	// Pull the replacement order so we can inspect its status.
	replacementOrder, err := ssa.GetOrder(ctx, &sapb.OrderRequest{Id: replacement.OrderID})
	if err != nil {
		if errors.Is(err, berrors.NotFound) {
			// The existing replacement order has been deleted. This should
			// never happen.
			ssa.log.Errf("replacement order %d for serial %q not found", replacement.OrderID, req.Serial)
			return &sapb.Exists{Exists: false}, nil
		}
	}

	switch replacementOrder.Status {
	case string(core.StatusPending), string(core.StatusReady), string(core.StatusProcessing), string(core.StatusValid):
		// An existing replacement order is either still being worked on or has
		// already been finalized.
		return &sapb.Exists{Exists: true}, nil

	case string(core.StatusInvalid):
		// The existing replacement order cannot be finalized. The requester
		// should create a new replacement order.
		return &sapb.Exists{Exists: false}, nil

	default:
		// Replacement order is in an unknown state. This should never happen.
		return nil, fmt.Errorf("unknown replacement order status: %q", replacementOrder.Status)
	}
}

// GetSerialsByKey returns a stream of serials for all unexpired certificates
// whose public key matches the given SPKIHash. This is useful for revoking all
// certificates affected by a key compromise.
func (ssa *SQLStorageAuthorityRO) GetSerialsByKey(req *sapb.SPKIHash, stream grpc.ServerStreamingServer[sapb.Serial]) error {
	clauses := `
		WHERE keyHash = ?
		AND certNotAfter > ?`
	params := []interface{}{
		req.KeyHash,
		ssa.clk.Now(),
	}

	selector, err := db.NewMappedSelector[keyHashModel](ssa.dbReadOnlyMap)
	if err != nil {
		return fmt.Errorf("initializing db map: %w", err)
	}

	rows, err := selector.QueryContext(stream.Context(), clauses, params...)
	if err != nil {
		return fmt.Errorf("reading db: %w", err)
	}

	return rows.ForEach(func(row *keyHashModel) error {
		return stream.Send(&sapb.Serial{Serial: row.CertSerial})
	})
}

// GetSerialsByAccount returns a stream of all serials for all unexpired
// certificates issued to the given RegID. This is useful for revoking all of
// an account's certs upon their request.
func (ssa *SQLStorageAuthorityRO) GetSerialsByAccount(req *sapb.RegistrationID, stream grpc.ServerStreamingServer[sapb.Serial]) error {
	clauses := `
		WHERE registrationID = ?
		AND expires > ?`
	params := []interface{}{
		req.Id,
		ssa.clk.Now(),
	}

	selector, err := db.NewMappedSelector[recordedSerialModel](ssa.dbReadOnlyMap)
	if err != nil {
		return fmt.Errorf("initializing db map: %w", err)
	}

	rows, err := selector.QueryContext(stream.Context(), clauses, params...)
	if err != nil {
		return fmt.Errorf("reading db: %w", err)
	}

	return rows.ForEach(func(row *recordedSerialModel) error {
		return stream.Send(&sapb.Serial{Serial: row.Serial})
	})
}

// CheckIdentifiersPaused takes a slice of identifiers and returns a slice of
// the first 15 identifier values which are currently paused for the provided
// account. If no matches are found, an empty slice is returned.
func (ssa *SQLStorageAuthorityRO) CheckIdentifiersPaused(ctx context.Context, req *sapb.PauseRequest) (*sapb.Identifiers, error) {
	if core.IsAnyNilOrZero(req.RegistrationID, req.Identifiers) {
		return nil, errIncompleteRequest
	}

	idents, err := newIdentifierModelsFromPB(req.Identifiers)
	if err != nil {
		return nil, err
	}

	if len(idents) == 0 {
		// No identifier values to check.
		return nil, nil
	}

	identsByType := map[uint8][]string{}
	for _, id := range idents {
		identsByType[id.Type] = append(identsByType[id.Type], id.Value)
	}

	// Build a query to retrieve up to 15 paused identifiers using OR clauses
	// for conditions specific to each type. This approach handles mixed
	// identifier types in a single query. Assuming 3 DNS identifiers and 1 IP
	// identifier, the resulting query would look like:
	//
	// SELECT identifierType, identifierValue
	// FROM paused WHERE registrationID = ? AND
	// unpausedAt IS NULL AND
	//     ((identifierType = ? AND identifierValue IN (?, ?, ?)) OR
	//      (identifierType = ? AND identifierValue IN (?)))
	// LIMIT 15
	//
	// Corresponding args array for placeholders: [<regID>, 0, "example.com",
	// "example.net", "example.org", 1, "1.2.3.4"]

	var conditions []string
	args := []interface{}{req.RegistrationID}
	for idType, values := range identsByType {
		conditions = append(conditions,
			fmt.Sprintf("identifierType = ? AND identifierValue IN (%s)",
				db.QuestionMarks(len(values)),
			),
		)
		args = append(args, idType)
		for _, value := range values {
			args = append(args, value)
		}
	}

	query := fmt.Sprintf(`
        SELECT identifierType, identifierValue
        FROM paused
        WHERE registrationID = ? AND unpausedAt IS NULL AND (%s) LIMIT 15`,
		strings.Join(conditions, " OR "))

	var matches []identifierModel
	_, err = ssa.dbReadOnlyMap.Select(ctx, &matches, query, args...)
	if err != nil && !db.IsNoRows(err) {
		// Error querying the database.
		return nil, err
	}

	return newPBFromIdentifierModels(matches)
}

// GetPausedIdentifiers returns a slice of paused identifiers for the provided
// account. If no paused identifiers are found, an empty slice is returned. The
// results are limited to the first 15 paused identifiers.
func (ssa *SQLStorageAuthorityRO) GetPausedIdentifiers(ctx context.Context, req *sapb.RegistrationID) (*sapb.Identifiers, error) {
	if core.IsAnyNilOrZero(req.Id) {
		return nil, errIncompleteRequest
	}

	var matches []identifierModel
	_, err := ssa.dbReadOnlyMap.Select(ctx, &matches, `
		SELECT identifierType, identifierValue
		FROM paused
		WHERE
			registrationID = ? AND
			unpausedAt IS NULL
		LIMIT 15`,
		req.Id,
	)
	if err != nil && !db.IsNoRows(err) {
		return nil, err
	}

	return newPBFromIdentifierModels(matches)
}

// GetRateLimitOverride retrieves a rate limit override for the given bucket key
// and limit. If no override is found, a NotFound error is returned.
func (ssa *SQLStorageAuthorityRO) GetRateLimitOverride(ctx context.Context, req *sapb.GetRateLimitOverrideRequest) (*sapb.RateLimitOverrideResponse, error) {
	if core.IsAnyNilOrZero(req, req.LimitEnum, req.BucketKey) {
		return nil, errIncompleteRequest
	}

	obj, err := ssa.dbReadOnlyMap.Get(ctx, overrideModel{}, req.LimitEnum, req.BucketKey)
	if db.IsNoRows(err) {
		return nil, berrors.NotFoundError(
			"no rate limit override found for limit %d and bucket key %s",
			req.LimitEnum,
			req.BucketKey,
		)
	}
	if err != nil {
		return nil, err
	}
	row := obj.(*overrideModel)

	return &sapb.RateLimitOverrideResponse{
		Override:  newPBFromOverrideModel(row),
		Enabled:   row.Enabled,
		UpdatedAt: timestamppb.New(row.UpdatedAt),
	}, nil
}

// GetEnabledRateLimitOverrides retrieves all enabled rate limit overrides from
// the database. The results are returned as a stream. If no enabled overrides
// are found, an empty stream is returned.
func (ssa *SQLStorageAuthorityRO) GetEnabledRateLimitOverrides(_ *emptypb.Empty, stream sapb.StorageAuthorityReadOnly_GetEnabledRateLimitOverridesServer) error {
	selector, err := db.NewMappedSelector[overrideModel](ssa.dbReadOnlyMap)
	if err != nil {
		return fmt.Errorf("initializing selector: %w", err)
	}

	rows, err := selector.QueryContext(stream.Context(), "WHERE enabled = true")
	if err != nil {
		return fmt.Errorf("querying enabled overrides: %w", err)
	}

	return rows.ForEach(func(m *overrideModel) error {
		return stream.Send(newPBFromOverrideModel(m))
	})
}
