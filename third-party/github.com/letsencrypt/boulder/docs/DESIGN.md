# Boulder flow diagrams

Boulder is built out of multiple components that can be deployed in different
security contexts.

In order for you to understand how Boulder works and ensure it's working correctly,
this document lays out how various operations flow through boulder. It is
expected you're already familiar with the [ACME
protocol](https://github.com/ietf-wg-acme/acme). We show a diagram of how calls
go between Boulder components, and provide notes on what each
component does to help the process along.  Each step is in its own subsection
below, in roughly the order that they happen in certificate issuance for both
ACME v1 and ACME v2.

A couple of notes:

* For simplicity, we do not show interactions with the Storage Authority.
  The SA simply acts as a common data store for the various components.  It
  is written to by the RA (registrations and authorizations) and the CA
  (certificates), and read by WFEv2, RA, and CA.

* The interactions shown in the diagrams are the calls that go between
  components.  These calls are done via [gRPC](https://grpc.io/).

* In various places the Boulder implementation of ACME diverges from the current
  RFC draft. These divergences are documented in [docs/acme-divergences.md](https://github.com/letsencrypt/boulder/blob/main/docs/acme-divergences.md).

* The RFC draft leaves many decisions on it's implementation to the discretion
  of server and client developers. The ACME RFC is also silent on some matters,
  as the relevant implementation details would be influenced by other RFCs.
  Several of these details and decisions particular to Boulder are documented in [docs/acme-implementation_details.md](https://github.com/letsencrypt/boulder/blob/main/docs/acme-implementation_details.md).

* We focus on the primary ACME operations and do not include all possible
  interactions (e.g. account key change, authorization deactivation)

* We presently ignore the POST-as-GET construction introduced in
  [draft-15](https://tools.ietf.org/html/draft-ietf-acme-acme-15) and show
  unauthenticated GET requests for ACME v2 operations.

## New Account/Registration

ACME v2:

```
1: Client ---newAccount---> WFEv2
2:                          WFEv2 ---NewRegistration--> RA
3:                          WFEv2 <-------return------- RA
4: Client <---------------- WFEv2
```

Notes:

* 1-2: WFEv2 does the following:
  * Verify that the request is a POST
  * Verify the JWS signature on the POST body
  * Parse the registration/account object
  * Filters illegal fields from the registration/account object
  * We ignore the WFEv2 possibly returning early based on the OnlyReturnExisting
    flag to simplify explanation.

* 2-3: RA does the following:
  * Verify that the registered account key is acceptable
  * Create a new registration/account and add the client's information
  * Store the registration/account (which gives it an ID)
  * Return the registration/account as stored

* 3-4: WFEv2 does the following:
  * Return the registration/account, with a unique URL


## Updated Registration

ACME v2:

```
1: Client ---acct--> WFEv2
2:                   WFEv2 ---UpdateRegistration--> RA
3:                   WFEv2 <--------return--------- RA
4: Client <--------- WFEv2
```

* 1-2: WFEv2 does the following:
  * Verify that the request is a POST
  * Verify the JWS signature on the POST body
  * Verify that the JWS signature is by a registered key
  * Verify that the JWS key matches the registration for the URL
  * WFEv2: Verify that the account agrees to the terms of service
  * Parse the registration/account object
  * Filter illegal fields from the registration/account object

* 2-3: RA does the following:
  * Merge the update into the existing registration/account
  * Store the updated registration/account
  * Return the updated registration/account

* 3-4: WFEv2 does the following:
  * Return the updated registration/account

## New Authorization (ACME v1 Only)

ACME v2:
We do not implement "pre-authorization" and the newAuthz endpoint for ACME v2.
Clients are expected to get authorizations by way of creating orders.

* 1-2: WFEv2 does the following:
  * Verify that the request is a POST
  * Verify the JWS signature on the POST body
  * Verify that the JWS signature is by a registered key
  * Verify that the client has indicated agreement to terms
  * Parse the initial authorization object

* 2-3: RA does the following:
  * Verify that the requested identifier is allowed by policy
  * Verify that the CAA policy for for each DNS identifier allows issuance
  * Create challenges as required by policy
  * Construct URIs for the challenges
  * Store the authorization

* 3-4: WFEv2 does the following:
  * Return the authorization, with a unique URL

## New Order (ACME v2 Only)

ACME v2:
```
1: Client ---newOrder---> WFEv2
2:                        WFEv2 -------NewOrder------> RA
3:                        WFEv2 <-------return-------- RA
4: Client <-------------- WFEv2
```

* 1-2: WFEv2 does the following:
  * Verify that the request is a POST
  * Verify the JWS signature on the POST body
  * Verify that the JWS signature is by a registered key
  * Parse the initial order object and identifiers

* 2-3: RA does the following:
  * Verify that the requested identifiers are allowed by policy
  * Create authorizations and challenges as required by policy
  * Construct URIs for the challenges and authorizations
  * Store the authorizations and challenges

* 3-4: WFEv2 does the following:
  * Return the order object, containing authorizations and challenges, with
  a unique URL

## Challenge Response

ACME v2:

```
1: Client ---chal--> WFEv2
2:                   WFEv2 ---UpdateAuthorization--> RA
3:                                                   RA ---PerformValidation--> VA
4: Client <~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~> VA
5:                                                   RA <-------return--------- VA
6:                   WFEv2 <--------return---------- RA
7: Client <--------- WFEv2
```

* 1-2: WFEv2 does the following:
  * Look up the referenced authorization object
  * Look up the referenced challenge within the authorization object
  * Verify that the request is a POST
  * Verify the JWS signature on the POST body
  * Verify that the JWS signature is by a registered key
  * Verify that the JWS key corresponds to the authorization

* 2-3: RA does the following:
  * Store the updated authorization object

* 3-4: VA does the following:
  * Dispatch a goroutine to do validation

* 4-5: RA does the following:
  * Return the updated authorization object

* 5-6: WFEv2 does the following:
  * Return the updated authorization object

* 6: VA does the following:
  * Validate domain control according to the challenge responded to
  * Notify the RA of the result

* 6-7: RA does the following:
  * Check that a sufficient set of challenges has been validated
  * Mark the authorization as valid or invalid
  * Store the updated authorization object

* 6-7: WFEv2 does the following:
  * Return the updated challenge object

## Authorization Poll

ACME v2:

```
1: Client ---authz--> WFEv2
2: Client <---------- WFEv2
```

* 1-2: WFEv2 does the following:
  * Look up the referenced authorization
  * Verify that the request is a GET
  * Return the authorization object

## Order Poll (ACME v2 Only)

ACME v1:
This version of the protocol does not use order objects.

ACME v2:

```
1: Client ---order--> WFEv2
2: Client <---------- WFEv2
```

* 1-2: WFEv2 does the following:
  * Look up the referenced order
  * Return the order object

## New Certificate (ACME v1 Only)

ACME v2:
This version of the protocol expects certificate issuance to occur only through
order finalization and does not offer the new-cert endpoint.

* 1-2: WFEv2 does the following:
  * Verify that the request is a POST
  * Verify the JWS signature on the POST body
  * Verify that the JWS signature is by a registered key
  * Verify that the client has indicated agreement to terms
  * Parse the certificate request object

* 3-4: RA does the following:
  * Verify the PKCS#10 CSR in the certificate request object
  * Verify that the CSR has a non-zero number of domain names
  * Verify that the public key in the CSR is different from the account key
  * For each authorization referenced in the certificate request
    * Retrieve the authorization from the database
    * Verify that the authorization corresponds to the account key
    * Verify that the authorization is valid
    * Verify that the CAA policy for the identifier is still valid
  * Verify that all domains in the CSR are covered by authorizations
  * Compute the earliest expiration date among the authorizations
  * Instruct the CA to issue a precertificate

* 3-4: CA does the following:
  * Verify that the public key in the CSR meets quality requirements
    * RSA only for the moment
    * Modulus >= 2048 bits and not divisible by small primes
    * Exponent > 2^16
  * Remove any duplicate names in the CSR
  * Verify that all names are allowed by policy (also checked at new-authz time)
  * Verify that the issued cert will not be valid longer than the CA cert
  * Verify that the issued cert will not be valid longer than the underlying authorizations
  * Open a CA DB transaction and allocate a new serial number
  * Sign a poisoned precertificate

* 5-6: RA does the following:
  * Collect the SCTs needed to satisfy the ctpolicy
  * Instruct the CA to issue a final certificate with the SCTs

* 5-6: CA does the following:
  * Remove the precertificate poison and sign a final certificate with SCTs provided by the RA
  * Create the first OCSP response for the final certificate
  * Sign the final certificate and the first OCSP response
  * Store the final certificate
  * Commit the CA DB transaction if everything worked
  * Return the final certificate serial number

* 6-7: RA does the following:
  * Log the success or failure of the request
  * Return the certificate object

* 7-8: WFEv2 does the following:
  * Create a URL from the certificate's serial number
  * Return the certificate with its URL

## Order Finalization (ACME v2 Only)

ACME v2:

```
1: Client ---order finalize--> WFEv2
2:                       WFEv2 ----FinalizeOrder--> RA
3:                                                  RA ----------IssuePreCertificate---------> CA
4:                                                  RA <---------------return----------------- CA
5:                                                  RA ---IssueCertificateForPrecertificate--> CA
6:                                                  RA <---------------return----------------- CA
7:                       WFEv2 <----return--------- RA
8: Client <------------- WFEv2
```

* 1-2: WFEv2 does the following:
  * Verify that the request is a POST
  * Verify the JWS signature on the POST body
  * Verify that the JWS signature is by a registered key
  * Verify the registered account owns the order being finalized
  * Parse the certificate signing request (CSR) from the request

* 2-4: RA does the following:
  * Verify the PKCS#10 CSR in the certificate request object
  * Verify that the CSR has a non-zero number of domain names
  * Verify that the public key in the CSR is different from the account key
  * Retrieve and verify the status and expiry of the order object
  * For each identifier referenced in the order request
    * Retrieve the authorization from the database
    * Verify that the authorization corresponds to the account key
    * Verify that the authorization is valid
    * Verify that the CAA policy for the identifier is still valid
  * Verify that all domains in the order are included in the CSR
  * Instruct the CA to issue a precertificate

* 3-4: CA does the following:
  * Verify that the public key in the CSR meets quality requirements
    * RSA only for the moment
    * Modulus >= 2048 bits and not divisible by small primes
    * Exponent > 2^16
  * Remove any duplicate names in the CSR
  * Verify that all names are allowed by policy (also checked at new-authz time)
  * Verify that the issued cert will not be valid longer than the CA cert
  * Verify that the issued cert will not be valid longer than the underlying authorizations
  * Open a CA DB transaction and allocate a new serial number
  * Sign a poisoned precertificate

* 5-6: RA does the following
  * Collect the SCTs needed to satisfy the ctpolicy
  * Instruct the CA to issue a final certificate with the SCTs

* 5-6: CA does the following:
  * Sign a final certificate with SCTs provided by the RA
  * Create the first OCSP response for the final certificate
  * Sign the final certificate and the first OCSP response
  * Store the final certificate
  * Commit the CA DB transaction if everything worked
  * Return the final certificate serial number

* 6-7: RA does the following:
  * Log the success or failure of the request
  * Updates the order to have status valid if the request succeeded
  * Updates the order with the serial number of the certificate object

* 7-8: WFEv2 does the following:
  * Create a URL from the order's certificate's serial number
  * Return the order with a certificate URL

## Revoke Certificate

ACME v2:

```
1: Client ---cert--> WFEv2
2:                   WFEv2 ---RevokeCertByApplicant--> RA
3:                   WFEv2 <-----------return--------- RA
4: Client <--------- WFEv2
```
or
```
1: Client ---cert--> WFEv2
2:                   WFEv2 ------RevokeCertByKey-----> RA
3:                   WFEv2 <-----------return--------- RA
4: Client <--------- WFEv2
```


* 1-2:WFEv2 does the following:
  * Verify that the request is a POST
  * Verify the JWS signature on the POST body
  * Verify that the JWS signature is either:
    * The account key for the certificate, or
    * The account key for an account with valid authorizations for all names in
      the certificate, or
    * The public key from the certificate
  * Parse the certificate request object

* 3-4: RA does the following:
  * Mark the certificate as revoked.
  * Log the success or failure of the revocation

* Later, (not-pictured) the CA will:
  * Sign an OCSP response indicating revoked status for this certificate
  * Store the OCSP response in the database

* 3-4: WFEv2 does the following:
  * Return an indication of the success or failure of the revocation
