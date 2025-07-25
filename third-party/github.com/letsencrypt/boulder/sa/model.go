package sa

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/db"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/revocation"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

// errBadJSON is an error type returned when a json.Unmarshal performed by the
// SA fails. It includes both the Unmarshal error and the original JSON data in
// its error message to make it easier to track down the bad JSON data.
type errBadJSON struct {
	msg  string
	json []byte
	err  error
}

// Error returns an error message that includes the json.Unmarshal error as well
// as the bad JSON data.
func (e errBadJSON) Error() string {
	return fmt.Sprintf(
		"%s: error unmarshaling JSON %q: %s",
		e.msg,
		string(e.json),
		e.err)
}

// badJSONError is a convenience function for constructing a errBadJSON instance
// with the provided args.
func badJSONError(msg string, jsonData []byte, err error) error {
	return errBadJSON{
		msg:  msg,
		json: jsonData,
		err:  err,
	}
}

const regFields = "id, jwk, jwk_sha256, agreement, createdAt, LockCol, status"

// ClearEmail removes the provided email address from one specified registration. If
// there are multiple email addresses present, it does not modify other ones. If the email
// address is not present, it does not modify the registration and will return a nil error.
func ClearEmail(ctx context.Context, dbMap db.DatabaseMap, regID int64, email string) error {
	_, overallError := db.WithTransaction(ctx, dbMap, func(tx db.Executor) (interface{}, error) {
		curr, err := selectRegistration(ctx, tx, "id", regID)
		if err != nil {
			return nil, err
		}

		currPb, err := registrationModelToPb(curr)
		if err != nil {
			return nil, err
		}

		// newContacts will be a copy of all emails in currPb.Contact _except_ the one to be removed
		var newContacts []string
		for _, contact := range currPb.Contact {
			if contact != "mailto:"+email {
				newContacts = append(newContacts, contact)
			}
		}

		if slices.Equal(currPb.Contact, newContacts) {
			return nil, nil
		}

		// We don't want to write literal JSON "null" strings into the database if the
		// list of contact addresses is empty. Replace any possibly-`nil` slice with
		// an empty JSON array. We don't need to check reg.ContactPresent, because
		// we're going to write the whole object to the database anyway.
		jsonContact := []byte("[]")
		if len(newContacts) != 0 {
			jsonContact, err = json.Marshal(newContacts)
			if err != nil {
				return nil, err
			}
		}

		// UPDATE the row with a direct database query, in order to avoid LockCol issues.
		result, err := tx.ExecContext(ctx,
			"UPDATE registrations SET contact = ? WHERE id = ? LIMIT 1",
			jsonContact,
			regID,
		)
		if err != nil {
			return nil, err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil || rowsAffected != 1 {
			return nil, berrors.InternalServerError("no registration updated with new contact field")
		}

		return nil, nil
	})
	if overallError != nil {
		return overallError
	}

	return nil
}

// selectRegistration selects all fields of one registration model
func selectRegistration(ctx context.Context, s db.OneSelector, whereCol string, args ...interface{}) (*regModel, error) {
	if whereCol != "id" && whereCol != "jwk_sha256" {
		return nil, fmt.Errorf("column name %q invalid for registrations table WHERE clause", whereCol)
	}

	var model regModel
	err := s.SelectOne(
		ctx,
		&model,
		"SELECT "+regFields+" FROM registrations WHERE "+whereCol+" = ? LIMIT 1",
		args...,
	)
	return &model, err
}

const certFields = "id, registrationID, serial, digest, der, issued, expires"

// SelectCertificate selects all fields of one certificate object identified by
// a serial. If more than one row contains the same serial only the first is
// returned.
func SelectCertificate(ctx context.Context, s db.OneSelector, serial string) (*corepb.Certificate, error) {
	var model certificateModel
	err := s.SelectOne(
		ctx,
		&model,
		"SELECT "+certFields+" FROM certificates WHERE serial = ? LIMIT 1",
		serial,
	)
	return model.toPb(), err
}

const precertFields = "registrationID, serial, der, issued, expires"

// SelectPrecertificate selects all fields of one precertificate object
// identified by serial.
func SelectPrecertificate(ctx context.Context, s db.OneSelector, serial string) (*corepb.Certificate, error) {
	var model lintingCertModel
	err := s.SelectOne(
		ctx,
		&model,
		"SELECT "+precertFields+" FROM precertificates WHERE serial = ? LIMIT 1",
		serial)
	if err != nil {
		return nil, err
	}
	return model.toPb(), nil
}

// SelectCertificates selects all fields of multiple certificate objects
//
// Returns a slice of *corepb.Certificate along with the highest ID field seen
// (which can be used as input to a subsequent query when iterating in primary
// key order).
func SelectCertificates(ctx context.Context, s db.Selector, q string, args map[string]interface{}) ([]*corepb.Certificate, int64, error) {
	var models []certificateModel
	_, err := s.Select(
		ctx,
		&models,
		"SELECT "+certFields+" FROM certificates "+q, args)
	var pbs []*corepb.Certificate
	var highestID int64
	for _, m := range models {
		pbs = append(pbs, m.toPb())
		if m.ID > highestID {
			highestID = m.ID
		}
	}
	return pbs, highestID, err
}

type CertStatusMetadata struct {
	ID                    int64             `db:"id"`
	Serial                string            `db:"serial"`
	Status                core.OCSPStatus   `db:"status"`
	OCSPLastUpdated       time.Time         `db:"ocspLastUpdated"`
	RevokedDate           time.Time         `db:"revokedDate"`
	RevokedReason         revocation.Reason `db:"revokedReason"`
	LastExpirationNagSent time.Time         `db:"lastExpirationNagSent"`
	NotAfter              time.Time         `db:"notAfter"`
	IsExpired             bool              `db:"isExpired"`
	IssuerID              int64             `db:"issuerID"`
}

const certStatusFields = "id, serial, status, ocspLastUpdated, revokedDate, revokedReason, lastExpirationNagSent, notAfter, isExpired, issuerID"

// SelectCertificateStatus selects all fields of one certificate status model
// identified by serial
func SelectCertificateStatus(ctx context.Context, s db.OneSelector, serial string) (*corepb.CertificateStatus, error) {
	var model certificateStatusModel
	err := s.SelectOne(
		ctx,
		&model,
		"SELECT "+certStatusFields+" FROM certificateStatus WHERE serial = ? LIMIT 1",
		serial,
	)
	return model.toPb(), err
}

// RevocationStatusModel represents a small subset of the columns in the
// certificateStatus table, used to determine the authoritative revocation
// status of a certificate.
type RevocationStatusModel struct {
	Status        core.OCSPStatus   `db:"status"`
	RevokedDate   time.Time         `db:"revokedDate"`
	RevokedReason revocation.Reason `db:"revokedReason"`
}

// SelectRevocationStatus returns the authoritative revocation information for
// the certificate with the given serial.
func SelectRevocationStatus(ctx context.Context, s db.OneSelector, serial string) (*sapb.RevocationStatus, error) {
	var model RevocationStatusModel
	err := s.SelectOne(
		ctx,
		&model,
		"SELECT status, revokedDate, revokedReason FROM certificateStatus WHERE serial = ? LIMIT 1",
		serial,
	)
	if err != nil {
		return nil, err
	}

	statusInt, ok := core.OCSPStatusToInt[model.Status]
	if !ok {
		return nil, fmt.Errorf("got unrecognized status %q", model.Status)
	}

	return &sapb.RevocationStatus{
		Status:        int64(statusInt),
		RevokedDate:   timestamppb.New(model.RevokedDate),
		RevokedReason: int64(model.RevokedReason),
	}, nil
}

var mediumBlobSize = int(math.Pow(2, 24))

type issuedNameModel struct {
	ID           int64     `db:"id"`
	ReversedName string    `db:"reversedName"`
	NotBefore    time.Time `db:"notBefore"`
	Serial       string    `db:"serial"`
}

// regModel is the description of a core.Registration in the database before
type regModel struct {
	ID        int64     `db:"id"`
	Key       []byte    `db:"jwk"`
	KeySHA256 string    `db:"jwk_sha256"`
	Agreement string    `db:"agreement"`
	CreatedAt time.Time `db:"createdAt"`
	LockCol   int64
	Status    string `db:"status"`
}

func registrationPbToModel(reg *corepb.Registration) (*regModel, error) {
	// Even though we don't need to convert from JSON to an in-memory JSONWebKey
	// for the sake of the `Key` field, we do need to do the conversion in order
	// to compute the SHA256 key digest.
	var jwk jose.JSONWebKey
	err := jwk.UnmarshalJSON(reg.Key)
	if err != nil {
		return nil, err
	}
	sha, err := core.KeyDigestB64(jwk.Key)
	if err != nil {
		return nil, err
	}

	var createdAt time.Time
	if !core.IsAnyNilOrZero(reg.CreatedAt) {
		createdAt = reg.CreatedAt.AsTime()
	}

	return &regModel{
		ID:        reg.Id,
		Key:       reg.Key,
		KeySHA256: sha,
		Agreement: reg.Agreement,
		CreatedAt: createdAt,
		Status:    reg.Status,
	}, nil
}

func registrationModelToPb(reg *regModel) (*corepb.Registration, error) {
	if reg.ID == 0 || len(reg.Key) == 0 {
		return nil, errors.New("incomplete Registration retrieved from DB")
	}

	return &corepb.Registration{
		Id:        reg.ID,
		Key:       reg.Key,
		Agreement: reg.Agreement,
		CreatedAt: timestamppb.New(reg.CreatedAt.UTC()),
		Status:    reg.Status,
	}, nil
}

type recordedSerialModel struct {
	ID             int64
	Serial         string
	RegistrationID int64
	Created        time.Time
	Expires        time.Time
}

type lintingCertModel struct {
	ID             int64
	Serial         string
	RegistrationID int64
	DER            []byte
	Issued         time.Time
	Expires        time.Time
}

func (model lintingCertModel) toPb() *corepb.Certificate {
	return &corepb.Certificate{
		RegistrationID: model.RegistrationID,
		Serial:         model.Serial,
		Digest:         "",
		Der:            model.DER,
		Issued:         timestamppb.New(model.Issued),
		Expires:        timestamppb.New(model.Expires),
	}
}

type certificateModel struct {
	ID             int64     `db:"id"`
	RegistrationID int64     `db:"registrationID"`
	Serial         string    `db:"serial"`
	Digest         string    `db:"digest"`
	DER            []byte    `db:"der"`
	Issued         time.Time `db:"issued"`
	Expires        time.Time `db:"expires"`
}

func (model certificateModel) toPb() *corepb.Certificate {
	return &corepb.Certificate{
		RegistrationID: model.RegistrationID,
		Serial:         model.Serial,
		Digest:         model.Digest,
		Der:            model.DER,
		Issued:         timestamppb.New(model.Issued),
		Expires:        timestamppb.New(model.Expires),
	}
}

type certificateStatusModel struct {
	ID                    int64             `db:"id"`
	Serial                string            `db:"serial"`
	Status                core.OCSPStatus   `db:"status"`
	OCSPLastUpdated       time.Time         `db:"ocspLastUpdated"`
	RevokedDate           time.Time         `db:"revokedDate"`
	RevokedReason         revocation.Reason `db:"revokedReason"`
	LastExpirationNagSent time.Time         `db:"lastExpirationNagSent"`
	NotAfter              time.Time         `db:"notAfter"`
	IsExpired             bool              `db:"isExpired"`
	IssuerID              int64             `db:"issuerID"`
}

func (model certificateStatusModel) toPb() *corepb.CertificateStatus {
	return &corepb.CertificateStatus{
		Serial:                model.Serial,
		Status:                string(model.Status),
		OcspLastUpdated:       timestamppb.New(model.OCSPLastUpdated),
		RevokedDate:           timestamppb.New(model.RevokedDate),
		RevokedReason:         int64(model.RevokedReason),
		LastExpirationNagSent: timestamppb.New(model.LastExpirationNagSent),
		NotAfter:              timestamppb.New(model.NotAfter),
		IsExpired:             model.IsExpired,
		IssuerID:              model.IssuerID,
	}
}

// orderModel represents one row in the orders table. The CertificateProfileName
// column is a pointer because the column is NULL-able.
type orderModel struct {
	ID                     int64
	RegistrationID         int64
	Expires                time.Time
	Created                time.Time
	Error                  []byte
	CertificateSerial      string
	BeganProcessing        bool
	CertificateProfileName *string
	Replaces               *string
}

type orderToAuthzModel struct {
	OrderID int64
	AuthzID int64
}

func orderToModel(order *corepb.Order) (*orderModel, error) {
	// Make a local copy so we can take a reference to it below.
	profile := order.CertificateProfileName
	replaces := order.Replaces

	om := &orderModel{
		ID:                     order.Id,
		RegistrationID:         order.RegistrationID,
		Expires:                order.Expires.AsTime(),
		Created:                order.Created.AsTime(),
		BeganProcessing:        order.BeganProcessing,
		CertificateSerial:      order.CertificateSerial,
		CertificateProfileName: &profile,
		Replaces:               &replaces,
	}

	if order.Error != nil {
		errJSON, err := json.Marshal(order.Error)
		if err != nil {
			return nil, err
		}
		if len(errJSON) > mediumBlobSize {
			return nil, fmt.Errorf("Error object is too large to store in the database")
		}
		om.Error = errJSON
	}
	return om, nil
}

func modelToOrder(om *orderModel) (*corepb.Order, error) {
	profile := ""
	if om.CertificateProfileName != nil {
		profile = *om.CertificateProfileName
	}
	replaces := ""
	if om.Replaces != nil {
		replaces = *om.Replaces
	}
	order := &corepb.Order{
		Id:                     om.ID,
		RegistrationID:         om.RegistrationID,
		Expires:                timestamppb.New(om.Expires),
		Created:                timestamppb.New(om.Created),
		CertificateSerial:      om.CertificateSerial,
		BeganProcessing:        om.BeganProcessing,
		CertificateProfileName: profile,
		Replaces:               replaces,
	}
	if len(om.Error) > 0 {
		var problem corepb.ProblemDetails
		err := json.Unmarshal(om.Error, &problem)
		if err != nil {
			return &corepb.Order{}, badJSONError(
				"failed to unmarshal order model's error",
				om.Error,
				err)
		}
		order.Error = &problem
	}
	return order, nil
}

var challTypeToUint = map[string]uint8{
	"http-01":     0,
	"dns-01":      1,
	"tls-alpn-01": 2,
}

var uintToChallType = map[uint8]string{
	0: "http-01",
	1: "dns-01",
	2: "tls-alpn-01",
}

var identifierTypeToUint = map[string]uint8{
	"dns": 0,
	"ip":  1,
}

var uintToIdentifierType = map[uint8]identifier.IdentifierType{
	0: "dns",
	1: "ip",
}

var statusToUint = map[core.AcmeStatus]uint8{
	core.StatusPending:     0,
	core.StatusValid:       1,
	core.StatusInvalid:     2,
	core.StatusDeactivated: 3,
	core.StatusRevoked:     4,
}

var uintToStatus = map[uint8]core.AcmeStatus{
	0: core.StatusPending,
	1: core.StatusValid,
	2: core.StatusInvalid,
	3: core.StatusDeactivated,
	4: core.StatusRevoked,
}

func statusUint(status core.AcmeStatus) uint8 {
	return statusToUint[status]
}

// authzFields is used in a variety of places in sa.go, and modifications to
// it must be carried through to every use in sa.go
const authzFields = "id, identifierType, identifierValue, registrationID, certificateProfileName, status, expires, challenges, attempted, attemptedAt, token, validationError, validationRecord"

// authzModel represents one row in the authz2 table. The CertificateProfileName
// column is a pointer because the column is NULL-able.
type authzModel struct {
	ID                     int64      `db:"id"`
	IdentifierType         uint8      `db:"identifierType"`
	IdentifierValue        string     `db:"identifierValue"`
	RegistrationID         int64      `db:"registrationID"`
	CertificateProfileName *string    `db:"certificateProfileName"`
	Status                 uint8      `db:"status"`
	Expires                time.Time  `db:"expires"`
	Challenges             uint8      `db:"challenges"`
	Attempted              *uint8     `db:"attempted"`
	AttemptedAt            *time.Time `db:"attemptedAt"`
	Token                  []byte     `db:"token"`
	ValidationError        []byte     `db:"validationError"`
	ValidationRecord       []byte     `db:"validationRecord"`
}

// rehydrateHostPort mutates a validation record. If the URL in the validation
// record cannot be parsed, an error will be returned. If the Hostname and Port
// fields already exist in the validation record, they will be retained.
// Otherwise, the Hostname and Port will be derived and set from the URL field
// of the validation record.
func rehydrateHostPort(vr *core.ValidationRecord) error {
	if vr.URL == "" {
		return fmt.Errorf("rehydrating validation record, URL field cannot be empty")
	}

	parsedUrl, err := url.Parse(vr.URL)
	if err != nil {
		return fmt.Errorf("parsing validation record URL %q: %w", vr.URL, err)
	}

	if vr.Hostname == "" {
		hostname := parsedUrl.Hostname()
		if hostname == "" {
			return fmt.Errorf("hostname missing in URL %q", vr.URL)
		}
		vr.Hostname = hostname
	}

	if vr.Port == "" {
		// CABF BRs section 1.6.1: Authorized Ports: One of the following ports: 80
		// (http), 443 (https)
		if parsedUrl.Port() == "" {
			// If there is only a scheme, then we'll determine the appropriate port.
			switch parsedUrl.Scheme {
			case "https":
				vr.Port = "443"
			case "http":
				vr.Port = "80"
			default:
				// This should never happen since the VA should have already
				// checked the scheme.
				return fmt.Errorf("unknown scheme %q in URL %q", parsedUrl.Scheme, vr.URL)
			}
		} else if parsedUrl.Port() == "80" || parsedUrl.Port() == "443" {
			// If :80 or :443 were embedded in the URL field
			// e.g. '"url":"https://example.com:443"'
			vr.Port = parsedUrl.Port()
		} else {
			return fmt.Errorf("only ports 80/tcp and 443/tcp are allowed in URL %q", vr.URL)
		}
	}

	return nil
}

// SelectAuthzsMatchingIssuance looks for a set of authzs that would have
// authorized a given issuance that is known to have occurred. The returned
// authzs will all belong to the given regID, will have potentially been valid
// at the time of issuance, and will have the appropriate identifier type and
// value. This may return multiple authzs for the same identifier type and value.
//
// This returns "potentially" valid authzs because a client may have set an
// authzs status to deactivated after issuance, so we return both valid and
// deactivated authzs. It also uses a small amount of leeway (1s) to account
// for possible clock skew.
//
// This function doesn't do anything special for authzs with an expiration in
// the past. If the stored authz has a valid status, it is returned with a
// valid status regardless of whether it is also expired.
func SelectAuthzsMatchingIssuance(
	ctx context.Context,
	s db.Selector,
	regID int64,
	issued time.Time,
	idents identifier.ACMEIdentifiers,
) ([]*corepb.Authorization, error) {
	// The WHERE clause returned by this function does not contain any
	// user-controlled strings; all user-controlled input ends up in the
	// returned placeholder args.
	identConditions, identArgs := buildIdentifierQueryConditions(idents)
	query := fmt.Sprintf(`SELECT %s FROM authz2 WHERE
			registrationID = ? AND
			status IN (?, ?) AND
			expires >= ? AND
			attemptedAt <= ? AND
			(%s)`,
		authzFields,
		identConditions)
	var args []any
	args = append(args,
		regID,
		statusToUint[core.StatusValid], statusToUint[core.StatusDeactivated],
		issued.Add(-1*time.Second), // leeway for clock skew
		issued.Add(1*time.Second),  // leeway for clock skew
	)
	args = append(args, identArgs...)

	var authzModels []authzModel
	_, err := s.Select(ctx, &authzModels, query, args...)
	if err != nil {
		return nil, err
	}

	var authzs []*corepb.Authorization
	for _, model := range authzModels {
		authz, err := modelToAuthzPB(model)
		if err != nil {
			return nil, err
		}
		authzs = append(authzs, authz)

	}
	return authzs, err
}

// hasMultipleNonPendingChallenges checks if a slice of challenges contains
// more than one non-pending challenge
func hasMultipleNonPendingChallenges(challenges []*corepb.Challenge) bool {
	nonPending := false
	for _, c := range challenges {
		if c.Status == string(core.StatusValid) || c.Status == string(core.StatusInvalid) {
			if !nonPending {
				nonPending = true
			} else {
				return true
			}
		}
	}
	return false
}

// newAuthzReqToModel converts an sapb.NewAuthzRequest to the authzModel storage
// representation. It hardcodes the status to "pending" because it should be
// impossible to create an authz in any other state.
func newAuthzReqToModel(authz *sapb.NewAuthzRequest, profile string) (*authzModel, error) {
	am := &authzModel{
		IdentifierType:  identifierTypeToUint[authz.Identifier.Type],
		IdentifierValue: authz.Identifier.Value,
		RegistrationID:  authz.RegistrationID,
		Status:          statusToUint[core.StatusPending],
		Expires:         authz.Expires.AsTime(),
	}

	if profile != "" {
		am.CertificateProfileName = &profile
	}

	for _, challType := range authz.ChallengeTypes {
		// Set the challenge type bit in the bitmap
		am.Challenges |= 1 << challTypeToUint[challType]
	}

	token, err := base64.RawURLEncoding.DecodeString(authz.Token)
	if err != nil {
		return nil, err
	}
	am.Token = token

	return am, nil
}

// authzPBToModel converts a protobuf authorization representation to the
// authzModel storage representation.
// Deprecated: this function is only used as part of test setup, do not
// introduce any new uses in production code.
func authzPBToModel(authz *corepb.Authorization) (*authzModel, error) {
	ident := identifier.FromProto(authz.Identifier)

	am := &authzModel{
		IdentifierType:  identifierTypeToUint[ident.ToProto().Type],
		IdentifierValue: ident.Value,
		RegistrationID:  authz.RegistrationID,
		Status:          statusToUint[core.AcmeStatus(authz.Status)],
		Expires:         authz.Expires.AsTime(),
	}
	if authz.CertificateProfileName != "" {
		profile := authz.CertificateProfileName
		am.CertificateProfileName = &profile
	}
	if authz.Id != "" {
		// The v1 internal authorization objects use a string for the ID, the v2
		// storage format uses a integer ID. In order to maintain compatibility we
		// convert the integer ID to a string.
		id, err := strconv.Atoi(authz.Id)
		if err != nil {
			return nil, err
		}
		am.ID = int64(id)
	}
	if hasMultipleNonPendingChallenges(authz.Challenges) {
		return nil, errors.New("multiple challenges are non-pending")
	}
	// In the v2 authorization style we don't store individual challenges with their own
	// token, validation errors/records, etc. Instead we store a single token/error/record
	// set, a bitmap of available challenge types, and a row indicating which challenge type
	// was 'attempted'.
	//
	// Since we don't currently have the singular token/error/record set abstracted out to
	// the core authorization type yet we need to extract these from the challenges array.
	// We assume that the token in each challenge is the same and that if any of the challenges
	// has a non-pending status that it should be considered the 'attempted' challenge and
	// we extract the error/record set from that particular challenge.
	var tokenStr string
	for _, chall := range authz.Challenges {
		// Set the challenge type bit in the bitmap
		am.Challenges |= 1 << challTypeToUint[chall.Type]
		tokenStr = chall.Token
		// If the challenge status is not core.StatusPending we assume it was the 'attempted'
		// challenge and extract the relevant fields we need.
		if chall.Status == string(core.StatusValid) || chall.Status == string(core.StatusInvalid) {
			attemptedType := challTypeToUint[chall.Type]
			am.Attempted = &attemptedType

			// If validated Unix timestamp is zero then keep the core.Challenge Validated object nil.
			var validated *time.Time
			if !core.IsAnyNilOrZero(chall.Validated) {
				val := chall.Validated.AsTime()
				validated = &val
			}
			am.AttemptedAt = validated

			// Marshal corepb.ValidationRecords to core.ValidationRecords so that we
			// can marshal them to JSON.
			records := make([]core.ValidationRecord, len(chall.Validationrecords))
			for i, recordPB := range chall.Validationrecords {
				if chall.Type == string(core.ChallengeTypeHTTP01) {
					// Remove these fields because they can be rehydrated later
					// on from the URL field.
					recordPB.Hostname = ""
					recordPB.Port = ""
				}
				var err error
				records[i], err = grpc.PBToValidationRecord(recordPB)
				if err != nil {
					return nil, err
				}
			}
			var err error
			am.ValidationRecord, err = json.Marshal(records)
			if err != nil {
				return nil, err
			}
			// If there is a error associated with the challenge marshal it to JSON
			// so that we can store it in the database.
			if chall.Error != nil {
				prob, err := grpc.PBToProblemDetails(chall.Error)
				if err != nil {
					return nil, err
				}
				am.ValidationError, err = json.Marshal(prob)
				if err != nil {
					return nil, err
				}
			}
		}
		token, err := base64.RawURLEncoding.DecodeString(tokenStr)
		if err != nil {
			return nil, err
		}
		am.Token = token
	}

	return am, nil
}

// populateAttemptedFields takes a challenge and populates it with the validation fields status,
// validation records, and error (the latter only if the validation failed) from an authzModel.
func populateAttemptedFields(am authzModel, challenge *corepb.Challenge) error {
	if len(am.ValidationError) != 0 {
		// If the error is non-empty the challenge must be invalid.
		challenge.Status = string(core.StatusInvalid)
		var prob probs.ProblemDetails
		err := json.Unmarshal(am.ValidationError, &prob)
		if err != nil {
			return badJSONError(
				"failed to unmarshal authz2 model's validation error",
				am.ValidationError,
				err)
		}
		challenge.Error, err = grpc.ProblemDetailsToPB(&prob)
		if err != nil {
			return err
		}
	} else {
		// If the error is empty the challenge must be valid.
		challenge.Status = string(core.StatusValid)
	}
	var records []core.ValidationRecord
	err := json.Unmarshal(am.ValidationRecord, &records)
	if err != nil {
		return badJSONError(
			"failed to unmarshal authz2 model's validation record",
			am.ValidationRecord,
			err)
	}
	challenge.Validationrecords = make([]*corepb.ValidationRecord, len(records))
	for i, r := range records {
		// Fixes implicit memory aliasing in for loop so we can deference r
		// later on for rehydrateHostPort.
		r := r
		if challenge.Type == string(core.ChallengeTypeHTTP01) {
			err := rehydrateHostPort(&r)
			if err != nil {
				return err
			}
		}
		challenge.Validationrecords[i], err = grpc.ValidationRecordToPB(r)
		if err != nil {
			return err
		}
	}
	return nil
}

func modelToAuthzPB(am authzModel) (*corepb.Authorization, error) {
	identType, ok := uintToIdentifierType[am.IdentifierType]
	if !ok {
		return nil, fmt.Errorf("unrecognized identifier type encoding %d", am.IdentifierType)
	}

	profile := ""
	if am.CertificateProfileName != nil {
		profile = *am.CertificateProfileName
	}

	pb := &corepb.Authorization{
		Id:                     fmt.Sprintf("%d", am.ID),
		Status:                 string(uintToStatus[am.Status]),
		Identifier:             identifier.ACMEIdentifier{Type: identType, Value: am.IdentifierValue}.ToProto(),
		RegistrationID:         am.RegistrationID,
		Expires:                timestamppb.New(am.Expires),
		CertificateProfileName: profile,
	}
	// Populate authorization challenge array. We do this by iterating through
	// the challenge type bitmap and creating a challenge of each type if its
	// bit is set. Each of these challenges has the token from the authorization
	// model and has its status set to core.StatusPending by default. If the
	// challenge type is equal to that in the 'attempted' row we set the status
	// to core.StatusValid or core.StatusInvalid depending on if there is anything
	// in ValidationError and populate the ValidationRecord and ValidationError
	// fields.
	for pos := uint8(0); pos < 8; pos++ {
		if (am.Challenges>>pos)&1 == 1 {
			challType := uintToChallType[pos]
			challenge := &corepb.Challenge{
				Type:   challType,
				Status: string(core.StatusPending),
				Token:  base64.RawURLEncoding.EncodeToString(am.Token),
			}
			// If the challenge type matches the attempted type it must be either
			// valid or invalid and we need to populate extra fields.
			// Also, once any challenge has been attempted, we consider the other
			// challenges "gone" per https://tools.ietf.org/html/rfc8555#section-7.1.4
			if am.Attempted != nil {
				if uintToChallType[*am.Attempted] == challType {
					err := populateAttemptedFields(am, challenge)
					if err != nil {
						return nil, err
					}
					// Get the attemptedAt time and assign to the challenge validated time.
					var validated *timestamppb.Timestamp
					if am.AttemptedAt != nil {
						validated = timestamppb.New(*am.AttemptedAt)
					}
					challenge.Validated = validated
					pb.Challenges = append(pb.Challenges, challenge)
				}
			} else {
				// When no challenge has been attempted yet, all challenges are still
				// present.
				pb.Challenges = append(pb.Challenges, challenge)
			}
		}
	}
	return pb, nil
}

type keyHashModel struct {
	ID           int64
	KeyHash      []byte
	CertNotAfter time.Time
	CertSerial   string
}

var stringToSourceInt = map[string]int{
	"API":           1,
	"admin-revoker": 2,
}

// incidentModel represents a row in the 'incidents' table.
type incidentModel struct {
	ID          int64     `db:"id"`
	SerialTable string    `db:"serialTable"`
	URL         string    `db:"url"`
	RenewBy     time.Time `db:"renewBy"`
	Enabled     bool      `db:"enabled"`
}

func incidentModelToPB(i incidentModel) sapb.Incident {
	return sapb.Incident{
		Id:          i.ID,
		SerialTable: i.SerialTable,
		Url:         i.URL,
		RenewBy:     timestamppb.New(i.RenewBy),
		Enabled:     i.Enabled,
	}
}

// incidentSerialModel represents a row in an 'incident_*' table.
type incidentSerialModel struct {
	Serial         string     `db:"serial"`
	RegistrationID *int64     `db:"registrationID"`
	OrderID        *int64     `db:"orderID"`
	LastNoticeSent *time.Time `db:"lastNoticeSent"`
}

// crlEntryModel has just the certificate status fields necessary to construct
// an entry in a CRL.
type crlEntryModel struct {
	Serial        string            `db:"serial"`
	Status        core.OCSPStatus   `db:"status"`
	RevokedReason revocation.Reason `db:"revokedReason"`
	RevokedDate   time.Time         `db:"revokedDate"`
}

// orderFQDNSet contains the SHA256 hash of the lowercased, comma joined names
// from a new-order request, along with the corresponding orderID, the
// registration ID, and the order expiry. This is used to find
// existing orders for reuse.
type orderFQDNSet struct {
	ID             int64
	SetHash        []byte
	OrderID        int64
	RegistrationID int64
	Expires        time.Time
}

func addFQDNSet(ctx context.Context, db db.Inserter, idents identifier.ACMEIdentifiers, serial string, issued time.Time, expires time.Time) error {
	return db.Insert(ctx, &core.FQDNSet{
		SetHash: core.HashIdentifiers(idents),
		Serial:  serial,
		Issued:  issued,
		Expires: expires,
	})
}

// addOrderFQDNSet creates a new OrderFQDNSet row using the provided
// information. This function accepts a transaction so that the orderFqdnSet
// addition can take place within the order addition transaction. The caller is
// required to rollback the transaction if an error is returned.
func addOrderFQDNSet(
	ctx context.Context,
	db db.Inserter,
	idents identifier.ACMEIdentifiers,
	orderID int64,
	regID int64,
	expires time.Time) error {
	return db.Insert(ctx, &orderFQDNSet{
		SetHash:        core.HashIdentifiers(idents),
		OrderID:        orderID,
		RegistrationID: regID,
		Expires:        expires,
	})
}

// deleteOrderFQDNSet deletes a OrderFQDNSet row that matches the provided
// orderID. This function accepts a transaction so that the deletion can
// take place within the finalization transaction. The caller is required to
// rollback the transaction if an error is returned.
func deleteOrderFQDNSet(
	ctx context.Context,
	db db.Execer,
	orderID int64) error {

	result, err := db.ExecContext(ctx, `
	  DELETE FROM orderFqdnSets
		WHERE orderID = ?`,
		orderID)
	if err != nil {
		return err
	}
	rowsDeleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	// We always expect there to be an order FQDN set row for each
	// pending/processing order that is being finalized. If there isn't one then
	// something is amiss and should be raised as an internal server error
	if rowsDeleted == 0 {
		return berrors.InternalServerError("No orderFQDNSet exists to delete")
	}
	return nil
}

func addIssuedNames(ctx context.Context, queryer db.Execer, cert *x509.Certificate, isRenewal bool) error {
	if len(cert.DNSNames) == 0 && len(cert.IPAddresses) == 0 {
		return berrors.InternalServerError("certificate has no DNSNames or IPAddresses")
	}

	multiInserter, err := db.NewMultiInserter("issuedNames", []string{"reversedName", "serial", "notBefore", "renewal"})
	if err != nil {
		return err
	}
	for _, name := range cert.DNSNames {
		err = multiInserter.Add([]interface{}{
			reverseFQDN(name),
			core.SerialToString(cert.SerialNumber),
			cert.NotBefore.Truncate(24 * time.Hour),
			isRenewal,
		})
		if err != nil {
			return err
		}
	}
	for _, ip := range cert.IPAddresses {
		err = multiInserter.Add([]interface{}{
			ip.String(),
			core.SerialToString(cert.SerialNumber),
			cert.NotBefore.Truncate(24 * time.Hour),
			isRenewal,
		})
		if err != nil {
			return err
		}
	}
	return multiInserter.Insert(ctx, queryer)
}

// EncodeIssuedName translates a FQDN to/from the issuedNames table by reversing
// its dot-separated elements, and translates an IP address by returning its
// normal string form.
//
// This is for strings of ambiguous identifier values. If you know your string
// is a FQDN, use reverseFQDN(). If you have an IP address, use
// netip.Addr.String() or net.IP.String().
func EncodeIssuedName(name string) string {
	netIP, err := netip.ParseAddr(name)
	if err == nil {
		return netIP.String()
	}
	return reverseFQDN(name)
}

// reverseFQDN reverses the elements of a dot-separated FQDN.
//
// If your string might be an IP address, use EncodeIssuedName() instead.
func reverseFQDN(fqdn string) string {
	labels := strings.Split(fqdn, ".")
	for i, j := 0, len(labels)-1; i < j; i, j = i+1, j-1 {
		labels[i], labels[j] = labels[j], labels[i]
	}
	return strings.Join(labels, ".")
}

func addKeyHash(ctx context.Context, db db.Inserter, cert *x509.Certificate) error {
	if cert.RawSubjectPublicKeyInfo == nil {
		return errors.New("certificate has a nil RawSubjectPublicKeyInfo")
	}
	h := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	khm := &keyHashModel{
		KeyHash:      h[:],
		CertNotAfter: cert.NotAfter,
		CertSerial:   core.SerialToString(cert.SerialNumber),
	}
	return db.Insert(ctx, khm)
}

var blockedKeysColumns = "keyHash, added, source, comment"

// statusForOrder examines the status of a provided order's authorizations to
// determine what the overall status of the order should be. In summary:
//   - If the order has an error, the order is invalid
//   - If any of the order's authorizations are in any state other than
//     valid or pending, the order is invalid.
//   - If any of the order's authorizations are pending, the order is pending.
//   - If all of the order's authorizations are valid, and there is
//     a certificate serial, the order is valid.
//   - If all of the order's authorizations are valid, and we have began
//     processing, but there is no certificate serial, the order is processing.
//   - If all of the order's authorizations are valid, and we haven't begun
//     processing, then the order is status ready.
//
// An error is returned for any other case.
func statusForOrder(order *corepb.Order, authzValidityInfo []authzValidity, now time.Time) (string, error) {
	// Without any further work we know an order with an error is invalid
	if order.Error != nil {
		return string(core.StatusInvalid), nil
	}

	// If the order is expired the status is invalid and we don't need to get
	// order authorizations. Its important to exit early in this case because an
	// order that references an expired authorization will be itself have been
	// expired (because we match the order expiry to the associated authz expiries
	// in ra.NewOrder), and expired authorizations may be purged from the DB.
	// Because of this purging fetching the authz's for an expired order may
	// return fewer authz objects than expected, triggering a 500 error response.
	if order.Expires.AsTime().Before(now) {
		return string(core.StatusInvalid), nil
	}

	// If getAuthorizationStatuses returned a different number of authorization
	// objects than the order's slice of authorization IDs something has gone
	// wrong worth raising an internal error about.
	if len(authzValidityInfo) != len(order.V2Authorizations) {
		return "", berrors.InternalServerError(
			"getAuthorizationStatuses returned the wrong number of authorization statuses "+
				"(%d vs expected %d) for order %d",
			len(authzValidityInfo), len(order.V2Authorizations), order.Id)
	}

	// Keep a count of the authorizations seen
	pendingAuthzs := 0
	validAuthzs := 0
	otherAuthzs := 0
	expiredAuthzs := 0

	// Loop over each of the order's authorization objects to examine the authz status
	for _, info := range authzValidityInfo {
		switch uintToStatus[info.Status] {
		case core.StatusPending:
			pendingAuthzs++
		case core.StatusValid:
			validAuthzs++
		case core.StatusInvalid:
			otherAuthzs++
		case core.StatusDeactivated:
			otherAuthzs++
		case core.StatusRevoked:
			otherAuthzs++
		default:
			return "", berrors.InternalServerError(
				"Order is in an invalid state. Authz has invalid status %d",
				info.Status)
		}
		if info.Expires.Before(now) {
			expiredAuthzs++
		}
	}

	// An order is invalid if **any** of its authzs are invalid, deactivated,
	// revoked, or expired, see https://tools.ietf.org/html/rfc8555#section-7.1.6
	if otherAuthzs > 0 || expiredAuthzs > 0 {
		return string(core.StatusInvalid), nil
	}
	// An order is pending if **any** of its authzs are pending
	if pendingAuthzs > 0 {
		return string(core.StatusPending), nil
	}

	// An order is fully authorized if it has valid authzs for each of the order
	// identifiers
	fullyAuthorized := len(order.Identifiers) == validAuthzs

	// If the order isn't fully authorized we've encountered an internal error:
	// Above we checked for any invalid or pending authzs and should have returned
	// early. Somehow we made it this far but also don't have the correct number
	// of valid authzs.
	if !fullyAuthorized {
		return "", berrors.InternalServerError(
			"Order has the incorrect number of valid authorizations & no pending, " +
				"deactivated or invalid authorizations")
	}

	// If the order is fully authorized and the certificate serial is set then the
	// order is valid
	if fullyAuthorized && order.CertificateSerial != "" {
		return string(core.StatusValid), nil
	}

	// If the order is fully authorized, and we have began processing it, then the
	// order is processing.
	if fullyAuthorized && order.BeganProcessing {
		return string(core.StatusProcessing), nil
	}

	if fullyAuthorized && !order.BeganProcessing {
		return string(core.StatusReady), nil
	}

	return "", berrors.InternalServerError(
		"Order %d is in an invalid state. No state known for this order's "+
			"authorizations", order.Id)
}

// authzValidity is a subset of authzModel
type authzValidity struct {
	IdentifierType  uint8     `db:"identifierType"`
	IdentifierValue string    `db:"identifierValue"`
	Status          uint8     `db:"status"`
	Expires         time.Time `db:"expires"`
}

// getAuthorizationStatuses takes a sequence of authz IDs, and returns the
// status and expiration date of each of them.
func getAuthorizationStatuses(ctx context.Context, s db.Selector, ids []int64) ([]authzValidity, error) {
	var params []interface{}
	for _, id := range ids {
		params = append(params, id)
	}
	var validities []authzValidity
	_, err := s.Select(
		ctx,
		&validities,
		fmt.Sprintf("SELECT identifierType, identifierValue, status, expires FROM authz2 WHERE id IN (%s)",
			db.QuestionMarks(len(ids))),
		params...,
	)
	if err != nil {
		return nil, err
	}

	return validities, nil
}

// authzForOrder retrieves the authorization IDs for an order.
func authzForOrder(ctx context.Context, s db.Selector, orderID int64) ([]int64, error) {
	var v2IDs []int64
	_, err := s.Select(
		ctx,
		&v2IDs,
		"SELECT authzID FROM orderToAuthz2 WHERE orderID = ?",
		orderID,
	)
	return v2IDs, err
}

// crlShardModel represents one row in the crlShards table. The ThisUpdate and
// NextUpdate fields are pointers because they are NULL-able columns.
type crlShardModel struct {
	ID          int64      `db:"id"`
	IssuerID    int64      `db:"issuerID"`
	Idx         int        `db:"idx"`
	ThisUpdate  *time.Time `db:"thisUpdate"`
	NextUpdate  *time.Time `db:"nextUpdate"`
	LeasedUntil time.Time  `db:"leasedUntil"`
}

// revokedCertModel represents one row in the revokedCertificates table. It
// contains all of the information necessary to populate a CRL entry or OCSP
// response for the indicated certificate.
type revokedCertModel struct {
	ID            int64             `db:"id"`
	IssuerID      int64             `db:"issuerID"`
	Serial        string            `db:"serial"`
	NotAfterHour  time.Time         `db:"notAfterHour"`
	ShardIdx      int64             `db:"shardIdx"`
	RevokedDate   time.Time         `db:"revokedDate"`
	RevokedReason revocation.Reason `db:"revokedReason"`
}

// replacementOrderModel represents one row in the replacementOrders table. It
// contains all of the information necessary to link a renewal order to the
// certificate it replaces.
type replacementOrderModel struct {
	// ID is an auto-incrementing row ID.
	ID int64 `db:"id"`
	// Serial is the serial number of the replaced certificate.
	Serial string `db:"serial"`
	// OrderId is the ID of the replacement order
	OrderID int64 `db:"orderID"`
	// OrderExpiry is the expiry time of the new order. This is used to
	// determine if we can accept a new replacement order for the same Serial.
	OrderExpires time.Time `db:"orderExpires"`
	// Replaced is a boolean indicating whether the certificate has been
	// replaced, i.e. whether the new order has been finalized. Once this is
	// true, no new replacement orders can be accepted for the same Serial.
	Replaced bool `db:"replaced"`
}

// addReplacementOrder inserts or updates the replacementOrders row matching the
// provided serial with the details provided. This function accepts a
// transaction so that the insert or update takes place within the new order
// transaction.
func addReplacementOrder(ctx context.Context, db db.SelectExecer, serial string, orderID int64, orderExpires time.Time) error {
	var existingID []int64
	_, err := db.Select(ctx, &existingID, `
		SELECT id
		FROM replacementOrders
		WHERE serial = ?
		LIMIT 1`,
		serial,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checking for existing replacement order: %w", err)
	}

	if len(existingID) > 0 {
		// Update existing replacementOrder row.
		_, err = db.ExecContext(ctx, `
			UPDATE replacementOrders
			SET orderID = ?, orderExpires = ?
			WHERE id = ?`,
			orderID, orderExpires,
			existingID[0],
		)
		if err != nil {
			return fmt.Errorf("updating replacement order: %w", err)
		}
	} else {
		// Insert new replacementOrder row.
		_, err = db.ExecContext(ctx, `
			INSERT INTO replacementOrders (serial, orderID, orderExpires)
			VALUES (?, ?, ?)`,
			serial, orderID, orderExpires,
		)
		if err != nil {
			return fmt.Errorf("creating replacement order: %w", err)
		}
	}
	return nil
}

// setReplacementOrderFinalized sets the replaced flag for the replacementOrder
// row matching the provided orderID to true. This function accepts a
// transaction so that the update can take place within the finalization
// transaction.
func setReplacementOrderFinalized(ctx context.Context, db db.Execer, orderID int64) error {
	_, err := db.ExecContext(ctx, `
		UPDATE replacementOrders
		SET replaced = true
		WHERE orderID = ?
		LIMIT 1`,
		orderID,
	)
	if err != nil {
		return err
	}
	return nil
}

type identifierModel struct {
	Type  uint8  `db:"identifierType"`
	Value string `db:"identifierValue"`
}

func newIdentifierModelFromPB(pb *corepb.Identifier) (identifierModel, error) {
	idType, ok := identifierTypeToUint[pb.Type]
	if !ok {
		return identifierModel{}, fmt.Errorf("unsupported identifier type %q", pb.Type)
	}

	return identifierModel{
		Type:  idType,
		Value: pb.Value,
	}, nil
}

func newPBFromIdentifierModel(id identifierModel) (*corepb.Identifier, error) {
	idType, ok := uintToIdentifierType[id.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported identifier type %d", id.Type)
	}

	return &corepb.Identifier{
		Type:  string(idType),
		Value: id.Value,
	}, nil
}

func newIdentifierModelsFromPB(pbs []*corepb.Identifier) ([]identifierModel, error) {
	ids := make([]identifierModel, 0, len(pbs))
	for _, pb := range pbs {
		id, err := newIdentifierModelFromPB(pb)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func newPBFromIdentifierModels(ids []identifierModel) (*sapb.Identifiers, error) {
	pbs := make([]*corepb.Identifier, 0, len(ids))
	for _, id := range ids {
		pb, err := newPBFromIdentifierModel(id)
		if err != nil {
			return nil, err
		}
		pbs = append(pbs, pb)
	}
	return &sapb.Identifiers{Identifiers: pbs}, nil
}

// buildIdentifierQueryConditions takes a slice of identifiers and returns a
// string (conditions to use within the prepared statement) and a slice of anys
// (arguments for the prepared statement), both to use within a WHERE clause for
// queries against the authz2 table.
//
// Although this function takes user-controlled input, it does not include any
// of that input directly in the returned SQL string. The resulting string
// contains only column names, boolean operators, and questionmark placeholders.
func buildIdentifierQueryConditions(idents identifier.ACMEIdentifiers) (string, []any) {
	if len(idents) == 0 {
		// No identifier values to check.
		return "FALSE", []any{}
	}

	identsByType := map[identifier.IdentifierType][]string{}
	for _, id := range idents {
		identsByType[id.Type] = append(identsByType[id.Type], id.Value)
	}

	var conditions []string
	var args []any
	for idType, idValues := range identsByType {
		conditions = append(conditions,
			fmt.Sprintf("identifierType = ? AND identifierValue IN (%s)",
				db.QuestionMarks(len(idValues)),
			),
		)
		args = append(args, identifierTypeToUint[string(idType)])
		for _, idValue := range idValues {
			args = append(args, idValue)
		}
	}

	return strings.Join(conditions, " OR "), args
}

// pausedModel represents a row in the paused table. It contains the
// registrationID of the paused account, the time the (account, identifier) pair
// was paused, and the time the pair was unpaused. The UnpausedAt field is
// nullable because the pair may not have been unpaused yet. A pair is
// considered paused if there is a matching row in the paused table with a NULL
// UnpausedAt time.
type pausedModel struct {
	identifierModel
	RegistrationID int64      `db:"registrationID"`
	PausedAt       time.Time  `db:"pausedAt"`
	UnpausedAt     *time.Time `db:"unpausedAt"`
}

type overrideModel struct {
	LimitEnum int64     `db:"limitEnum"`
	BucketKey string    `db:"bucketKey"`
	Comment   string    `db:"comment"`
	PeriodNS  int64     `db:"periodNS"`
	Count     int64     `db:"count"`
	Burst     int64     `db:"burst"`
	UpdatedAt time.Time `db:"updatedAt"`
	Enabled   bool      `db:"enabled"`
}

func overrideModelForPB(pb *sapb.RateLimitOverride, updatedAt time.Time, enabled bool) overrideModel {
	return overrideModel{
		LimitEnum: pb.LimitEnum,
		BucketKey: pb.BucketKey,
		Comment:   pb.Comment,
		PeriodNS:  pb.Period.AsDuration().Nanoseconds(),
		Count:     pb.Count,
		Burst:     pb.Burst,
		UpdatedAt: updatedAt,
		Enabled:   enabled,
	}
}

func newPBFromOverrideModel(m *overrideModel) *sapb.RateLimitOverride {
	return &sapb.RateLimitOverride{
		LimitEnum: m.LimitEnum,
		BucketKey: m.BucketKey,
		Comment:   m.Comment,
		Period:    durationpb.New(time.Duration(m.PeriodNS)),
		Count:     m.Count,
		Burst:     m.Burst,
	}
}
