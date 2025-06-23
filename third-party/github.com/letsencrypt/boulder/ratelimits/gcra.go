package ratelimits

import (
	"time"

	"github.com/jmhodges/clock"
)

// maybeSpend uses the GCRA algorithm to decide whether to allow a request. It
// returns a Decision struct with the result of the decision and the updated
// TAT. The cost must be 0 or greater and <= the burst capacity of the limit.
func maybeSpend(clk clock.Clock, rl limit, tat time.Time, cost int64) *Decision {
	if cost < 0 || cost > rl.Burst {
		// The condition above is the union of the conditions checked in Check
		// and Spend methods of Limiter. If this panic is reached, it means that
		// the caller has introduced a bug.
		panic("invalid cost for maybeSpend")
	}
	nowUnix := clk.Now().UnixNano()
	tatUnix := tat.UnixNano()

	// If the TAT is in the future, use it as the starting point for the
	// calculation. Otherwise, use the current time. This is to prevent the
	// bucket from being filled with capacity from the past.
	if nowUnix > tatUnix {
		tatUnix = nowUnix
	}

	// Compute the cost increment.
	costIncrement := rl.emissionInterval * cost

	// Deduct the cost to find the new TAT and residual capacity.
	newTAT := tatUnix + costIncrement
	difference := nowUnix - (newTAT - rl.burstOffset)

	if difference < 0 {
		// Too little capacity to satisfy the cost, deny the request.
		residual := (nowUnix - (tatUnix - rl.burstOffset)) / rl.emissionInterval
		return &Decision{
			Allowed:   false,
			Remaining: residual,
			RetryIn:   -time.Duration(difference),
			ResetIn:   time.Duration(tatUnix - nowUnix),
			newTAT:    time.Unix(0, tatUnix).UTC(),
		}
	}

	// There is enough capacity to satisfy the cost, allow the request.
	var retryIn time.Duration
	residual := difference / rl.emissionInterval
	if difference < costIncrement {
		retryIn = time.Duration(costIncrement - difference)
	}
	return &Decision{
		Allowed:   true,
		Remaining: residual,
		RetryIn:   retryIn,
		ResetIn:   time.Duration(newTAT - nowUnix),
		newTAT:    time.Unix(0, newTAT).UTC(),
	}
}

// maybeRefund uses the Generic Cell Rate Algorithm (GCRA) to attempt to refund
// the cost of a request which was previously spent. The refund cost must be 0
// or greater. A cost will only be refunded up to the burst capacity of the
// limit. A partial refund is still considered successful.
func maybeRefund(clk clock.Clock, rl limit, tat time.Time, cost int64) *Decision {
	if cost < 0 || cost > rl.Burst {
		// The condition above is checked in the Refund method of Limiter. If
		// this panic is reached, it means that the caller has introduced a bug.
		panic("invalid cost for maybeRefund")
	}
	nowUnix := clk.Now().UnixNano()
	tatUnix := tat.UnixNano()

	// The TAT must be in the future to refund capacity.
	if nowUnix > tatUnix {
		// The TAT is in the past, therefore the bucket is full.
		return &Decision{
			Allowed:   false,
			Remaining: rl.Burst,
			RetryIn:   time.Duration(0),
			ResetIn:   time.Duration(0),
			newTAT:    tat,
		}
	}

	// Compute the refund increment.
	refundIncrement := rl.emissionInterval * cost

	// Subtract the refund increment from the TAT to find the new TAT.
	newTAT := tatUnix - refundIncrement

	// Ensure the new TAT is not earlier than now.
	if newTAT < nowUnix {
		newTAT = nowUnix
	}

	// Calculate the new capacity.
	difference := nowUnix - (newTAT - rl.burstOffset)
	residual := difference / rl.emissionInterval

	return &Decision{
		Allowed:   (newTAT != tatUnix),
		Remaining: residual,
		RetryIn:   time.Duration(0),
		ResetIn:   time.Duration(newTAT - nowUnix),
		newTAT:    time.Unix(0, newTAT).UTC(),
	}
}
