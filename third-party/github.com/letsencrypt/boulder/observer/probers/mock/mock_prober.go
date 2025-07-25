package probers

import (
	"time"

	"github.com/letsencrypt/boulder/config"
)

type MockProber struct {
	name    string
	kind    string
	took    config.Duration
	success bool
}

func (p MockProber) Name() string {
	return p.name
}

func (p MockProber) Kind() string {
	return p.kind
}

func (p MockProber) Probe(timeout time.Duration) (bool, time.Duration) {
	return p.success, p.took.Duration
}
