# go-matching-engine

A lock-free **single-producer / single-consumer (SPSC)** ring buffer in Go,
designed with mechanical-sympathy principles (cache-line padding, bitmask
indexing, atomic sequences, no mutexes, no channels).

## Layout

```
go-matching-engine/
├── go.mod
├── cmd/
│   └── demo/
│       └── main.go              # runnable demo: 1M publish/consume + throughput
└── ringbuffer/
    ├── ringbuffer.go            # SPSC ring buffer + Event type
    └── ringbuffer_test.go       # correctness + blocking tests + benchmark
```

## Design highlights

| Concern               | Choice                                            |
| --------------------- | ------------------------------------------------- |
| Capacity              | `1024` (power of two)                             |
| Index                 | `seq & (size - 1)` bitmask                        |
| Producer sequence     | single `atomic.Int64`, sole writer = producer     |
| Consumer sequence     | single `atomic.Int64`, sole writer = consumer     |
| False-sharing guard   | 64-byte padding before & between both sequences   |
| Full-buffer handling  | `Publish` spins on `runtime.Gosched()`            |
| Empty-buffer handling | `Consume` spins on `runtime.Gosched()`            |
| Synchronization       | no `sync.Mutex`, no channels — only `sync/atomic` |

### Event type

```go
type Event struct {
    OrderID  int64   // 8
    Price    int64   // 8
    Quantity int64   // 8
    Side     byte    // 1
    _        [39]byte // pad to 64 bytes (one cache line per slot)
}
```

Payload is 25 bytes; Go's natural alignment would size it to 32 bytes, which
puts **two events per cache line** and causes false sharing between adjacent
slots. Padding each `Event` to 64 bytes gives every slot its own line.

### RingBuffer layout

```
┌──────────── 64 B ─────────────┐
│ _pad0  (isolation)            │
├───────────────────────────────┤
│ producerSeq (8 B) + pad (56)  │  ← producer's hot line
├───────────────────────────────┤
│ consumerSeq (8 B) + pad (56)  │  ← consumer's hot line
├───────────────────────────────┤
│ buffer[1024] × 64 B           │  = 65 536 B
└───────────────────────────────┘
```

## Run it

```bash
# unit tests (includes size + cache-line assertions)
go test ./ringbuffer/ -v

# benchmark
go test ./ringbuffer/ -run=^$ -bench=. -benchmem

# throughput demo: 1 producer, 1 consumer, 1M events
go run ./cmd/demo
```

## API

```go
rb := ringbuffer.New()

// producer goroutine
rb.Publish(ringbuffer.Event{OrderID: 1, Price: 100, Quantity: 5, Side: 'B'})

// consumer goroutine
e := rb.Consume()
```

`Publish` blocks (yields the goroutine) when the ring is full; `Consume`
blocks (yields) when the ring is empty. Both are safe for exactly **one**
producer goroutine and **one** consumer goroutine — not more.

<!-- Run it:

cd /Users/kenlau/Documents/Personal/repo/go-matching-engine
go test ./ringbuffer/ -v                 # unit tests
go test ./ringbuffer/ -run=^$ -bench=.   # benchmark
go run ./cmd/demo                        # 1M publish/consume demo -->
