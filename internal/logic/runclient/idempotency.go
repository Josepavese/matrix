package runclient

// Idempotency is the wire contract of a keyed call on the run surface: the
// caller names the key it retries with, and the answer reports that it repeats
// an earlier call carrying the same key.
//
// The names are published here, once, because the surfaces that accept a key —
// a submitted run and an acknowledged wakeup — are served by the same server and
// are called by two clients in this tree. A retry sent under a header the server
// does not read is a duplicate the caller believes it prevented.
const (
	IdempotencyKeyHeader      = "Idempotency-Key"
	IdempotencyReplayedHeader = "Idempotency-Replayed"
)
