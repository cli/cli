package goodkey

import (
	"crypto/rsa"
	"encoding/hex"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestKnown(t *testing.T) {
	modBytes, err := hex.DecodeString("D673252AF6723C3F72529403EAB7C30DEF3C52F97E799825F4A70191C616ADCF1ECE1113F1625971074C492C592025FDEADBDB146A081826BDF0D77C3C913DCF1B6F0B3B78F5108D2E493AD0EEE8CA5C021711ADC13D358E61133870FCD19C8E5C22403959782AA82E72AEE53A3D491E3912CE27B27E1A85EA69C19A527D28F7934C9823B7E56FDD657DAC83FDC65BB22A98D843DF73238919781B714C81A5E2AFEC71F5C54AA2A27C590AD94C03C1062D50EFCFFAC743E3C8A3AE056846A1D756EB862BF4224169D467C35215ADE0AFCC11E85FE629AFB802C4786FF2E9C929BCCF502B3D3B8876C6A11785CC398B389F1D86BDD9CB0BD4EC13956EC3FA270D")
	test.AssertNotError(t, err, "Failed to decode modulus bytes")
	mod := &big.Int{}
	mod.SetBytes(modBytes)
	testKey := rsa.PublicKey{N: mod}
	otherKey := rsa.PublicKey{N: big.NewInt(2020)}

	wk := &WeakRSAKeys{suffixes: make(map[truncatedHash]struct{})}
	err = wk.addSuffix("8df20e6961a16398b85a")
	// a3853d0c563765e504c18df20e6961a16398b85a
	test.AssertNotError(t, err, "WeakRSAKeys.addSuffix failed")
	test.Assert(t, wk.Known(&testKey), "WeakRSAKeys.Known failed to find suffix that has been added")
	test.Assert(t, !wk.Known(&otherKey), "WeakRSAKeys.Known found a suffix that has not been added")
}

func TestLoadKeys(t *testing.T) {
	modBytes, err := hex.DecodeString("D673252AF6723C3F72529403EAB7C30DEF3C52F97E799825F4A70191C616ADCF1ECE1113F1625971074C492C592025FDEADBDB146A081826BDF0D77C3C913DCF1B6F0B3B78F5108D2E493AD0EEE8CA5C021711ADC13D358E61133870FCD19C8E5C22403959782AA82E72AEE53A3D491E3912CE27B27E1A85EA69C19A527D28F7934C9823B7E56FDD657DAC83FDC65BB22A98D843DF73238919781B714C81A5E2AFEC71F5C54AA2A27C590AD94C03C1062D50EFCFFAC743E3C8A3AE056846A1D756EB862BF4224169D467C35215ADE0AFCC11E85FE629AFB802C4786FF2E9C929BCCF502B3D3B8876C6A11785CC398B389F1D86BDD9CB0BD4EC13956EC3FA270D")
	test.AssertNotError(t, err, "Failed to decode modulus bytes")
	mod := &big.Int{}
	mod.SetBytes(modBytes)
	testKey := rsa.PublicKey{N: mod}
	tempDir := t.TempDir()
	tempPath := filepath.Join(tempDir, "a.json")
	err = os.WriteFile(tempPath, []byte("[\"8df20e6961a16398b85a\"]"), os.ModePerm)
	test.AssertNotError(t, err, "Failed to create temporary file")

	wk, err := LoadWeakRSASuffixes(tempPath)
	test.AssertNotError(t, err, "Failed to load suffixes from directory")
	test.Assert(t, wk.Known(&testKey), "WeakRSAKeys.Known failed to find suffix that has been added")
}
