package crl

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/test"
)

func TestId(t *testing.T) {
	thisUpdate := time.Now()
	out := Id(1337, 1, Number(thisUpdate))
	expectCRLId := fmt.Sprintf("{\"issuerID\":1337,\"shardIdx\":1,\"crlNumber\":%d}", big.NewInt(thisUpdate.UnixNano()))
	test.AssertEquals(t, string(out), expectCRLId)
}
