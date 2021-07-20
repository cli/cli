// package httpunix provides an http.RoundTripper which dials a server via a unix socket.
package httpunix

import (
	"net"
	"net/http"
)

// NewRoundTripper returns an http.RoundTripper which sends requests via a unix
// socket at socketPath.
func NewRoundTripper(socketPath string) http.RoundTripper {
	dial := func(network, addr string) (net.Conn, error) {
		return net.Dial("unix", socketPath)
	}

	return &http.Transport{
		Dial:              dial,
		DialTLS:           dial,
		DisableKeepAlives: true,
	}
}
