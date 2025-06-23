package lints

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
	"golang.org/x/crypto/cryptobyte"
	"golang.org/x/crypto/cryptobyte/asn1"
)

var onlyContainsUserCertsTag = asn1.Tag(1).ContextSpecific()
var onlyContainsCACertsTag = asn1.Tag(2).ContextSpecific()

func TestReadOptionalASN1BooleanWithTag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		// incoming will be mutated by the function under test
		incoming     []byte
		out          bool
		defaultValue bool
		asn1Tag      asn1.Tag
		expectedOk   bool
		// expectedTrailer counts the remaining bytes from incoming after having
		// been advanced by the function under test
		expectedTrailer int
		expectedOut     bool
	}{
		{
			name:            "Good: onlyContainsUserCerts",
			incoming:        cryptobyte.String([]byte{0x81, 0x01, 0xFF}),
			asn1Tag:         onlyContainsUserCertsTag,
			expectedOk:      true,
			expectedTrailer: 0,
			expectedOut:     true,
		},
		{
			name:            "Good: onlyContainsCACerts",
			incoming:        cryptobyte.String([]byte{0x82, 0x01, 0xFF}),
			asn1Tag:         onlyContainsCACertsTag,
			expectedOk:      true,
			expectedTrailer: 0,
			expectedOut:     true,
		},
		{
			name:            "Good: Bytes are read and trailer remains",
			incoming:        cryptobyte.String([]byte{0x82, 0x01, 0xFF, 0xC0, 0xFF, 0xEE, 0xCA, 0xFE}),
			asn1Tag:         onlyContainsCACertsTag,
			expectedOk:      true,
			expectedTrailer: 5,
			expectedOut:     true,
		},
		{
			name:            "Bad: Read the tag, but out should be false, no trailer",
			incoming:        cryptobyte.String([]byte{0x82, 0x01, 0x00}),
			asn1Tag:         onlyContainsCACertsTag,
			expectedOk:      true,
			expectedTrailer: 0,
			expectedOut:     false,
		},
		{
			name:            "Bad: Read the tag, but out should be false, trailer remains",
			incoming:        cryptobyte.String([]byte{0x82, 0x01, 0x00, 0x99}),
			asn1Tag:         onlyContainsCACertsTag,
			expectedOk:      true,
			expectedTrailer: 1,
			expectedOut:     false,
		},
		{
			name:            "Bad: Wrong asn1Tag compared to incoming bytes, no bytes read",
			incoming:        cryptobyte.String([]byte{0x81, 0x01, 0xFF}),
			asn1Tag:         onlyContainsCACertsTag,
			expectedOk:      true,
			expectedTrailer: 3,
			expectedOut:     false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// ReadOptionalASN1BooleanWithTag accepts nil as a valid outParam to
			// maintain the style of upstream x/crypto/cryptobyte, but we
			// currently don't pass nil. Instead we use a reference to a
			// pre-existing boolean here and in the lint code. Passing in nil
			// will _do the wrong thing (TM)_ in our CRL lints.
			var outParam bool
			ok := ReadOptionalASN1BooleanWithTag((*cryptobyte.String)(&tc.incoming), &outParam, tc.asn1Tag, false)
			t.Log("Check if reading the tag was successful:")
			test.AssertEquals(t, ok, tc.expectedOk)
			t.Log("Check value of the optional boolean:")
			test.AssertEquals(t, outParam, tc.expectedOut)
			t.Log("Bytes should be popped off of incoming as they're successfully read:")
			test.AssertEquals(t, len(tc.incoming), tc.expectedTrailer)
		})
	}
}
