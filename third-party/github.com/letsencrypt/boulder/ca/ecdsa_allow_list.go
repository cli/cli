package ca

import (
	"os"

	"github.com/letsencrypt/boulder/strictyaml"
)

// ECDSAAllowList acts as a container for a map of Registration IDs.
type ECDSAAllowList struct {
	regIDsMap map[int64]bool
}

// permitted checks if ECDSA issuance is permitted for the specified
// Registration ID.
func (e *ECDSAAllowList) permitted(regID int64) bool {
	return e.regIDsMap[regID]
}

func makeRegIDsMap(regIDs []int64) map[int64]bool {
	regIDsMap := make(map[int64]bool)
	for _, regID := range regIDs {
		regIDsMap[regID] = true
	}
	return regIDsMap
}

// NewECDSAAllowListFromFile is exported to allow `boulder-ca` to construct a
// new `ECDSAAllowList` object. It returns the ECDSAAllowList, the size of allow
// list after attempting to load it (for CA logging purposes so inner fields don't need to be exported), or an error.
func NewECDSAAllowListFromFile(filename string) (*ECDSAAllowList, int, error) {
	configBytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, 0, err
	}

	var regIDs []int64
	err = strictyaml.Unmarshal(configBytes, &regIDs)
	if err != nil {
		return nil, 0, err
	}

	allowList := &ECDSAAllowList{regIDsMap: makeRegIDsMap(regIDs)}
	return allowList, len(allowList.regIDsMap), nil
}
