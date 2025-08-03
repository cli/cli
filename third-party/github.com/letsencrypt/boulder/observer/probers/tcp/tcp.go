package tcp

import (
	"context"
	"time"

	"github.com/letsencrypt/boulder/observer/obsdialer"
)

type TCPProbe struct {
	hostport string
}

// Name returns a string that uniquely identifies the monitor.

func (p TCPProbe) Name() string {
	return p.hostport
}

// Kind returns a name that uniquely identifies the `Kind` of `Prober`.
func (p TCPProbe) Kind() string {
	return "TCP"
}

// Probe performs the configured TCP dial.
func (p TCPProbe) Probe(timeout time.Duration) (bool, time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	c, err := obsdialer.Dialer.DialContext(ctx, "tcp", p.hostport)
	if err != nil {
		return false, time.Since(start)
	}
	c.Close()
	return true, time.Since(start)
}
