package ratelimits

import (
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/test"
)

func TestDecide(t *testing.T) {
	clk := clock.NewFake()
	limit := limit{Burst: 10, Count: 1, Period: config.Duration{Duration: time.Second}}
	limit.precompute()

	// Begin by using 1 of our 10 requests.
	d := maybeSpend(clk, limit, clk.Now(), 1)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(9))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Second)

	// Immediately use another 9 of our remaining requests.
	d = maybeSpend(clk, limit, d.newTAT, 9)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(0))
	// We should have to wait 1 second before we can use another request but we
	// used 9 so we should have to wait 9 seconds to make an identical request.
	test.AssertEquals(t, d.RetryIn, time.Second*9)
	test.AssertEquals(t, d.ResetIn, time.Second*10)

	// Our new TAT should be 10 seconds (limit.Burst) in the future.
	test.AssertEquals(t, d.newTAT, clk.Now().Add(time.Second*10))

	// Let's try using just 1 more request without waiting.
	d = maybeSpend(clk, limit, d.newTAT, 1)
	test.Assert(t, !d.Allowed, "should not be allowed")
	test.AssertEquals(t, d.Remaining, int64(0))
	test.AssertEquals(t, d.RetryIn, time.Second)
	test.AssertEquals(t, d.ResetIn, time.Second*10)

	// Let's try being exactly as patient as we're told to be.
	clk.Add(d.RetryIn)
	d = maybeSpend(clk, limit, d.newTAT, 0)
	test.AssertEquals(t, d.Remaining, int64(1))

	// We are 1 second in the future, we should have 1 new request.
	d = maybeSpend(clk, limit, d.newTAT, 1)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(0))
	test.AssertEquals(t, d.RetryIn, time.Second)
	test.AssertEquals(t, d.ResetIn, time.Second*10)

	// Let's try waiting (10 seconds) for our whole bucket to refill.
	clk.Add(d.ResetIn)

	// We should have 10 new requests. If we use 1 we should have 9 remaining.
	d = maybeSpend(clk, limit, d.newTAT, 1)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(9))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Second)

	// Wait just shy of how long we're told to wait for refilling.
	clk.Add(d.ResetIn - time.Millisecond)

	// We should still have 9 remaining because we're still 1ms shy of the
	// refill time.
	d = maybeSpend(clk, limit, d.newTAT, 0)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(9))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Millisecond)

	// Spending 0 simply informed us that we still have 9 remaining, let's see
	// what we have after waiting 20 hours.
	clk.Add(20 * time.Hour)

	// C'mon, big money, no whammies, no whammies, STOP!
	d = maybeSpend(clk, limit, d.newTAT, 0)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(10))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Duration(0))

	// Turns out that the most we can accrue is 10 (limit.Burst). Let's empty
	// this bucket out so we can try something else.
	d = maybeSpend(clk, limit, d.newTAT, 10)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(0))
	// We should have to wait 1 second before we can use another request but we
	// used 10 so we should have to wait 10 seconds to make an identical
	// request.
	test.AssertEquals(t, d.RetryIn, time.Second*10)
	test.AssertEquals(t, d.ResetIn, time.Second*10)

	// If you spend 0 while you have 0 you should get 0.
	d = maybeSpend(clk, limit, d.newTAT, 0)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(0))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Second*10)

	// We don't play by the rules, we spend 1 when we have 0.
	d = maybeSpend(clk, limit, d.newTAT, 1)
	test.Assert(t, !d.Allowed, "should not be allowed")
	test.AssertEquals(t, d.Remaining, int64(0))
	test.AssertEquals(t, d.RetryIn, time.Second)
	test.AssertEquals(t, d.ResetIn, time.Second*10)

	// Okay, maybe we should play by the rules if we want to get anywhere.
	clk.Add(d.RetryIn)

	// Our patience pays off, we should have 1 new request. Let's use it.
	d = maybeSpend(clk, limit, d.newTAT, 1)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(0))
	test.AssertEquals(t, d.RetryIn, time.Second)
	test.AssertEquals(t, d.ResetIn, time.Second*10)

	// Refill from empty to 5.
	clk.Add(d.ResetIn / 2)

	// Attempt to spend 7 when we only have 5. We should be denied but the
	// decision should reflect a retry of 2 seconds, the time it would take to
	// refill from 5 to 7.
	d = maybeSpend(clk, limit, d.newTAT, 7)
	test.Assert(t, !d.Allowed, "should not be allowed")
	test.AssertEquals(t, d.Remaining, int64(5))
	test.AssertEquals(t, d.RetryIn, time.Second*2)
	test.AssertEquals(t, d.ResetIn, time.Second*5)
}

func TestMaybeRefund(t *testing.T) {
	clk := clock.NewFake()
	limit := limit{Burst: 10, Count: 1, Period: config.Duration{Duration: time.Second}}
	limit.precompute()

	// Begin by using 1 of our 10 requests.
	d := maybeSpend(clk, limit, clk.Now(), 1)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(9))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Second)

	// Refund back to 10.
	d = maybeRefund(clk, limit, d.newTAT, 1)
	test.AssertEquals(t, d.Remaining, int64(10))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Duration(0))

	// Refund 0, we should still have 10.
	d = maybeRefund(clk, limit, d.newTAT, 0)
	test.AssertEquals(t, d.Remaining, int64(10))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Duration(0))

	// Spend 1 more of our 10 requests.
	d = maybeSpend(clk, limit, d.newTAT, 1)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(9))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Second)

	// Wait for our bucket to refill.
	clk.Add(d.ResetIn)

	// Attempt to refund from 10 to 11.
	d = maybeRefund(clk, limit, d.newTAT, 1)
	test.Assert(t, !d.Allowed, "should not be allowed")
	test.AssertEquals(t, d.Remaining, int64(10))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Duration(0))

	// Spend 10 all 10 of our requests.
	d = maybeSpend(clk, limit, d.newTAT, 10)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(0))
	// We should have to wait 1 second before we can use another request but we
	// used 10 so we should have to wait 10 seconds to make an identical
	// request.
	test.AssertEquals(t, d.RetryIn, time.Second*10)
	test.AssertEquals(t, d.ResetIn, time.Second*10)

	// Attempt a refund of 10.
	d = maybeRefund(clk, limit, d.newTAT, 10)
	test.AssertEquals(t, d.Remaining, int64(10))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Duration(0))

	// Wait 11 seconds to catching up to TAT.
	clk.Add(11 * time.Second)

	// Attempt to refund to 11, then ensure it's still 10.
	d = maybeRefund(clk, limit, d.newTAT, 1)
	test.Assert(t, !d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(10))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Duration(0))

	// Spend 5 of our 10 requests, then refund 1.
	d = maybeSpend(clk, limit, d.newTAT, 5)
	d = maybeRefund(clk, limit, d.newTAT, 1)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(6))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))

	// Wait, a 2.5 seconds to refill to 8.5 requests.
	clk.Add(time.Millisecond * 2500)

	// Ensure we have 8.5 requests.
	d = maybeSpend(clk, limit, d.newTAT, 0)
	test.Assert(t, d.Allowed, "should be allowed")
	test.AssertEquals(t, d.Remaining, int64(8))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	// Check that ResetIn represents the fractional earned request.
	test.AssertEquals(t, d.ResetIn, time.Millisecond*1500)

	// Refund 2 requests, we should only have 10, not 10.5.
	d = maybeRefund(clk, limit, d.newTAT, 2)
	test.AssertEquals(t, d.Remaining, int64(10))
	test.AssertEquals(t, d.RetryIn, time.Duration(0))
	test.AssertEquals(t, d.ResetIn, time.Duration(0))
}
