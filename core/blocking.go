package core

import (
	"context"
	"fmt"
	"math"
	"time"
)

// BlockingBucket waits for tokens to become available instead of rejecting.
// Obtain one via Bucket.AsBlocking().
type BlockingBucket struct {
	r reserver
}

// releaseTimeout bounds the best-effort refund issued when a wait is cancelled:
// the caller's own ctx is already done, so the release runs on an independent,
// short-lived context instead (docs/adr/0018) rather than inheriting no bound at
// all.
const releaseTimeout = 5 * time.Second

// Consume blocks until n tokens have been consumed or ctx is done. It reserves
// the tokens up front (computing the exact wait) and then sleeps — no busy-spin.
//
//   - n < 1                     -> ErrNonPositiveTokens
//   - n > capacity              -> ErrTokensExceedCapacity (never satisfiable)
//   - ctx has a deadline the
//     wait would exceed         -> context.DeadlineExceeded (nothing reserved)
//   - ctx cancelled/expired
//     while waiting             -> the reservation is released and the bare
//     ctx.Err() is returned; if the release itself fails, the error also
//     satisfies errors.Is(err, ErrReservationNotReleased) — a best-effort
//     refund, not a guaranteed one (docs/adr/0018). If the wait had already
//     elapsed at the moment the cancellation was observed, the consumption
//     stands: nil is returned and nothing is released.
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
		// t.Stop() returning false is the only reliable signal that the wait
		// had already elapsed: select's choice between two simultaneously-ready
		// cases is arbitrary, so this cannot be decided inside the select.
		return reservationOutcome(!t.Stop(), ctx.Err(), func() error {
			relCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
			defer cancel()
			return bb.r.releaseTokens(relCtx, n)
		})
	}
}

// reservationOutcome decides what a done ctx means for an in-flight
// reservation, given whether the wait's timer had already fired (timerElapsed)
// at the moment the cancellation was observed. If it had, the consumption
// already stands and release must not run — that would double-spend the
// tokens back to the caller, which is worse than the bug this guards against.
// Otherwise release walks the reservation back; a failed release folds
// ErrReservationNotReleased into the returned ctxErr rather than replacing it.
func reservationOutcome(timerElapsed bool, ctxErr error, release func() error) error {
	if timerElapsed {
		return nil
	}
	if err := release(); err != nil {
		return fmt.Errorf("%w: %w", ctxErr, ErrReservationNotReleased)
	}
	return ctxErr
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
