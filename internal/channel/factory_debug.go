//go:build debug

package channel

// New creates a new channel
// In debug builds, this returns an unbuffered channel (ignores size)
func New[T any](size int) Channel[T] {
	return NewUnbuffered[T]()
}
