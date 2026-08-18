# LocalBucket uses a mutex, not a lock-free CAS loop

`LocalBucket` guards its state with `sync.Mutex`. We benchmarked the obvious
lock-free alternative — an `atomic.Pointer` compare-and-swap loop — and it was
1.2-1.5x *slower* on the hot path (accepted consumes), because publishing a
new state under CAS allocates on every attempt. The mutex has no such cost.
Should this path ever need to be faster, `core/local.go`'s benchmark note
puts the better target at the clock (`monotonicNanos` accounts for more of
`TryConsume`'s latency than the mutex does), not the locking strategy.
