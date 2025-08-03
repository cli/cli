package crl

import (
	"encoding/json"
	"math/big"
	"time"

	"github.com/letsencrypt/boulder/issuance"
)

// number represents the 'crlNumber' field of a CRL. It must be constructed by
// calling `Number()`.
type number *big.Int

// Number derives the 'CRLNumber' field for a CRL from the value of the
// 'thisUpdate' field provided as a `time.Time`.
func Number(thisUpdate time.Time) number {
	// Per RFC 5280 Section 5.2.3, 'CRLNumber' is a monotonically increasing
	// sequence number for a given CRL scope and CRL that MUST be at most 20
	// octets. A 64-bit (8-byte) integer will never exceed that requirement, but
	// lets us guarantee that the CRL Number is always increasing without having
	// to store or look up additional state.
	return number(big.NewInt(thisUpdate.UnixNano()))
}

// id is a unique identifier for a CRL which is primarily used for logging. This
// identifier is composed of the 'Issuer', 'CRLNumber', and the shard index
// (e.g. {"issuerID": 123, "crlNum": 456, "shardIdx": 78}). It must be constructed
// by calling `Id()`.
type id string

// Id is a utility function which constructs a new `id`.
func Id(issuerID issuance.NameID, shardIdx int, crlNumber number) id {
	type info struct {
		IssuerID  issuance.NameID `json:"issuerID"`
		ShardIdx  int             `json:"shardIdx"`
		CRLNumber number          `json:"crlNumber"`
	}
	jsonBytes, err := json.Marshal(info{issuerID, shardIdx, crlNumber})
	if err != nil {
		panic(err)
	}
	return id(jsonBytes)
}
