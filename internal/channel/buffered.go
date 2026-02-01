package channel

// Buffered is a buffered channel implementation
type Buffered[T any] struct {
	ch chan T
}

// NewBuffered creates a new buffered channel with the given size
func NewBuffered[T any](size int) *Buffered[T] {
	return &Buffered[T]{ch: make(chan T, size)}
}

// Send sends a value to the channel
func (b *Buffered[T]) Send(v T) {
	b.ch <- v
}

// Receive returns the receive-only channel
func (b *Buffered[T]) Receive() <-chan T {
	return b.ch
}

// Len returns the number of items currently in the buffer
func (b *Buffered[T]) Len() int {
	return len(b.ch)
}

// Close closes the channel
func (b *Buffered[T]) Close() {
	close(b.ch)
}
