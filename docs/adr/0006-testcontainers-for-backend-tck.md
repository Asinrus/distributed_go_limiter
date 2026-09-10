# Backend conformance runs against real containers, not mocks

redisstore and pgstore run the shared conformance suite (`core/storetest`,
named `core/tck` when this was written) against real
Redis/Postgres via `testcontainers-go`, not a mocked `CASStore`/`LockStore`.
The whole point of the suite (AC-46: provider-swap) is proving that the actual
CAS/locking semantics of a real backend satisfy `core`'s contract — a mock
would only prove the mock does, which begs the question. The cost is a
Docker-compatible runtime at test time (`DOCKER_HOST`); there is no CI-less
fallback for these two modules by design.
