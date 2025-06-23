# `ceremony`

```
ceremony --config path/to/config.yml
```

`ceremony` is a tool designed for Certificate Authority specific key and certificate ceremonies. The main design principle is that unlike most ceremony tooling there is a single user input, a configuration file, which is required to complete a root, intermediate, or key ceremony. The goal is to make ceremonies as simple as possible and allow for simple verification of a single file, instead of verification of a large number of independent commands.

`ceremony` has these modes:
* `root` - generates a signing key on HSM and creates a self-signed root certificate that uses the generated key, outputting a PEM public key, and a PEM certificate. After generating such a root for public trust purposes, it should be submitted to [as many root programs as is possible/practical](https://github.com/daknob/root-programs).
* `intermediate` - creates a intermediate certificate and signs it using a signing key already on a HSM, outputting a PEM certificate
* `cross-csr` - creates a CSR for signing by a third party, outputting a PEM CSR.
* `cross-certificate` - issues a certificate for one root, signed by another root. This is distinct from an intermediate because there is no path length constraint and there are no EKUs.
* `ocsp-signer` - creates a delegated OCSP signing certificate and signs it using a signing key already on a HSM, outputting a PEM certificate
* `crl-signer` - creates a delegated CRL signing certificate and signs it using a signing key already on a HSM, outputting a PEM certificate
* `key` - generates a signing key on HSM, outputting a PEM public key
* `ocsp-response` - creates a OCSP response for the provided certificate and signs it using a signing key already on a HSM, outputting a base64 encoded response
* `crl` - creates a CRL with the IDP extension and `onlyContainsCACerts = true` from the provided profile and signs it using a signing key already on a HSM, outputting a PEM CRL

These modes are set in the `ceremony-type` field of the configuration file.

This tool always generates key pairs such that the public and private key are both stored on the device with the same label. Ceremony types that use a key on a device ask for a "signing key label". During setup this label is used to find the public key of a keypair. Once the public key is loaded, the private key is looked up by CKA\_ID.

## Configuration format

`ceremony` uses YAML for its configuration file, mainly as it allows for commenting. Each ceremony type has a different set of configuration fields.

### Root ceremony

- `ceremony-type`: string describing the ceremony type, `root`.
- `pkcs11`: object containing PKCS#11 related fields.
    | Field | Description |
    | --- | --- |
    | `module` | Path to the PKCS#11 module to use to communicate with a HSM. |
    | `pin` | Specifies the login PIN, should only be provided if the HSM device requires one to interact with the slot. |
    | `store-key-in-slot` | Specifies which HSM object slot the generated signing key should be stored in. |
    | `store-key-with-label` | Specifies the HSM object label for the generated signing key. Both public and private key objects are stored with this label. |
- `key`: object containing key generation related fields.
    | Field | Description |
    | --- | --- |
    | `type` | Specifies the type of key to be generated, either `rsa` or `ecdsa`. If `rsa` the generated key will have an exponent of 65537 and a modulus length specified by `rsa-mod-length`. If `ecdsa` the curve is specified by `ecdsa-curve`. |
    | `ecdsa-curve` | Specifies the ECDSA curve to use when generating key, either `P-224`, `P-256`, `P-384`, or `P-521`. |
    | `rsa-mod-length` | Specifies the length of the RSA modulus, either `2048` or `4096`.
- `outputs`: object containing paths to write outputs.
    | Field | Description |
    | --- | --- |
    | `public-key-path` | Path to store generated PEM public key. |
    | `certificate-path` | Path to store signed PEM certificate. |
- `certificate-profile`: object containing profile for certificate to generate. Fields are documented [below](#certificate-profile-format).

Example:

```yaml
ceremony-type: root
pkcs11:
    module: /usr/lib/opensc-pkcs11.so
    store-key-in-slot: 0
    store-key-with-label: root signing key
key:
    type: ecdsa
    ecdsa-curve: P-384
outputs:
    public-key-path: /home/user/root-signing-pub.pem
    certificate-path: /home/user/root-cert.pem
certificate-profile:
    signature-algorithm: ECDSAWithSHA384
    common-name: CA intermediate
    organization: good guys
    country: US
    not-before: 2020-01-01 12:00:00
    not-after: 2040-01-01 12:00:00
    key-usages:
        - Cert Sign
        - CRL Sign
```

This config generates a ECDSA P-384 key in the HSM with the object label `root signing key` and uses this key to sign a self-signed certificate. The public key for the key generated is written to `/home/user/root-signing-pub.pem` and the certificate is written to `/home/user/root-cert.pem`.

### Intermediate or Cross-Certificate ceremony

- `ceremony-type`: string describing the ceremony type, `intermediate` or `cross-certificate`.
- `pkcs11`: object containing PKCS#11 related fields.
    | Field | Description |
    | --- | --- |
    | `module` | Path to the PKCS#11 module to use to communicate with a HSM. |
    | `pin` | Specifies the login PIN, should only be provided if the HSM device requires one to interact with the slot. |
    | `signing-key-slot` | Specifies which HSM object slot the signing key is in. |
    | `signing-key-label` | Specifies the HSM object label for the signing keypair's public key. |
- `inputs`: object containing paths for inputs
    | Field | Description |
    | --- | --- |
    | `public-key-path` | Path to PEM subject public key for certificate. |
    | `issuer-certificate-path` | Path to PEM issuer certificate. |
- `outputs`: object containing paths to write outputs.
    | Field | Description |
    | --- | --- |
    | `certificate-path` | Path to store signed PEM certificate. |
- `certificate-profile`: object containing profile for certificate to generate. Fields are documented [below](#certificate-profile-format).

Example:

```yaml
ceremony-type: intermediate
pkcs11:
    module: /usr/lib/opensc-pkcs11.so
    signing-key-slot: 0
    signing-key-label: root signing key
inputs:
    public-key-path: /home/user/intermediate-signing-pub.pem
    issuer-certificate-path: /home/user/root-cert.pem
outputs:
    certificate-path: /home/user/intermediate-cert.pem
certificate-profile:
    signature-algorithm: ECDSAWithSHA384
    common-name: CA root
    organization: good guys
    country: US
    not-before: 2020-01-01 12:00:00
    not-after: 2040-01-01 12:00:00
    ocsp-url: http://good-guys.com/ocsp
    crl-url:  http://good-guys.com/crl
    issuer-url:  http://good-guys.com/root
    policies:
        - oid: 1.2.3
        - oid: 4.5.6
          cps-uri: "http://example.com/cps"
    key-usages:
        - Digital Signature
        - Cert Sign
        - CRL Sign
```

This config generates an intermediate certificate signed by a key in the HSM, identified by the object label `root signing key` and the object ID `ffff`. The subject key used is taken from `/home/user/intermediate-signing-pub.pem` and the issuer is `/home/user/root-cert.pem`, the resulting certificate is written to `/home/user/intermediate-cert.pem`.

Note: Intermediate certificates always include the extended key usages id-kp-serverAuth as required by 7.1.2.2.g of the CABF Baseline Requirements. Since we also include id-kp-clientAuth in end-entity certificates in boulder we also include it in intermediates, if this changes we may remove this inclusion.

### Cross-CSR ceremony

- `ceremony-type`: string describing the ceremony type, `cross-csr`.
- `pkcs11`: object containing PKCS#11 related fields.
    | Field | Description |
    | --- | --- |
    | `module` | Path to the PKCS#11 module to use to communicate with a HSM. |
    | `pin` | Specifies the login PIN, should only be provided if the HSM device requires one to interact with the slot. |
    | `signing-key-slot` | Specifies which HSM object slot the signing key is in. |
    | `signing-key-label` | Specifies the HSM object label for the signing keypair's public key. |
- `inputs`: object containing paths for inputs
    | Field | Description |
    | --- | --- |
    | `public-key-path` | Path to PEM subject public key for certificate. |
- `outputs`: object containing paths to write outputs.
    | Field | Description |
    | --- | --- |
    | `csr-path` | Path to store PEM CSR for cross-signing, optional. |
- `certificate-profile`: object containing profile for certificate to generate. Fields are documented [below](#certificate-profile-format). Should only include Subject related fields `common-name`, `organization`, `country`.

Example:

```yaml
ceremony-type: cross-csr
pkcs11:
    module: /usr/lib/opensc-pkcs11.so
    signing-key-slot: 0
    signing-key-label: intermediate signing key
inputs:
    public-key-path: /home/user/intermediate-signing-pub.pem
outputs:
    csr-path: /home/user/csr.pem
certificate-profile:
    common-name: CA root
    organization: good guys
    country: US
```

This config generates a CSR signed by a key in the HSM, identified by the object label `intermediate signing key`, and writes it to `/home/user/csr.pem`.

### OCSP Signing Certificate ceremony

- `ceremony-type`: string describing the ceremony type, `ocsp-signer`.
- `pkcs11`: object containing PKCS#11 related fields.
    | Field | Description |
    | --- | --- |
    | `module` | Path to the PKCS#11 module to use to communicate with a HSM. |
    | `pin` | Specifies the login PIN, should only be provided if the HSM device requires one to interact with the slot. |
    | `signing-key-slot` | Specifies which HSM object slot the signing key is in. |
    | `signing-key-label` | Specifies the HSM object label for the signing keypair's public key. |
- `inputs`: object containing paths for inputs
    | Field | Description |
    | --- | --- |
    | `public-key-path` | Path to PEM subject public key for certificate. |
    | `issuer-certificate-path` | Path to PEM issuer certificate. |
- `outputs`: object containing paths to write outputs.
    | Field | Description |
    | --- | --- |
    | `certificate-path` | Path to store signed PEM certificate. |
- `certificate-profile`: object containing profile for certificate to generate. Fields are documented [below](#certificate-profile-format). The key-usages, ocsp-url, and crl-url fields must not be set.

When generating an OCSP signing certificate the key usages field will be set to just Digital Signature and an EKU extension will be included with the id-kp-OCSPSigning usage. Additionally an id-pkix-ocsp-nocheck extension will be included in the certificate.

Example:

```yaml
ceremony-type: ocsp-signer
pkcs11:
    module: /usr/lib/opensc-pkcs11.so
    signing-key-slot: 0
    signing-key-label: intermediate signing key
inputs:
    public-key-path: /home/user/ocsp-signer-signing-pub.pem
    issuer-certificate-path: /home/user/intermediate-cert.pem
outputs:
    certificate-path: /home/user/ocsp-signer-cert.pem
certificate-profile:
    signature-algorithm: ECDSAWithSHA384
    common-name: CA OCSP signer
    organization: good guys
    country: US
    not-before: 2020-01-01 12:00:00
    not-after: 2040-01-01 12:00:00
    issuer-url:  http://good-guys.com/root
```

This config generates a delegated OCSP signing certificate signed by a key in the HSM, identified by the object label `intermediate signing key` and the object ID `ffff`. The subject key used is taken from `/home/user/ocsp-signer-signing-pub.pem` and the issuer is `/home/user/intermediate-cert.pem`, the resulting certificate is written to `/home/user/ocsp-signer-cert.pem`.

### CRL Signing Certificate ceremony

- `ceremony-type`: string describing the ceremony type, `crl-signer`.
- `pkcs11`: object containing PKCS#11 related fields.
    | Field | Description |
    | --- | --- |
    | `module` | Path to the PKCS#11 module to use to communicate with a HSM. |
    | `pin` | Specifies the login PIN, should only be provided if the HSM device requires one to interact with the slot. |
    | `signing-key-slot` | Specifies which HSM object slot the signing key is in. |
    | `signing-key-label` | Specifies the HSM object label for the signing keypair's public key. |
- `inputs`: object containing paths for inputs
    | Field | Description |
    | --- | --- |
    | `public-key-path` | Path to PEM subject public key for certificate. |
    | `issuer-certificate-path` | Path to PEM issuer certificate. |
- `outputs`: object containing paths to write outputs.
    | Field | Description |
    | --- | --- |
    | `certificate-path` | Path to store signed PEM certificate. |
- `certificate-profile`: object containing profile for certificate to generate. Fields are documented [below](#certificate-profile-format). The key-usages, ocsp-url, and crl-url fields must not be set.

When generating a CRL signing certificate the key usages field will be set to just CRL Sign.

Example:

```yaml
ceremony-type: crl-signer
pkcs11:
    module: /usr/lib/opensc-pkcs11.so
    signing-key-slot: 0
    signing-key-label: intermediate signing key
inputs:
    public-key-path: /home/user/crl-signer-signing-pub.pem
    issuer-certificate-path: /home/user/intermediate-cert.pem
outputs:
    certificate-path: /home/user/crl-signer-cert.pem
certificate-profile:
    signature-algorithm: ECDSAWithSHA384
    common-name: CA CRL signer
    organization: good guys
    country: US
    not-before: 2020-01-01 12:00:00
    not-after: 2040-01-01 12:00:00
    issuer-url:  http://good-guys.com/root
```

This config generates a delegated CRL signing certificate signed by a key in the HSM, identified by the object label `intermediate signing key` and the object ID `ffff`. The subject key used is taken from `/home/user/crl-signer-signing-pub.pem` and the issuer is `/home/user/intermediate-cert.pem`, the resulting certificate is written to `/home/user/crl-signer-cert.pem`.

### Key ceremony

- `ceremony-type`: string describing the ceremony type, `key`.
- `pkcs11`: object containing PKCS#11 related fields.
    | Field | Description |
    | --- | --- |
    | `module` | Path to the PKCS#11 module to use to communicate with a HSM. |
    | `pin` | Specifies the login PIN, should only be provided if the HSM device requires one to interact with the slot. |
    | `store-key-in-slot` | Specifies which HSM object slot the generated signing key should be stored in. |
    | `store-key-with-label` | Specifies the HSM object label for the generated signing key. Both public and private key objects are stored with this label. |
- `key`: object containing key generation related fields.
    | Field | Description |
    | --- | --- |
    | `type` | Specifies the type of key to be generated, either `rsa` or `ecdsa`. If `rsa` the generated key will have an exponent of 65537 and a modulus length specified by `rsa-mod-length`. If `ecdsa` the curve is specified by `ecdsa-curve`. |
    | `ecdsa-curve` | Specifies the ECDSA curve to use when generating key, either `P-224`, `P-256`, `P-384`, or `P-521`. |
    | `rsa-mod-length` | Specifies the length of the RSA modulus, either `2048` or `4096`.
- `outputs`: object containing paths to write outputs.
    | Field | Description |
    | --- | --- |
    | `public-key-path` | Path to store generated PEM public key. |

Example:

```yaml
ceremony-type: key
pkcs11:
    module: /usr/lib/opensc-pkcs11.so
    store-key-in-slot: 0
    store-key-with-label: intermediate signing key
key:
    type: ecdsa
    ecdsa-curve: P-384
outputs:
    public-key-path: /home/user/intermediate-signing-pub.pem
```

This config generates an ECDSA P-384 key in the HSM with the object label `intermediate signing key`. The public key is written to `/home/user/intermediate-signing-pub.pem`.

### OCSP Response ceremony

- `ceremony-type`: string describing the ceremony type, `ocsp-response`.
- `pkcs11`: object containing PKCS#11 related fields.
    | Field | Description |
    | --- | --- |
    | `module` | Path to the PKCS#11 module to use to communicate with a HSM. |
    | `pin` | Specifies the login PIN, should only be provided if the HSM device requires one to interact with the slot. |
    | `signing-key-slot` | Specifies which HSM object slot the signing key is in. |
    | `signing-key-label` | Specifies the HSM object label for the signing keypair's public key. |
- `inputs`: object containing paths for inputs
    | Field | Description |
    | --- | --- |
    | `certificate-path` | Path to PEM certificate to create a response for. |
    | `issuer-certificate-path` | Path to PEM issuer certificate. |
    | `delegated-issuer-certificate-path` | Path to PEM delegated issuer certificate, if one is being used. |
- `outputs`: object containing paths to write outputs.
    | Field | Description |
    | --- | --- |
    | `response-path` | Path to store signed base64 encoded response. |
- `ocsp-profile`: object containing profile for the OCSP response.
    | Field | Description |
    | --- | --- |
    | `this-update` | Specifies the OCSP response thisUpdate date, in the format `2006-01-02 15:04:05`. The time will be interpreted as UTC. |
    | `next-update` | Specifies the OCSP response nextUpdate date, in the format `2006-01-02 15:04:05`. The time will be interpreted as UTC. |
    | `status` | Specifies the OCSP response status, either `good` or `revoked`. |

Example:

```yaml
ceremony-type: ocsp-response
pkcs11:
    module: /usr/lib/opensc-pkcs11.so
    signing-key-slot: 0
    signing-key-label: root signing key
inputs:
    certificate-path: /home/user/certificate.pem
    issuer-certificate-path: /home/user/root-cert.pem
outputs:
    response-path: /home/user/ocsp-resp.b64
ocsp-profile:
    this-update: 2020-01-01 12:00:00
    next-update: 2021-01-01 12:00:00
    status: good
```

This config generates a OCSP response signed by a key in the HSM, identified by the object label `root signing key` and object ID `ffff`. The response will be for the certificate in `/home/user/certificate.pem`, and will be written to `/home/user/ocsp-resp.b64`.

### CRL ceremony

- `ceremony-type`: string describing the ceremony type, `crl`.
- `pkcs11`: object containing PKCS#11 related fields.
    | Field | Description |
    | --- | --- |
    | `module` | Path to the PKCS#11 module to use to communicate with a HSM. |
    | `pin` | Specifies the login PIN, should only be provided if the HSM device requires one to interact with the slot. |
    | `signing-key-slot` | Specifies which HSM object slot the signing key is in. |
    | `signing-key-label` | Specifies the HSM object label for the signing keypair's public key. |
- `inputs`: object containing paths for inputs
    | Field | Description |
    | --- | --- |
    | `issuer-certificate-path` | Path to PEM issuer certificate. |
- `outputs`: object containing paths to write outputs.
    | Field | Description |
    | --- | --- |
    | `crl-path` | Path to store signed PEM CRL. |
- `crl-profile`: object containing profile for the CRL.
    | Field | Description |
    | --- | --- |
    | `this-update` | Specifies the CRL thisUpdate date, in the format `2006-01-02 15:04:05`. The time will be interpreted as UTC. |
    | `next-update` | Specifies the CRL nextUpdate date, in the format `2006-01-02 15:04:05`. The time will be interpreted as UTC. |
    | `number` | Specifies the CRL number. Each CRL should have a unique monotonically increasing number. |
    | `revoked-certificates` | Specifies any revoked certificates that should be included in the CRL. May be empty. If present it should be a list of objects with the fields `certificate-path`, containing the path to the revoked certificate, `revocation-date`, containing the date the certificate was revoked, in the format `2006-01-02 15:04:05`, and `revocation-reason`, containing a non-zero CRLReason code for the revocation taken from RFC 5280. |

Example:

```yaml
ceremony-type: crl
pkcs11:
    module: /usr/lib/opensc-pkcs11.so
    signing-key-slot: 0
    signing-key-label: root signing key
inputs:
    issuer-certificate-path: /home/user/root-cert.pem
outputs:
    crl-path: /home/user/crl.pem
crl-profile:
    this-update: 2020-01-01 12:00:00
    next-update: 2021-01-01 12:00:00
    number: 80
    revoked-certificates:
        - certificate-path: /home/user/revoked-cert.pem
          revocation-date: 2019-12-31 12:00:00
```

This config generates a CRL that must only contain subordinate CA certificates signed by a key in the HSM, identified by the object label `root signing key` and object ID `ffff`. The CRL will have the number `80` and will contain revocation information for the certificate `/home/user/revoked-cert.pem`. Each of the revoked certificates provided are checked to ensure they have the `IsCA` flag set to `true`.

### Certificate profile format

The certificate profile defines a restricted set of fields that are used to generate root and intermediate certificates.

| Field | Description |
| --- | --- |
| `signature-algorithm` | Specifies the signing algorithm to use, one of `SHA256WithRSA`, `SHA384WithRSA`, `SHA512WithRSA`, `ECDSAWithSHA256`, `ECDSAWithSHA384`, `ECDSAWithSHA512` |
| `common-name` | Specifies the subject commonName |
| `organization` | Specifies the subject organization |
| `country` | Specifies the subject country |
| `not-before` | Specifies the certificate notBefore date, in the format `2006-01-02 15:04:05`. The time will be interpreted as UTC. |
| `not-after` | Specifies the certificate notAfter date, in the format `2006-01-02 15:04:05`. The time will be interpreted as UTC. |
| `ocsp-url` | Specifies the AIA OCSP responder URL |
| `crl-url` | Specifies the cRLDistributionPoints URL |
| `issuer-url` | Specifies the AIA caIssuer URL |
| `policies` | Specifies contents of a certificatePolicies extension. Should contain a list of policies with the fields `oid`, indicating the policy OID, and a `cps-uri` field, containing the CPS URI to use, if the policy should contain a id-qt-cps qualifier. Only single CPS values are supported. |
| `key-usages` | Specifies list of key usage bits should be set, list can contain `Digital Signature`, `CRL Sign`, and `Cert Sign` |
