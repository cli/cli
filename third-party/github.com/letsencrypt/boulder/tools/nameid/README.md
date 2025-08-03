# Overview

The `nameid` tool displays a statistically-unique small ID which can be computed
from both CA and end-entity certs to link them together into a validation chain.
It is computed as a truncated hash over the issuer Subject Name bytes. It should
only be used on issuer certificates e.g. [when the CA boolean is
asserted](https://www.rfc-editor.org/rfc/rfc5280#section-4.2.1.9) which in the
`//crypto/x509` `Certificate` struct is `IsCA: true`.

For implementation details, please see the `//issuance` package
[here](https://github.com/letsencrypt/boulder/blob/30c6e592f7f6825c2782b6a7d5da566979445674/issuance/issuer.go#L79-L83).

# Usage

```
# Display help
go run ./tools/nameid/nameid.go -h

# Output the certificate path and nameid, one per line
go run ./tools/nameid/nameid.go /path/to/cert1.pem /path/to/cert2.pem ...

# Output just the nameid, one per line
go run ./tools/nameid/nameid.go -s /path/to/cert1.pem /path/to/cert2.pem ...
```
