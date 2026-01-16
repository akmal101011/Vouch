package ring

import (
	"errors"
	"sync"

	"github.com/slyt3/Vouch/internal/assert"
)

var ErrBufferFull = errors.New("ring buffer is full")
var ErrBufferEmpty = errors.New("ring buffer is empty")

// Buffer is a thread-safe, fixed-size ring buffer
// NASA Rule 2: Fixed loop bounds (via capacity)
type Buffer[T any] struct {
	data     []T
	capacity int
	head     int
	tail     int
	count    int
	mu       sync.Mutex
}

// New creates a new fixed-size ring buffer
func New[T any](capacity int) (*Buffer[T], error) {
	if err := assert.Check(capacity > 0, "capacity must be positive"); err != nil {
		return nil, err
	}
	return &Buffer[T]{
		data:     make([]T, capacity),
		capacity: capacity,
		head:     0,
		tail:     0,
		count:    0,
	}, nil
}

// Push adds an item to the buffer. Returns error if full.
// Zero allocation.
func (b *Buffer[T]) Push(item T) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// NASA Rule 5: Check Preconditions
	if b.count == b.capacity {
		return ErrBufferFull
	}
	// Bounds check (explicit, though go does it)
	assert.InRange(b.tail, 0, b.capacity-1, "tail index")

	b.data[b.tail] = item
	b.tail = (b.tail + 1) % b.capacity
	b.count++
	return nil
}

// Pop removes an item from the buffer. Returns error if empty.
func (b *Buffer[T]) Pop() (T, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var zero T
	if b.count == 0 {
		return zero, ErrBufferEmpty
	}

	assert.InRange(b.head, 0, b.capacity-1, "head index")
	item := b.data[b.head]

	// Optional: Clear reference to help GC if T is a pointer
	// b.data[b.head] = zero

	b.head = (b.head + 1) % b.capacity
	b.count--
	return item, nil
}

// IsFull returns true if buffer is full
func (b *Buffer[T]) IsFull() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count == b.capacity
}

// IsEmpty returns true if buffer is empty
func (b *Buffer[T]) IsEmpty() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count == 0
}

// Len returns current number of items
func (b *Buffer[T]) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}

// Cap returns capacity
func (b *Buffer[T]) Cap() int {
	return b.capacity
}
