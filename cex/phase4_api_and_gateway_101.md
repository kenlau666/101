# CEX Matching Engine — Phase 4 Course (Go)

> API & Gateway
> Two lessons. Linux networking primitives inline. Harsh drills.

---

## Phase 4 Introduction

Phase 1 made the engine fast. Phase 2 made it reliable. Phase 3 made it correct. Phase 4 exposes it to the internet — where the entire world tries to break it.

Your beautiful in-memory engine doing 5M ops/sec is useless if the gateway in front can only fan out to 1,000 WebSocket subscribers. Your perfect risk engine is useless if an attacker can submit a million orders per second and crash it before the rate limiter even fires.

Two lessons:

1. **WebSocket Scaling:** serving 100k+ concurrent connections with real-time order book updates. Where goroutine-per-connection dies and you learn what `epoll` actually is.
2. **Rate Limiting:** protecting the engine from toxic flow, quote stuffing, and DDoS. Layered defense from the edge to the matching core.

The user-facing gateway is where the glory meets the grind: most exchange engineers never see the matching core, but everyone hits the gateway. Every millisecond of gateway overhead is a millisecond added to every user's experience. Every bug is exposed directly to adversarial traders trying to exploit it.

Same format as before: Linux primitives explained inline, Go-specific warnings where your instincts will fail you, harsh drills at the end.

---

# Lesson 10: WebSocket Scaling

### Why This Lesson Exists

A CEX must push market data — order book updates, trades, ticker prices — to every connected trader in real time. "Real time" here means sub-10ms from matching engine event to client receipt. "Connected trader" means anywhere from 10,000 (small exchange) to 500,000+ (Binance-scale).

You can't poll. HTTP polling at 1 req/sec × 100k clients = 100k req/sec just for one endpoint, and the data is up to 1 second stale. Polling at 100ms intervals = 1M req/sec and still 100ms stale. Dead on arrival.

The solution is **push via WebSocket**: a persistent TCP connection the server keeps open, over which it can send messages whenever there's data. Client connects once, stays connected for hours. Server pushes updates as they happen. Near-zero latency.

Sounds simple. It isn't. At 100k connections, everything breaks:

- Go's default goroutine-per-connection model uses ~800MB of RAM just for goroutine stacks.
- The Linux kernel limits file descriptors per process (default 1024).
- The default TCP socket buffer sizes waste gigabytes of kernel memory.
- GC pauses become visible because you now have hundreds of thousands of live objects.
- A slow client (one trader on a bad mobile connection) can block your push path for others.
- Broadcasting a single order book update to 100k clients takes real CPU if you do it naively.

This lesson is about how to actually do it. You'll learn `epoll`, why goroutine-per-connection doesn't scale, the snapshot+delta pattern, backpressure, and fan-out architecture.

---

### Linux Primer 5: Sockets, File Descriptors, epoll

You need these four things. Even if you never write kernel-level code, you need to understand what `epoll` is doing because at 100k connections everything depends on it.

**Socket.** A socket is a file descriptor that points to a network endpoint. You `socket()` to create one, `bind()` to a local address, `listen()` + `accept()` for a TCP server. In Go, `net.Listen` wraps these. Every TCP connection — server-side or client-side — is a socket and thus a file descriptor.

**File descriptor limits.** Each process has a maximum number of file descriptors it can hold open. Linux default: 1024 (!). This is a hard limit — your 1025th connection fails with `EMFILE: Too many open files`. Check with `ulimit -n`. Raise with `ulimit -n 1000000` (or in systemd: `LimitNOFILE=1000000`). Also check system-wide limit: `/proc/sys/fs/file-max`.

At 100k connections, each one is a file descriptor. You need limits raised or your server dies at connection 1024.

**TCP socket buffers.** Every TCP connection has two kernel buffers: send buffer and receive buffer. Default on Linux: 16-64 KB each. With 100k connections:

```
100,000 connections × 64 KB × 2 (send + recv) = 12.8 GB
```

That's 12.8 GB of kernel memory, just for TCP buffers, before you've sent a byte. You can tune per-connection buffer sizes (`SO_SNDBUF`, `SO_RCVBUF`) smaller if your messages are small. 8 KB per buffer is often enough for market data.

Check current settings:
```
sysctl net.core.wmem_default net.core.wmem_max
sysctl net.core.rmem_default net.core.rmem_max
```

**The polling problem (select, poll).** Classic Unix I/O: when you have many sockets, how do you know which ones have data ready?

- **`select()`** — pass in a bitmap of up to 1024 fds. Kernel scans all of them every call and returns which are ready. O(N) per call. Broken at scale.
- **`poll()`** — pass in an array of fd structs, no 1024 limit, but still O(N) per call. Scan all N fds every time.

At 100k connections checked 1000 times per second, that's 100M bitmap checks per second. Kernel melts.

**`epoll` (Linux-specific).** The fix. Instead of asking "which of these N fds are ready?" every time, you register fds once, and the kernel maintains a ready-list for you. You call `epoll_wait()` and it returns only the fds that have events. O(k) where k is the number of ready events, not total fds. Scales to millions.

Three syscalls:
- `epoll_create()` — make an epoll instance. Returns an fd.
- `epoll_ctl()` — add / modify / remove fds in the set.
- `epoll_wait()` — block until events arrive, return them.

BSD equivalent: `kqueue`. Windows equivalent: IOCP. Go's runtime uses these under the hood — it calls `epoll` on Linux automatically for all net.Conn operations. You don't call it directly unless you're building something custom. But you need to *understand* it.

**Why this matters for 100k WebSockets:** Go's default model is "one goroutine per connection blocked in `Read()`." Under the hood, Go's runtime multiplexes all of those blocked reads onto a few OS threads using `epoll`. One OS thread with `epoll_wait()` can service thousands of connections. The goroutines are cheap *until* you hit memory limits or GC pressure. More on this in a minute.

---

### WebSocket Primer

The WebSocket protocol is a persistent bidirectional framing layer on top of TCP. Designed to work through HTTP infrastructure (proxies, load balancers) by starting as an HTTP request and then "upgrading."

**Handshake:**

Client sends an HTTP GET with special headers:
```
GET /ws HTTP/1.1
Host: api.exchange.com
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
```

Server responds:
```
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

After this exchange, the same TCP connection is now a WebSocket. Both sides send framed messages.

**Frame structure:**
```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-------+-+-------------+-------------------------------+
|F|R|R|R| opcode|M| Payload len |    Extended payload length    |
|I|S|S|S|  (4)  |A|     (7)     |             (16/64)           |
|N|V|V|V|       |S|             |   (if payload len==126/127)   |
| |1|2|3|       |K|             |                               |
+-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
...
```

Small overhead: 2-14 bytes per frame. Opcodes: text (1), binary (2), close (8), ping (9), pong (10).

**Use binary frames for performance.** Text frames require UTF-8 validation, binary frames don't. Parse binary directly to your struct. Saves CPU at scale.

**Per-message deflate (RFC 7692).** Optional extension that compresses each message. Can reduce bandwidth 5-10x for order book data (which is repetitive). Cost: CPU to compress, per-connection memory for the compression dictionary (~32 KB per direction = 64 KB × 100k connections = 6.4 GB). Enable only if bandwidth is your bottleneck.

**Ping / Pong.** Either side can send ping; the other must reply pong. Used for keepalive (detect dead connections) and RTT measurement. Implement an application-level heartbeat every 15-30 seconds. If no pong in 2× interval, disconnect.

---

### The 100k Connection Problem in Go

Go's concurrency model — one goroutine per connection — is beautiful until it isn't.

**Memory cost per goroutine.**
- Goroutine stack: starts at 2 KB, grows up to whatever. Typical steady state: 4-8 KB.
- Each WebSocket library usually uses 2 goroutines per connection (one reader, one writer).
- At 100k connections: 200k goroutines × 8 KB = 1.6 GB just for stacks.
- Plus connection state, buffers, etc.: probably 2-3 GB total.

2-3 GB is within reach of a modern server, but you're paying for it.

**Scheduler pressure.** Go's scheduler runs a cost-amortized algorithm. Normally very fast. At 200k goroutines where most are just blocked on network I/O, the scheduler handles it fine — they're parked, not runnable. The cost appears when you want to wake them all simultaneously (broadcast pattern): "push to 100k clients" means making 100k goroutines runnable at once, and the scheduler grinds.

**GC pressure.** Each connection holds references: buffer pointers, message pointers, subscription sets. At 100k connections, GC scan work scales. With careful zero-allocation design (Lesson 3), this stays manageable. Done naively, p99 latency spikes during GC.

**Approach A: goroutine per connection (gorilla/websocket style).**

Works up to ~50-100k connections on beefy hardware. The standard Go pattern. Fine for most exchanges.

**Approach B: event-loop (gobwas/ws style).**

One goroutine per CPU core, each running an event loop that handles thousands of connections via `epoll`. Lower memory (no per-connection stacks), higher throughput. Harder to code.

**Approach C: many small gateway servers behind a load balancer.**

Shard connections across 10 gateway servers, each handling 10k. Reduces per-server pressure to manageable levels. Requires a fan-out mechanism (below) so all servers receive all the market data events to broadcast.

In practice: large exchanges use C (horizontal scaling) *combined with* A or B per server. You don't have to choose one globally.

---

### The Fan-Out Architecture

Single-gateway architecture doesn't scale. Here's the real topology:

```
[Matching Engine]
      │
      ↓ (publishes events)
[Market Data Topic: Kafka / NATS / Redis pub-sub / custom multicast]
      │
      ├─────────────┬─────────────┬─────────────┐
      ↓             ↓             ↓             ↓
  [WS Gateway 1] [WS Gateway 2] [WS Gateway 3] [WS Gateway N]
      │             │             │             │
   (30k clients) (30k clients) (30k clients) (30k clients)
```

Every gateway subscribes to the market data topic. Every event gets delivered to every gateway. Each gateway forwards to its connected clients that have subscribed to the relevant channel.

**Topic design.** At the matching engine:
- `trades.BTC-USDT` — every trade
- `book.BTC-USDT.diff` — every book change
- `book.BTC-USDT.snapshot` — periodic full snapshots
- `ticker.BTC-USDT` — aggregated ticker

Clients subscribe by channel. Gateway maintains a map: channel → set of connected client IDs. When an event arrives, push to the set.

**Why Kafka vs Redis pub-sub?** System Design 101 Lesson 3 covered this. Quick recap:
- Kafka: durable, replayable. Client disconnects for 5 seconds; when it reconnects, you want to replay from its last sequence. Kafka's log does this. Redis pub-sub doesn't.
- Redis pub-sub: fire-and-forget. Missed messages gone forever.
- For matching engine events: Kafka. Always. Missing a trade event is unacceptable.

**NATS JetStream** is a modern alternative. Lower latency than Kafka, built-in replay. Worth considering.

For HFT-scale internal message bus: **Aeron** (from Phase 2 Lesson 5). Sub-microsecond within datacenter.

---

### Snapshot + Delta Pattern

Naive approach: send full order book on every update. Order book is 1000+ levels × 50 bytes = 50 KB. Update rate: 100+/sec per active symbol. Bandwidth: 5 MB/sec per symbol per client × 100k clients = impossible.

The right approach: **snapshot + deltas.**

**Snapshot:** full state of the order book at a point in time. Sent once when the client subscribes. Large but infrequent.

**Delta:** incremental change since the last message. "Price level 50,000 now has quantity 2.5" or "price level 49,999 deleted." Small, frequent.

Each message carries a sequence number. Client applies deltas in order. If a gap is detected (expected seq N, got N+2), client requests a new snapshot and resumes.

```
Connection opens.
  Server: send snapshot. seq = 1000.
  Server: send delta. seq = 1001.
  Server: send delta. seq = 1002.
  ...
  Server: send delta. seq = 1234.
  (network blip)
  Server: send delta. seq = 1237.
  Client: gap detected (expected 1235, got 1237).
  Client: requests snapshot.
  Server: sends snapshot. seq = 1238.
  (resumes)
```

**Snapshot frequency.** Can be periodic (every 30 seconds) or on-demand (client requests after gap). Periodic snapshots are simpler but bandwidth-heavy. On-demand is more efficient but requires a recovery channel.

**Delta efficiency.** At peak, a BTC-USDT book might update 100-500 times/sec. Each delta is ~50-200 bytes. Total: ~50 KB/sec per symbol per client. With 100k clients, 5 GB/sec — still a lot, but at least feasible with tuning and compression.

For really active traders, per-symbol bandwidth matters. For casual traders, you might push aggregated snapshots (top 20 levels every 100ms) rather than full delta streams.

---

### Backpressure: The Slow Client Problem

Your gateway pushes 1 MB/sec of updates. A client has a 100 KB/sec connection (bad Wi-Fi). Their TCP send buffer fills up. Your `Write()` call blocks until kernel buffer drains. Meanwhile, new events pile up.

**Two bad outcomes:**

1. If each client has its own goroutine writing, the blocked goroutine blocks only that client's thread. But you're accumulating a write queue in userspace. Eventually memory explodes.
2. If one goroutine writes to many clients (broadcast loop), a slow client blocks the entire loop. Everyone else stalls.

**Three correct patterns:**

**Pattern 1: bounded per-client queue + drop on overflow.** Each client has a bounded queue (say, 100 messages). Writer goroutine drains it. If the queue fills, drop the newest message (or disconnect the client). The client loses data but the gateway keeps running.

```go
type Client struct {
    conn *websocket.Conn
    outQueue chan []byte  // buffered channel, size 100
}

// Broadcast side:
func (c *Client) Send(msg []byte) {
    select {
    case c.outQueue <- msg:
        // enqueued
    default:
        // queue full; client is slow
        close(c.outQueue)   // signal writer to disconnect
    }
}

// Writer goroutine:
func (c *Client) writeLoop() {
    for msg := range c.outQueue {
        c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
        if err := c.conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
            return  // disconnect on any write error
        }
    }
    c.conn.Close()
}
```

**Pattern 2: disconnect slow clients aggressively.** If any write takes > 1 second, disconnect. Fast clients unaffected. Slow clients re-connect and (per the snapshot+delta pattern) get a fresh snapshot. Pragmatic.

**Pattern 3: separate "speed lanes."** Free-tier clients get aggregated/throttled updates (100ms snapshot cadence). Pro-tier clients get full delta stream. Different service levels, different backpressure budgets.

**The cardinal rule: one slow client must never slow down the others.** The broadcast path is shared; the per-client path is where you isolate slowness.

---

### The Writer Loop (Zero-Lag Broadcast)

```
[Event Bus] ──► [Gateway Dispatch Goroutine] ──► [Per-Client Queues] ──► [Per-Client Writers]
```

**Dispatch goroutine** receives events from the bus. For each event:
1. Determine which channel (e.g., `book.BTC-USDT.diff`).
2. Look up subscribers to that channel (a slice of client pointers).
3. Serialize the message once (to bytes).
4. Non-blocking push to each client's queue.

Key: **serialize once, not per-client.** Encoding 100k times burns CPU. Encode once, share the byte slice via pointers.

```go
func (g *Gateway) dispatch(channel string, event Event) {
    // Serialize once
    msg := serialize(event)   // []byte
    
    // Look up subscribers
    g.mu.RLock()
    subs := g.subscriptions[channel]   // []*Client
    g.mu.RUnlock()
    
    // Fan out, non-blocking
    for _, c := range subs {
        c.SendNonBlocking(msg)  // drops if queue full
    }
}
```

For 100k subscribers to a hot channel:
- Serialization: once (~100 μs for a book update).
- Fan-out loop: 100k × ~100 ns (channel send) = 10 ms total.

10 ms is too long for HFT, acceptable for most exchanges. For zero-lag HFT-grade fan-out, you need UDP multicast or Aeron (Phase 2 Lesson 5).

**The subscription map** uses a `sync.RWMutex` so dispatch reads without blocking. Subscribes/unsubscribes take the write lock (rare compared to dispatch).

---

### Message Ordering Guarantees

What happens if client subscribes, and between "subscription registered" and "next event published," a message fires? Does the client get it?

**Potential race:**
```
T=0: client sends SUBSCRIBE book.BTC-USDT.diff
T=1: dispatch publishes book event (not yet registered)
T=2: gateway registers subscription
T=3: dispatch publishes next book event (client receives)
→ Client missed the T=1 event.
```

Fix: register the subscription *first*, then send a snapshot. Any events that fire between subscription-register and snapshot-send are received (buffered in the client's queue). The snapshot contains a sequence number; the client drops any delta events with sequence ≤ snapshot's sequence (they're already reflected in the snapshot).

This is *critical* and subtly easy to get wrong. Walk through your code carefully to make sure there's no gap.

---

### Connection Lifecycle

Real handling of a client connection:

```
1. Client opens TCP + HTTP upgrade → WebSocket handshake.
2. Server authenticates (JWT in cookie or first message).
3. Server creates Client struct, starts reader + writer goroutines.
4. Client sends SUBSCRIBE messages.
5. Server registers subscriptions, sends snapshots.
6. Pings every 15 seconds; client replies pong.
7. Normal operation: server pushes, client receives.
8. Client disconnects cleanly (WebSocket close frame) OR
   Client times out (no pong within 30 seconds) OR
   Server disconnects slow client (write timeout) OR
   Server shuts down (graceful disconnect with close frame and code).
9. Server cleans up: remove from subscriptions, close queues, free buffers.
```

Every path must lead to cleanup. Leaked client structs at 100k scale = running out of memory in hours.

**Graceful shutdown during deployment.** Gateway gets a signal to shut down. It:
1. Stops accepting new connections.
2. Sends a "please reconnect" close frame to all existing clients.
3. Waits for outstanding writes to flush (with a timeout).
4. Exits.

Clients with auto-reconnect handle this smoothly. No trader experiences a gap; they reconnect to a different gateway instance.

---

### Go-Specific Warnings

**1. `gorilla/websocket` is maintenance-only.** The library is stable but no longer actively developed. For new code, `nhooyr.io/websocket` is the modern choice (cleaner API, context-based). For 100k+ connections, `gobwas/ws` (with manual epoll) is the performant option.

**2. `net.TCPConn.SetNoDelay(true)` disables Nagle's algorithm.** Critical for real-time push. Without this, small messages get buffered in the kernel for up to 40ms. Enable it.

**3. `io.Copy` allocates.** If your writer uses `io.Copy`, profile — it uses a buffer, may allocate. Use `WriteMessage` directly with pre-allocated buffers.

**4. JSON serialization is a CPU hog.** At 100k clients × 100 events/sec = 10M serializations/sec. If each takes 5μs, that's 50 CPU-seconds of work per real second = 50 cores. Use binary protocols (protobuf, flatbuffers, or hand-rolled `encoding/binary`) to cut this 10×. Or: serialize once per event, send the same bytes to all clients.

**5. `sync.Map` is slower than `map + RWMutex` for most workloads.** Despite the name, only use `sync.Map` for append-once-read-many patterns. For your subscription map, plain `map[string][]*Client` with `RWMutex` is usually faster.

**6. Goroutine leaks at connection count.** A forgotten close channel or unrecovered panic in a goroutine leaks it. At 100k connections, one leaked goroutine per 1000 connections = 100 leaks/sec of high-load traffic = OOM in hours. Use `context.Context` everywhere. `go vet`. Profile with `/debug/pprof/goroutine`.

**7. `websocket.Conn.WriteMessage` is not goroutine-safe.** Only one goroutine can write at a time. That's why the "writer goroutine" pattern exists. If two goroutines call `WriteMessage` concurrently, the protocol breaks (interleaved frames) and the connection dies.

**8. TLS termination is expensive.** TLS handshakes are CPU-heavy. Terminate at the load balancer (NGINX, Envoy, HAProxy) and let the gateway handle plain WebSocket. Saves 30-50% CPU.

**9. File descriptor limits.** Before running, set `ulimit -n 1000000` or systemd `LimitNOFILE=1000000`. On boot: edit `/etc/security/limits.conf` or `/etc/systemd/system.conf`. Verify with `cat /proc/$PID/limits` on the running process.

---

### Architecture Summary

```
Load Balancer (TLS termination)
      │
      ↓ plain WebSocket
┌────────────────────────────────────┐
│         Gateway Servers            │
│   ┌──────┐  ┌──────┐  ┌──────┐     │
│   │ WS-1 │  │ WS-2 │  │ WS-N │     │  Each: 10-30k connections
│   └──┬───┘  └──┬───┘  └──┬───┘     │        Subscribes to Kafka
└──────┼────────┼────────┼───────────┘
       │        │        │
       ↑        ↑        ↑
┌──────────────────────────────┐
│   Market Data Topic (Kafka)  │
└──────────────┬───────────────┘
               ↑
               │ publishes events
┌──────────────┴──────────────┐
│     Matching Engine          │
└──────────────────────────────┘
```

---

### Summary: The Rules

1. **Use WebSocket with binary frames.** Not JSON, not polling.
2. **Snapshot + delta with sequence numbers.** Client detects gaps and re-snapshots.
3. **Fan out via a message bus (Kafka).** Don't tie gateways to the engine directly.
4. **One serialization per event, shared across subscribers.** CPU cost is fixed.
5. **Per-client bounded queue + drop/disconnect on overflow.** One slow client ≠ everyone slow.
6. **Terminate TLS at the load balancer.** Save gateway CPU.
7. **`ulimit -n`, `SetNoDelay(true)`, tuned socket buffers.** Kernel defaults will kill you.
8. **Every connection path ends in cleanup.** Leaks compound.

---

### Drill 10

**Q1. Linux mechanics.**
Explain in your own words:
(a) Why `select()` doesn't scale to 100k connections.
(b) What `epoll_wait()` returns and why it's O(k) not O(N).
(c) What happens at the kernel level when a WebSocket client is in a state where its TCP receive buffer is full (slow network). What happens on the server side when you call `conn.Write(data)` on that connection?

**Q2. Minimum viable gateway.**
Build a Go WebSocket gateway that:
- Listens on a port, accepts WebSocket connections.
- Clients can send `SUBSCRIBE book.BTC-USDT.diff` messages.
- The server has a single "publisher" goroutine generating synthetic book updates at 100 events/sec and pushing to all subscribers.
- Uses `gorilla/websocket` or `nhooyr.io/websocket`.
- Uses a per-client bounded queue (size 100). Clients whose queue fills get disconnected.
- Binary frames; messages are JSON or your choice of encoding.

Run it locally. Connect 100 clients (one Go benchmark spawning 100 goroutines as clients). Verify each receives updates at ~100/sec.

**Q3. Benchmark to 10k.**
Extend your Q2 harness to spawn 10,000 clients. Measure:
- Memory usage of the gateway (`/proc/$PID/status` RSS).
- CPU usage during broadcast.
- Per-message end-to-end latency (publish time → client receive time). Report p50, p99, p99.9.

At 10k, memory should be ~1-2 GB, CPU should be a few cores, p99 < 50ms. If any of these is worse, profile and explain.

**Q4. Benchmark to 100k.**
Push to 100k clients. Document what breaks first. Options:
- File descriptor exhaustion (did you `ulimit -n`?).
- Memory (how many GB of goroutine stacks?).
- CPU saturation (which functions burn cycles?).
- Slow clients stalling broadcast (Pattern 1 from the lesson).
- GC pauses (`GODEBUG=gctrace=1`).

Fix the first failure. Iterate. How close can you get to 100k on a single server before hitting a wall you can't fix without architecture changes?

**Q5. Snapshot + delta.**
Implement the snapshot+delta pattern properly:
- Gateway maintains a live order book state, updated from Matching Engine events.
- New client sends SUBSCRIBE → gateway registers subscription, then sends a full snapshot with sequence N, then starts pushing deltas with seq > N.
- Client verifies sequence continuity. If a gap is detected, client sends RESNAP request; server responds with fresh snapshot.

Test by deliberately dropping messages on the client side. Verify recovery works. Report how many messages you can drop and still recover vs. at what point the client declares the connection broken.

**Q6. Slow-client test.**
Simulate a slow client: connect one client that reads at 1 KB/sec while the others read normally. At 100k clients total:
- What happens to the slow client (disconnected? message queue grows?)
- What happens to all the other clients (any latency impact? any disconnects?)

If other clients see latency impact, your backpressure isolation is broken. Fix it.

**Q7. Multi-server fan-out.**
Run two gateway instances. Both subscribe to the same Kafka topic. Half the clients connect to gateway A, half to gateway B. Verify all clients receive the same message stream at roughly the same time.

Measure cross-gateway latency: from Kafka publish to client receipt. Compare to single-gateway latency.

**Q8. Reading.**
Read the Binance WebSocket API documentation (https://binance-docs.github.io/apidocs/spot/en/#websocket-market-streams). Identify:
- Their snapshot+delta pattern.
- Their channel naming convention.
- Their rate limit / connection limit per user.
- Their ping/pong policy.

Pick one design decision they made and argue whether you'd make the same decision. 2-3 paragraphs.

---

# Lesson 11: Rate Limiting

### Why This Lesson Exists

Your gateway is serving 100k happy clients (Lesson 10). Tuesday 9am, BTC pumps 5%. Trading volume spikes 10x. Two things happen:

1. **A legitimate user's algorithm goes haywire** — they're sending 10,000 orders per second trying to catch the move. This isn't malicious, but it's hammering your matching engine.
2. **A malicious actor launches a DDoS** — 100 million connection attempts from a botnet. Your load balancer is saturated. Legitimate users can't connect.

Without rate limiting, both scenarios take your exchange down. Legitimate and illegitimate traffic have the same symptoms at the gateway: too many requests, overwhelming resources.

Rate limiting has two goals:

1. **Fairness:** no single user can consume resources at the expense of others.
2. **Protection:** the matching engine and internal services receive only as much traffic as they can handle — even during a DDoS or a flash event.

This lesson covers the full stack: network-level DDoS protection, gateway-level rate limits, user-level order rate limits, and the special category called "toxic flow" — patterns that look like normal trading but are actually market manipulation or resource exhaustion.

Most of the algorithms were introduced in System Design 101 Lesson 7. This lesson goes deeper on exchange-specific concerns.

---

### Toxic Flow: the Threat Beyond Volume

"Toxic flow" is industry jargon for order flow that hurts the exchange or other market participants, whether intentional or not. It's not just high volume — it's specific patterns:

**1. Quote stuffing.** Thousands of orders placed and cancelled within milliseconds. Purpose: overwhelm the matching engine or slow down competitors who have to process all the quotes. Quantity > liquidity provided.

**2. Spoofing / layering.** Large non-bona-fide orders placed to create the appearance of demand/supply. Cancelled before they can fill. Illegal (SEC/CFTC consider it market manipulation). You need to detect patterns of "place large order, cancel before fill" and warn or suspend.

**3. Wash trading.** Trader trades with themselves (directly or via related accounts) to inflate volume stats. Against exchange rules almost everywhere. Detect by tracking self-trade / related-account trades.

**4. Self-cross / self-trade prevention.** User has a buy and sell on the same book that would match. Usually the exchange *prevents* this execution (cancels one side). Otherwise wash trading is trivial.

**5. Flash quotes / fat finger.** Single erroneous order that's 100x typical size or 50% off market. Not toxic in intent but toxic in effect. Price band (Lesson 8) catches most; some slip through.

**6. Stale order pile-up.** Market maker disconnects, leaves stale quotes. Another trader picks them off. Not exactly toxic from the taker's side but hurts the market maker. Mitigated by heartbeat-triggered order cancellation (see below).

Rate limiting alone doesn't catch all toxic flow. You need *behavioral* checks on top: message-to-fill ratios, order lifetime distributions, per-user flagging.

---

### DDoS Threats

Distributed Denial of Service attacks come in several flavors. Your defenses need to match.

**Volumetric (L3/L4).** Massive network traffic. Clogs your link. Defense: upstream DDoS mitigation (Cloudflare, AWS Shield, Akamai). The attacker's traffic never reaches your infrastructure. Essential for any public-facing exchange.

**Protocol attacks (L4).** SYN floods, TCP/UDP flood. Fake half-open connections consume kernel resources. Defense: SYN cookies, connection rate limits at the firewall, upstream scrubbing.

**Application attacks (L7).** Botnet sends legitimate-looking HTTP requests. Each is cheap for the attacker but expensive for you (e.g., "list all my orders" with no filter). Defense: per-IP rate limiting, CAPTCHA, bot detection, request cost analysis.

**Slowloris.** Open connections and send data very slowly, keeping them open forever. Exhausts your connection pool. Defense: per-connection timeouts, aggressive slow-client disconnect (from Lesson 10).

**Amplification.** Spoofed requests to open reflectors (DNS, NTP) that return much larger responses to the victim. Mostly affects network infrastructure, not application.

A real exchange is attacked multiple times per day. Each layer of defense is necessary.

---

### The Layered Defense Model

```
Internet
   │
   ↓
[Cloudflare / AWS Shield]      ← L3/L4 DDoS scrubbing; bot detection
   │
   ↓
[Load Balancer / WAF]           ← TLS termination; per-IP connection limits; basic WAF rules
   │
   ↓
[API Gateway]                    ← JWT; per-user rate limits; request cost weighting
   │
   ↓
[Matching Engine + Ledger]       ← final guard: circuit breaker if overwhelmed
```

No single layer is sufficient. DDoS that bypasses Cloudflare must still face the LB's rate limits. Toxic flow that looks legitimate to the WAF must face the gateway's user-level limits. A bug in the gateway must be contained by the matching engine's circuit breaker.

**Defense in depth.**

---

### Rate Limiting Algorithms

Recap from System Design 101 Lesson 7, with more depth.

**Fixed window.**
Count requests in a time window (e.g., per minute). Reset at window boundary. Allow up to N per window.

```
[12:00:00 - 12:01:00] Count: 0, 1, 2, ..., 99 → 429, 429, 429, ...
[12:01:00 - 12:02:00] Count: 0, 1, 2, ...
```

**Fatal flaw:** the boundary attack. At 12:00:59 submit 99 requests. At 12:01:00 submit 99 more. Total 198 requests in 2 seconds, all allowed. Effective limit 2x the configured limit for anyone clever. **Never use fixed window for serious rate limiting.**

**Sliding window (log-based).**
Track exact timestamps of each request. Count how many fall within the last N seconds.

```
request 1: 12:00:30.123
request 2: 12:00:45.678
request 3: 12:01:02.456
At 12:01:05, last 60s includes all 3.
At 12:01:35, last 60s includes only request 3.
```

Accurate. Expensive: store timestamps, scan on every check. Memory grows with request rate.

**Sliding window (counter-based).**
Approximation that avoids storing individual timestamps. Store counts per small sub-window (say, per second). Sum the last N sub-windows.

```
Last 60 seconds, sub-window = 1 second:
  [12:00:06]=5, [12:00:07]=3, [12:00:08]=0, ..., [12:01:05]=2
  Sum = total in last 60s
```

Accurate enough for most use cases. Bounded memory. Standard choice when fixed-window isn't acceptable.

**Token bucket.**
The algorithm of choice for exchanges. Conceptually: a bucket holds tokens. Each request consumes 1 token. Tokens refill at a fixed rate. If the bucket is empty, the request is rejected (429) or blocked.

```
bucket_size = 50
refill_rate = 10 tokens/sec
```

- Steady state: up to 10 req/sec sustained.
- Burst: up to 50 req in a 5-second window (use all accumulated tokens at once).
- Ideal for real traders who sometimes burst (during volatile markets) but don't sustain extreme rates.

Token bucket allows **legitimate bursting**. A market maker replacing 20 quotes in 100ms is allowed. A script spamming 1000 orders/sec is blocked. Fixed-window can't distinguish these.

**Leaky bucket.**
Requests enter a queue. Queue drains at fixed rate. If queue is full, request is rejected.

Smooths output. Regardless of burst size in, output is steady. Useful for rate-limiting *to* a downstream service — protects the downstream from bursts. Different use case than token bucket.

---

### Variable-Weight Rate Limiting

Here's where exchanges get clever. Not all requests cost the same. A market-data snapshot is cheap. An order placement is expensive. A bulk cancel is *extremely* expensive.

**Assign weights based on cost:**

| Endpoint                | Weight |
|-------------------------|--------|
| `GET /ticker`           | 1      |
| `GET /orderbook`        | 2      |
| `GET /mytrades`         | 3      |
| `POST /order` (new)     | 10     |
| `POST /order/cancel`    | 1      |
| `POST /orders/cancel`   | 100    |

Now your token bucket deducts `weight` tokens per request, not 1.

- 1000 tokens/min budget: 1000 ticker reads *or* 100 new orders *or* 10 bulk cancels.
- User can't disguise expensive operations as many cheap ones.

Binance, Coinbase, FTX (R.I.P.) all use variable-weight rate limiting.

---

### Where Rate Limit State Lives

**Option A: Per-instance in-memory.** Each gateway server has its own counters. Simplest. Problem: user's requests may route to different servers, bypassing per-instance limits. A 10-server cluster with 1000 req/min limit each = effective 10,000 req/min. Broken.

**Option B: Centralized Redis.** All gateway servers read/write the same counters. Correct but adds 0.5-1ms per request (Redis round trip). For hot paths, painful.

**Option C: Hybrid — local counter synced to Redis.** Gateway tracks local count, periodically syncs to Redis (every 100ms). Check is local (fast); global coordination is eventual. Good enough for most rate limits.

**Option D: Sticky routing.** Load balancer routes a user's requests always to the same gateway (consistent hash on user ID). Local counters work. Fails if that gateway dies (user migrates, briefly gets double budget). Usually fine.

For a serious exchange: B for sensitive limits (withdrawals, API keys), C or D for everything else.

---

### Redis + Lua: The Atomic Increment

Naive Redis rate limit (System Design 101 Lesson 7 covered this — critical, worth repeating):

```
GET ratelimit:user:123      → 99 (current count)
if 99 < 100: INCR ratelimit:user:123
```

Broken. Two instances can both read 99 and both increment to 100. Two requests allowed when only one should be. Race.

**Fix: Lua script. Atomic.**

```lua
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local cost = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

local current = tonumber(redis.call('GET', key) or 0)
if current + cost > limit then
    return -1  -- rate limited
end

redis.call('INCRBY', key, cost)
if current == 0 then
    redis.call('EXPIRE', key, ttl)
end
return limit - current - cost  -- remaining tokens
```

Redis executes this as one indivisible operation. No race. This is the canonical pattern. Every serious rate limiter uses some variant.

For token bucket specifically:

```lua
-- KEYS[1] = bucket key
-- ARGV[1] = bucket size
-- ARGV[2] = refill rate (tokens/sec)
-- ARGV[3] = now (unix ms)
-- ARGV[4] = cost

local bucket = redis.call('HMGET', KEYS[1], 'tokens', 'last')
local tokens = tonumber(bucket[1]) or tonumber(ARGV[1])
local last = tonumber(bucket[2]) or tonumber(ARGV[3])
local now = tonumber(ARGV[3])
local size = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local cost = tonumber(ARGV[4])

-- Refill based on time elapsed
local elapsed = (now - last) / 1000.0
tokens = math.min(size, tokens + elapsed * rate)
last = now

if tokens < cost then
    redis.call('HMSET', KEYS[1], 'tokens', tokens, 'last', last)
    redis.call('EXPIRE', KEYS[1], 60)
    return -1
end

tokens = tokens - cost
redis.call('HMSET', KEYS[1], 'tokens', tokens, 'last', last)
redis.call('EXPIRE', KEYS[1], 60)
return math.floor(tokens)
```

One atomic call per rate-limit check. Stateless from the application's perspective. Scales.

---

### Connection Rate Limiting (Pre-Auth)

Before the user even authenticates, protect against connection floods:

- **Per-IP connection limit.** No single IP can have > N simultaneous connections. Tune based on whether you expect NAT users (mobile carriers, large offices can have many legitimate users behind one IP — be careful).
- **Per-IP connection rate.** No single IP can open > M connections/sec. Defeats connection flood attacks.
- **Geographic / ASN filtering.** Some exchanges block entire regions for regulatory reasons. Can also block ASNs known for attacks (botnet ISPs).
- **TLS fingerprinting (JA3).** Detect automated tools (scripts, bots) by their TLS handshake characteristics. More aggressive: reject non-browser fingerprints from web endpoints.

These checks happen at the load balancer or WAF, not the gateway. The gateway shouldn't see the attacker's packets at all.

---

### Order-Specific Rate Limiting

Beyond HTTP-level rate limiting, exchanges have order-specific rules:

- **Max orders in flight:** user can have at most N open orders simultaneously. Limits memory use.
- **Max order creation rate:** N new orders per second (token bucket).
- **Max cancellation rate:** usually higher than creation (bulk cancels during volatility are legit).
- **Max active symbols:** user can trade at most K symbols at once (prevents one user from opening state across all 1000 trading pairs).
- **Message-to-fill ratio:** if user places and cancels 1000 orders without any fills, that's quote stuffing. Flag, warn, eventually suspend.
- **Cancel-on-disconnect:** if a user disconnects unexpectedly, auto-cancel their open orders. Prevents stale-order pick-off. Option the user can enable/disable.

These are implemented in the risk engine (Phase 3 Lesson 8) or a dedicated policy layer.

---

### Circuit Breakers (Gateway → Engine)

If the matching engine is slow or degraded (e.g., GC pause, network hiccup), the gateway can protect it by circuit-breaking.

Three states:
- **Closed** (normal): pass through.
- **Open** (engine sick): reject immediately with 503. Give the engine time to recover.
- **Half-open**: after cooldown, allow a few test requests. If they succeed, close. If they fail, reopen.

Detection: track per-request latency to the engine. If p99 > 500ms for 10 consecutive seconds, open. If p99 < 100ms for 30 seconds after a test probe, close.

**Without a circuit breaker:** gateway keeps flooding a struggling engine with requests. Requests timeout. Retries pile up. Engine thrashes. System spirals.

**With a circuit breaker:** gateway stops feeding the engine. Engine recovers. Test probes confirm. Normal resumed.

Essential for production. Go libraries: `sony/gobreaker`, `afex/hystrix-go`.

---

### Penalty Escalation

Not all over-limit users are attackers. Grade your response:

| Violation         | Response                      |
|-------------------|-------------------------------|
| First over-limit  | Return 429 + warning header   |
| Repeated (>5 in 1hr) | Temp ban 1 hour            |
| Persistent        | Ban 24 hours, notify user     |
| Clear attack      | Permanent ban, IP block       |
| Toxic flow (spoofing, wash) | Trading suspension + manual review |

Headers to send on 429:
- `Retry-After: <seconds>`
- `X-RateLimit-Limit: <total>`
- `X-RateLimit-Remaining: <remaining>`
- `X-RateLimit-Reset: <unix timestamp>`

Good clients respect these and back off. Bad clients don't — and then you escalate.

---

### Rate Limiter in Go: Zero-Allocation Fast Path

For the gateway's hot path (every request), you want a rate check in < 1 μs. Redis round trip is 100-500 μs — too slow for checking every request. Pattern:

1. Check a local in-memory token bucket (microseconds).
2. If local allows, accept request.
3. Periodically (every 100ms) sync with Redis to get global state.

```go
type LocalBucket struct {
    tokens     int64  // atomic
    lastRefill int64  // atomic, unix ns
    size       int64
    refillRate int64  // tokens per second
}

func (b *LocalBucket) TryConsume(cost int64) bool {
    now := time.Now().UnixNano()
    last := atomic.LoadInt64(&b.lastRefill)
    elapsed := now - last  // nanoseconds
    refill := (elapsed * b.refillRate) / int64(time.Second)
    if refill > 0 && atomic.CompareAndSwapInt64(&b.lastRefill, last, now) {
        for {
            cur := atomic.LoadInt64(&b.tokens)
            newVal := cur + refill
            if newVal > b.size {
                newVal = b.size
            }
            if atomic.CompareAndSwapInt64(&b.tokens, cur, newVal) {
                break
            }
        }
    }
    for {
        cur := atomic.LoadInt64(&b.tokens)
        if cur < cost {
            return false
        }
        if atomic.CompareAndSwapInt64(&b.tokens, cur, cur-cost) {
            return true
        }
    }
}
```

Pure atomics. No mutex. Zero allocations. Works fine for per-instance limits. For global limits, sync to Redis asynchronously.

---

### Go-Specific Warnings

**1. `golang.org/x/time/rate` is great but watch allocations.** The standard rate-limiter package. Uses `time.Time` and some internal allocations. Fine for moderate rates; for million-RPS hot paths, roll your own atomic version.

**2. Concurrent map access in rate limiters.** If you maintain a `map[string]*LocalBucket` keyed by user ID, concurrent access needs protection. `sync.Map` is acceptable here (appends heavy on boot, reads dominate after). Or shard the map (16 shards, hash user ID to pick shard, per-shard mutex).

**3. Clock skew in Redis Lua scripts.** If each client passes `now` as an argument, clocks on different gateway servers may disagree. For token bucket rate, use `redis.call('TIME')` inside the Lua script to get Redis's own clock. Single source of truth.

**4. `context.Context` deadlines.** On every rate-limited request, pass a context with a short deadline (e.g., 50ms). If Redis Lua call takes too long, fail open (allow the request) rather than hanging. Safer to let an extra request through than to deadlock.

**5. Don't rate limit on the response path.** Check limits *before* doing work. A 429 that comes after the engine has already matched the order is worse than useless.

---

### Summary: The Rules

1. **Layer your defenses.** Cloudflare → LB → Gateway → Engine. Each a backstop for the previous.
2. **Token bucket for user-level rate limiting.** Allows legitimate bursts, blocks sustained abuse.
3. **Variable-weight limiting.** Expensive operations cost more tokens.
4. **Redis + Lua scripts for atomic rate-limit state.** Never `GET` + check + `INCR`.
5. **Local fast path + async Redis sync for hot paths.** Sub-μs checks, eventual global consistency.
6. **Self-trade prevention, spoofing detection, cancel-on-disconnect.** Toxic flow is beyond volume.
7. **Circuit breaker between gateway and engine.** Protect the core from overload.
8. **Penalty escalation: warning → temp ban → permanent ban.** Humans over machines.

---

### Drill 11

**Q1. Boundary attack.**
Implement a *fixed-window* rate limiter (100 req/min). Write a test that exploits the boundary: submit 99 requests at 12:00:59 and 99 more at 12:01:00. Verify your fixed-window limiter allows all 198 requests within a 2-second span. Then replace with sliding window (counter-based) and verify the same attack is blocked.

**Q2. Token bucket in Go.**
Implement a lock-free token bucket using atomics only (pattern from the lesson). Benchmark:
- Single-goroutine: target > 20M ops/sec.
- 16-goroutine concurrent: target > 100M ops/sec aggregate.
- Zero allocations per `TryConsume` call.

Report numbers.

**Q3. Variable-weight.**
Extend your token bucket so each operation has a cost. Define the weight table from the lesson (ticker=1, order=10, bulk-cancel=100). Build a mock trading loop: a user placing/cancelling/reading at realistic rates. Verify the user hits the limit at the expected mix of operations.

**Q4. Redis + Lua.**
Implement the token bucket Lua script from the lesson. Verify atomicity: launch 1000 concurrent Go clients hammering Redis with `TryConsume` calls. Count how many the script says allowed vs rejected. The number allowed must equal `bucket_size + elapsed × refill_rate` (within small rounding). If more are allowed, your Lua has a race condition.

**Q5. Local + Redis hybrid.**
Build a gateway rate limiter with:
- Per-instance local token bucket (sub-μs check).
- Every 100ms, sync to Redis: drain any extra usage since last sync, pull global total.
- If Redis is unreachable, fail open (log an alert, but don't break the gateway).

Benchmark end-to-end latency of a rate-limited request:
- Local-only path: target < 1 μs.
- Path that happens to trigger a Redis sync: target < 2 ms.
- Failure path (Redis down): target < 10 ms (with timeout).

**Q6. Circuit breaker.**
Implement a circuit breaker between your gateway and a mock matching engine. Simulate the engine going into a slow state (add a 2-second sleep to 10% of requests). Verify:
- Circuit opens after N slow requests.
- Subsequent requests get immediate 503.
- After a cooldown, a test probe is allowed. If engine has recovered (no sleep), circuit closes. If still slow, stays open.

Measure: how many requests were served 503 during the degraded period? What would have happened without the circuit breaker? (Hint: compute how many goroutines would be blocked waiting for slow responses.)

**Q7. DDoS simulation.**
Using your Q3 setup (WebSocket + rate-limited HTTP gateway), simulate an attacker opening 10,000 connections from 100 IP addresses as fast as possible. Verify:
- Per-IP connection limit stops the flood at 100 connections per IP (1000 total).
- Legitimate traffic (slower, from different IPs) still gets through.
- Memory and CPU on the gateway stay below 80%.

If legitimate traffic degrades, your isolation is broken. Find and fix.

**Q8. Toxic flow detection.**
Build a simple message-to-fill ratio detector. Track per-user:
- Orders placed in last hour.
- Orders filled in last hour.
- Ratio.

If ratio > 100 (100 orders per fill, suggesting quote stuffing), flag the user. Add it to your Lesson 8 risk engine as a warning signal.

Run a simulation of 1000 legitimate traders and 10 attackers running quote-stuffing bots. Verify the detector identifies all 10 attackers and zero legitimate traders (false positive rate = 0).

**Q9. Reading.**
Read:
- Binance's rate-limit documentation (search: "binance spot api rate limits").
- A post-mortem of any major exchange DDoS incident (e.g., Coinbase during the "pepe surge" of 2023, Bitstamp 2022).

Answer:
- What specific limits does Binance enforce? How do weights work on their API?
- In the post-mortem, which layer of defense failed? Was it absent, misconfigured, or overwhelmed?
- What would you change in the defenses described in this lesson to prevent that incident?

---

## Phase 4 Master Rules

### WebSocket Scaling
- Binary frames, not JSON. Snapshot + delta with sequence numbers.
- Fan out via Kafka (or equivalent). Gateways are stateless replicas.
- One serialization per event, shared across all subscribers.
- Per-client bounded queue + disconnect on overflow. One slow client cannot slow others.
- TLS terminated at the load balancer; gateway handles plain WebSocket.
- `ulimit -n 1000000`, `TCP_NODELAY`, tuned socket buffers. Defaults kill you.
- Every connection path ends in cleanup. Leaks compound to OOM.
- For > 100k on a single server: event-loop (gobwas/ws) or shard across multiple servers.

### Rate Limiting
- Layer: Cloudflare → LB → Gateway → Engine. Every layer a backstop.
- Token bucket for user rate limits. Variable weight per operation.
- Redis + Lua for atomic state. Never `GET` + check + `INCR`.
- Local atomic fast path + async Redis sync. Sub-μs checks, eventual global state.
- Circuit breaker between gateway and engine. Contain overload.
- Toxic-flow detection is separate from volume rate limits — behavioral, not just rate.
- Penalty escalation: 429 → temp ban → permanent ban → IP block.
- Cancel-on-disconnect, self-trade prevention, message-to-fill ratio monitoring.

### If You Do Phase 4 Right
- 100k concurrent connections serving real-time market data with p99 latency < 100ms.
- DDoS attacks stopped at the edge; legitimate users unaffected.
- No single user, legitimate or malicious, can degrade the experience of others.
- Matching engine protected from overload by layered rate limits and circuit breakers.
- Toxic flow detected and logged; patterns flagged for manual review.
- Graceful degradation: during a genuine overload, the system serves less traffic rather than crashing.

---

## Final Phase Summary: Where You've Been

| Phase | Core Concern       | Key Techniques                                       |
|-------|--------------------|------------------------------------------------------|
| 1     | Latency            | Ring buffer, determinism, zero-allocation hot path   |
| 2     | Reliability        | WAL, Raft, fork-style snapshots                      |
| 3     | Correctness        | Double-entry ledger, risk engine, hot/cold custody   |
| 4     | Gateway & API      | WebSocket fan-out, rate limiting, DDoS defense       |

You now have the skeleton for a production-grade CEX matching engine in Go. The skeleton does not make a business — integration with compliance, liquidity, market making, customer support, and regulatory relationships is the other 90% of actually running an exchange. But without the skeleton, none of the rest matters, because you'd blow up on day one under load.

**Realistic next steps** if you want to build this for real:

1. Write Phase 1 end-to-end. You'll have working opinions about Go's viability after that.
2. Integrate Phase 2 (WAL + Raft) and crash-test repeatedly. This is where you learn the operational lessons that aren't in books.
3. Build Phase 3's ledger carefully with the invariant check always on. Fuzz it.
4. Build Phase 4 behind Cloudflare. Load-test with realistic order patterns, not just hammer-every-endpoint.
5. Shadow-deploy against a real exchange's API for a few weeks to compare outputs. This is how you find the edge cases you didn't imagine.

Good luck. Build well.

*End of course.*
