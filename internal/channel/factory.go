//go:build !debug

package channel

// New creates a new channel with the given buffer size
// In production builds, this returns a buffered channel
func New[T any](size int) Channel[T] {
	return NewBuffered[T](size)
}
