package main

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/kenlau/go-matching-engine/ringbuffer"
)

func main() {
	fmt.Printf("sizeof(Event)      = %d bytes\n", unsafe.Sizeof(ringbuffer.Event{}))
	fmt.Printf("sizeof(RingBuffer) = %d bytes\n", unsafe.Sizeof(ringbuffer.RingBuffer{}))

	r := ringbuffer.New()
	const N = 1_000_000
	var wg sync.WaitGroup
	wg.Add(1)

	start := time.Now()
	go func() {
		defer wg.Done()
		var sum int64
		for i := 0; i < N; i++ {
			e := r.Consume()
			sum += e.OrderID
		}
		fmt.Printf("consumed %d events, checksum=%d\n", N, sum)
	}()

	for i := int64(0); i < N; i++ {
		r.Publish(ringbuffer.Event{
			OrderID:  i,
			Price:    10_000 + i,
			Quantity: 1,
			Side:     'B',
		})
	}
	wg.Wait()
	elapsed := time.Since(start)
	fmt.Printf("elapsed: %s  (%.2f ns/op, %.2f M ops/s)\n",
		elapsed,
		float64(elapsed.Nanoseconds())/float64(N),
		float64(N)/elapsed.Seconds()/1e6)
}
