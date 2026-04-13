package middleware

// Archive defines the cross-platform interface for extracting compressed files.
type Archive interface {
	// Extract uncompresses a .zip or .tar.gz file from src to dest directory.
	Extract(src, dest string) error
}
