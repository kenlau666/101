package ringbuffer

import (
	"runtime"
	"sync/atomic"
)

const (
	cacheLine  = 64
	BufferSize = 1024
	bufferMask = BufferSize - 1
)

type Event struct {
	OrderID  int64
	Price    int64
	Quantity int64
	Side     byte
	_        [cacheLine - 25]byte
}

type RingBuffer struct {
	_pad0       [cacheLine]byte
	producerSeq atomic.Int64
	cachedCons  int64 // producer-private view of consumerSeq
	_pad1       [cacheLine - 16]byte
	consumerSeq atomic.Int64
	cachedProd  int64 // consumer-private view of producerSeq
	_pad2       [cacheLine - 16]byte
	buffer      [BufferSize]Event
}

func New() *RingBuffer { return &RingBuffer{} }

func (r *RingBuffer) Publish(e Event) {
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

func (r *RingBuffer) Consume() Event {
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
