package ratelimits

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/strictyaml"
)

// errLimitDisabled indicates that the limit name specified is valid but is not
// currently configured.
var errLimitDisabled = errors.New("limit disabled")

type limit struct {
	// Burst specifies maximum concurrent allowed requests at any given time. It
	// must be greater than zero.
	Burst int64

	// Count is the number of requests allowed per period. It must be greater
	// than zero.
	Count int64

	// Period is the duration of time in which the count (of requests) is
	// allowed. It must be greater than zero.
	Period config.Duration

	// name is the name of the limit. It must be one of the Name enums defined
	// in this package.
	name Name

	// emissionInterval is the interval, in nanoseconds, at which tokens are
	// added to a bucket (period / count). This is also the steady-state rate at
	// which requests can be made without being denied even once the burst has
	// been exhausted. This is precomputed to avoid doing the same calculation
	// on every request.
	emissionInterval int64

	// burstOffset is the duration of time, in nanoseconds, it takes for a
	// bucket to go from empty to full (burst * (period / count)). This is
	// precomputed to avoid doing the same calculation on every request.
	burstOffset int64

	// overrideKey is the key used to look up this limit in the overrides map.
	overrideKey string
}

// isOverride returns true if the limit is an override.
func (l *limit) isOverride() bool {
	return l.overrideKey != ""
}

// precompute calculates the emissionInterval and burstOffset for the limit.
func (l *limit) precompute() {
	l.emissionInterval = l.Period.Nanoseconds() / l.Count
	l.burstOffset = l.emissionInterval * l.Burst
}

func validateLimit(l limit) error {
	if l.Burst <= 0 {
		return fmt.Errorf("invalid burst '%d', must be > 0", l.Burst)
	}
	if l.Count <= 0 {
		return fmt.Errorf("invalid count '%d', must be > 0", l.Count)
	}
	if l.Period.Duration <= 0 {
		return fmt.Errorf("invalid period '%s', must be > 0", l.Period)
	}
	return nil
}

type limits map[string]limit

// loadDefaults marshals the defaults YAML file at path into a map of limits.
func loadDefaults(path string) (limits, error) {
	lm := make(limits)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = strictyaml.Unmarshal(data, &lm)
	if err != nil {
		return nil, err
	}
	return lm, nil
}

type overrideYAML struct {
	limit `yaml:",inline"`
	// Ids is a list of ids that this override applies to.
	Ids []struct {
		Id string `yaml:"id"`
		// Comment is an optional field that can be used to provide additional
		// context for the override.
		Comment string `yaml:"comment,omitempty"`
	} `yaml:"ids"`
}

type overridesYAML []map[string]overrideYAML

// loadOverrides marshals the YAML file at path into a map of overrides.
func loadOverrides(path string) (overridesYAML, error) {
	ov := overridesYAML{}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = strictyaml.Unmarshal(data, &ov)
	if err != nil {
		return nil, err
	}
	return ov, nil
}

// parseOverrideNameId is broken out for ease of testing.
func parseOverrideNameId(key string) (Name, string, error) {
	if !strings.Contains(key, ":") {
		// Avoids a potential panic in strings.SplitN below.
		return Unknown, "", fmt.Errorf("invalid override %q, must be formatted 'name:id'", key)
	}
	nameAndId := strings.SplitN(key, ":", 2)
	nameStr := nameAndId[0]
	if nameStr == "" {
		return Unknown, "", fmt.Errorf("empty name in override %q, must be formatted 'name:id'", key)
	}

	name, ok := stringToName[nameStr]
	if !ok {
		return Unknown, "", fmt.Errorf("unrecognized name %q in override limit %q, must be one of %v", nameStr, key, limitNames)
	}
	id := nameAndId[1]
	if id == "" {
		return Unknown, "", fmt.Errorf("empty id in override %q, must be formatted 'name:id'", key)
	}
	return name, id, nil
}

// loadAndParseOverrideLimits loads override limits from YAML. The YAML file
// must be formatted as a list of maps, where each map has a single key
// representing the limit name and a value that is a map containing the limit
// fields and an additional 'ids' field that is a list of ids that this override
// applies to.
func loadAndParseOverrideLimits(path string) (limits, error) {
	fromFile, err := loadOverrides(path)
	if err != nil {
		return nil, err
	}
	parsed := make(limits)

	for _, ov := range fromFile {
		for k, v := range ov {
			err = validateLimit(v.limit)
			if err != nil {
				return nil, fmt.Errorf("validating override limit %q: %w", k, err)
			}
			name, ok := stringToName[k]
			if !ok {
				return nil, fmt.Errorf("unrecognized name %q in override limit, must be one of %v", k, limitNames)
			}
			v.limit.name = name

			for _, entry := range v.Ids {
				limit := v.limit
				id := entry.Id
				err = validateIdForName(name, id)
				if err != nil {
					return nil, fmt.Errorf(
						"validating name %s and id %q for override limit %q: %w", name, id, k, err)
				}
				limit.overrideKey = joinWithColon(name.EnumString(), id)
				if name == CertificatesPerFQDNSet {
					// FQDNSet hashes are not a nice thing to ask for in a
					// config file, so we allow the user to specify a
					// comma-separated list of FQDNs and compute the hash here.
					id = fmt.Sprintf("%x", core.HashNames(strings.Split(id, ",")))
				}
				limit.precompute()
				parsed[joinWithColon(name.EnumString(), id)] = limit
			}
		}
	}
	return parsed, nil
}

// loadAndParseDefaultLimits loads default limits from YAML, validates them, and
// parses them into a map of limits keyed by 'Name'.
func loadAndParseDefaultLimits(path string) (limits, error) {
	fromFile, err := loadDefaults(path)
	if err != nil {
		return nil, err
	}
	parsed := make(limits, len(fromFile))

	for k, v := range fromFile {
		err := validateLimit(v)
		if err != nil {
			return nil, fmt.Errorf("parsing default limit %q: %w", k, err)
		}
		name, ok := stringToName[k]
		if !ok {
			return nil, fmt.Errorf("unrecognized name %q in default limit, must be one of %v", k, limitNames)
		}
		v.name = name
		v.precompute()
		parsed[name.EnumString()] = v
	}
	return parsed, nil
}

type limitRegistry struct {
	// defaults stores default limits by 'name'.
	defaults limits

	// overrides stores override limits by 'name:id'.
	overrides limits
}

func newLimitRegistry(defaults, overrides string) (*limitRegistry, error) {
	var err error
	registry := &limitRegistry{}
	registry.defaults, err = loadAndParseDefaultLimits(defaults)
	if err != nil {
		return nil, err
	}

	if overrides == "" {
		// No overrides specified, initialize an empty map.
		registry.overrides = make(limits)
		return registry, nil
	}

	registry.overrides, err = loadAndParseOverrideLimits(overrides)
	if err != nil {
		return nil, err
	}

	return registry, nil
}

// getLimit returns the limit for the specified by name and bucketKey, name is
// required, bucketKey is optional. If bucketkey is empty, the default for the
// limit specified by name is returned. If no default limit exists for the
// specified name, errLimitDisabled is returned.
func (l *limitRegistry) getLimit(name Name, bucketKey string) (limit, error) {
	if !name.isValid() {
		// This should never happen. Callers should only be specifying the limit
		// Name enums defined in this package.
		return limit{}, fmt.Errorf("specified name enum %q, is invalid", name)
	}
	if bucketKey != "" {
		// Check for override.
		ol, ok := l.overrides[bucketKey]
		if ok {
			return ol, nil
		}
	}
	dl, ok := l.defaults[name.EnumString()]
	if ok {
		return dl, nil
	}
	return limit{}, errLimitDisabled
}
