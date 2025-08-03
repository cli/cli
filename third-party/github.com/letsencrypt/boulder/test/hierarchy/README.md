# Boulder Test Hierarchy

This directory contains certificates which are analogues of Let's Encrypt's
active hierarchy. These are useful for ensuring that our tests cover all of
our actual situations, such as cross-signed intermediates, cross-signed roots,
both RSA and ECDSA roots and intermediates, and having issuance chains with
more than one intermediate in them. Also included are a selection of fake
end-entity certificates, issued from each of the intermediates. This directory
does not include private keys for the roots, as Boulder should never perform
any operations which require access to root private keys.

## Usage

These certificates (particularly their subject info and public key info) are
subject to change at any time. Values derived from these certificates, such as
their `Serial`, `IssuerID`, `Fingerprint`, or `IssuerNameID` should never be
hard-coded in tests or mocks. If you need to assert facts about those values
in a test, load the cert from disk and compute those values dynamically.

In general, loading and using one of these certificates for a test might
look like:

```go
ee, _ := CA.IssuePrecertificate(...)
cert, _ := issuance.LoadCertificate("test/hierarchy/int-e1.cert.pem")
test.AssertEqual(t, issuance.GetIssuerNameID(ee), issuer.NameID())
```
