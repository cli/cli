# The Issuance Cycle

What happens during an ACME finalize request?

At a high level:

1. Check that all authorizations are good.
2. Recheck CAA for hostnames that need it.
3. Allocate and store a serial number.
4. Select a certificate profile.
5. Generate and store linting certificate, set status to "wait" (precommit).
6. Sign, log (and don't store) precertificate, set status to "good".
7. Submit precertificate to CT.
8. Generate linting final certificate. Not logged or stored.
9. Sign, log, and store final certificate.

Revocation can happen at any time after (5), whether or not step (6) was successful. We do things this way so that even in the event of a power failure or error storing data, we have a record of what we planned to sign (the tbsCertificate bytes of the linting certificate).

Note that to avoid needing a migration, we chose to store the linting certificate from (5)in the "precertificates" table, which is now a bit of a misnomer.

# OCSP Status state machine:

wait -> good -> revoked
   \
    -> revoked

Serial numbers with a "wait" status recorded have not been submitted to CT,
because issuing the precertificate is a prerequisite to setting the status to
"good". And because they haven't been submitted to CT, they also haven't been
turned into a final certificate, nor have they been returned to a user.

OCSP requests for serial numbers in "wait" status will return 500, but we expect
not to serve any 500s in practice because these serial numbers never wind up in
users' hands. Serial numbers in "wait" status are not added to CRLs.

Note that "serial numbers never wind up in users' hands" does not relieve us of
any compliance duties. Our duties start from the moment of signing a
precertificate with trusted key material.

Since serial numbers in "wait" status _may_ have had a precertificate signed,
We need the ability to set revocation status for them. For instance if the public key
we planned to sign for turns out to be weak or compromised, we would want to serve
a revoked status for that serial. However since they also _may not_ have had a
Precertificate signed, we also can't serve an OCSP "good" status. That's why we
serve 500. A 500 is appropriate because the only way a serial number can have "wait"
status for any significant amount of time is if there was an internal error of some
sort: an error during or before signing, or an error storing a record of the
signing success in the database.

For clarity, "wait" is not an RFC 6960 status, but is an internal placeholder
value specific to Boulder.
