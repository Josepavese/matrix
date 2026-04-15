package middleware

// Storage defines the single source of truth vault interface
type Storage interface {
	Get(key string) ([]byte, error)
	Set(key string, val []byte) error
	// Delete removes a key from the store
	Delete(key string) error
	// List returns all keys that start with the given prefix
	List(prefix string) ([]string, error)
}
