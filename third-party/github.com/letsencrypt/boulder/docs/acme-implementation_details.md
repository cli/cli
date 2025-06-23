# Boulder implementation details

The ACME specification ([RFC 8555]) clearly dictates what Clients and Servers
must do to properly implement the protocol.

The specification is intentionally silent, or vague, on certain points to give
developers freedom in making certain decisions or to follow guidance from other
RFCs.  Due to this, two ACME Servers might fully conform to the RFC but behave
slightly differently.  ACME Clients should not "over-fit" on Boulder or the 
Let's Encrypt production service, and aim to be compatible with a wide range of
ACME Servers, including the [Pebble](https://github.com/letsencrypt/pebble)
test server.

The following items are a partial listing of RFC-conformant design decisions
Boulder and/or LetsEncrypt have made.  This listing is not complete, and is
based on known details which have caused issues for developers in the past. This
listing may not reflect the current status of Boulder or the configuration of
LetsEncrypt's production instance and is provided only as a reference for client
developers.

Please note: these design implementation decisions are fully conformant with the
RFC specification and are not
[divergences](https://github.com/letsencrypt/boulder/blob/main/docs/acme-divergences.md).


## Object Reuse

The ACME specification does not prohibit certain objects to be re-used.

### Authorization

Boulder may recycle previously "valid" or "pending" `Authorizations` for a given
`Account` when creating a new `Order`.

### Order

Boulder may return a previously created `Order` when a given `Account` submits
a new `Order` that is identical to a previously submitted `Order` that is in
the "pending" or "ready" state.

## Alternate Chains

The production Boulder instance for LetsEncrypt in enabled with support for
Alternate chains.


## Certificate Request Domains

The RFC states the following:

	The CSR MUST indicate the exact same
	set of requested identifiers as the initial newOrder request.
	Identifiers of type "dns" MUST appear either in the commonName
	portion of the requested subject name or in an extensionRequest
	attribute [RFC2985] requesting a subjectAltName extension, or both.

Boulder requires all domains to be specified in the `subjectAltName` 
extension, and will reject a CSR if a domain specified in the `commonName` is
not present in the  `subjectAltName`.  Additionally, usage of the `commonName`
was previously deprecated by the CA/B Forum and in earlier RFCs.

For more information on this see [Pebble Issue #304](https://github.com/letsencrypt/pebble/issues/304)
and [Pebble Issue #233](https://github.com/letsencrypt/pebble/issues/233).


## RSA Key Size

The ACME specification is silent as to minimum key size.
The [CA/Browser Forum](https://cabforum.org/) sets the key size requirements
which LetsEncrypt adheres to.

Effective 2020-09-17, LetsEncrypt further requires all RSA keys for end-entity
(leaf) certificates have a modulus of length 2048, 3072, or 4096. Other CAs may
or may not have the same restricted set of supported RSA key sizes.
For more information 
[read the Official Announcement](https://community.letsencrypt.org/t/issuing-for-common-rsa-key-sizes-only/133839).
