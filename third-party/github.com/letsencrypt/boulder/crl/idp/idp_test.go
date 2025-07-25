package idp

import (
	"encoding/hex"
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestMakeUserCertsExt(t *testing.T) {
	t.Parallel()
	dehex := func(s string) []byte { r, _ := hex.DecodeString(s); return r }
	tests := []struct {
		name string
		urls []string
		want []byte
	}{
		{
			name: "one (real) url",
			urls: []string{"http://prod.c.lencr.org/20506757847264211/126.crl"},
			want: dehex("303AA035A0338631687474703A2F2F70726F642E632E6C656E63722E6F72672F32303530363735373834373236343231312F3132362E63726C8101FF"),
		},
		{
			name: "two urls",
			urls: []string{"http://old.style/12345678/90.crl", "http://new.style/90.crl"},
			want: dehex("3042A03DA03B8620687474703A2F2F6F6C642E7374796C652F31323334353637382F39302E63726C8617687474703A2F2F6E65772E7374796C652F39302E63726C8101FF"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := MakeUserCertsExt(tc.urls)
			test.AssertNotError(t, err, "should never fail to marshal asn1 to bytes")
			test.AssertDeepEquals(t, got.Id, idpOID)
			test.AssertEquals(t, got.Critical, true)
			test.AssertDeepEquals(t, got.Value, tc.want)
		})
	}
}
