# CEX Matching Engine — Phase 1 Course (Go)

> Low Latency & Determinism
> Three lessons. Harsh mode. Built for Go. Drill at the end of each lesson.

---

## Lesson 1: The LMAX Disruptor Pattern

### Why This Lesson Exists

A matching engine's job is to take orders, match them against the book, and emit fills — deterministically, in order, at microsecond latency. Not millisecond. **Micro**second. That's a 1000x difference.

The reason exchanges obsess over this: if your engine takes 5ms to match, an HFT firm with a 500μs engine eats your lunch by frontrunning every event you process. Even if you don't care about HFT traders, slow engines crash during volatility — exactly when users need them to work.

The LMAX Disruptor was built by a UK derivatives exchange in 2010 when they realized their Java-based engine, written with standard queues and locks, could only do ~60K orders/sec. They rewrote it using the pattern you're about to learn and hit **6 million orders/sec on a single thread**. That's a 100x improvement from removing locks and respecting the CPU.

Every serious CEX matching engine today uses some variant of this pattern. Binance. Coinbase. Deribit. If you want to build one, you have to understand why locks are the enemy and how a ring buffer replaces them.

---

### Prerequisite: What "Fast" Actually Means

You cannot reason about performance if you don't have the latency numbers in your head. Memorize these. I mean it — write them on a post-it.

```
Operation                          Latency        Analogy (if 1 CPU cycle = 1 second)
────────────────────────────────────────────────────────────────────────────────────
1 CPU cycle                        ~0.3 ns        1 second
L1 cache reference                 ~1 ns          3 seconds
L2 cache reference                 ~4 ns          12 seconds
L3 cache reference                 ~12 ns         40 seconds
Main memory (RAM) reference        ~100 ns        5 minutes
Mutex lock/unlock (uncontended)    ~25 ns         80 seconds
Mutex lock/unlock (contended)      ~1000+ ns      1 hour+
Context switch (OS)                ~1000-10000 ns several hours
SSD random read                    ~100,000 ns    1 day
Network round trip (same DC)       ~500,000 ns    5 days
Network round trip (cross-country) ~40,000,000 ns  4 years
```

Key takeaways:
- **RAM is 100x slower than L1 cache.** The CPU waits 300 cycles staring at the wall when you miss cache.
- **A contended mutex costs 1000+ ns.** At 10M ops/sec, that's 10 entire seconds of overhead per second of work. The system dies.
- **You will never hit microsecond latency if you go to disk or across the network in the hot path.** Ever.

The matching engine's hot path must stay in L1/L2 cache and never allocate. That's the whole game.

---

### How the CPU Actually Reads Memory (Cache Lines)

When your Go code does `x := arr[i]`, here's what actually happens:

1. CPU looks for the address in L1 cache (3-4 cycles).
2. If not there, checks L2 (~12 cycles).
3. If not there, checks L3 (~40 cycles — and L3 is shared across cores).
4. If not there, fetches from RAM (~300 cycles — and brings back a whole 64-byte chunk, not just your variable).

**Cache line = 64 bytes.** This is the unit of memory transfer. If you read byte 0, the CPU pulls bytes 0-63 into cache. This has enormous consequences:

- **Arrays are fast.** Sequential access means every N bytes = 1 cache miss for 64 bytes of data.
- **Linked lists are garbage.** Each node is somewhere random in memory = cache miss per node.
- **Two variables on the same cache line are linked whether you like it or not.** This is the root of false sharing.

In Go, `[]int64` of size 1024 occupies 8192 bytes = 128 cache lines. Walking it sequentially touches 128 cache lines. Walking a `list.List` with 1024 nodes touches up to 1024 cache lines. That's an 8x difference in cache miss rate before you've done any real work.

---

### What a Lock Actually Does

`sync.Mutex` in Go looks innocent. It's not. Here's what `mu.Lock()` does at the machine level:

**Fast path (uncontended):** one atomic compare-and-swap (CAS) instruction flips a flag from 0 to 1. Cost: ~25ns. Fine.

**Slow path (contended):**
1. CAS fails because another goroutine holds the lock.
2. Go runtime spins briefly (hoping lock frees up).
3. If still contended, the runtime *parks the goroutine* — removes it from scheduler runqueue.
4. Under the hood on Linux, this eventually traps into the kernel via a `futex` syscall.
5. Kernel puts the OS thread to sleep.
6. When the lock releases, kernel wakes thread, scheduler reschedules goroutine, goroutine re-attempts CAS.

Total cost: **1,000-10,000 nanoseconds**, sometimes more. That's the equivalent of 40 RAM accesses or 10,000 L1 cache hits. For a single lock operation.

There's another, sneakier cost: **cache-line bouncing.** The mutex itself lives at some memory address. When core 1 acquires it, the CPU invalidates that cache line on all other cores (MESI cache coherence protocol — don't worry about the acronym, just know that "invalidate" means "force other cores to refetch"). When core 2 tries to acquire, it misses cache, refetches from L3 or RAM, writes, invalidates core 1 and 3...

At high contention, a mutex becomes a ping-pong ball between CPU cores, and every bounce costs ~40ns just in cache coherence traffic. Throughput collapses.

**This is why the LMAX paper's central insight is "locks kill performance." It's not hyperbole — measure it.**

---

### Atomic Operations: Locks' Cheaper Cousin

Instead of a mutex, you can use `sync/atomic`. These compile to single CPU instructions with built-in memory ordering:

```go
import "sync/atomic"

var counter int64

atomic.AddInt64(&counter, 1)          // single LOCK XADD instruction
atomic.LoadInt64(&counter)            // single MOV with barrier
atomic.StoreInt64(&counter, 42)       // single MOV with barrier
atomic.CompareAndSwapInt64(&counter, old, new)  // single LOCK CMPXCHG
```

- No OS involvement.
- No goroutine parking.
- Cost: ~5-20ns even under contention (cache line bouncing is the only cost).
- 50-1000x faster than a contended mutex.

The Disruptor is built on atomics. Specifically, on two atomic int64 counters: a *producer sequence* and a *consumer sequence*.

---

### Memory Barriers and Why Your Intuition Is Wrong

Here's something that will trip you up if you don't know it. Modern CPUs **reorder your instructions** for speed. The Go compiler also reorders. Consider:

```go
// Goroutine A                      // Goroutine B
data = 42                            if ready == 1 {
ready = 1                                fmt.Println(data)  // might print 0 !!
                                     }
```

You'd expect B to see `data = 42` because A wrote it *before* `ready = 1`. Nope. Without a memory barrier:
- Compiler might reorder A's writes.
- CPU's store buffer might flush `ready = 1` to cache before `data = 42`.
- B sees `ready = 1` but reads stale `data`.

This is why `atomic.StoreInt64` and `atomic.LoadInt64` aren't just "thread-safe ints." They include **memory barriers** — they guarantee that all writes before the atomic store are visible to any goroutine that sees the atomic load. This is the "happens-before" relationship.

In the Disruptor, the producer writes the event data, then does `atomic.StoreInt64(&producerSeq, newSeq)`. The consumer does `atomic.LoadInt64(&producerSeq)`, sees the new value, and is *guaranteed* that the event data is visible. That's the entire synchronization mechanism. No locks.

---

### False Sharing: The Silent Killer

Two variables on the same cache line behave as if they were one variable, from the cache coherence system's point of view. Example:

```go
type Counters struct {
    producerSeq int64   // 8 bytes
    consumerSeq int64   // 8 bytes
}
var c Counters
// Both fields live in the same 64-byte cache line.
```

Goroutine P writes `producerSeq` constantly. Goroutine C reads `consumerSeq` constantly. Logically unrelated. But physically:
- P's write invalidates the cache line on C's core.
- C's read on `consumerSeq` now misses cache and refetches the entire line.
- Each operation pays ~40ns of cache coherence penalty.

Measured impact: **10x slowdown**. For literally no reason. Two independent variables, wrecking each other through the cache.

**The fix: pad to cache-line size.** In Go:

```go
type PaddedSeq struct {
    _   [7]int64   // 56 bytes of padding before
    val int64      // 8 bytes
    _   [7]int64   // 56 bytes of padding after (in case next struct lands adjacent)
}
```

Now `producerSeq` and `consumerSeq` live 64+ bytes apart. No more false sharing. Disruptor implementations do this religiously.

Go does not have a nice built-in like Java's `@Contended`. You write the padding by hand.

---

### The Ring Buffer

Finally — the data structure.

A **ring buffer** is a fixed-size array where writes wrap around. You never grow it, never shrink it, never allocate. You pre-allocate it at startup and that's it for the life of the process.

```
Size = 8 slots

Index:   0    1    2    3    4    5    6    7
       [E0] [E1] [E2] [E3] [  ] [  ] [  ] [  ]
         ↑                   ↑
       consumer          producer
```

Producer keeps an ever-incrementing sequence counter. To write event N, it writes to slot `N & (size-1)`. When N reaches 8, slot is `8 & 7 = 0` — wraps around to the start, overwriting E0 (which consumer must have read by now, or producer stalls).

**Two magic details:**

1. **Size must be a power of 2.** Why? Because `N & (size-1)` is only equal to `N % size` when size is a power of 2. `N & (size-1)` is a single CPU instruction (~1 cycle). `N % size` is an integer division (~20-40 cycles). At 100M ops/sec, that difference is 2-4 entire CPU cores' worth of wasted work.

2. **Slots are never allocated.** The array is pre-allocated with zeroed events. Producer *mutates in place*. No `make()`, no `new()`, no GC pressure.

---

### Sequences: How Producer and Consumer Coordinate Without Locks

The producer maintains an `int64` called `producerSeq`. The consumer maintains `consumerSeq`. Both start at -1.

**Producer flow:**
```
1. next := producerSeq + 1
2. Check: is slot (next & mask) free? (has consumer caught up past it?)
   i.e.,  next - consumerSeq < bufferSize
3. If not free: wait (spin, yield, or block).
4. If free: write event to ring[next & mask]
5. atomic.StoreInt64(&producerSeq, next)   // publish
```

**Consumer flow:**
```
1. next := consumerSeq + 1
2. published := atomic.LoadInt64(&producerSeq)
3. If next > published: wait (nothing new to consume)
4. Else: read event from ring[next & mask]
5. Process event
6. atomic.StoreInt64(&consumerSeq, next)   // acknowledge
```

That's it. No mutex. Two atomic counters. The producer is the single writer to the ring array (single-writer principle — nobody else mutates it). The consumer only reads. Memory barriers in the atomic store/load handle visibility.

This is the core of the Disruptor. Everything else is decoration.

---

### Wait Strategies

When the consumer is caught up or the producer is blocked, something has to wait. Three options:

**Busy spin:**
```go
for atomic.LoadInt64(&producerSeq) < next {
    // burn CPU
}
```
Latency: sub-100ns to detect new event. CPU cost: 100% of a core. Used in HFT with a dedicated core.

**Yielding spin:**
```go
for i := 0; atomic.LoadInt64(&producerSeq) < next; i++ {
    if i > 100 {
        runtime.Gosched()  // give scheduler a chance
        i = 0
    }
}
```
Latency: microseconds. CPU cost: moderate. Reasonable default.

**Blocking (condition variable):**
Latency: 1-10ms. CPU cost: near zero when idle. Defeats the point — if you're blocking, just use a Go channel.

**Matching engines use busy spin on pinned cores.** They burn the CPU. That's the deal.

---

### Core Pinning (Brief)

Even with busy spin, if the OS schedules another thread onto your core, you pay for a context switch (~3000ns). To prevent that, you pin your critical goroutine to a specific CPU core.

In Go, this is hard. `runtime.LockOSThread()` pins a goroutine to an OS thread, but Go's scheduler can still put *other* goroutines on that thread. To pin the thread to a specific core you need `syscall.Syscall(unix.SYS_SCHED_SETAFFINITY, ...)` on Linux — ugly. Some HFT shops patch the Go runtime. Most use C/C++/Rust instead. This is one reason Go is not the dominant HFT language despite being excellent otherwise.

For your matching engine, `runtime.LockOSThread()` plus careful GOMAXPROCS tuning will get you 90% of the way.

---

### Go-Specific Warnings

**1. Go channels are not lock-free.** A buffered `chan Event` uses a mutex internally (`hchan.lock`). It's a multi-producer multi-consumer queue with all the coordination overhead that implies. Throughput caps around 5-20M ops/sec on a single channel. A hand-rolled SPSC ring buffer hits 100M+.

**2. Go's GC will pause you.** Even the concurrent GC has stop-the-world phases (sub-ms these days, but still). Every `make()`, every `new()`, every slice append in your hot path adds GC pressure. In Lesson 3 you'll learn how to eliminate allocations.

**3. `interface{}` / `any` in the hot path = hidden allocation.** Every boxing into an interface heap-allocates. Use concrete types on your ring buffer. `[]Event`, not `[]any`.

**4. Go's escape analysis is unreliable.** A variable you think is stack-allocated might escape to the heap if you take its pointer in a weird place. Use `go build -gcflags="-m"` to see which variables escape.

---

### Putting It All Together: A Real SPSC Disruptor in Go

```go
package ringbuf

import (
    "runtime"
    "sync/atomic"
)

type Event struct {
    Payload [56]byte  // sized so Event is 64 bytes (1 cache line)
}

const bufferSize = 1024        // power of 2
const mask = bufferSize - 1    // bitmask

type Disruptor struct {
    _           [7]int64        // padding
    producerSeq int64           // written only by producer
    _           [7]int64        // padding (prevents false sharing)
    consumerSeq int64           // written only by consumer
    _           [7]int64        // padding
    ring        [bufferSize]Event
}

func New() *Disruptor {
    return &Disruptor{
        producerSeq: -1,
        consumerSeq: -1,
    }
}

// Publish is called by the single producer goroutine.
func (d *Disruptor) Publish(e Event) {
    next := d.producerSeq + 1
    // Wait until consumer has caught up enough to free the slot
    for next-atomic.LoadInt64(&d.consumerSeq) > bufferSize {
        runtime.Gosched()
    }
    d.ring[next&mask] = e
    atomic.StoreInt64(&d.producerSeq, next)
}

// Consume is called by the single consumer goroutine.
func (d *Disruptor) Consume() Event {
    next := d.consumerSeq + 1
    for atomic.LoadInt64(&d.producerSeq) < next {
        runtime.Gosched()
    }
    e := d.ring[next&mask]
    atomic.StoreInt64(&d.consumerSeq, next)
    return e
}
```

Note:
- Producer only writes `producerSeq` and `ring[i]`. Consumer only reads them.
- Consumer only writes `consumerSeq`. Producer only reads it.
- The two sequence counters are padded apart, on separate cache lines.
- No mutex anywhere.
- No allocation in `Publish` or `Consume`.
- Ring is fixed-size, pre-allocated.

That is a Disruptor. Everything beyond this (multi-consumer, dependency barriers, batch processing) is built on top of this core.

---

### Summary: The Rules

1. **Locks kill throughput at high contention.** Use atomics.
2. **Memory access patterns matter more than instruction count.** Cache misses dominate.
3. **Pre-allocate everything in the hot path.** No `make`, no `new`.
4. **Single-writer principle:** one goroutine owns writes to a given memory region.
5. **Coordinate with atomic sequence counters, not with locks.**
6. **Ring buffer size must be a power of 2.** Use bitmask for indexing.
7. **Pad hot variables to cache-line size (64 bytes) to prevent false sharing.**
8. **Go channels are not fast enough for a matching engine hot path.** Good for everything else.

---

### Drill 1

Answer each question. Show your work — code, numbers, mechanism. "I think X" without mechanism gets zero credit.

**Q1. Mechanism question.**
Explain in your own words, at the hardware level, why a mutex-based queue is ~10x slower than a lock-free SPSC ring buffer at 10M ops/sec. Name at least three distinct costs a mutex pays that the ring buffer doesn't. Don't say "locks are slow" — explain what "slow" means physically.

**Q2. Implementation.**
Write a complete SPSC ring buffer in Go. Requirements:
- Size 1024. Use a bitmask for indexing.
- Single atomic int64 for producer sequence, single atomic int64 for consumer sequence.
- Padding between the two sequences to prevent false sharing.
- `Publish(e Event)` blocks (yields) when the buffer is full.
- `Consume() Event` blocks (yields) when the buffer is empty.
- No `sync.Mutex`, no channels.
- Event type: a struct containing an order ID (int64), a price (int64), a quantity (int64), and a side (byte). Compute the struct's size. Is it cache-line friendly? If not, pad it.

**Q3. Bitmask question.**
Why must the ring size be a power of 2? Show the bit pattern. If I tell you "I want the ring to hold exactly 1000 slots," what do you say to me, and why?

**Q4. Benchmark.**
Using Go's `testing.B`, benchmark three implementations passing 10M `Event`s from one producer goroutine to one consumer goroutine:
- (a) `chan Event` with buffer size 1024.
- (b) A `sync.Mutex`-protected slice-based queue of size 1024.
- (c) Your ring buffer from Q2.

Report ns/op for each. Which is fastest? Which is slowest? By how much? If your ring buffer isn't at least 3x faster than the channel, something is wrong — find it.

**Q5. False sharing hunt.**
In your Q2 implementation, point to the exact line(s) where false sharing would occur if you removed the padding. Explain which two variables would collide on a cache line and which cores would be fighting for it. Then remove the padding, re-run the benchmark, and report the performance delta. If you don't see a delta, explain why your particular struct layout happened to avoid it by accident.

**Q6. Reading.**
Read the original LMAX Disruptor technical paper by Martin Thompson, Dave Farley, et al. (2011, "LMAX Disruptor: High Performance Alternative to Bounded Queues for Exchanging Data Between Concurrent Threads"). It's ~11 pages. After reading, answer:
- What throughput did LMAX achieve on what hardware?
- What does "mechanical sympathy" mean, and what is one specific example from the paper?
- Why does the paper argue that queues (even lock-free queues) are architecturally wrong for this problem?

---

## Lesson 2: Determinism

### Why This Lesson Exists

A matching engine that gives different answers on different runs is worthless. You can't fail over. You can't replay. You can't audit. You can't even debug — "it worked on my machine" becomes "it worked that time" and you're screwed.

Worse: exchanges run replicas. Primary in one data center, hot standby in another. If primary dies, standby takes over mid-second with zero data loss. For that to work, the standby must be in the *exact same state* as the primary — bit-for-bit identical. The only way to achieve that without expensive state transfer is **determinism**: both run the same code over the same input, produce the same output. Cheap, correct, audit-friendly.

This lesson is short on code and long on concepts because the concepts are where people fail. Once you understand the mental model, the code is obvious.

---

### The Pure Function

Write this on your wall:

```
match(state, event) -> (state', outputs)
```

The matching engine is a pure function. Given a state (the order book) and an event (a new order, a cancel, etc.), it produces a new state and a list of outputs (fills, ack messages). Same inputs, same outputs. Always. Forever.

"Pure" means:
- No hidden inputs (no reading the wall clock, no reading environment variables, no calling the network)
- No hidden outputs (no logging side effects that affect state, no global counters)
- No randomness
- No ordering dependencies on anything external

If you can write your engine as a pure function, everything becomes easy: replication is just "run the function on both machines," replay is just "run the function again on the log," debugging is just "find the event that produced the bug and run the function with that event."

---

### Event Sourcing (One Paragraph)

The authoritative state is the ordered log of inputs, not the in-memory book. The book is derived. You can throw the book away and rebuild it from the log.

For the matching engine: the Kafka topic of "order events" IS the exchange. The in-memory order book is a cache.

---

### State Machine Replication

Two replicas consuming the same input log through the same pure function will end up in identical state. This is called **state machine replication**, and it's the theoretical foundation of distributed consensus (Raft, Paxos).

The hard part is agreeing on the input order. That's what consensus protocols solve. **The matching engine itself doesn't need to know anything about consensus** — it just needs to be deterministic. Consensus is upstream (the sequencer). Your engine's job is to be a pure function that produces identical output given identical input.

This separation is elegant and powerful. It's also why "deterministic matching engine" is a commodity building block, while "consensus" is a research problem. You build the first, you consume the second.

---

### Sources of Non-Determinism in Go (MEMORIZE)

Go has specific landmines that will trash your determinism if you're not careful.

**1. `time.Now()` — wall clock**

Wall clock moves forward, unpredictably, at different rates on different machines. Two replicas calling `time.Now()` get different answers. NEVER call `time.Now()` inside the matching engine.

Fix: the event carries the timestamp. The sequencer upstream stamps it once. The engine reads `event.Timestamp`. That's the engine's only notion of time.

```go
// WRONG
func (e *Engine) handleOrder(o Order) {
    trade := Trade{
        Timestamp: time.Now().UnixNano(),  // non-deterministic!
    }
}

// RIGHT
func (e *Engine) handleOrder(o Order, logicalTime int64) {
    trade := Trade{
        Timestamp: logicalTime,  // from the event
    }
}
```

**2. Map iteration order**

Go deliberately randomizes map iteration order on every run. This was a design decision to stop people from depending on iteration order. This is normally a good thing. For determinism it's a disaster.

```go
// WRONG - different order on every run, across replicas
for price, level := range e.bids {
    // ...
}
```

Fix: never iterate a map in logic that produces output. Use sorted slices or a tree.

**3. Goroutine scheduling**

If two goroutines are mutating state, their interleaving is non-deterministic. Replica A might process event 1 then 2; replica B might process 2 then 1. Different state.

Fix: **the matching engine is single-goroutine.** One goroutine reads from the input ring buffer, processes each event fully before the next. No concurrent mutation. This is non-negotiable.

**4. Floating point math**

`0.1 + 0.2 != 0.3` is the classic example. But it gets worse: floating-point operations on some CPUs use 80-bit intermediate precision, on others 64-bit, and the compiler can reorder operations in ways that change results. `(a + b) + c` can differ from `a + (b + c)` in float.

Never use `float64` for money. Ever. Even if it "seems fine." It's a ticking bomb.

Fix: **fixed-point integer arithmetic.** Store prices and quantities as `int64` with an implicit decimal scale.

```go
// Price is in units of 1e-8 (satoshis). So 50000.12345678 USD becomes:
const priceScale = 100_000_000

priceTicks := int64(50_000_12345678)  // 50000.12345678 * 1e8
```

All arithmetic is integer arithmetic. Deterministic on every platform, every compiler. Exchanges call this "ticks" or "pips."

**5. `math/rand` without fixed seed**

Non-deterministic. Don't use it in the engine. If you need randomness (you shouldn't), seed from the event ID.

**6. Pointer addresses leaking into logic**

`fmt.Sprintf("%p", ptr)` or sorting by pointer value is non-deterministic — memory addresses change between runs.

Less obviously: Go's map uses the memory address of pointer keys as part of the hash, which is one reason iteration order differs.

**7. `string([]byte)` via unsafe**

If you use `unsafe.Pointer` tricks to avoid allocation when converting between string and `[]byte`, you can introduce weird bugs that depend on memory layout. More on this in Lesson 3.

**8. Concurrent Go code with `sync.Map`**

`sync.Map` has non-deterministic behavior under contention. Not relevant for a single-threaded engine, but don't reach for it.

---

### The Order Book Data Structure (the hard part)

You can't use a Go map for the order book because map iteration order is random. You need a structure that:

1. Lets you find the best bid / best ask in O(1) or O(log n).
2. Lets you iterate price levels in sorted order (descending for bids, ascending for asks).
3. Preserves time priority within a price level (FIFO).
4. Is deterministic.

**Standard choice: a sorted tree.** Red-black tree or B-tree keyed by price. Go's stdlib doesn't have one; use `github.com/google/btree` or similar. For a toy engine you can start with a sorted slice — insertion is O(n) but for small books it's faster than a tree due to cache locality.

**Price-time priority:** within a price level, orders are matched in the order they arrived. Store each price level as a FIFO queue.

```go
type PriceLevel struct {
    Price  int64         // fixed-point price
    Orders []*Order      // FIFO queue, head matched first
}

type OrderBook struct {
    // Sorted descending by price (best bid at index 0)
    Bids []*PriceLevel
    // Sorted ascending by price (best ask at index 0)
    Asks []*PriceLevel
}
```

When a new order arrives:
1. Match against the opposite side, highest-priority first.
2. If any quantity remains, insert into the same side, maintaining sort order.

Simple, deterministic, works.

---

### A Deterministic Matching Engine Skeleton

```go
// Event is the input. Timestamp is logical time from the sequencer.
type Event struct {
    EventID   uint64
    Timestamp int64   // set upstream, engine never calls time.Now()
    Type      EventType  // ORDER_PLACE, ORDER_CANCEL
    Order     Order
}

type Engine struct {
    book *OrderBook
    // Output channel for trades and acks. Consumed downstream.
    out chan<- Output
}

// Step is the pure function. Given current state (self) and an event,
// mutates state and emits outputs.
func (e *Engine) Step(evt Event) {
    switch evt.Type {
    case OrderPlace:
        e.handlePlace(evt.Order, evt.Timestamp)
    case OrderCancel:
        e.handleCancel(evt.Order.ID, evt.Timestamp)
    }
}

// Single goroutine loop. No concurrency inside.
func (e *Engine) Run(in <-chan Event) {
    for evt := range in {
        e.Step(evt)
    }
}
```

That's the engine. Single goroutine, processes one event at a time, never touches the clock, operates on sorted structures. Replace the `chan Event` with your Lesson 1 ring buffer for real performance, but the semantics are identical.

---

### The Replay Test

Here's the killer test for determinism. Feed the engine a sequence of 10,000 events from a JSON file. Record every output (every trade, every ack, every book snapshot) to a second file. Restart the engine. Replay the same events. Diff the output files.

**They must be byte-identical. Not "roughly the same." Byte-identical.**

If they're not, you have a non-determinism bug. Hunt it down immediately. Common causes:
- You logged `time.Now()` somewhere.
- You ranged over a map.
- You used a float.
- You had concurrent writes.

This test is called a **determinism harness** and every serious exchange runs it in CI on every commit. If any commit breaks determinism, the CI fails. That's how you keep the property — you enforce it mechanically.

---

### Why Determinism Makes Replication Trivial

Imagine two machines, primary and standby. Both consume the same Kafka topic. Both run the same engine code. If the engine is deterministic, their in-memory state is bit-identical at every moment.

Primary dies at event 1,000,523. Standby has already processed event 1,000,523 (or is about to). Failover: standby becomes primary. Zero state transfer, zero risk of inconsistency, sub-second failover. This is the whole reason deterministic matching engines exist.

Non-deterministic engines need state snapshots: primary periodically writes its in-memory state to disk, standby reads it. Gigabytes of data, seconds of downtime, all kinds of consistency edge cases. Don't be that exchange.

---

### Summary: The Rules

1. **Write the engine as `match(state, event) -> (state', outputs)`.** Pure function. No hidden inputs, no hidden outputs.
2. **Never call `time.Now()` in the engine.** Time comes from the event.
3. **Never iterate a Go map in output-producing logic.** Use sorted slices or trees.
4. **Never use `float64` for money.** Fixed-point `int64` only.
5. **Single goroutine.** No concurrency inside the engine.
6. **Run a replay test in CI.** Byte-identical outputs on identical inputs. Every commit.
7. **Consensus and time are upstream concerns.** The engine just needs to be deterministic.

---

### Drill 2

**Q1. Enumerate.**
List every source of non-determinism in this code snippet. There are at least 5. Name the mechanism for each.

```go
func (e *Engine) handleOrder(o Order) {
    o.CreatedAt = time.Now()
    var totalVolume float64
    for _, level := range e.bids {  // e.bids is map[int64]*PriceLevel
        for _, ord := range level.Orders {
            totalVolume += float64(ord.Quantity) * float64(ord.Price)
        }
    }
    if totalVolume > 1_000_000.0 {
        go e.snapshotState()
    }
    e.log.Printf("processed order %s at %p", o.ID, &o)
}
```

**Q2. Implementation.**
Write a minimal deterministic matching engine in Go that:
- Reads newline-delimited JSON events from `stdin`. Event format: `{"type": "place", "order_id": "...", "side": "buy"|"sell", "price": "50000.50", "quantity": "1.5", "timestamp": 1234567890}`. Price and quantity are strings to avoid JSON float parsing.
- Supports only two event types: `place` (limit order) and `cancel`.
- Matches with price-time priority.
- Emits trades to `stdout` as newline-JSON: `{"taker_order_id": "...", "maker_order_id": "...", "price": "...", "quantity": "...", "timestamp": ...}`.
- Uses fixed-point int64 for all prices and quantities (scale: 1e8).
- Uses no floats. Uses no maps in logic. Uses no `time.Now()`.

**Q3. Determinism harness.**
Write a shell script (or Go program) that:
- Runs your engine on `input.jsonl`, saves output to `run1.jsonl`.
- Runs your engine again on `input.jsonl`, saves output to `run2.jsonl`.
- Diffs them. Exits nonzero if they differ.

Generate an `input.jsonl` with at least 1,000 events. Run your harness 10 times. They must all pass.

**Q4. Break it on purpose.**
Now deliberately break your engine in four ways, one at a time, re-running the harness each time to confirm it detects the break:

(a) Replace your fixed-point `int64` price with `float64`. Does the harness catch it? Explain why or why not.
(b) Replace your sorted-slice order book iteration with a `map[int64]*PriceLevel` and range over it. Does the harness catch it?
(c) Add `trade.Timestamp = time.Now().UnixNano()` inside the matching function. Does the harness catch it?
(d) Launch a second goroutine that mutates the book concurrently. Does the harness catch it?

For each, report: did the harness catch the break on the first run? The 5th run? Explain why — some of these are *probabilistic* failures and won't be caught immediately. That probabilistic behavior is exactly why determinism harnesses must run many times.

**Q5. Conceptual.**
Explain, in 3-4 sentences, why two deterministic replicas consuming the same Kafka partition end up in identical state without any coordination. What happens if replica B is slightly behind replica A? What happens if replica B crashes and restarts?

---

## Lesson 3: Memory Management

### Why This Lesson Exists

You now have a deterministic engine (Lesson 2) coordinating through a lock-free ring buffer (Lesson 1). You run a benchmark. It does 1M orders/sec with p50 of 5μs. You're thrilled. Then you look at p99. It's 2 milliseconds.

Where did 2ms come from? **Garbage collection.** The Go runtime paused your hot loop to scan the heap. For a matching engine promising microsecond latency, 2ms is catastrophic — during that pause, thousands of events piled up, and every trader got a terrible experience.

This lesson is about making your hot path **allocation-free** so the GC never runs in your critical section. The techniques are specific and countable. Once you know them, they're not hard. You just have to follow them every time.

---

### How Go's GC Works (Just Enough to Hate It)

Go uses a **concurrent mark-sweep tri-color garbage collector with write barriers**. In plain English:

1. **Mark phase:** the GC walks the heap starting from roots (globals, goroutine stacks), coloring reachable objects. Runs concurrently with your program for the most part.
2. **Sweep phase:** unmarked objects are freed.
3. **Write barrier:** every pointer write in your code, while GC is running, triggers a tiny bit of extra bookkeeping so the GC can track changes.

The GC tries hard to avoid stopping your program, but it still has short stop-the-world (STW) pauses — measured in hundreds of microseconds to low milliseconds, depending on heap size and workload. Modern Go (1.20+) targets sub-ms pauses. They mostly hit it. For HFT, "mostly" is not acceptable.

**Three specific ways the GC hurts you:**

1. **Stop-the-world pauses.** Even 500μs is 500x your target latency.
2. **Write-barrier overhead during mark.** Every pointer write in your hot loop gets slower (5-10ns each). If you have many pointers, throughput drops.
3. **CPU stolen for marking.** The GC runs concurrent marking on some of your CPU cores. Less CPU for your engine.

**The knob you have:** `GOGC` environment variable (or `runtime/debug.SetGCPercent`). Default is 100, meaning GC runs when heap doubles. Setting it to 500 means GC runs less often but uses more RAM. Setting `GOGC=off` disables it entirely — dangerous unless you have zero allocations.

There's also `GOMEMLIMIT` (Go 1.19+) which caps memory instead of heap growth ratio. Useful for container environments.

**The only real fix: stop allocating.** If your hot loop doesn't allocate, the GC has nothing to do. `GOGC=off` becomes safe. Pauses disappear. You're running at Rust speed in Go.

---

### Measuring Allocations

Before you can fix allocations, you have to see them.

**Method 1: `go test -benchmem`**

```
go test -bench=. -benchmem

BenchmarkEngine-8    1000000   1523 ns/op   248 B/op   4 allocs/op
```

`4 allocs/op` is the number that matters. For a hot path, your target is **0 allocs/op**. Not "low." Zero.

**Method 2: `GODEBUG=gctrace=1`**

Set this env var and run your program. The runtime prints a line every GC cycle:

```
gc 1 @0.012s 5%: 0.018+0.53+0.015 ms clock, 0.14+0.19/0.52/1.2+0.12 ms cpu, 4->4->0 MB, 5 MB goal, 8 P
```

The `0.018+0.53+0.015 ms` is STW + concurrent + STW. If you see these numbers growing, you're allocating.

**Method 3: pprof heap profile**

```
go test -bench=. -memprofile=mem.prof
go tool pprof mem.prof
(pprof) top
```

Shows you which functions allocate the most. Fix the biggest first.

**Method 4: `go build -gcflags="-m"`**

Shows escape analysis — which variables escape to the heap. If you see `moved to heap: x` for something you thought was stack-allocated, you have a problem.

---

### The Allocation Sources in Go (know all of them)

Things that allocate on the heap, often silently:

**1. `make()`, `new()`**
Obvious. `make([]Order, 100)` allocates 100 orders on the heap. In the hot path, bad.

**2. Slice growth via `append`**
```go
orders := make([]Order, 0, 10)   // cap 10
for i := 0; i < 100; i++ {
    orders = append(orders, Order{...})  // will realloc at cap 10, 20, 40, 80, 160
}
```
Each reallocation copies the entire slice. 5 allocations for a 100-element fill. Pre-size your slice, or use a pool.

**3. Interface boxing**
```go
func process(x interface{}) { }  // or 'any' in Go 1.18+
process(42)  // 42 gets boxed into an interface{}, allocating the int on the heap
```
Every interface method call on a concrete type can box the receiver. `fmt.Println(x)` boxes `x`. JSON marshaling boxes everything. Avoid `interface{}` in the hot path.

**4. Closures capturing variables**
```go
for _, order := range orders {
    go func() { process(order) }()  // captures order by reference — escapes to heap
}
```
Every closure that captures a variable causes that variable to escape. Prefer passing as argument:
```go
for _, order := range orders {
    go func(o Order) { process(o) }(order)  // by value, may stay on stack
}
```

**5. String / byte conversions**
```go
s := string(byteSlice)   // allocates a new string
b := []byte(s)           // allocates a new byte slice
```
Both copy the data. In the hot path, use `unsafe` tricks or avoid conversion altogether.

**6. `fmt.Sprintf`, `fmt.Printf`**
Allocates like crazy. Use `strconv.AppendInt` into a pre-allocated buffer.

**7. `encoding/json` marshal/unmarshal**
Allocates heavily. For the hot path, use code-generated serializers (`easyjson`, `ffjson`) or, better, a binary format like `flatbuffers` or hand-rolled `encoding/binary` with pre-allocated buffers.

**8. Map operations**
`m[k] = v` can trigger a rehash and allocate new buckets. Maps grow; they don't shrink. Sizes grow in powers of 2.

**9. Taking address of local variables (sometimes)**
```go
func f() *Order {
    o := Order{}     // might be on stack
    return &o        // now escapes to heap
}
```
Escape analysis will catch most of these, but `go build -gcflags="-m"` is your friend.

**10. Boxing small types in maps / slices of interfaces**
`map[string]interface{}` boxes every value. `[]interface{}` boxes every element.

---

### Object Pools: The Primary Technique

If you must reuse objects, pool them. Go has `sync.Pool`:

```go
var orderPool = sync.Pool{
    New: func() interface{} {
        return &Order{}
    },
}

// Get
o := orderPool.Get().(*Order)
// use o
// Done with o: zero it and return
*o = Order{}  // important — avoid leaking old data
orderPool.Put(o)
```

Caveats:
- `sync.Pool` objects can be freed by the GC at any time. It's a cache, not a guarantee.
- The pool uses per-P (per-processor) free lists, so it has some synchronization cost under contention.
- Don't pool things that are expensive to zero.

**For a matching engine, a hand-rolled pool is usually better:**

```go
type OrderPool struct {
    orders [65536]Order   // pre-allocated, fixed size
    free   []uint32       // free list: indices into orders
}

func (p *OrderPool) Alloc() *Order {
    if len(p.free) == 0 {
        panic("order pool exhausted")
    }
    idx := p.free[len(p.free)-1]
    p.free = p.free[:len(p.free)-1]
    return &p.orders[idx]
}

func (p *OrderPool) Free(o *Order) {
    idx := uint32((uintptr(unsafe.Pointer(o)) - uintptr(unsafe.Pointer(&p.orders[0]))) / unsafe.Sizeof(Order{}))
    *o = Order{}
    p.free = append(p.free, idx)
}
```

Zero GC pressure. Deterministic. You know exactly how much memory the engine uses. If the pool is exhausted, that's a capacity planning bug, not a runtime crash.

---

### The Flyweight Pattern (Indices, Not Pointers)

Taking it one step further: **stop using pointers entirely.** Represent orders by `uint32` indices into the pool array.

```go
type OrderRef uint32

type PriceLevel struct {
    Price     int64
    OrderRefs []OrderRef  // FIFO of indices
}

// Lookup:
order := &pool.orders[ref]
```

Why this matters:
- `OrderRef` is 4 bytes; `*Order` is 8 bytes. Half the memory.
- `[]OrderRef` is cache-friendlier than `[]*Order` because it's a tight array of integers.
- No pointers means no GC scanning — the GC doesn't need to scan the FIFO queues.
- No pointer means no indirection when iterating — the order data itself is still in the pool array, but `OrderRefs` is packed.

This is the start of **data-oriented design**.

---

### Pre-Allocating Everything

Your engine at startup should grab all the memory it will ever need:

```go
func NewEngine() *Engine {
    e := &Engine{
        orderPool:  NewOrderPool(1 << 20),    // 1M orders
        tradePool:  NewTradePool(1 << 20),    // 1M trades
        // Pre-size every slice and map to expected capacity
        bidLevels:  make([]PriceLevel, 0, 10000),
        askLevels:  make([]PriceLevel, 0, 10000),
        // Buffer for serializing output events
        outBuf:     make([]byte, 0, 65536),
    }
    // Fill the free lists
    for i := uint32(0); i < 1<<20; i++ {
        e.orderPool.free = append(e.orderPool.free, i)
    }
    return e
}
```

Now the hot path never calls `make()`, never calls `new()`, never allocates. All allocations happened at startup. Warmup period (first few thousand events) may trigger some lazy initialization, but once steady state is reached, `B/op` and `allocs/op` are both zero.

---

### Data-Oriented Design: Struct of Arrays

Instead of:

```go
type Order struct {
    ID        uint64
    Price     int64
    Quantity  int64
    Side      byte
    Timestamp int64
}
// Then: orders [1M]Order
```

...which is **Array of Structs (AoS)** — you lay out memory as:

```go
type OrderPool struct {
    IDs        [1 << 20]uint64
    Prices     [1 << 20]int64
    Quantities [1 << 20]int64
    Sides      [1 << 20]byte
    Timestamps [1 << 20]int64
}
```

This is **Struct of Arrays (SoA)**. Why it matters:

When the matching loop scans a price level looking for orders to match, it reads each order's Price and Quantity. With AoS, each order is 32+ bytes, so the CPU pulls the full order into cache even if you only need 2 fields. With SoA, Prices and Quantities are packed in their own arrays — the CPU pulls 8 prices per cache line, 8 quantities per cache line. 4-8x better cache utilization in the scanning loop.

This is how HFT systems squeeze microseconds. Is it worth it for your first engine? No — premature. But know the technique exists. When you're profiling and see cache misses in your match loop, you'll know what to do.

---

### Off-Heap Memory in Go

Java has a rich off-heap ecosystem (Agrona, Chronicle, `sun.misc.Unsafe`). Go has... `unsafe.Pointer` and `mmap`. That's basically it. You can:

```go
import "syscall"

data, err := syscall.Mmap(-1, 0, 1<<30,
    syscall.PROT_READ|syscall.PROT_WRITE,
    syscall.MAP_ANON|syscall.MAP_PRIVATE)
// 'data' is a []byte backed by 1GB of off-heap memory
// Interpret as you wish with unsafe.Pointer
```

You manage this memory yourself. The GC doesn't scan it. It's truly off-heap.

Most Go engines don't bother. They pre-allocate big on-heap arrays (which the GC scans but rarely, because they contain no pointers — an `[]int64` is scanned once, not per-element) and call it a day. When you need off-heap, you know.

---

### Go-Specific Gotchas Specific to Engines

**1. Don't store `*Order` in maps or slices scanned frequently.**
GC scans pointer-containing structures. If your price level is `[]*Order`, the GC walks 1M pointers on each mark. If it's `[]OrderRef` (alias for `[]uint32`), the GC skips it entirely.

**2. Watch out for `interface{}` in error handling.**
`error` is an interface. Creating errors allocates. Use sentinel errors (`var ErrBadOrder = errors.New(...)`) at startup, never `fmt.Errorf` in the hot path.

**3. `defer` has a tiny allocation cost in older Go.**
Go 1.14+ made `defer` allocation-free in most cases. Still, if you see it in profiles, inline the cleanup.

**4. JSON is garbage collection poison.**
Never use `encoding/json` in the engine hot path. Use binary formats or generate code with `easyjson`. For inter-service communication outside the hot path, regular JSON is fine.

**5. Buffered channels allocate per send.**
Actually no — buffered channels pre-allocate their buffer. But the `select` statement's internals can allocate under some conditions. For your engine, prefer the ring buffer from Lesson 1 over channels in the hot path.

**6. `string(buf)` where buf is `[]byte` always allocates.**
Go guarantees string immutability, so converting `[]byte → string` must copy. There's an `unsafe` trick (`*(*string)(unsafe.Pointer(&buf))`) that skips the copy but breaks string immutability if you mutate the buffer afterwards. Use at your own risk. In the hot path where every ns counts, people use it and are careful.

---

### Benchmarking Your Engine

```go
func BenchmarkEngine(b *testing.B) {
    engine := NewEngine()
    events := prepareEvents(b.N)  // generate ahead of time
    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        engine.Step(events[i])
    }
}
```

Run with `go test -bench=. -benchmem -benchtime=10s -cpuprofile=cpu.prof -memprofile=mem.prof`.

Targets for a toy engine on modern hardware (your laptop):
- Throughput: 1M+ events/sec (toy), 5-10M events/sec (tuned)
- p50: <1 μs
- p99: <10 μs
- p99.9: <50 μs
- allocs/op in steady state: 0
- B/op in steady state: 0

If your p99.9 is 100μs or more and you see GC pauses in `gctrace`, you have allocations. Hunt them. If throughput is 100K/sec, you have lock contention or something silly — profile and fix.

---

### Summary: The Rules

1. **Zero allocations in the hot path.** Measure with `-benchmem`. Target `0 allocs/op`.
2. **Pre-allocate at startup.** Orders, trades, buffers — all of it. The engine's memory footprint is fixed.
3. **Use object pools.** `sync.Pool` is OK; hand-rolled index-based pools are better.
4. **Represent entities by indices (`uint32`), not pointers.** Keeps GC out of your way.
5. **Never use `float64` for money, never use `interface{}` in hot loops, never use JSON in the engine.**
6. **Measure before optimizing.** `pprof`, `-benchmem`, `gctrace=1` tell you the truth; your intuition doesn't.
7. **The engine's hot path is `match(state, event) -> outputs`. Nothing in that function allocates.**

---

### Drill 3

**Q1. Allocation audit.**
Take your Lesson 2 engine. Run `go test -bench=. -benchmem` on a benchmark that processes 1M events. Report:
- ns/op
- B/op
- allocs/op

If allocs/op > 0, identify every allocation source. Post the offending lines and the reason they allocate.

**Q2. Make it zero-alloc.**
Modify your engine so that the hot path (the `Step` function and everything it calls) performs zero allocations after warmup. Techniques you must use:
- Pre-allocate an order pool with at least 1M entries at startup.
- Represent orders in price levels by index (`uint32`), not pointer.
- Pre-size all internal slices.
- Replace any JSON marshaling in the hot path with a binary protocol (`encoding/binary`) writing into a pre-allocated buffer. If you still need JSON for the output file, do the serialization *outside* `Step`, in the consumer goroutine.

Re-run the benchmark. `allocs/op` must be 0. `B/op` must be 0 (or very close — some runtime metadata may show up).

**Q3. Latency histogram.**
Measure p50, p95, p99, p99.9, and p99.99 latency of `Step()` over 10M events. Use `github.com/HdrHistogram/hdrhistogram-go` or equivalent (not a Go map — that allocates). Report the numbers.

Now run the same benchmark with `GOGC=off`. Compare. If there's no difference, you genuinely have zero allocations — congrats. If p99.9 drops significantly with `GOGC=off`, you still have allocations somewhere. Find them.

**Q4. Data-oriented design experiment.**
Convert the order storage from Array-of-Structs to Struct-of-Arrays. Keep `[]uint32` references in price levels. Benchmark match-loop latency specifically (when a large order sweeps 100 price levels). Report the delta. Is SoA faster on your hardware? Explain the result in terms of cache lines.

**Q5. Off-heap.**
Using `syscall.Mmap`, allocate 256MB of off-heap memory at engine startup. Use it as the backing store for your order pool — cast the `[]byte` to `[]Order` with `unsafe.Pointer`. Verify with `GODEBUG=gctrace=1` that the GC ignores this memory (look at the heap size in the gctrace output). What's the risk of this approach? When would you use it in production, and when wouldn't you?

**Q6. Putting it all together.**
Your final Phase 1 engine should have:
- Lesson 1's ring buffer feeding events into the engine.
- Lesson 2's deterministic pure-function core.
- Lesson 3's zero-allocation hot path.

Run the full pipeline at 10M events through a pre-generated event log. Report:
- Throughput (events/sec sustained)
- Latency percentiles (p50, p99, p99.9, p99.99)
- Number of GC cycles during the run (from `gctrace`)
- Total bytes allocated during the run (from `runtime.MemStats.TotalAlloc`)

Target for your laptop: 5M+ events/sec sustained, p99.9 under 50μs, zero GC cycles after warmup. If you hit these, you've built the skeleton of a real matching engine.

---

## Phase 1 Master Rules

### Low Latency
- Know the latency numbers by heart. RAM is 100x slower than L1 cache. A contended mutex is 1000+ ns.
- Locks kill throughput under contention. Use atomics (compile to single CPU instructions).
- Cache lines are 64 bytes. Accessing one byte pulls in 63 more — plan your data layout accordingly.
- False sharing is invisible and catastrophic. Pad hot variables to 64 bytes of separation.
- Ring buffer size must be a power of 2. Bitmask indexing is a single cycle; modulo is 20-40.
- Single-writer principle. One goroutine owns writes to a given region. Others read only.
- Go channels cap around 5-20M ops/sec. A hand-rolled SPSC ring buffer hits 100M+.

### Determinism
- Matching engine is a pure function: `match(state, event) -> (state', outputs)`.
- Time comes from the event, not `time.Now()`.
- Never iterate a Go map in output-producing logic. Maps are randomized by design.
- Never use `float64` for money. Fixed-point `int64` only.
- Single goroutine in the engine. Concurrency is outside.
- Run a byte-diff replay harness in CI. Every commit.

### Memory Management
- Target zero allocations in the hot path. Measure with `go test -benchmem`.
- Pre-allocate at startup — orders, trades, buffers, output space. Fixed memory footprint.
- Represent entities by `uint32` index into pool arrays, not `*Order` pointers. GC never scans.
- Never use `interface{}` or `encoding/json` in the engine hot path.
- Struct-of-Arrays beats Array-of-Structs when scan loops only need a few fields.
- `GOGC=off` is safe only when you genuinely have zero allocations.

### If You Do Phase 1 Right
- 5M+ events/sec throughput on a laptop.
- p99.9 latency under 50μs.
- Zero GC cycles after warmup.
- Byte-identical output on replay.
- Foundation for Phase 2 (sequencer, risk checks, networking, persistence).

---

*Phase 1 complete. Phase 2: Sequencer, FIX/WebSocket gateway, pre-trade risk, state persistence.*
