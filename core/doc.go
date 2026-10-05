// Package core defines the token-bucket API and algorithm for a distributed
// rate limiter. The token math lives here, once: a storage backend supplies
// an atomic store (CASStore or LockStore, see store.go) and nothing else —
// swapping backends changes one constructor line and the rate-limiting logic
// stays identical. core itself is transport-agnostic and has zero external
// runtime dependencies: no net/http, no gRPC, no vendor client.
//
// Semantics are rewritten from bucket4j (https://github.com/bucket4j/bucket4j);
// no bucket4j source is copied, and both projects are Apache-2.0.
package core
