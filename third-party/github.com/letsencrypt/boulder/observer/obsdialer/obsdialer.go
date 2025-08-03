// package obsdialer contains a custom dialer for use in observers.
package obsdialer

import "net"

// Dialer is a custom dialer for use in observers. It disables IPv6-to-IPv4
// fallback so we don't mask failures of IPv6 connectivity.
var Dialer = net.Dialer{
	FallbackDelay: -1, // Disable IPv6-to-IPv4 fallback
}
