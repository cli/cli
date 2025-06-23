This directory contains two utilities for checking ocsp.

"checkocsp" is a command-line tool to check the OCSP response for a certificate
or a list of certificates.

"ocsp_forever" is a similar tool that runs as a daemon and continually checks
OCSP for a list of certificates, and exports Prometheus stats.

Both of these are useful for monitoring a Boulder instance. "checkocsp" is also
useful for debugging.
