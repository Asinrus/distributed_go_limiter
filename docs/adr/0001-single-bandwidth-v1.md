# Single bandwidth per bucket in v1

bucket4j supports multiple bandwidths (stacked limits, e.g. 100/minute *and*
1000/hour) per bucket. We deliberately ported only single-bandwidth: one
capacity, one refill rate, per bucket. Multi-bandwidth means multiple
`bucketState` slots, a combine-and-report step across them, and a wire format
that carries N states instead of one — a real design surface, not a small
addition. We chose to ship the smaller, provider-swappable core first and
defer that surface rather than guess its shape now. Async consumption,
verbose diagnostics, and batching are deferred for the same reason.
