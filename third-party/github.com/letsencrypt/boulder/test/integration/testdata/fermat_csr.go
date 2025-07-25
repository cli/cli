package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"os"
)

const (
	// bits is the size of the resulting RSA key, also known as "nlen" or "Length
	// of the modulus N". Usually 1024, 2048, or 4096.
	bits = 2048
	// gap is the exponent of the different between the prime factors of the RSA
	// key, i.e. |p-q| ~= 2^gap. For FIPS compliance, set this to (bits/2 - 100).
	gap = 516
)

func main() {
	// Generate q, which will be the smaller of the two factors. We set its length
	// so that the product of two similarly-sized factors will be the desired
	// bit length.
	q, err := rand.Prime(rand.Reader, (bits+1)/2)
	if err != nil {
		log.Fatalln(err)
	}

	// Our starting point for p is q + 2^gap.
	p := new(big.Int).Add(q, new(big.Int).Exp(big.NewInt(2), big.NewInt(gap), nil))

	// Now we just keep incrementing P until we find a prime. You might think
	// this would take a while, but it won't: there are a lot of primes.
	attempts := 0
	for {
		// Using 34 rounds of Miller-Rabin primality testing is enough for the go
		// stdlib, so it's enough for us.
		if p.ProbablyPrime(34) {
			break
		}

		// We know P is odd because it started as a prime (odd) plus a power of two
		// (even), so we can increment by 2 to remain odd.
		p.Add(p, big.NewInt(2))
		attempts++
	}

	fmt.Println("p:", p.String())
	fmt.Println("q:", q.String())
	fmt.Println("Differ by", fmt.Sprintf("2^%d + %d", gap, 2*attempts))

	// Construct the public modulus N from the prime factors.
	n := new(big.Int).Mul(p, q)

	// Construct the public key from the modulus and (fixed) public exponent.
	pubkey := rsa.PublicKey{
		N: n,
		E: 65537,
	}

	// Construct the private exponent D from the prime factors.
	p_1 := new(big.Int).Sub(p, big.NewInt(1))
	q_1 := new(big.Int).Sub(q, big.NewInt(1))
	field := new(big.Int).Mul(p_1, q_1)
	d := new(big.Int).ModInverse(big.NewInt(65537), field)

	// Construct the private key from the factors and private exponent.
	privkey := rsa.PrivateKey{
		PublicKey: pubkey,
		D:         d,
		Primes:    []*big.Int{p, q},
	}
	privkey.Precompute()

	// Sign a CSR using this key, so we can use it in integration tests.
	// Note that this step *only works on go1.23 and earlier*. Later versions of
	// go detect that the prime factors are too close together and refuse to
	// produce a signature.
	csrDER, err := x509.CreateCertificateRequest(
		rand.Reader,
		&x509.CertificateRequest{
			Subject:   pkix.Name{CommonName: "example.com"},
			PublicKey: &pubkey,
		},
		&privkey)
	if err != nil {
		log.Fatalln(err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})
	fmt.Fprint(os.Stdout, string(csrPEM))
}
