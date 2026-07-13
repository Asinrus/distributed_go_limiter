package core

// ConsumptionProbe is the result of TryConsumeAndReturnRemaining.
type ConsumptionProbe struct {
	// Consumed reports whether the tokens were taken.
	Consumed bool
	// RemainingTokens is the tokens left after the operation (never negative).
	RemainingTokens int64
	// NanosToWaitForRefill is, on rejection, the time until enough tokens accrue
	// (0 when Consumed).
	NanosToWaitForRefill int64
	// NanosToWaitForReset is the time until the bucket is full again.
	NanosToWaitForReset int64
}

// EstimationProbe is the result of EstimateAbilityToConsume (non-mutating).
type EstimationProbe struct {
	CanBeConsumed        bool
	RemainingTokens      int64
	NanosToWaitForRefill int64
}
