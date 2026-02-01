// Package channel provides generic channel interfaces for decoupled communication.
package channel

// Receiver provides read access to a channel.
type Receiver[T any] interface {
	Receive() <-chan T
	Len() int
}

// Sender provides write access to a channel.
type Sender[T any] interface {
	Send(T)
}

// Channel combines read and write access.
type Channel[T any] interface {
	Receiver[T]
	Sender[T]
	Close()
}
