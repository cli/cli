The test files in this directory can be recreated with the following small program:

  https://gist.github.com/cpu/df50564a473b3e8556917eb80d99ea56

Crucially the public keys in the generated JWKs/Certs are shared within
algorithm/parameters. E.g. the ECDSA JWK has the same public key as the ECDSA
Cert.
