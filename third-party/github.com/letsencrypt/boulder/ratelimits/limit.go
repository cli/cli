package ratelimits

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/strictyaml"
)

// errLimitDisabled indicates that the limit name specified is valid but is not
// currently configured.
var errLimitDisabled = errors.New("limit disabled")

// LimitConfig defines the exportable configuration for a rate limit or a rate
// limit override, without a `limit`'s internal fields.
//
// The zero value of this struct is invalid, because some of the fields must be
// greater than zero.
type LimitConfig struct {
	// Burst specifies maximum concurrent allowed requests at any given time. It
	// must be greater than zero.
	Burst int64

	// Count is the number of requests allowed per period. It must be greater
	// than zero.
	Count int64

	// Period is the duration of time in which the count (of requests) is
	// allowed. It must be greater than zero.
	Period config.Duration
}

type LimitConfigs map[string]*LimitConfig

// limit defines the configuration for a rate limit or a rate limit override.
//
// The zero value of this struct is invalid, because some of the fields must
// be greater than zero.
type limit struct {
	// burst specifies maximum concurrent allowed requests at any given time. It
	// must be greater than zero.
	burst int64

	// count is the number of requests allowed per period. It must be greater
	// than zero.
	count int64

	// period is the duration of time in which the count (of requests) is
	// allowed. It must be greater than zero.
	period config.Duration

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

	// isOverride is true if the limit is an override.
	isOverride bool
}

// precompute calculates the emissionInterval and burstOffset for the limit.
func (l *limit) precompute() {
	l.emissionInterval = l.period.Nanoseconds() / l.count
	l.burstOffset = l.emissionInterval * l.burst
}

func validateLimit(l *limit) error {
	if l.burst <= 0 {
		return fmt.Errorf("invalid burst '%d', must be > 0", l.burst)
	}
	if l.count <= 0 {
		return fmt.Errorf("invalid count '%d', must be > 0", l.count)
	}
	if l.period.Duration <= 0 {
		return fmt.Errorf("invalid period '%s', must be > 0", l.period)
	}
	return nil
}

type limits map[string]*limit

// loadDefaults marshals the defaults YAML file at path into a map of limits.
func loadDefaults(path string) (LimitConfigs, error) {
	lm := make(LimitConfigs)
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
	LimitConfig `yaml:",inline"`
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

// parseOverrideLimits validates a YAML list of override limits. It must be
// formatted as a list of maps, where each map has a single key representing the
// limit name and a value that is a map containing the limit fields and an
// additional 'ids' field that is a list of ids that this override applies to.
func parseOverrideLimits(newOverridesYAML overridesYAML) (limits, error) {
	parsed := make(limits)

	for _, ov := range newOverridesYAML {
		for k, v := range ov {
			name, ok := stringToName[k]
			if !ok {
				return nil, fmt.Errorf("unrecognized name %q in override limit, must be one of %v", k, limitNames)
			}

			lim := &limit{
				burst:      v.Burst,
				count:      v.Count,
				period:     v.Period,
				name:       name,
				isOverride: true,
			}
			lim.precompute()

			err := validateLimit(lim)
			if err != nil {
				return nil, fmt.Errorf("validating override limit %q: %w", k, err)
			}

			for _, entry := range v.Ids {
				id := entry.Id
				err = validateIdForName(name, id)
				if err != nil {
					return nil, fmt.Errorf(
						"validating name %s and id %q for override limit %q: %w", name, id, k, err)
				}

				// We interpret and compute the override values for two rate
				// limits, since they're not nice to ask for in a config file.
				switch name {
				case CertificatesPerDomain:
					// Convert IP addresses to their covering /32 (IPv4) or /64
					// (IPv6) prefixes in CIDR notation.
					ip, err := netip.ParseAddr(id)
					if err == nil {
						prefix, err := coveringPrefix(ip)
						if err != nil {
							return nil, fmt.Errorf(
								"computing prefix for IP address %q: %w", id, err)
						}
						id = prefix.String()
					}
				case CertificatesPerFQDNSet:
					// Compute the hash of a comma-separated list of identifier
					// values.
					var idents identifier.ACMEIdentifiers
					for _, value := range strings.Split(id, ",") {
						ip, err := netip.ParseAddr(value)
						if err == nil {
							idents = append(idents, identifier.NewIP(ip))
						} else {
							idents = append(idents, identifier.NewDNS(value))
						}
					}
					id = fmt.Sprintf("%x", core.HashIdentifiers(idents))
				}

				parsed[joinWithColon(name.EnumString(), id)] = lim
			}
		}
	}
	return parsed, nil
}

// parseDefaultLimits validates a map of default limits and rekeys it by 'Name'.
func parseDefaultLimits(newDefaultLimits LimitConfigs) (limits, error) {
	parsed := make(limits)

	for k, v := range newDefaultLimits {
		name, ok := stringToName[k]
		if !ok {
			return nil, fmt.Errorf("unrecognized name %q in default limit, must be one of %v", k, limitNames)
		}

		lim := &limit{
			burst:  v.Burst,
			count:  v.Count,
			period: v.Period,
			name:   name,
		}

		err := validateLimit(lim)
		if err != nil {
			return nil, fmt.Errorf("parsing default limit %q: %w", k, err)
		}

		lim.precompute()
		parsed[name.EnumString()] = lim
	}
	return parsed, nil
}

type limitRegistry struct {
	// defaults stores default limits by 'name'.
	defaults limits

	// overrides stores override limits by 'name:id'.
	overrides limits
}

func newLimitRegistryFromFiles(defaults, overrides string) (*limitRegistry, error) {
	defaultsData, err := loadDefaults(defaults)
	if err != nil {
		return nil, err
	}

	if overrides == "" {
		return newLimitRegistry(defaultsData, nil)
	}

	overridesData, err := loadOverrides(overrides)
	if err != nil {
		return nil, err
	}

	return newLimitRegistry(defaultsData, overridesData)
}

func newLimitRegistry(defaults LimitConfigs, overrides overridesYAML) (*limitRegistry, error) {
	regDefaults, err := parseDefaultLimits(defaults)
	if err != nil {
		return nil, err
	}

	regOverrides, err := parseOverrideLimits(overrides)
	if err != nil {
		return nil, err
	}

	return &limitRegistry{
		defaults:  regDefaults,
		overrides: regOverrides,
	}, nil
}

// getLimit returns the limit for the specified by name and bucketKey, name is
// required, bucketKey is optional. If bucketkey is empty, the default for the
// limit specified by name is returned. If no default limit exists for the
// specified name, errLimitDisabled is returned.
func (l *limitRegistry) getLimit(name Name, bucketKey string) (*limit, error) {
	if !name.isValid() {
		// This should never happen. Callers should only be specifying the limit
		// Name enums defined in this package.
		return nil, fmt.Errorf("specified name enum %q, is invalid", name)
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
	return nil, errLimitDisabled
}
