package core

import (
	"context"
	"math"
	"time"
)

// BlockingBucket waits for tokens to become available instead of rejecting.
// Obtain one via Bucket.AsBlocking().
type BlockingBucket struct {
	r reserver
}

// Consume blocks until n tokens have been consumed or ctx is done. It reserves
// the tokens up front (computing the exact wait) and then sleeps — no busy-spin.
//
//   - n < 1                     -> ErrNonPositiveTokens
//   - n > capacity              -> ErrTokensExceedCapacity (never satisfiable)
//   - ctx has a deadline the
//     wait would exceed         -> context.DeadlineExceeded (nothing reserved)
//   - ctx cancelled while waiting-> ctx.Err() (the reservation stands and is
//     repaid by refill)
func (bb *BlockingBucket) Consume(ctx context.Context, n int64) error {
	if err := validateConsume(n); err != nil {
		return err
	}
	if n > bb.r.capacity() {
		return ErrTokensExceedCapacity
	}

	maxWait := maxWaitNanosFromContext(ctx)
	if maxWait <= 0 {
		return context.DeadlineExceeded
	}

	wait, ok, err := bb.r.reserveTokens(ctx, n, maxWait)
	if err != nil {
		return err
	}
	if !ok {
		// Cannot obtain the tokens before the deadline.
		return context.DeadlineExceeded
	}
	if wait <= 0 {
		return nil
	}

	t := time.NewTimer(time.Duration(wait))
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// maxWaitNanosFromContext returns how long we may wait: until ctx's deadline, or
// effectively forever when ctx has none.
func maxWaitNanosFromContext(ctx context.Context) int64 {
	deadline, ok := ctx.Deadline()
	if !ok {
		return math.MaxInt64
	}
	return int64(time.Until(deadline))
}
