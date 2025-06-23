package notmain

import (
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestInflight(t *testing.T) {
	ifl := newInflight()
	test.AssertEquals(t, ifl.len(), 0)
	test.AssertEquals(t, ifl.min(), uint64(0))

	ifl.add(1337)
	test.AssertEquals(t, ifl.len(), 1)
	test.AssertEquals(t, ifl.min(), uint64(1337))

	ifl.remove(1337)
	test.AssertEquals(t, ifl.len(), 0)
	test.AssertEquals(t, ifl.min(), uint64(0))

	ifl.add(7341)
	ifl.add(3317)
	ifl.add(1337)
	test.AssertEquals(t, ifl.len(), 3)
	test.AssertEquals(t, ifl.min(), uint64(1337))

	ifl.remove(3317)
	ifl.remove(1337)
	ifl.remove(7341)
	test.AssertEquals(t, ifl.len(), 0)
	test.AssertEquals(t, ifl.min(), uint64(0))
}
