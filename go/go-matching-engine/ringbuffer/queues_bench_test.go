package ringbuffer

import (
	"sync"
	"testing"
)

// (a) buffered channel
func BenchmarkChannel(b *testing.B) {
	ch := make(chan Event, BufferSize)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			<-ch
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch <- Event{OrderID: int64(i)}
	}
	wg.Wait()
}

// (b) slice queue protected by sync.Mutex.
// Capacity 1024, fixed ring indices, condition variables for blocking.
type mutexQueue struct {
	mu       sync.Mutex
	notFull  *sync.Cond
	notEmpty *sync.Cond
	buf      [BufferSize]Event
	head     int64 // next read
	tail     int64 // next write
	count    int64
}

func newMutexQueue() *mutexQueue {
	q := &mutexQueue{}
	q.notFull = sync.NewCond(&q.mu)
	q.notEmpty = sync.NewCond(&q.mu)
	return q
}

func (q *mutexQueue) Push(e Event) {
	q.mu.Lock()
	for q.count == BufferSize {
		q.notFull.Wait()
	}
	q.buf[q.tail&bufferMask] = e
	q.tail++
	q.count++
	q.notEmpty.Signal()
	q.mu.Unlock()
}

func (q *mutexQueue) Pop() Event {
	q.mu.Lock()
	for q.count == 0 {
		q.notEmpty.Wait()
	}
	e := q.buf[q.head&bufferMask]
	q.head++
	q.count--
	q.notFull.Signal()
	q.mu.Unlock()
	return e
}

func BenchmarkMutexQueue(b *testing.B) {
	q := newMutexQueue()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			_ = q.Pop()
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Push(Event{OrderID: int64(i)})
	}
	wg.Wait()
}

// (c) lock-free SPSC ring buffer from Q2
func BenchmarkRingBuffer(b *testing.B) {
	r := New()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < b.N; i++ {
			_ = r.Consume()
		}
	}()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Publish(Event{OrderID: int64(i)})
	}
	wg.Wait()
}
