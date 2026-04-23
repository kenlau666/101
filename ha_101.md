# CEX Matching Engine — Phase 2 Course (Go)

> Reliability: Persistence & High Availability
> Three lessons. Linux primitives taught inline. Harsh drills at the end.

---

## Phase 2 Introduction

Phase 1 gave you a matching engine that runs fast on one machine. Necessary, not sufficient. That machine can crash. The power can fail. A kernel panic can kill the process mid-match. A SSD can die. A rack PDU can trip.

When (not if) this happens, your users lose money if you lose state. Orders that were confirmed disappear. Trades that happened vanish from history. Regulators fine you. Traders leave. The exchange dies.

Phase 2 is about surviving failure. Three lessons:

1. **Persistence (Event Sourcing + WAL):** write every event to durable storage *before* processing. If the process dies, we replay the log and reconstruct state.
2. **High Availability (Raft + Aeron):** run multiple replicas. When one fails, another takes over in milliseconds with zero data loss.
3. **Snapshotting (Copy-on-Write):** periodically checkpoint state so recovery doesn't replay from the dawn of time. Use `fork()` and COW to snapshot without pausing trading.

You said you don't know Linux. Fine. I'll teach the primitives as they come up. Don't skip them — the concepts are the vocabulary of everything else in this phase.

---

## Linux Primer 0: Process, File Descriptor, Syscall

You need these three concepts before anything else. Five minutes, once.

### Process

When you run `./my_engine`, the kernel creates a **process**. A process is an isolated environment:
- Its own **memory space** (starts empty, fills as code runs)
- Its own **file descriptors** (handles to open files, sockets, etc.)
- Its own **PID** (process ID — a number like 12345)
- One or more **threads** of execution

Each process thinks it's alone in the universe. The kernel provides the isolation. Process A can't read process B's memory, can't touch its files, can't send it signals unless explicitly allowed. If A crashes, B is unaffected.

Your Go program = one process. Goroutines run inside that process, multiplexed across OS threads by Go's runtime. Different goroutines share memory; different processes don't.

### File Descriptor (fd)

When your program opens a file, the kernel doesn't give you the file — it gives you an **integer**. Usually small: 0, 1, 2 are reserved for stdin/stdout/stderr, and your first `os.Open` returns fd 3, next is 4, and so on. This integer is an index into a per-process table the kernel maintains.

```go
f, _ := os.Open("foo.txt")
// f wraps an fd (e.g., 3)
// Every Read/Write on f is a syscall with fd=3
```

File descriptors aren't just for files. They point to:
- Files on disk
- Network sockets (TCP/UDP connections)
- Pipes (inter-process communication)
- Special kernel objects (epoll for event notification, timerfd for timers)

The Unix philosophy: "everything is a file." Every I/O = read/write on a file descriptor. Your Go code hides this, but it's always there.

### Syscall

Your Go program runs in **userspace** — a restricted environment where you can't directly touch hardware, can't directly read other processes' memory, can't send network packets. To do anything interesting, you ask the kernel to do it for you. That request is a **syscall** (system call).

```
Your code:              f.Write(data)
Go runtime:             calls os.File.Write → eventually SYS_WRITE
CPU:                    SYSCALL instruction → switches to kernel mode
Kernel:                 does the actual write, returns result
CPU:                    switches back to user mode
Your code:              gets Write return value
```

Each syscall costs ~100-500ns just for the mode switch, before any actual work. For HFT, you batch — write 100 events in one syscall rather than 100 syscalls of 1 event each.

**Userspace / kernel boundary.** This boundary is why your process can't corrupt the kernel, can't read other processes' memory, can't break the system. It's also why syscalls are slow. Every system-level operation crosses it.

OK. Equipped with process/fd/syscall, let's do persistence.

---

# Lesson 4: Event Sourcing & Write-Ahead Log (WAL)

### Why This Lesson Exists

Your Phase 1 matching engine processes events from a ring buffer. Great. But where do those events come from? And what happens when the engine crashes?

Think about it. The engine's in-memory order book *is* the exchange's state. If the process dies, that RAM is gone. The order book is gone. Every resting order, every position — vanished. Reboot the engine and it comes up empty. Traders who had orders in the book have no way to know the orders evaporated.

This is unacceptable. The exchange must be able to recover to exactly the state it was in before the crash. To do that, it must have a **durable record of every event** that got it there.

**Enter the Write-Ahead Log (WAL).** The rule: before processing any event, write it to durable storage. If we crash, we replay the log and reconstruct state. If the log is safe, the exchange is safe.

This is called **event sourcing**: state is derived from an ordered log of events. The log is truth. The in-memory state is a cache.

Write this on your wall:

```
Write event to log. Wait for durable. THEN process event.
```

Not the other way around. Never the other way around.

---

### Linux Primer 1: Disks and File I/O

Before you can build a WAL, you need to know how writes actually reach disk.

**Disk hierarchy:**

```
Medium              Random read        Sequential write       Cost
HDD (spinning)      ~10ms              ~100 MB/s              cheap
SATA SSD            ~100μs             ~500 MB/s              mid
NVMe SSD            ~10μs              ~3 GB/s                pricier
```

Matching engines use NVMe. And they write **sequentially** — an append-only log is the fastest possible write pattern on any medium. Even on SSD, where random writes are "fast," sequential writes pack multiple adjacent blocks into one flash programming operation.

**Page cache.** The Linux kernel maintains an in-RAM cache of disk data called the **page cache**. When your program does `write(fd, data, len)`, by default, data goes into the page cache — RAM, not disk. `write()` returns immediately. The kernel writes to the actual SSD later (seconds later, typically), as a background operation.

This is fast but dangerous. If the machine loses power between `write()` returning and the kernel's background flush, your data is gone. **`write()` returning does NOT mean "on disk."**

```
Your write(fd, data, len):
  data → page cache (RAM, in the kernel)
  returns success immediately
  ...seconds later...
  kernel background flush → actual SSD
```

If you believe `write()` = durable, your exchange will lose data and you'll lose your job.

**fsync(fd).** The syscall that forces durability. `fsync(fd)` blocks until all data for that file is on actual disk. It is slow:
- NVMe: ~100μs
- SATA SSD: ~1ms
- HDD: ~10ms

But after `fsync` returns, your data is durable. Power loss won't affect it (assuming the disk isn't lying about flush semantics — enterprise SSDs don't, consumer ones sometimes do).

The durable-write sequence:

```
write(fd, data, len)    // in page cache (RAM); fast (~µs); NOT durable
fsync(fd)               // flush to disk; slow (~100μs-1ms); durable
```

For a WAL, every "event accepted" acknowledgment must come *after* fsync. If you ack before fsync, then crash, your trader thinks the order is placed, but the WAL doesn't have it, so recovery loses it. Trader sees a filled order that the exchange has no record of. That's a lawsuit.

**O_DIRECT.** A flag when opening a file that bypasses the page cache. Writes go straight to disk. Databases sometimes use this because they have their own cache and want to avoid double-caching. For a WAL, you usually *don't* use O_DIRECT — you want the page cache for fast reads during recovery. Also, O_DIRECT requires buffer alignment (buffer address and size must both be multiples of the disk block size, usually 4KB) which is a pain.

**Sequential vs random I/O.** A WAL appends to the end of a file. On HDD, no seek means 100x faster than random writes. On NVMe, sequential still beats random by ~5x. The WAL's append-only design is a performance feature.

---

### The WAL Design

Minimum viable WAL:

```
wal.log (append-only file)
┌──────────────────────────────────────────┐
│ event_1 │ event_2 │ event_3 │ event_4 │...│
│                                          │
│ ↑ appends go here, never modified        │
└──────────────────────────────────────────┘
```

An append-only file. Every event is serialized and appended. Never edited, never deleted.

Each event entry needs:
- **Length prefix** (so the reader knows where the next entry starts)
- **Sequence number** (monotonically increasing, unique per entry)
- **Timestamp** (logical time — matches Lesson 2's determinism rules)
- **Event payload** (the actual order/cancel/whatever)
- **Checksum** (CRC32 — detect corruption from partial writes on crash)

Binary format, not JSON. JSON in a WAL is a mistake: slow, bloaty, ambiguous, and breaks under partial writes.

```go
// Binary layout per entry:
// [length:4][seq:8][timestamp:8][payload:N][crc32:4]

type WAL struct {
    file *os.File
    seq  uint64
    buf  []byte
}

func (w *WAL) Append(payload []byte, timestamp int64) (uint64, error) {
    w.seq++
    total := 4 + 8 + 8 + len(payload) + 4

    // Pre-allocated buf; grow if needed (rare)
    if cap(w.buf) < total {
        w.buf = make([]byte, total)
    }
    w.buf = w.buf[:total]

    binary.LittleEndian.PutUint32(w.buf[0:4], uint32(total))
    binary.LittleEndian.PutUint64(w.buf[4:12], w.seq)
    binary.LittleEndian.PutUint64(w.buf[12:20], uint64(timestamp))
    copy(w.buf[20:20+len(payload)], payload)
    crc := crc32.ChecksumIEEE(w.buf[:20+len(payload)])
    binary.LittleEndian.PutUint32(w.buf[20+len(payload):], crc)

    if _, err := w.file.Write(w.buf); err != nil {
        return 0, err
    }
    if err := w.file.Sync(); err != nil {  // fsync
        return 0, err
    }
    return w.seq, nil
}
```

This durably persists events one at a time. Correct. But slow: one fsync per event × 100μs per fsync = max 10K events/sec. A real exchange wants 1M+. We need to fix this.

---

### Group Commit: Amortizing the fsync Tax

fsync is the bottleneck. We can't eliminate it — we need durability. But we can **amortize** it.

**Group commit** (also called batched fsync): buffer many events, then fsync once for the whole batch.

```
Engine produces events:  E1, E2, E3, ..., E1000
                                  ↓
                           one fsync
                                  ↓
                               disk
```

If we fsync once per 1000 events, we get ~100 fsyncs/sec × 1000 events = 100K events/sec durable. Each event's *durable latency* (time from "event received" to "event durable") increases — it waits for the batch to flush — but throughput jumps 100x.

The pattern: engine acknowledges events only after fsync. Events are buffered. A separate goroutine triggers the fsync and signals waiters.

```go
type GroupCommitWAL struct {
    file    *os.File
    pending []pendingEntry
    mu      sync.Mutex
    cond    *sync.Cond
    seq     uint64
}

type pendingEntry struct {
    payload   []byte
    timestamp int64
    ack       chan uint64
}

func (w *GroupCommitWAL) AppendSync(payload []byte, ts int64) uint64 {
    ack := make(chan uint64, 1)
    w.mu.Lock()
    w.pending = append(w.pending, pendingEntry{payload, ts, ack})
    w.mu.Unlock()
    w.cond.Signal()
    return <-ack
}

func (w *GroupCommitWAL) flushLoop() {
    for {
        w.mu.Lock()
        for len(w.pending) == 0 {
            w.cond.Wait()
        }
        batch := w.pending
        w.pending = w.pending[:0]
        w.mu.Unlock()

        seqs := make([]uint64, len(batch))
        for i, e := range batch {
            w.seq++
            seqs[i] = w.seq
            w.writeEntry(w.seq, e.payload, e.timestamp)
        }
        w.file.Sync()  // ONE fsync for the whole batch
        for i, e := range batch {
            e.ack <- seqs[i]
        }
    }
}
```

This is the standard pattern. PostgreSQL, MySQL, Kafka, etcd, ZooKeeper — they all do something like it. It's called "group commit" in database literature.

**Tuning knobs:**
- **Batch size trigger:** flush when N events pending.
- **Timer trigger:** flush when T microseconds elapsed since last flush.
- Whichever comes first. Typical values: N = 1000, T = 1ms.

For a matching engine, a 1ms durable latency is fine. For lower-latency requirements, use smaller timers at the cost of throughput.

---

### How the WAL Fits Into the Engine

```
[Client] → [Gateway] → [Sequencer] → [WAL] → [Matching Engine] → [Output]
                          ↓         (fsync)    (in-memory)
                      assigns seq
```

The **sequencer** assigns a monotonically increasing sequence number to each event. The WAL persists the event with that sequence. Only after fsync returns does the event flow to the matching engine.

**This ordering is non-negotiable.** Process first then persist = crash mid-processing leaves state changes that aren't in the log. Recovery can't reproduce them. Non-determinism. Don't do that.

Write-ahead = write to log, ahead of state change. Every time.

---

### Recovery

The engine crashes. Process restarts. What does it do?

1. Open the WAL file(s).
2. Read entries from the beginning, one at a time.
3. For each entry, verify the CRC. If valid, feed to the matching engine's `Step()`. If invalid, stop.
4. After the last entry, the engine is in the same state it was in just before the crash.
5. Resume accepting events from the sequencer, starting at seq + 1.

This is why determinism (Lesson 2) matters. Replay produces identical state because the engine is a pure function. Non-deterministic engines can't recover this way — their replay diverges from the original run.

**What about partial writes?** If the machine crashed mid-write, the last entry in the WAL may be truncated (only half written) or torn (some bytes flushed, others not). The CRC catches this. On recovery, when you hit a CRC mismatch, assume it's the partial-write tail — truncate the log there and continue. The sequencer will detect the missing sequence number and re-send that event.

```go
func (w *WAL) Recover(cb func(seq uint64, ts int64, payload []byte)) error {
    for {
        // Read length prefix
        var lengthBuf [4]byte
        if _, err := io.ReadFull(w.file, lengthBuf[:]); err != nil {
            if err == io.EOF {
                return nil  // clean end of log
            }
            return err
        }
        length := binary.LittleEndian.Uint32(lengthBuf[:])

        // Read rest of entry
        entry := make([]byte, length-4)
        if _, err := io.ReadFull(w.file, entry); err != nil {
            // Truncated tail — crash happened mid-write
            return nil  // truncate, stop recovery
        }

        seq := binary.LittleEndian.Uint64(entry[0:8])
        ts := int64(binary.LittleEndian.Uint64(entry[8:16]))
        payload := entry[16 : len(entry)-4]
        storedCRC := binary.LittleEndian.Uint32(entry[len(entry)-4:])

        // Reconstruct what was CRC'd
        all := append(lengthBuf[:], entry[:len(entry)-4]...)
        if crc32.ChecksumIEEE(all) != storedCRC {
            return nil  // corrupt tail, truncate
        }
        cb(seq, ts, payload)
    }
}
```

---

### WAL Rotation

An append-only file grows forever. At 1M events/sec × 200 bytes each = 200 MB/s = 720 GB/hour = 17 TB/day. You can't keep one file.

**Segments.** Split the log into chunks — typically 1 GB each. When segment N is full, start segment N+1.

```
wal/
├── wal.00000001.log   (1 GB, full)
├── wal.00000002.log   (1 GB, full)
├── wal.00000003.log   (1 GB, full)
└── wal.00000004.log   (active, appending)
```

Old segments can be:
- Kept on disk for forensics / audit / regulator
- Archived to cheap storage (S3, Glacier)
- Deleted after a snapshot covering their events is durable (Lesson 6)

Recovery reads all segments in order. Naming convention: zero-padded integers so lex ordering = temporal ordering.

---

### Go-Specific Warnings

**1. `os.File.Sync()` IS fsync.** Correct. Use it.

**2. `bufio.Writer` defeats durability.** It buffers in userspace. If you wrap a bufio writer around your WAL file, `Sync()` on the file doesn't flush the bufio buffer — your "durable" write is still sitting in a userspace buffer. Always `Flush()` the bufio before `Sync()` the file, or don't use bufio.

**3. `O_DSYNC` flag.** Opening a file with this flag makes every write act like an implicit fsync for data. Slower than group commit; usually not worth it.

**4. Pre-allocate WAL segments.** Use `file.Truncate(1<<30)` at creation to reserve 1 GB. Writing into pre-allocated space avoids filesystem metadata updates on every append. Real performance difference on ext4/xfs.

**5. Directory fsync.** When you *create* a new file, the file's existence is a *directory* metadata change. `fsync(file)` does not make the directory entry durable. You need to also fsync the directory:

```go
dir, _ := os.Open("/path/to/wal/dir")
dir.Sync()
```

Only after first creating the file, not on every write. Subtle and real.

**6. Pool your buffers.** Per Lesson 3, every `make([]byte, ...)` in the hot path is a heap allocation. Your WAL entry buffer should be pre-allocated, re-used. `sync.Pool` for per-event payload buffers.

---

### Summary: The Rules

1. **Write-ahead:** append event to WAL, fsync, THEN process.
2. **fsync or die.** `write()` alone is not durable. `Sync()` is.
3. **Group commit for throughput.** 10-100x gain. Tune batch size + timer.
4. **Binary format, length-prefixed, CRC-protected.** Not JSON.
5. **Recovery = replay.** Works *only* because the engine is deterministic (Lesson 2).
6. **Rotate into segments.** Not one file forever.
7. **Pre-allocate files. Fsync directories on create. Don't buffer in userspace.**

---

### Drill 4

**Q1. Linux mechanics.**
Explain in your own words, with the specific syscalls named, the difference between `write()` returning successfully and data being durable on disk. Draw (in ASCII) the journey of 1 KB of data from your Go program to the NVMe drive, labeling userspace, kernel, page cache, and disk. Note where power loss causes data loss at each stage.

**Q2. Simple WAL.**
Implement a single-threaded WAL in Go with:
- Binary entry format: `[length:4][seq:8][timestamp:8][payload:N][crc32:4]` (little-endian).
- `Append(payload []byte, ts int64) uint64` that returns the sequence number *after* fsync.
- `Recover(cb func(seq, ts, payload))` that reads the WAL from the beginning and calls the callback for each valid entry, stopping gracefully at the first CRC mismatch.
- File pre-allocation to 1 GB at creation.

Benchmark: how many events/sec can you sustain with `Append` fsyncing every event? On NVMe, expect 5-10K/sec. Anything above 50K/sec means you're not actually fsyncing — check your code.

**Q3. Group commit.**
Extend your WAL to group commit. One flusher goroutine drains a queue of pending events, writes them all, fsyncs once, signals waiters. Triggers: batch size N = 1000 or timer T = 1ms, whichever first. Benchmark throughput. Target: 500K+ events/sec on NVMe. Report numbers and your N, T choices.

**Q4. Crash test.**
Write a test that:
1. Forks a subprocess running the WAL writer.
2. Parent sends 10,000 events. Subprocess appends them.
3. After N acks received (say N=5000), parent sends SIGKILL to the subprocess.
4. Parent restarts the subprocess. Subprocess runs `Recover`.
5. Verify every acknowledged sequence number is present in the recovered log. Verify the recovered CRC tail is clean.

If any *acknowledged* event is missing after recovery, your WAL is broken. Classic bug: acknowledging before fsync returns (look for `defer`-order bugs in your flusher). Find and fix.

**Q5. Integration.**
Wire your Phase 1 matching engine to this WAL. Sequencer → WAL → ring buffer → engine. Benchmark end-to-end:
- Throughput (events/sec sustained)
- p50, p99, p99.9 latency from "sequencer received" to "engine output emitted"
- Compare to Phase 1 numbers without WAL. How much slower? Where's the bottleneck — fsync latency or queueing?

**Q6. Reading.**
Read the PostgreSQL WAL documentation (search "postgresql write-ahead logging"). Identify three design decisions that align with what you built, and one that differs from your build. Why might they differ?

---

# Lesson 5: High Availability (Raft + Aeron)

### Why This Lesson Exists

Your WAL protects against process crashes. Restart the process, replay the log, you're back. Good.

But what if the **machine** dies? Power supply fails. SSD fails. Network card fails. Motherboard fails. A datacenter flood. Real incident: an exchange lost 4 hours of trading when an HVAC failure cooked a rack. Another lost the primary when a janitor unplugged it to run a vacuum.

Single-machine recovery doesn't help if the machine is gone. You need **multiple machines**, each with the state, ready to take over when one fails.

This lesson has three parts:
1. The **replication problem**: how do multiple machines agree on what happened?
2. **Raft** (consensus): the algorithm that solves it.
3. **Aeron** (messaging): how replicas talk to each other fast enough to matter.

Before any of that, networking primer.

---

### Linux Primer 2: Networking and Shared Memory

You need these five things.

**TCP.** "Transmission Control Protocol." Connection-oriented, reliable, ordered. When you write bytes to a TCP socket, the OS guarantees they arrive at the other end in order, without duplication, without loss (or the connection breaks). Great for correctness. Slow for HFT because it does a lot:
- 3-way handshake to establish a connection (3 round trips)
- ACK every packet
- Retransmit lost packets
- Congestion control (slow down when network is busy)
- Nagle's algorithm (coalesce small writes, adds latency)

Per-message latency on localhost: ~10-30μs. On datacenter LAN: ~50-200μs. Too slow for matching engine replication at microsecond scale.

**UDP.** "User Datagram Protocol." Connectionless, unreliable, unordered. Send a packet, hope it arrives. No retries. No ordering. Super fast — just encode, shove into the network card, done. Per-message latency on LAN: ~5-20μs.

UDP is what you use when you want control. You build reliability on top yourself (with sequence numbers and retransmission), but you get to choose how much reliability costs you.

**Multicast.** Normally, network traffic is **unicast**: one sender, one receiver. To tell 10 replicas something via TCP, you send 10 copies over 10 connections. Inefficient.

**IP multicast** is one-sender, many-receivers at the network layer. A sender writes one packet to a multicast IP address (like 239.1.2.3). The switch fanouts the packet to every host that subscribed to that address. One network write → N deliveries. Huge win for replication and market data.

Two caveats:
- Multicast uses UDP. Same unreliability — you build reliability on top.
- Not all networks support multicast. Cloud providers (AWS, GCP) often restrict it. Most exchange datacenters have dedicated networks with full multicast support.

**Sockets.** A socket is a file descriptor for a network endpoint. You `socket()` to create one, `bind()` it to a local address, `connect()` or `listen()` + `accept()` for TCP, or just `sendto()` / `recvfrom()` for UDP. In Go, `net.Dial`, `net.Listen`, `net.ListenPacket` wrap these syscalls.

**Shared memory.** Two processes normally have isolated memory. But they can explicitly share a region:
- Process A creates a shared memory segment (via `shm_open` or `mmap` with MAP_SHARED).
- Process B maps the same segment.
- Both see the same bytes in RAM. Changes by one are visible to the other instantly.

No syscall needed to "send" — just write to the shared region. Coordinate via atomic operations in the shared region (the producer/consumer sequence numbers we built in Lesson 1, but now in memory shared across processes). This is the fastest IPC mechanism on Linux. Sub-microsecond.

Shared memory is how Aeron achieves its latency. Two processes on the same machine communicate via shared memory, not via TCP loopback. Much faster.

OK. Networking and shared memory: done. Now the replication problem.

---

### The Replication Problem

You have two machines, A and B. Both run your matching engine. Both must end up with the same in-memory state. How?

**Naive attempt:** A and B both subscribe to the Kafka topic of events, process them independently.

Problem: A and B consume Kafka at slightly different speeds. At any moment, they disagree on which events have been processed. If A dies mid-processing, B isn't at the exact same state — B might be 50 events behind.

When A dies, how does B know where A was? What if A processed events 1-1000 and B only processed 1-950? B continues from 951. Fine. But what if B had processed 1-1020 (ahead of A)? B re-processes... wait, B already has that state. No problem either. OK maybe this is fine?

**Until:** A and B diverge in their outputs. Say A processed order 500 as a fill, then emitted the trade to downstream systems. Then A dies. B also processed order 500 — because determinism, same outcome — and emits the trade. Now downstream sees the trade twice.

Or, worse, what if A and B disagree about order 500's outcome? They shouldn't — determinism — but if their input streams are different (because Kafka consumer offsets differ, or because A saw an event B hasn't yet), they emit different outputs for different events.

The core issue: **when A fails, which replica is authoritative? Who becomes primary?** Picking wrong = double fills, lost orders, state divergence.

This is the problem consensus algorithms solve.

---

### The Sequencer and the Consensus Problem

Separate the pipeline into two parts:

1. **The sequencer:** one logical component that receives all events and assigns them a global order. Its output is a totally-ordered stream. Everyone downstream processes events in the same order.
2. **The matching engine:** a pure function over the ordered stream. Given the same input order, same output.

If the sequencer is deterministic, multiple engines consuming its output stay in sync. The engine part is easy (Lesson 2). The sequencer is the hard part — it's the thing that must be highly available, and it's the thing multiple machines must agree on.

**The sequencer is a single point of truth for event ordering.** If we naively run one sequencer, it's a SPOF. If we run many, they might disagree on the order of events. We need **consensus**: an algorithm that lets multiple machines agree on a single value (or, repeatedly, a sequence of values) despite failures.

---

### Raft (Just Enough to Understand)

**Paxos** is the original consensus algorithm (Leslie Lamport, 1989). Mathematically elegant, famously hard to understand, hard to implement correctly. **Raft** (Diego Ongaro, 2014) solves the same problem and is explicitly designed to be understandable. They have equivalent power. Use Raft. Everyone does.

Raft has three core ideas:

**1. A cluster of N machines (usually 3 or 5).** One is the **leader**, the rest are **followers**. At any time, there is at most one leader.

**2. The leader is the sequencer.** Clients send events to the leader. The leader:
- Assigns a sequence number.
- Appends to its own log.
- Sends the entry to all followers.
- Waits for a majority (⌈N/2⌉ + 1) to acknowledge they've appended to *their* logs.
- Marks the entry "committed" once majority acks received.
- Tells followers about the new commit index.
- Applies committed entries to its state machine (the matching engine!).

A write is "committed" when a majority has it. That means if the leader dies, some follower in the majority still has the entry. No data loss.

**3. If the leader dies, followers elect a new leader.** Elections use heartbeats: followers expect heartbeats from the leader every ~50ms. If no heartbeat for, say, 500ms, a follower becomes a "candidate" and asks for votes. First candidate to get a majority wins, becomes leader, resumes serving.

Three invariants make Raft correct:
- **Election safety:** at most one leader per term (a "term" is a monotonically-increasing number incremented each election).
- **Log matching:** if two logs have an entry with the same index and term, they have the same entry at that position (and all preceding positions).
- **Leader completeness:** any committed entry is present in all future leaders' logs.

The paper is short (12 pages in the extended version) and readable. Read it after this section.

---

### What Raft Buys You

A cluster of N replicas, where:
- Clients write to a single logical endpoint (the leader, found via discovery).
- Writes survive up to ⌊(N-1)/2⌋ machine failures (N=3 → survives 1 failure; N=5 → survives 2 failures).
- Failover takes ~500ms-2s (election time).
- No data loss within that window for committed events.
- Split-brain impossible (a minority partition can't elect a new leader).

**Split-brain** is when two machines both think they're the leader, both accepting writes, state diverges, reconciliation is a nightmare. Raft prevents this because a leader must win a majority vote. Only one side of a network partition can have a majority. The minority side knows it can't win an election → stays a follower → rejects writes.

---

### Raft + Matching Engine Architecture

```
Clients
  ↓
[Gateway] → [Raft Leader (Sequencer)]
                 ↓                   ↓
           [WAL + state]       [Raft followers]
                                     ↓
                              [WAL + state]
                                     ↓
                              [Matching engine]
                              (same state as leader)
```

The Raft log entries are your events. Each replica:
1. Has its own WAL (Raft appends to it before replicating).
2. Has its own in-memory engine.
3. Applies committed Raft entries to its engine.

All replicas end up with the same state because all consume the same committed log. When the leader dies:
1. A follower wins election.
2. New leader picks up where the old left off.
3. Clients reconnect to the new leader.
4. Trading resumes. Downtime: ~1 second typically.

Zero data loss (committed events are durable on a majority), sub-second failover, no state transfer. That's the win.

---

### Aeron: The Messaging Layer

Raft as described above uses the network to replicate entries. Naive Raft uses TCP: slow. HFT Raft uses **Aeron**.

**Aeron** is a low-latency reliable UDP library built by Martin Thompson (same guy who built the LMAX Disruptor — now you see the pattern). It achieves:
- Sub-microsecond IPC on the same machine (shared memory)
- ~5μs reliable delivery over LAN (UDP + retransmission)
- Millions of messages per second

How it gets there:
1. **Shared memory for local IPC.** Two processes on the same host talk via a shared memory ring buffer, not TCP loopback. Producer writes to shared memory; consumer reads. Atomic sequence counters coordinate. Zero syscalls on the hot path.
2. **UDP + NAK-based retransmission for network.** Sender tags each packet with a sequence number. Receivers detect gaps (missing sequence) and send **NAKs** (negative acknowledgments) asking for retransmission. Retransmitter keeps a buffer of recent messages. This is **reliable multicast**.
3. **Multicast for fan-out.** One sender can deliver to N followers with one network write — the switch does the fanout. (Works only in networks that support multicast; most exchange datacenters do.)
4. **Lock-free everything.** Mechanical sympathy principles applied at every layer.

Aeron is written in Java. There are C++ and C# ports, and a Go port (`go-aeron`) that is functional but less mature. For a learning project in Go, you can:
- Use `hashicorp/raft` with TCP (slower but works)
- Roll your own UDP-based replication (educational, rough)
- Use the Go Aeron client talking to a Java Aeron media driver (production-ish)

For Phase 2 drills, we'll use `hashicorp/raft` over TCP. You'll feel the latency but the concepts apply identically when you swap transports.

---

### Failure Modes and Gotchas

**1. Clock skew.** Raft's election timeouts rely on clocks not being too skewed. If node A thinks 500ms have passed and node B thinks 200ms, elections get weird. Use NTP. In HFT, PTP (Precision Time Protocol, microsecond accuracy).

**2. Disk stalls.** If a node's SSD suddenly takes 2 seconds to fsync (yes, this happens — SSDs do garbage collection internally), the node stops heartbeating. Cluster elects a new leader. When the stalled node recovers, it rejoins as follower. Workable, but can cause unnecessary failovers.

**3. Network partition.** The "minority" side of a partition cannot commit writes. This is by design (prevents split-brain) but means half your cluster is unwritable during a partition. For a matching engine, this is correct — better to halt than to accept conflicting writes.

**4. fsync must happen before ack.** Raft requires that an entry be durable before being ack'd to the leader. If a follower acks before fsync, then crashes, the entry can be "committed" (majority including that follower ack'd) but actually lost. Durability bug. The WAL lessons apply.

**5. Log compaction is non-trivial.** Your Raft log grows forever unless you compact it. Compaction = snapshot state, truncate log up to snapshot point. This is Lesson 6.

---

### Go-Specific: hashicorp/raft

The de facto Raft library in Go. Used by Consul, etcd (etcd uses its own version but heavily inspired), Nomad, and many others. Battle-tested.

Core types:
- `raft.Raft`: the Raft state machine.
- `raft.FSM`: interface you implement; this is your matching engine's applier.
- `raft.LogStore`, `raft.StableStore`: persistence. `raft-boltdb` is the usual impl.
- `raft.SnapshotStore`: snapshot persistence.
- `raft.Transport`: TCP transport usually.

Your `FSM` implements three methods:
```go
type FSM interface {
    // Apply is called on each log entry committed by Raft.
    // This is where your matching engine processes the event.
    Apply(log *raft.Log) interface{}

    // Snapshot returns a snapshot of current state.
    Snapshot() (FSMSnapshot, error)

    // Restore replaces state from a snapshot.
    Restore(r io.ReadCloser) error
}
```

`Apply` is called in strict sequence. Committed events are fed one at a time. Inside `Apply`, you call your matching engine's `Step()`. Done.

The library handles: leader election, log replication, snapshots, joining/leaving cluster, transport, persistence. You write `Apply`, `Snapshot`, `Restore`, and a main wrapping it together.

This is maybe a few hundred lines of Go to get a working HA matching engine. Not trivial, but not enormous either.

---

### Summary: The Rules

1. **Separate the sequencer from the engine.** The sequencer is where consensus happens. The engine is a pure function over the sequencer's output.
2. **Use Raft, not rolled-your-own.** Consensus is full of corner cases. Humans fail. Use `hashicorp/raft`.
3. **N=3 for 1-failure tolerance, N=5 for 2-failure tolerance.** Odd numbers always (majority requires odd for efficiency).
4. **Durability before acknowledgment.** Raft correctness depends on followers fsyncing before acking.
5. **TCP for learning, Aeron for production.** Same concepts; different transports.
6. **Clocks matter.** Use NTP minimum, PTP for HFT.
7. **Split-brain is the nightmare.** Consensus (majority) prevents it. Never build HA without it.

---

### Drill 5

**Q1. Explain the problem.**
In 4-6 sentences: why can't you just run two matching engine instances both subscribed to Kafka and call that "high availability"? What specifically goes wrong? Name two distinct failure modes.

**Q2. Explain Raft.**
Without looking at notes: what is a term? A leader? A follower? A candidate? What is the commit index? What does it mean for an entry to be "committed"? Why is the requirement "majority ack" rather than "one ack"? If I run a 5-node cluster and 2 nodes fail, is the cluster still writable? What if 3 fail?

**Q3. Implement.**
Using `hashicorp/raft`:
- Set up a 3-node Raft cluster, each node in a separate process (or separate goroutine + port — fine for testing).
- Implement an `FSM` that holds a simple integer counter.
- Clients send "increment" commands to the leader. The leader replicates. All three nodes apply.
- Run a workload: 10,000 increments from a client. Verify all nodes end at 10,000.
- Kill the leader. Verify a follower wins the election (measure the time). Verify the counter is still 10,000 on the survivors. Restart the dead node; verify it catches up.

**Q4. Matching engine FSM.**
Replace the counter with your Phase 1 matching engine.
- Events arrive at the leader.
- Raft replicates.
- All three nodes' FSMs call `engine.Step(event)`.
- Periodically query the leader's engine for its order book. Query a follower. They must match exactly.
- Benchmark throughput through Raft: events/sec end-to-end, p50/p99/p99.9 latency. How much slower is it than direct engine calls? Explain why.

**Q5. Chaos.**
With your 3-node cluster running the matching engine at 10K events/sec:
- Kill the leader. Measure time until a follower takes over and events flow again.
- Kill a follower. Verify no trading disruption.
- Partition the network (use `iptables -A INPUT -p tcp --dport {raft_port} -j DROP` on the leader). Measure behavior. Does a new leader emerge? What happens to events sent during the partition?
- Heal the partition. Does the old leader rejoin as follower?
Report all timings and observations.

**Q6. Reading.**
Read the Raft paper (Ongaro & Ousterhout 2014, "In Search of an Understandable Consensus Algorithm"). 12 pages extended version. Answer:
- What's the purpose of term numbers, beyond just being a counter?
- Why does Raft's leader election use *randomized* election timeouts?
- What is a "conflicting entry" during log replication and how does the leader resolve it?

---

# Lesson 6: Snapshotting with Copy-on-Write

### Why This Lesson Exists

Your engine has been running for a month. It's processed 2 billion events. The WAL is 400 GB. A machine crashes. Recovery starts: replay 2 billion events.

At 1M events/sec replay, that's 2000 seconds = 33 minutes of downtime. Unacceptable. Your recovery time objective is seconds, not half an hour.

The fix: **snapshots.** Periodically, serialize the engine's current state to disk. On recovery, load the latest snapshot, then replay the WAL from the snapshot's sequence number forward. If we snapshot every hour, recovery replays at most 1 hour of events. At 1M/s, that's 1 hour to rebuild... wait, same thing.

OK, snapshot every minute. 60 seconds of replay at 1M/s = 60M events. Load time depends on snapshot size. Still, minutes of downtime. Fine for some exchanges, not enough for others.

Snapshots reduce WAL replay time. The WAL can be truncated up to the snapshot point (old WAL segments deleted or archived). Storage cost drops too.

But snapshotting itself is a problem. A naive snapshot — "pause the engine, serialize state to disk, resume" — causes a multi-second pause during which no trading happens. At peak volume, this is catastrophic: orders pile up in the input queue, latencies spike into seconds, traders time out and retry, the retry storm makes it worse. This is the **snapshot stutter**, and it's the reason naive snapshotting is unacceptable.

The solution is **Copy-on-Write (COW) snapshots via fork()**. The engine doesn't pause. A child process takes a frozen snapshot of memory while the parent continues trading. No stutter.

To understand this, you need virtual memory.

---

### Linux Primer 3: Virtual Memory and Pages

Your Go program uses memory. Addresses like `0x7fff12345678`. These are **virtual addresses** — they're not the actual location in physical RAM.

**Virtual memory** is a layer of indirection between your program and physical RAM. Every process has its own virtual address space, typically 64-bit (so, theoretically, 16 exabytes; practically, whatever the kernel allocates).

The kernel maintains a **page table** per process. It maps virtual addresses to physical addresses. The mapping is granular at the **page** level.

**Page.** The unit of memory management. Usually 4 KB on x86-64 Linux (can be 2 MB or 1 GB for "huge pages"). Your virtual address space is divided into 4KB pages. Each page either maps to a physical RAM page or doesn't (in which case accessing it is a page fault).

```
Virtual space (per-process)      Page table          Physical RAM
┌────────────────┐              ┌────────┐         ┌──────────────┐
│ Page 0         │─────────────→│        │────────→│ Frame 47     │
│ Page 1         │─────────────→│        │────────→│ Frame 12     │
│ Page 2         │  (not        │        │         │              │
│                │   mapped)    │        │         │              │
│ Page 3         │─────────────→│        │────────→│ Frame 89     │
│ ...            │              └────────┘         └──────────────┘
└────────────────┘
```

The **MMU** (Memory Management Unit) is a hardware component that does this translation on every memory access, using the page table. Every `mov rax, [rbx]` (read from memory) triggers an MMU lookup. The MMU caches recent translations in a **TLB** (Translation Lookaside Buffer) so it doesn't consult the page table every time.

**Why this matters for COW:** because the kernel controls the page table, it can do clever things. Like mark a page "read-only" on one process while keeping it writable on another. Like make two processes share the same physical page but have different virtual addresses pointing to it. These are the tricks COW uses.

---

### Linux Primer 4: fork()

`fork()` is a syscall that duplicates the calling process. It returns twice: once in the parent (returning the child's PID), once in the child (returning 0).

```c
pid_t pid = fork();
if (pid == 0) {
    // I'm the child.
} else if (pid > 0) {
    // I'm the parent. 'pid' is the child's PID.
} else {
    // fork failed.
}
```

After fork, both processes continue from the same point, with identical state. Same memory contents. Same file descriptors. Same register values. They diverge only because `fork()` returned different values in each.

**How does fork give the child a copy of parent's memory?** Naively, the kernel would allocate fresh physical RAM for the child and `memcpy` every byte of the parent's memory. For a 32GB engine process, that's 32GB of RAM and ~10 seconds of copying. Unacceptable.

Reality: **Copy-on-Write.**

---

### Copy-on-Write (COW) Explained

When fork happens, the kernel does NOT copy the parent's memory. Instead:

1. It creates a new page table for the child, **pointing to the same physical pages** as the parent. Both parent and child now share physical RAM.
2. It marks every shared page as **read-only** in both page tables.
3. `fork()` returns. Both processes continue.

Now, if parent or child only *reads* memory, nothing special happens. Reads from read-only pages succeed. Zero copying.

But if parent or child *writes* to a page:

1. MMU sees the write is to a read-only page.
2. MMU triggers a **page fault** — hardware interrupt.
3. Kernel handler catches the fault. Realizes "oh, this is a COW-shared page that the process wants to write."
4. Kernel allocates a fresh physical page.
5. Kernel copies the shared page's contents into the fresh page.
6. Kernel updates the writing process's page table to point to the fresh page (read/write).
7. The other process still has the original page.
8. Kernel returns from fault. The write instruction retries and succeeds.

**Result:** only pages that are actually written get copied. Pages that stay unchanged remain shared. If the child doesn't write much, fork is nearly free — just the cost of setting up the page tables (microseconds to milliseconds, not seconds).

This is why `fork()` is fast on Linux despite looking like "duplicate 32GB of memory."

---

### Fork-Based Snapshotting

Here's the trick, straight out of the Redis playbook:

```
1. Engine is running. State in memory. Processing events from Raft log.
2. Snapshot time arrives.
3. Engine calls fork().
4. Parent returns from fork. Continues processing events. Mutates its state.
5. Child returns from fork. Has a frozen snapshot of the state as of fork time.
   Child serializes this state to disk. Exits when done.
6. Parent kept trading the whole time. Zero pause.
```

The magic: because of COW, the child sees the state *as of the moment fork returned*. The parent can mutate its state freely — each mutation triggers a page fault and a page copy. The child keeps the old pages and writes them, unhurried, to disk. When the child exits, those pages can be freed.

**The cost is temporary RAM.** During the snapshot, memory usage can spike — in the worst case, 2x — because every page the parent mutates gets duplicated. Typical case: only a fraction of pages are hot, so memory usage goes up by 10-30%.

**The benefit: no stutter.** The parent's event processing continues uninterrupted. No pause, no queue buildup, no latency spike.

This is exactly how Redis's RDB background save works. LMAX uses a similar technique. It's the standard approach for "snapshot huge in-memory state without pausing."

```
Memory during snapshot:

Before fork:
  Parent pages:  [P1][P2][P3][P4][P5]

After fork (all shared, read-only):
  Parent pages:  [P1][P2][P3][P4][P5]
  Child pages:   [P1][P2][P3][P4][P5]  (same physical pages!)

Parent writes to P2:
  → page fault → kernel copies P2 → updates parent's page table
  Parent pages:  [P1][P2'][P3][P4][P5]
  Child pages:   [P1][P2 ][P3][P4][P5]  (child still sees old P2)

Child serializes P1, P2, P3, P4, P5 to disk.
Child exits. Its pages are freed. Only the duplicated P2 remains consumed.
```

---

### Snapshot Format and Integration with WAL

A snapshot records:
- **The sequence number it corresponds to.** "This snapshot reflects the state after event N."
- **The serialized engine state.** Order book, balances, positions, everything.
- **A checksum** for integrity.

Recovery procedure:
1. Load the latest snapshot. State now matches "up to event N."
2. Open the WAL. Skip entries up to and including N.
3. Replay entries N+1, N+2, ... until end.
4. Engine now matches the state just before the crash.

WAL truncation:
1. Snapshot completes at sequence N.
2. WAL segments containing only events ≤ N can be deleted.
3. Storage reclaimed.

Frequency tradeoff:
- Snapshot often (every minute): fast recovery, heavy disk I/O.
- Snapshot rarely (every hour): slow recovery, light disk I/O.
- Typical: every 5-10 minutes for a high-throughput engine.

---

### Go and fork(): It's Complicated

Here's the bad news. Go's runtime was not designed with fork() in mind.

Go uses goroutines multiplexed on OS threads. Go's garbage collector and scheduler both run in concurrent threads. When you `fork()` a Go process:

1. The child has a copy of all OS threads' memory, but only one thread — the one that called fork — is actually running in the child. The other threads' stacks exist but no one is executing them.
2. If any of those "frozen" threads held a lock, the lock is stuck held forever in the child. Any attempt to acquire it deadlocks.
3. Go's runtime itself has locks (for goroutine scheduling, GC, etc.). These can be held by other threads at fork time.

Result: a naive `fork()` in a Go program often deadlocks in the child. Redis and similar are written in C specifically so they can fork reliably.

**Your options in Go:**

**Option A: Fork before the runtime is fully running.** Not practical for a matching engine.

**Option B: Use `syscall.ForkExec` to fork + exec.** The child immediately exec's a different program. No Go runtime problems because the new program starts fresh. But this loses the memory — the whole point of fork snapshot is keeping the memory.

**Option C: Use a C helper.** Write the snapshotting in C via cgo. C forks, C serializes. Works but adds complexity.

**Option D: Use a "shared memory" approach.** Your state lives in an mmap'd region with MAP_SHARED. A separate, dedicated snapshotter process also maps the same region. Snapshotter reads and serializes. Engine continues writing. No fork needed.

The catch with Option D: it's *not* a snapshot of a consistent state. The snapshotter reads memory while the engine writes, so you get torn reads. Workable if your state serialization is sequence-number-aware: snapshot includes "this reflects state as of seq N" and verifies all data corresponds to that same N (single-writer engine helps — changes happen atomically per event).

**Option E: Double-buffered state.** At snapshot time, the engine atomically swaps to a new state struct and snapshots the old one. Requires either:
- A lot of memory (two full state copies all the time), OR
- Persistent (immutable-with-sharing) data structures — each update returns a new version sharing unchanged parts.

Persistent data structures (Clojure/Haskell-style) give you structural sharing so "take a snapshot" = "keep a pointer to the old root." Elegant, but in Go you'd have to build your own persistent map/tree because the ecosystem is thin.

**Option F: Hybrid — checkpoint via Raft's built-in mechanism.** `hashicorp/raft` has its own snapshot interface. Your FSM implements `Snapshot()` which returns an `FSMSnapshot`. The library handles persistence. The `Snapshot()` call runs in the same goroutine as `Apply`, so events pause during snapshot — but only for that node. The leader can trigger snapshots on followers and keep serving. Not as elegant as true COW but adequate.

**For your Phase 2 project: use Option F first** (integrate with `hashicorp/raft`'s snapshotting) **then experiment with Option D** (shared memory + dedicated snapshotter) to understand COW semantics. True fork-based COW in pure Go is a research project, not a weekend.

---

### Implementing Snapshot via `hashicorp/raft`

```go
// FSMSnapshot is the thing returned by FSM.Snapshot().
// It holds a frozen view of state. Its Persist() is called by the library,
// on its own goroutine, to write to disk. Release() is called when done.

type EngineSnapshot struct {
    state *EngineState  // copy or reference
}

func (e *Engine) Snapshot() (raft.FSMSnapshot, error) {
    // OPTION: deep-copy engine state here.
    // If state is small (< 1GB), this is simple and fine.
    // If state is huge, you need COW-style tricks.
    stateCopy := e.state.Clone()
    return &EngineSnapshot{state: stateCopy}, nil
}

func (s *EngineSnapshot) Persist(sink raft.SnapshotSink) error {
    // Serialize s.state into sink.
    encoder := gob.NewEncoder(sink)
    if err := encoder.Encode(s.state); err != nil {
        sink.Cancel()
        return err
    }
    return sink.Close()
}

func (s *EngineSnapshot) Release() {
    // Release resources.
}

func (e *Engine) Restore(r io.ReadCloser) error {
    decoder := gob.NewDecoder(r)
    var state EngineState
    if err := decoder.Decode(&state); err != nil {
        return err
    }
    e.state = &state
    return nil
}
```

The library invokes `Snapshot()` periodically (configurable), calls `Persist()` on its own goroutine, and handles storage. On startup, if a snapshot exists, `Restore()` is called with the snapshot data, and then the log is replayed from the snapshot's sequence point.

---

### The Stutter Budget

If your snapshot method does pause the engine (Option F with a blocking `Snapshot()`), you have a **stutter budget**: how long can you pause before it matters?

- Crypto exchange at moderate volume: ~10-50ms is tolerable. Users don't notice.
- Crypto at peak: ~5-10ms. Queue buildup starts to matter.
- HFT-focused exchange: ~100μs. Very hard.

If your state is small (say, <500 MB), even a naive `Clone()` + serialize pauses for < 100ms. Fine for most exchanges. If it's huge (10+ GB), you need COW or you need to segment the snapshot (snapshot different pieces at different times).

Measure your stutter. If it's fine, don't over-engineer. If it's not, invest in COW.

---

### Summary: The Rules

1. **Snapshots reduce recovery time.** Without them, recovery = replay entire WAL from day one.
2. **Naive snapshot = stutter.** Pausing the engine during serialization is unacceptable at high volume.
3. **COW via fork() = no stutter.** Redis and LMAX's approach. Child copy snapshots, parent keeps trading.
4. **Go and fork don't mix well.** Naive `syscall.ForkExec` loses memory; true COW fork deadlocks with Go runtime.
5. **Practical Go approach: start with `hashicorp/raft`'s built-in snapshotting + a good `Clone()`.** Measure stutter. Upgrade if needed.
6. **Truncate the WAL after successful snapshot.** That's how storage stays bounded.
7. **Recovery: load snapshot, replay WAL from snapshot's sequence.**

---

### Drill 6

**Q1. Explain COW.**
In your own words, walk through step by step what happens at the kernel / MMU / page table level when a process forks and then the parent writes to a memory page. Why doesn't the child see the parent's write? Why is fork fast despite "duplicating" gigabytes of memory?

**Q2. Stutter measurement.**
Take your Phase 2 matching engine (with Raft + WAL). Implement naive snapshotting:
- At snapshot time, block `Apply()`. Serialize state with `gob` or equivalent. Write to disk. Unblock.
- Run a workload of 1M events/sec with snapshots every 30 seconds.
- Measure p99.9 latency during snapshot window vs. non-snapshot window.
- Report the stutter duration and the latency spike magnitude. State size, snapshot file size, and serialization time too.

**Q3. hashicorp/raft snapshot integration.**
Implement `FSM.Snapshot()`, `FSMSnapshot.Persist()`, `FSMSnapshot.Release()`, and `FSM.Restore()` for your matching engine. Use gob or a hand-rolled binary format (binary is much faster for large states; gob is easier).
- Trigger a manual snapshot via `raft.Snapshot()`.
- Kill the process. Verify on restart the snapshot is loaded and the WAL is replayed from the correct point. State must match pre-crash state.
- Demonstrate that WAL segments older than the snapshot get compacted away (raft-boltdb handles this).

**Q4. COW via shared memory (hard mode, optional but recommended).**
Rather than fork (which Go hates), implement Option D from the lesson:
- Allocate the engine's state in a `mmap`'d region with MAP_SHARED.
- At snapshot time, start a dedicated snapshotter goroutine (or subprocess for true isolation) that reads the region and serializes it, tagged with the current sequence number.
- Because the engine is single-writer, reads from the snapshotter won't tear at the event level if it reads state + sequence atomically (use a generation counter).
- Measure stutter. It should be near-zero.
- What inconsistencies could arise if the snapshot reads span many events? How do you handle them?

**Q5. End-to-end recovery test.**
With Raft (3 nodes) + WAL + snapshots:
1. Start the cluster.
2. Process 100M events (let this run).
3. Force a snapshot.
4. Process another 10M events.
5. Kill all three nodes.
6. Restart them.
7. Verify: all three recover to the same state. State matches what the leader had before the crash. Recovery time is dominated by loading the snapshot, not by WAL replay.

Report recovery time. If it's > 30 seconds, identify the bottleneck (deserialization CPU, disk read, WAL replay) and explain.

**Q6. Reading.**
Read the Redis RDB persistence documentation and the specific section on BGSAVE (background save). Identify exactly where Redis uses `fork()` and COW. How does Redis handle the "child writes to disk while parent keeps mutating" challenge? What are Redis's known pain points with this approach (hint: memory fragmentation, TLB shootdowns on large pages)?

---

## Phase 2 Master Rules

### Event Sourcing & WAL
- Write-ahead: event → WAL → fsync → process. Never reversed.
- fsync is durability; `write()` alone is not.
- Group commit amortizes fsync. N events per fsync, 100x throughput.
- Binary format, length-prefixed, CRC-checked. JSON is a bug.
- Recovery = deterministic replay. Requires Phase 1 Lesson 2 discipline.
- Pre-allocate WAL segments. Fsync the directory on file creation.
- Rotate into 1 GB segments. Delete after snapshot covers them.

### High Availability
- Separate sequencer from engine. Sequencer is where consensus lives.
- Use Raft, use `hashicorp/raft`. Don't invent consensus.
- N=3 survives 1 failure, N=5 survives 2. Always odd.
- Durability before acknowledgment — required for Raft correctness.
- Split-brain is prevented by requiring majority. Never bypass this.
- Aeron (UDP + shared memory) for HFT latency. TCP for learning.
- Clocks matter. NTP minimum, PTP for serious HFT.

### Snapshotting
- Without snapshots, recovery replays entire history. With, only the tail.
- Naive snapshot = stutter. COW (fork) = no stutter, for C. For Go, use alternatives.
- Go's runtime makes raw fork() unreliable. Use `hashicorp/raft` snapshots first.
- State must be clonable or mapable-and-single-writer for snapshots to work.
- Truncate the WAL after snapshot. Otherwise storage unbounded.
- Measure stutter. If it's fine, don't over-engineer.

### If You Do Phase 2 Right
- Zero data loss within the quorum (committed events survive any single failure).
- Sub-second failover when a node dies.
- Recovery time < 30 seconds from any crash (snapshot + tail of WAL).
- Snapshot doesn't cause noticeable latency spike in the live engine.
- Foundation for Phase 3 (risk checks, margining, settlement).

---

*Phase 2 complete. Phase 3: Pre-trade risk, margining, settlement, gateway protocols (FIX).*
