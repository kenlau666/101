package ringbuffer

import (
	"sync"
	"testing"
	"unsafe"
)

func TestEventSizeIsCacheLine(t *testing.T) {
	if got := unsafe.Sizeof(Event{}); got != 64 {
		t.Fatalf("Event size = %d, want 64", got)
	}
}

func TestSequencesOnSeparateCacheLines(t *testing.T) {
	var r RingBuffer
	p := uintptr(unsafe.Pointer(&r.producerSeq))
	c := uintptr(unsafe.Pointer(&r.consumerSeq))
	diff := c - p
	if diff < cacheLine {
		t.Fatalf("producer/consumer only %d bytes apart, want >= %d", diff, cacheLine)
	}
}

func TestPublishConsumeSingle(t *testing.T) {
	r := New()
	in := Event{OrderID: 42, Price: 100, Quantity: 5, Side: 'B'}
	r.Publish(in)
	out := r.Consume()
	if out.OrderID != in.OrderID || out.Price != in.Price ||
		out.Quantity != in.Quantity || out.Side != in.Side {
		t.Fatalf("round-trip mismatch: got %+v, want %+v", out, in)
	}
}

func TestPublishConsumeManyOrdered(t *testing.T) {
	const N = 100_000
	r := New()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := int64(0); i < N; i++ {
			got := r.Consume()
			if got.OrderID != i {
				t.Errorf("out of order: got %d want %d", got.OrderID, i)
				return
			}
		}
	}()
	for i := int64(0); i < N; i++ {
		r.Publish(Event{OrderID: i, Price: i * 2, Quantity: i + 1, Side: 'B'})
	}
	wg.Wait()
}

func TestBlocksWhenFull(t *testing.T) {
	// Fill buffer, then ensure Publish blocks until a Consume frees a slot.
	r := New()
	for i := int64(0); i < BufferSize; i++ {
		r.Publish(Event{OrderID: i})
	}
	done := make(chan struct{})
	go func() {
		r.Publish(Event{OrderID: BufferSize})
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("Publish did not block on full buffer")
	default:
	}
	_ = r.Consume() // frees one slot → producer proceeds
	<-done
}

func BenchmarkPublishConsume(b *testing.B) {
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
