package ringbuffer

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"
)

// ringBufferNoPad is identical to RingBuffer but with all padding removed.
// producerSeq and consumerSeq end up on the same cache line, demonstrating
// false sharing between the producer and consumer cores.
type ringBufferNoPad struct {
	producerSeq atomic.Int64
	cachedCons  int64
	consumerSeq atomic.Int64
	cachedProd  int64
	buffer      [BufferSize]Event
}

func (r *ringBufferNoPad) Publish(e Event) {
	seq := r.producerSeq.Load()
	if seq-r.cachedCons >= BufferSize {
		r.cachedCons = r.consumerSeq.Load()
		for seq-r.cachedCons >= BufferSize {
			runtime.Gosched()
			r.cachedCons = r.consumerSeq.Load()
		}
	}
	r.buffer[seq&bufferMask] = e
	r.producerSeq.Store(seq + 1)
}

func (r *ringBufferNoPad) Consume() Event {
	seq := r.consumerSeq.Load()
	if seq >= r.cachedProd {
		r.cachedProd = r.producerSeq.Load()
		for seq >= r.cachedProd {
			runtime.Gosched()
			r.cachedProd = r.producerSeq.Load()
		}
	}
	e := r.buffer[seq&bufferMask]
	r.consumerSeq.Store(seq + 1)
	return e
}

// Sanity check: confirm the two sequences actually share a cache line
// when padding is removed. If this test fails, the experiment is invalid
// (the OS/allocator isolated them by accident).
func TestNoPadSequencesShareCacheLine(t *testing.T) {
	var r ringBufferNoPad
	p := uintptr(unsafe.Pointer(&r.producerSeq))
	c := uintptr(unsafe.Pointer(&r.consumerSeq))
	if c-p >= cacheLine {
		t.Fatalf("seqs are %d bytes apart — not actually sharing a line, experiment invalid", c-p)
	}
	t.Logf("producerSeq @ %d, consumerSeq @ %d (delta=%d) — same cache line ✓", p, c, c-p)
}

func BenchmarkRingBufferNoPad(b *testing.B) {
	r := &ringBufferNoPad{}
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
