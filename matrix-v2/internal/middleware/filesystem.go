package middleware

// FS defines the Filesystem abstraction layer interface
type FS interface {
	Mount(dir string) error
	Unmount() error
	CreateDirectory(path string) error
}
