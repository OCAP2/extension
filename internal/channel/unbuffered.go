// internal/channel/unbuffered.go
package channel

// Unbuffered is an unbuffered channel implementation
type Unbuffered[T any] struct {
	ch chan T
}

// NewUnbuffered creates a new unbuffered channel
func NewUnbuffered[T any]() *Unbuffered[T] {
	return &Unbuffered[T]{ch: make(chan T)}
}

// Send sends a value to the channel (blocks until received)
func (u *Unbuffered[T]) Send(v T) {
	u.ch <- v
}

// Receive returns the receive-only channel
func (u *Unbuffered[T]) Receive() <-chan T {
	return u.ch
}

// Len always returns 0 for unbuffered channels
func (u *Unbuffered[T]) Len() int {
	return 0
}

// Close closes the channel
func (u *Unbuffered[T]) Close() {
	close(u.ch)
}
