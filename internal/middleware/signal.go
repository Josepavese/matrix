package middleware

// Signal defines the signal handling abstraction
type Signal interface {
	// Wait waits for a termination signal (SIGINT, SIGTERM, etc.)
	Wait()
}
