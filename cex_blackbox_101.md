# CEX Blackbox — Interview Prep (Go)

> Four modules. How the whole exchange actually works, end to end.
> Interview-ready talking points, trade-offs interviewers push on, drills.

---

## Preamble: How CEX Interviews Actually Work

Before the modules, a meta-skill note.

CEX system design interviews follow a predictable shape:

1. **The vague opening** (2-5 min). "Design a centralized cryptocurrency exchange." Or: "How would you build Binance?" Or a narrower cut: "Design a real-time order book." The interviewer wants to see you scope the problem, not start drawing boxes immediately.

2. **Scoping questions** (5 min). You ask: which order types? spot or futures? how many users? how many symbols? target latency? regulatory jurisdiction? The interviewer answers some, waves off others. You pick a target scale — say "100K concurrent users, 10K orders/sec peak, sub-millisecond matching."

3. **High-level sketch** (5-10 min). Draw the big picture: gateway → risk → matching → settlement. Name the major components. Don't go deep on any yet.

4. **Deep drill** (20-30 min). Interviewer picks a component and goes deep. Most common picks: the matching engine, the gateway, the settlement flow, the idempotency story. You discuss data structures, concurrency, failure modes, specific technologies.

5. **Trade-off probes** (5-10 min). "What if X fails?" "What if you need 10x the throughput?" "How do you handle a malicious user doing Y?" This is where candidates either demonstrate depth or expose shallow memorization.

**The candidates who win are the ones who can move up and down the abstraction stack fluidly.** They can zoom out to the whole architecture and zoom in to "the matching engine uses a B-tree keyed by price, with each node holding a doubly-linked list of orders for price-time priority" in the same conversation. They can defend specific design choices: "I'd use a ring buffer instead of a channel here because channels cap out around 5M ops/sec and we need 10M."

This course gives you the zoom levels. The four modules map to the four natural components interviewers probe. Each module ends with concrete talking points for "if asked X, say Y" — because what you know matters less than what you can *say* in 45 minutes under pressure.

---

# Module 1: Order Ingestion & The Edge

> Users are idiots who double-click buttons and drop network connections.
> If you don't handle this at the gateway, you double-charge them and bleed money.

### What the Interviewer Sees From the Outside

User types into their browser, clicks "Buy." An HTTP POST or WebSocket message arrives at your API. Something happens. The response comes back: "Order placed" or "Rejected."

**The blackbox question:** what happens between the click and the response?

### What Actually Happens

```
[Client: browser / mobile app / trading bot]
    │
    ↓ TLS
[CDN / DDoS scrubbing layer: Cloudflare, AWS Shield]
    │
    ↓ (malicious traffic dropped)
[L4/L7 Load Balancer: NGINX, Envoy, HAProxy]
    │
    ↓ (TLS terminated, routed)
[API Gateway: authentication, rate limit, idempotency check]
    │
    ↓ (authorized, unique)
[Pre-Trade Risk Engine]    ← Module 2
    │
    ↓ (funds locked)
[Sequencer → Matching Engine]    ← Module 3
```

At each layer, traffic drops. By the time an event reaches the matching engine, it's:
- Authenticated (we know who sent it)
- Rate-limited (sender hasn't exceeded their budget)
- Idempotent (not a duplicate of an earlier attempt)
- Risk-checked (sender has the funds)
- Sequenced (has a globally-ordered ID)

Each drop is a defense. Without Layer N, Layer N+1 takes the hit.

---

### Rate Limiting (Depth)

**Already covered in Phase 4 Lesson 11.** Here's the interview-ready compression:

**Algorithm choice for exchanges: token bucket, always.**
- Fixed window has the boundary attack (99 requests at 12:00:59 + 99 at 12:01:00 = 198 in 2 seconds). Exploitable.
- Sliding window is accurate but memory-expensive at scale.
- Token bucket allows legitimate bursting (market maker replacing 20 quotes in 100ms) while blocking sustained abuse. The right model for trading.

**Variable-weight limiting.** Not all requests cost the same:

| Endpoint              | Weight |
|-----------------------|--------|
| GET ticker            | 1      |
| GET orderbook         | 2      |
| POST order (new)      | 10     |
| DELETE order (cancel) | 1      |
| DELETE orders (bulk)  | 100    |

Binance's rate limits are weighted. So are Coinbase's and most serious exchanges'. A user can't disguise expensive operations as many cheap ones.

**Where the state lives:** local atomic counter + async Redis sync. Sub-μs fast path. Eventual global consistency. Redis-only is too slow for the hot path (100-500μs per check × millions of requests/sec = system collapses).

**The atomic Lua script** in Redis for global limits. Never GET + check + INCR from the application — race condition allows limit bypass under concurrent requests.

**Interview talking point:** "I'd use token bucket because traders legitimately burst during volatility. Variable weighting by endpoint cost. State kept locally in an atomic counter with periodic async sync to Redis — sub-microsecond per check, because this runs on every order and Redis round-trips are too slow for the hot path."

---

### WebSocket Streaming (Depth)

**Already covered in Phase 4 Lesson 10.** Compression:

**Why WebSocket and not polling.** HTTP polling at 1-second intervals = up to 1 second stale and 100K req/sec overhead at 100K users. Polling at 100ms = 10x worse. WebSocket = persistent connection, server pushes immediately on event, zero polling.

**The snapshot + delta pattern with sequence numbers.** Client connects, gets full book snapshot with seq=N, then receives deltas with seq=N+1, N+2, ... If client detects a gap (expected N+3, got N+5), it requests a resnapshot. Interviewers specifically ask about gap recovery.

**Backpressure isolation.** Per-client bounded queue (e.g., 100 messages). If queue fills, disconnect the slow client rather than block others. One slow client must never slow down the broadcast path. This is the #1 interview question on WebSocket at scale: "what happens when one client is slow?"

**Fan-out architecture.** Matching engine → Kafka topic → multiple WebSocket gateway servers → clients. Each gateway server subscribes to Kafka, maintains its own subscription map (channel → set of local clients), serializes each event once, fans out to local subscribers. Horizontal scaling via adding gateway servers.

**100K connections on a single server: hard in Go.** Goroutine-per-connection eats ~2GB of memory at 200K goroutines (reader + writer per connection). For genuine 100K+ you need `gobwas/ws` with manual epoll, or shard across multiple servers.

**Interview talking point:** "WebSocket with binary frames, snapshot + delta with sequence numbers for gap recovery. Per-client bounded queue, aggressive disconnect of slow clients to protect the broadcast path. Fan-out via Kafka across multiple gateway servers. Each serializes once per event and shares the bytes. At 100K+ connections per server, goroutine-per-connection is too expensive — switch to event-loop with epoll."

---

### Idempotency (Deep Dive — the Gap Nobody Talks About)

This is the module's highest-leverage topic because interviewers love to push on it and most candidates hand-wave.

**Why idempotency matters in trading specifically.** Users don't just submit orders once. They submit, the connection hangs, they hit retry. Their bot fires the same order twice because of a logic bug. The network duplicates a UDP packet. The mobile client goes offline mid-submit and the user clicks submit again on reconnect.

Without idempotency: the user places one logical order, the exchange processes two. Position is 2x what the user intended. If the market moves against them, they take 2x the loss. They complain. You refund. Every incident is money out of your pocket.

**The general pattern (restated):** every write-side operation carries a unique key. The server dedupes on that key within a time window. Duplicate submission → server returns the cached response of the first attempt. User sees the same order ID, understands it was processed once, moves on.

**Four design decisions** that interviewers probe:

---

#### 1. Who generates the idempotency key?

**Option A: Client generates.** Client produces a UUID (v7 preferred — time-ordered) before the first attempt. Reuses the same UUID on every retry.

```http
POST /v1/orders
Idempotency-Key: 018e8f3a-1234-7abc-89de-f0123456789a
Content-Type: application/json

{"side": "buy", "price": "50000", "qty": "0.5"}
```

Pros: client controls, retries are trivially correct (same key → same result).
Cons: requires client cooperation. Buggy clients may reuse keys across different requests, or never retry-with-same-key. You can't enforce it — you can only return an error if the key is missing.

**Option B: Server generates from request content hash.**

```go
key := sha256(userID + side + price + qty + timestamp_minute)
```

Pros: no client cooperation needed.
Cons: the deterministic hash can collide on legitimate distinct orders (user places two identical orders within the same minute). Either you accept this (annoying) or you require a client-side nonce anyway — back to Option A.

**The industry standard: require client-generated idempotency keys.** Binance, Coinbase, Kraken all require `newClientOrderId` or equivalent. Don't hand-wave; make it mandatory with HTTP 400 on missing.

**Interview talking point:** "Client generates UUID v7, sent as `Idempotency-Key` header. Required field — 400 if missing. Server stores `key → response` mapping in Redis with 24-hour TTL. On duplicate, return cached response directly without re-executing business logic."

---

#### 2. Where does the idempotency check live?

**Option A: At the gateway, before any business logic.**

```go
func (h *Handler) PlaceOrder(ctx context.Context, req *OrderRequest) (*OrderResponse, error) {
    cached, err := h.idem.Get(ctx, req.IdempotencyKey)
    if err == nil {
        return cached, nil  // duplicate; return first attempt's result
    }
    // Execute
    resp, err := h.engine.Submit(ctx, req)
    if err != nil {
        return nil, err
    }
    // Cache result
    h.idem.Set(ctx, req.IdempotencyKey, resp, 24*time.Hour)
    return resp, nil
}
```

Pros: saves downstream work entirely on duplicates.
Cons: race condition — two concurrent submissions of the same key can both miss the cache, both execute. You need an atomic "check and set placeholder" pattern.

**Option B: At the matching engine, as a hash-set lookup.**

The sequencer / matching engine maintains a set of seen event IDs. Duplicate events are dropped there.

Pros: works even if idempotency check at gateway fails.
Cons: duplicate reached the engine already, so you've spent the network + auth + risk work.

**Industry pattern: defense in depth. Both.**

Gateway-side handles the 99% case (duplicate from retry). Engine-side is the backstop for anything that slips through (replays from Kafka consumer lag, bugs in the gateway's idempotency, etc.).

---

#### 3. The atomic pattern (avoiding the "both requests executed" race)

The naive `GET → if missing, execute, SET` has a race: two concurrent duplicate requests both see missing, both execute, both SET. Two orders placed.

**Fix: `SET NX` with a placeholder.** Redis command `SET key value NX EX ttl` sets only if not exists, atomically.

```go
// Try to claim the idempotency key with a "pending" marker
claimed, err := redis.SetNX(ctx, req.IdempotencyKey, "pending", 60*time.Second).Result()
if err != nil {
    return nil, err
}
if !claimed {
    // Another request has this key. Wait for it to complete and return its result.
    return waitForResult(req.IdempotencyKey)
}

// We claimed it. Execute.
resp, err := h.engine.Submit(ctx, req)
if err != nil {
    redis.Del(ctx, req.IdempotencyKey)  // release claim on error
    return nil, err
}

// Overwrite placeholder with actual result, longer TTL
redis.Set(ctx, req.IdempotencyKey, serialize(resp), 24*time.Hour)
return resp, nil
```

Second concurrent request hits `SetNX` → fails → enters `waitForResult` → polls Redis until the placeholder is replaced by the real response → returns that result. One execution, two identical responses. Correct.

**`waitForResult` implementation:** poll every 50ms up to a timeout (say 10 seconds). If the first request crashed before completing, the placeholder expires in 60s and subsequent retries can re-execute.

**Interview talking point:** "Redis `SET NX` with a `pending` placeholder to atomically claim the key. Concurrent duplicates block and wait. First execution writes the response. Crash safety via short TTL on the placeholder — if the original crashes, a retry after 60 seconds can re-execute."

---

#### 4. What's the right TTL?

**Too short:** a user retry after the TTL expires gets re-executed. Double-order.
**Too long:** Redis memory grows. At 10K orders/sec × 200 bytes each × 1 day = 170GB just for idempotency state.

**Industry standard: 24 hours.** Long enough that any sane retry is covered. Short enough that memory is bounded. Mobile clients that come back online after a day simply get "order not found" and can choose to re-submit with a new key.

Binance uses 24h. Stripe uses 24h for their payment idempotency. Standard.

**For particularly sensitive operations (withdrawals, transfers):** longer (7-30 days) and often durable (Postgres-backed rather than Redis).

---

#### 5. Redis vs in-memory LRU cache

At very high scale, even Redis becomes a bottleneck:
- ~500μs round trip per check.
- Shared Redis is a single point of failure / bottleneck.

**In-memory LRU cache of seen keys** at the gateway: sub-μs lookup. Each gateway instance has its own. But:
- Multiple gateway instances don't share state. If user's first attempt hit gateway A and their retry hits gateway B, B doesn't know about the first attempt. Double-execution.

**Solution: sticky routing + LRU cache.** Load balancer routes by user ID (consistent hash) to always the same gateway. Local LRU cache works.

**Fallback if that gateway is down:** failover to Redis-backed check.

**Production pattern: local LRU (fast path) + Redis (fallback for cross-instance).** Most requests hit the LRU and return in microseconds. Redis is the safety net for cache misses and cross-instance duplicates.

---

### Module 1 Summary: The Rules

1. Token bucket rate limiting, variable-weighted per endpoint cost.
2. WebSocket with binary frames, snapshot + delta with sequence numbers.
3. Per-client bounded queues; disconnect slow clients to protect others.
4. Client-generated idempotency keys (UUID v7), required not optional.
5. Atomic claim via Redis `SET NX` with pending placeholder; 24-hour TTL.
6. Defense in depth: idempotency at gateway AND at engine.
7. Local LRU cache on each gateway with sticky routing for sub-μs fast path.

### Interview Questions to Expect

- "How do you prevent a user from placing the same order twice if they double-click or their network drops?"
- "Walk me through the idempotency check in pseudocode. What's the race condition in the naive version?"
- "What's the right TTL? Why?"
- "Token bucket vs sliding window vs fixed window — which for trading and why?"
- "I have 100K WebSocket clients. One of them is on a bad connection. What happens to the other 99,999?"
- "How do you scale WebSocket fan-out across multiple gateway servers?"
- "What's the attack if I don't have an idempotency key? Walk me through the exploit."

### Drill: Module 1

1. Implement the `SET NX` + pending placeholder pattern in Go with Redis. Write a concurrent test that submits 100 identical requests simultaneously and verifies exactly one is executed, all 100 get the same response.

2. Extend to the LRU + Redis hybrid. Benchmark: local LRU hit (< 1μs), local LRU miss + Redis hit (< 500μs), Redis miss + execute (measured).

3. Explain in 5 minutes out loud (record yourself): how does the CEX handle the case of a user submitting an order, losing their connection for 30 seconds, then retrying? Cover client-side, gateway, risk, engine. If your explanation is under 5 minutes, add detail. If over 5, cut.

---

# Module 2: Pre-Trade Risk Engine

> Before an order ever touches the matching engine, you must verify the user has the funds.
> You cannot do this with a Postgres query — too slow.
> Lock too much: user gets stuck. Lock too little: exchange takes bad debt.

### The Blackbox Question

"User submits a buy order for 1 BTC at 50,000 USDT. How do you check they have the money, and how do you prevent them from spending it on two orders at once?"

### Why Postgres Is Not the Answer

Naive implementation:

```go
func CheckAndLock(userID uint64, amount int64) error {
    tx, _ := db.BeginTx(ctx, nil)
    var available int64
    tx.QueryRow("SELECT available FROM balances WHERE user_id = $1 FOR UPDATE", userID).Scan(&available)
    if available < amount {
        tx.Rollback()
        return ErrInsufficientFunds
    }
    tx.Exec("UPDATE balances SET available = available - $1, locked = locked + $1 WHERE user_id = $2", amount, userID)
    tx.Commit()
    return nil
}
```

Looks right. Runs at ~5,000 ops/sec per Postgres instance under load. Your matching engine needs millions of orders/sec, and every order needs a risk check. Postgres is 1000x too slow.

**Why:**
- Every check is a DB round trip (~1ms).
- `FOR UPDATE` locks the row, serializing all operations on that user.
- Transaction commit involves WAL fsync (~100μs even on NVMe).

You cannot risk-check at matching-engine rate through a relational database.

---

### In-Memory Balance Locking (The Answer)

The risk engine maintains user balance state in RAM. Every balance change writes to the DB too (for durability), but the risk check itself is an in-memory read.

```go
type UserBalance struct {
    Available int64  // fixed-point (satoshis, etc.)
    Locked    int64
    _         [48]byte  // padding to 64 bytes (cache line)
}

type RiskEngine struct {
    balances [1 << 20]UserBalance  // pre-allocated, 1M users
}

// Single goroutine owns all writes. No mutex needed.
func (r *RiskEngine) Reserve(userID uint32, asset AssetID, amount int64) error {
    u := &r.balances[r.index(userID, asset)]
    if u.Available < amount {
        return ErrInsufficientFunds
    }
    u.Available -= amount
    u.Locked += amount
    return nil
}
```

Check + update in ~50 nanoseconds. No DB, no lock contention. Phase 3 Lesson 8 discipline.

**But:** memory is volatile. The DB is the source of truth. How do you keep them in sync without slowing down?

**Answer: event sourcing (Phase 2 Lesson 4 / Phase 3 Lesson 7 material).** Every balance change is an event in the WAL. The in-memory state is derived. On restart, replay the WAL to reconstruct. The DB update happens asynchronously — a downstream consumer reads events from the WAL (or Kafka) and updates the relational balance table. The risk engine never queries Postgres during a check.

```
[Order arrives]
    ↓
[Risk Engine: in-memory check + reserve]  ← sub-μs
    ↓ emits reservation event
[WAL (durable)]  ← the source of truth
    ↓ async consumed
[Postgres balance table]  ← eventually consistent, for reporting/audit
```

---

### The Lock-Too-Much vs Lock-Too-Little Problem (Interview Gold)

This is where the interviewer tests whether you actually understand exchange mechanics. Get it wrong and they know you're hand-waving.

**The core tension:**
- **Lock too much:** user has 1000 USDT. Places limit buy 1 ETH @ 3000. The order can never fill (insufficient funds to complete purchase). Should the risk engine reject it, or let it sit in the book forever tying up non-existent capital? If you're strict, user complains "I have funds but you said I don't." If you're lax, they place 100 fantasy orders cluttering your book.
- **Lock too little:** user has 1000 USDT. Places limit buy 1 ETH @ 1000. The risk engine reserves 1000. Then user places another limit buy 1 ETH @ 1000. The check reads "available = 0" — good. But what if the check is done in parallel (sharded risk engines)? Race condition → both orders reserve the same funds → user places 2 orders worth 1000 each → only 1000 USDT → one of them fails at fill time → exchange is on the hook.

**Three sub-problems to solve:**

#### Sub-problem 1: What exactly to lock for a limit order

**Naive: lock `price × qty`.** For a limit buy 1 BTC at 50,000, lock 50,000 USDT. Correct and tight.

**Refinement: include fees.** The user pays a taker fee (say 0.1%) = 50 USDT more. If you don't lock the fee, the order fills, user owes 50 USDT, but they have 0 USDT locked for it. Either:
- Lock `price × qty × (1 + fee_rate)` upfront (Binance does this).
- Or let the fee be deducted from the received asset (partial fee recovery).

**For market orders:** no limit price, so you don't know the final fill price. Options:
- Lock `available balance × safety multiplier` and refund after fill. User can't place other orders until this market order fills. Conservative, annoying.
- Lock based on current best ask × 1.1 (estimated worst-case fill price). Fails if market moves hard mid-fill.
- Reject market orders from users without sufficient "clearly enough" balance.

**Industry standard:** limit orders lock `price × qty × (1 + fee_rate)`. Market orders lock a conservative estimate and reconcile after fill.

#### Sub-problem 2: Cross-margin vs isolated margin (for futures)

In futures trading with leverage:

- **Isolated margin:** each position has its own margin pool. Losses on position A can't be covered by margin on position B. User's risk is bounded per position, but capital is less efficient.
- **Cross-margin:** all positions share a single collateral pool. Gains on A can offset losses on B. More capital-efficient but one bad position can liquidate the entire account.

The lock calculation differs:
- **Isolated:** lock `required IM for this position` against this position's margin pool.
- **Cross:** lock `required IM` against the user's total available margin (which includes unrealized PnL from other positions).

Cross-margin is harder to implement correctly because every mark price update changes every position's effective margin. Risk engine must recompute continuously.

#### Sub-problem 3: Concurrent risk checks

If you have multiple risk engine instances (for scale), two risk engines could approve two separate orders that together exceed the user's balance.

**Solution A: shard by user ID.** All orders for user 123 go to the same risk engine instance. Single-writer per user. No races. Scales by sharding users across instances.

**Solution B: single global risk engine.** All orders go to one instance. Absolute correctness but throughput-capped.

**Industry standard: sharded by user ID.** Binance, Coinbase, Deribit all do this. Hash(user_id) → risk engine instance. Each instance is single-threaded (single goroutine). Never a race.

The trade-off: if one risk engine instance dies, all users hashed to it can't trade until failover. Mitigation: Raft-replicated risk engines (Phase 2 Lesson 5 material).

---

### Balance Accounting Inside the Risk Engine

Each user's state:

```go
type UserBalance struct {
    Available int64   // can be spent or withdrawn
    Locked    int64   // reserved for open orders
    // For margin/futures:
    Position  int64   // signed: + long, − short
    EntryAvg  int64   // average entry price
}
```

Invariant: `Available + Locked = total balance` (at any moment).

**Events that change the state:**

| Event              | Available   | Locked      | Position |
|--------------------|-------------|-------------|----------|
| Deposit            | +X          | 0           | 0        |
| Withdraw (init)    | -X          | 0           | 0        |
| Place limit buy    | -(P*Q*F)    | +(P*Q*F)    | 0        |
| Cancel limit buy   | +(P*Q*F)    | -(P*Q*F)    | 0        |
| Fill limit buy     | 0           | -(P*Q*F)    | +Q       |
| Fill limit sell    | +(P*Q)      | 0           | -Q       |
| Liquidation        | varies      | varies      | → 0      |
| Funding payment    | ±X          | 0           | 0        |

(F = 1 + fee_rate for buys; fee taken from quote for buys, from proceeds for sells.)

Every event emits a ledger transaction (Phase 3 Lesson 7 discipline) that sums to zero. The risk engine's view is updated synchronously with the ledger.

---

### The Withdrawal Race Interview Question

Classic interview question: "User has 1000 USDT available. They send two simultaneous requests: (a) withdraw 800 USDT, (b) place a limit buy for 1 ETH at 300 USDT. Walk me through what should happen."

**The correct answer:**

Both requests arrive at the gateway. Idempotency check passes (different keys). Both arrive at the risk engine.

**Because risk engine is single-threaded per user (sharded on user_id), these are serialized.** One is processed first:
- If (a) wins: available drops to 200. Then (b) checks: needs 300, has 200 → REJECT. Correct.
- If (b) wins: available drops to 700, locked +300. Then (a) checks: needs 800, has 700 → REJECT. Correct.

**Either ordering is fine. The key is: they don't both succeed.** If you described parallel risk checks without explaining the sharding, you failed the question.

The interviewer may then push: "What if the user has two accounts that are actually the same person (multiple API keys)? What if they use multiple sub-accounts on a futures exchange?" This gets into account architecture — typically a tree of sub-accounts under a master account, with risk enforced at both levels. Senior-level question.

---

### Module 2 Summary: The Rules

1. **Risk in RAM, not in DB.** DB is too slow for the hot path. Keep risk in memory, derive from event log.
2. **Sharded by user_id, single-threaded per shard.** No races within a user's state.
3. **Lock `price × qty × (1 + fee_rate)` for limit orders.** Account for fees.
4. **Event-sourced balance updates.** Every state change is a zero-sum ledger transaction. In-memory and DB eventually consistent.
5. **Cross vs isolated margin trade-off: capital efficiency vs risk isolation.** Cross is harder to implement.
6. **Race conditions eliminated by sharding, not by locks.** The single-writer principle (Phase 1 Lesson 1) is still the answer.

### Interview Questions to Expect

- "Why can't you just use Postgres for balance checks? Show me the numbers."
- "Walk me through locking for a limit buy order. What about market orders? Futures?"
- "A user has $1000. Two parallel requests want to spend $800 each. What happens?"
- "What's the difference between cross-margin and isolated margin from the risk engine's perspective?"
- "If you shard risk engines by user_id, what happens when one shard is down?"
- "Walk me through what happens when the risk engine dies mid-reservation. How do you recover?"

### Drill: Module 2

1. Implement a single-goroutine risk engine in Go with `[1 << 20]UserBalance`. Benchmark: 5M Reserve ops/sec minimum on a single core.

2. Build a test that spawns 1000 goroutines, each submitting parallel reservation requests for a pool of 100 users. Route by `user_id % N` to one of N risk engine goroutines (one per shard). Verify no double-spend: after the test, `available + locked` equals initial balance for every user.

3. Out loud, explain the interaction between the risk engine and the ledger from Phase 3 Lesson 7. Specifically: when a user places an order, what happens in the risk engine's state, what ledger transaction is emitted, and what does it look like? 3 minutes.

---

# Module 3: The Matching Engine (The Core)

> The heart of the blackbox. Does not talk to a database. Lives entirely in RAM.
> Red-Black Trees or B-Trees for Price-Time Priority. Deterministic state machine.
> Sequence numbers. Event sourcing. Snapshots + journal. Instant recovery.

### The Blackbox Question

"Inside the matching engine: what's the data structure for the order book, and how does an incoming order actually match?"

This is the deepest drill in the interview. If you ace Module 3, the interviewer relaxes and assumes you know the rest too. If you stumble here, everything else is questioned.

---

### The Order Book Data Structure

Requirements (recap from Phase 1 Lesson 2):
1. Find best bid (highest buy price) in O(1) or O(log n).
2. Find best ask (lowest sell price) in O(1) or O(log n).
3. Iterate price levels in sorted order (descending for bids, ascending for asks).
4. Within a price level, match orders in FIFO order (price-time priority).
5. Insert / cancel in O(log n).
6. Deterministic (same operations → same output on every replay).

**What doesn't work:**
- **Hash map / Go map:** O(1) lookup but no ordering. Can't find best bid.
- **Sorted array:** O(log n) binary search, but insert is O(n) due to shifting. Dies on deep books.
- **Skip list:** O(log n) for everything but extra memory overhead and less cache-friendly.
- **Heap:** O(log n) insert and O(1) peek-at-top but can't iterate in order.

**What works: self-balancing binary search tree.**

---

### Red-Black Tree vs B-Tree

Both are self-balancing BSTs. Both give O(log n) for all operations. The difference is in cache behavior.

**Red-Black Tree:**
- Each node holds one key (price) and pointers to left child, right child, parent, color.
- Every node is a separate heap allocation.
- Traversal follows pointers → pointer chasing → cache misses.
- Memory per node: ~40-50 bytes with pointers + key + color bit.

**B-Tree:**
- Each node holds multiple keys (typically 16-64) in a contiguous array.
- Fewer nodes total. Fewer pointer chases.
- Cache-friendly: one node fits in a few cache lines, loaded with one memory access.
- Memory per node: ~256-1024 bytes; many keys per node.

**For order books specifically:**

A typical active crypto symbol's book has 1,000-10,000 distinct price levels. Red-black tree depth: ~13-17. B-tree depth (order 32): ~3. In a hot matching loop, the B-tree does 3 memory accesses (~40ns in L2 cache) vs the RBT's 13+ pointer chases (each a potential cache miss, potentially hundreds of ns).

**B-Tree wins on cache behavior by ~3-5x in practice.** LMAX and most serious exchanges use B-tree variants for price levels.

**Go ecosystem:**
- `github.com/google/btree` — solid, mature, widely used. The default.
- `github.com/emirpasic/gods` — has both RBT and B-tree. Good for learning, comparing.
- Custom B-tree optimized for int64 keys — what you'd write for production performance.

---

### Price-Time Priority at the Price Level

The tree is keyed by price. Each node (price level) needs to preserve order arrival for matching.

**The right structure: doubly-linked list or ring buffer of order references.**

```go
type OrderRef uint32  // index into order pool (Phase 1 Lesson 3)

type PriceLevel struct {
    Price     int64
    TotalQty  int64       // sum of qty across all orders at this level
    Head      OrderRef    // FIFO: oldest order
    Tail      OrderRef    // newest order
    // Each Order has Next/Prev OrderRef fields for the linked list
}
```

When an incoming order arrives at this level:
- Matching: start from `Head` (oldest). Fill against it. If consumed, advance `Head`. Repeat.
- Adding: append to `Tail`. Update `Tail.Next`, then update `Tail` pointer.
- Canceling: O(1) given the order's position (just unlink).

**Why linked list not slice:**
- Slice removal from the middle is O(n). At depth-of-market scale, this is murder.
- Linked list: O(1) unlink if you have the node pointer. O(n) to find a node by order ID — but we maintain a separate order ID → OrderRef map for that.

**Memory layout for speed:**
- Orders live in a pre-allocated flat array (`[1<<20]Order`). Phase 1 Lesson 3 pool.
- `OrderRef` is a 4-byte index into that array.
- The linked list pointers are index fields in the `Order` struct, not real pointers.
- This means the GC doesn't scan pointers (integers aren't pointers). Zero GC pressure on the book.

---

### The Full Book Structure

```go
type OrderBook struct {
    Bids  *btree.BTree    // keyed by -Price so "best" is min
    Asks  *btree.BTree    // keyed by Price, min is best
    Pool  *OrderPool      // flat array of all orders
    OrderIndex map[uint64]OrderRef  // order ID → ref, for O(1) cancel
}
```

Bids keyed by negated price (so min in B-tree = highest bid). Asks keyed by actual price (min = lowest ask). Both accessed via `btree.Min()` in O(1) (or near).

---

### The Matching Algorithm

```go
// Simplified: incoming limit buy order
func (b *OrderBook) MatchBuy(incoming *Order) []Trade {
    var trades []Trade
    for incoming.RemainingQty > 0 {
        bestAsk := b.Asks.Min()
        if bestAsk == nil || bestAsk.Price > incoming.Price {
            break  // no more matches
        }
        // Match against best ask's FIFO queue
        head := bestAsk.Head
        headOrder := b.Pool.Get(head)
        
        matchedQty := min(incoming.RemainingQty, headOrder.RemainingQty)
        trades = append(trades, Trade{
            Price: bestAsk.Price,  // maker price wins
            Qty:   matchedQty,
            MakerOrderID: headOrder.ID,
            TakerOrderID: incoming.ID,
        })
        
        incoming.RemainingQty -= matchedQty
        headOrder.RemainingQty -= matchedQty
        
        if headOrder.RemainingQty == 0 {
            // Fully filled; remove from level
            b.removeOrder(headOrder)
            if bestAsk.Head == 0 {  // level empty
                b.Asks.Delete(bestAsk)
            }
        }
    }
    
    // If any qty remains, add to book as resting order
    if incoming.RemainingQty > 0 {
        b.addOrder(incoming)  // inserts into bid side tree
    }
    return trades
}
```

Walk up the asks tree from the best, consume orders in FIFO, generate trades. Crucial details:

- **Trade price = maker's price (resting order).** The taker accepts the maker's price. This is universal.
- **Price-time priority:** oldest order at the best price fills first.
- **Pro-rata alternative:** some markets (CME for certain products) split fills across all orders at the best price, proportional to size. Rare in crypto. If asked, say: "Pro-rata is used in some traditional markets but crypto is nearly universally price-time priority."

---

### Deterministic State Machine

**The contract:** `match(state, event) → (state', outputs)` where state is the order book + any auxiliary structures, event is a single input (place, cancel, modify), state' is the next state, outputs are trades/acks emitted.

**Critical:** this function is pure. Given the same state and event, the same outputs always. No reading the clock, no random, no map iteration, no floating-point arithmetic, no concurrent goroutines.

**Why it matters (recap from Phase 1 Lesson 2):**
- **Replication:** two machines running the same function over the same event stream end up in identical states. Hot-standby failover becomes trivial.
- **Replay:** disaster? replay the journal from day zero and you get the same state byte-for-byte.
- **Debugging:** a production bug from 6 months ago? replay the events up to that point, observe the exact state, reproduce deterministically.

**The non-determinism pitfalls to name in an interview:**
1. `time.Now()` — use event timestamps only.
2. Map iteration — use B-tree or sorted slice.
3. Goroutine scheduling — single goroutine.
4. `float64` — use fixed-point int64.
5. `math/rand` without fixed seed.
6. Pointer addresses leaking into logic.

---

### Sequence Numbers

**Every event in the system has a sequence number.** This is non-negotiable for a serious exchange.

**Three distinct sequence number scopes:**

1. **Client order ID** (user-generated). Part of idempotency. Unique per user.
2. **Gateway event sequence** (gateway-generated). Monotonic per gateway instance. Used for debugging specific gateway flows.
3. **Global engine sequence** (sequencer-generated). The authoritative order of events in the system. The matching engine and all consumers derive state from this.

The global sequence is the most important. It's assigned by the sequencer (sits in front of the matching engine) and is the sole source of ordering truth.

**What the sequence is used for:**

- **Replay:** "restart from sequence N" tells you exactly where to resume.
- **Fan-out ordering:** downstream consumers process events in sequence order. Gap detection.
- **Client WebSocket recovery:** client reconnects with "last seen sequence = 12345"; gateway sends everything from 12346 onwards.
- **Snapshot metadata:** "this snapshot reflects state up through sequence 7,829,412."

Every trade, every ack, every book update carries its originating event's sequence number. If a client receives updates with seq 100, 101, 103 (no 102), they know to resync.

---

### Event Sourcing: Snapshots + Journal

Already covered in Phase 2 Lessons 4 and 6. Interview-compressed:

**The journal (WAL):** append-only file, every event durably logged. The authoritative history.

**Snapshots:** periodic full-state dumps. "At sequence N, the book looked like this."

**Recovery algorithm:**
1. Find latest snapshot (say, at seq N = 10,000,000).
2. Load snapshot into memory.
3. Replay journal from seq N+1 to end.
4. Resume accepting new events at seq (last) + 1.

**Recovery time:**
- Load snapshot: 10s of seconds to minutes depending on size.
- Replay tail: depends on how often you snapshot. If every 5 minutes, worst case replay is 5 minutes of events.
- Total: O(minutes), not O(hours).

**Without snapshots:** replay entire journal from day zero. Hours to days. Unacceptable.

**Snapshot challenge:** serializing the order book while continuing to accept orders = would pause trading = unacceptable. Solutions:
- Fork + Copy-on-Write (Redis-style, hard in Go — Phase 2 Lesson 6).
- Double-buffered state with pointer swap.
- `hashicorp/raft`'s snapshot interface with brief "stop the world" (acceptable if state is small).

Interviewers love asking about this. Know the options and the trade-offs.

---

### Matching Engine Throughput Numbers to Memorize

For interview credibility, know the magnitude:

- **LMAX original (Java, 2010, Disruptor pattern):** 6M TPS on a single thread.
- **Coinbase (historical disclosed):** ~10,000 TPS per trading pair.
- **Binance (estimated, not officially disclosed):** 1M+ TPS aggregate across all pairs.
- **Deribit (Rust, recent):** ~1M TPS per pair per engine.
- **HFT specialized engines:** 100M+ TPS achievable in C++.

A toy Go matching engine built per Phase 1 should do 1-5M TPS on a laptop. That's your baseline.

---

### Module 3 Summary: The Rules

1. **Order book = two B-trees (bids, asks) + FIFO linked list per price level.**
2. **Price-time priority matching. Trade price = maker's price.**
3. **Orders stored as indices into a pre-allocated flat array. GC-free.**
4. **Pure function: `match(state, event) → (state', outputs)`. No time, no maps, no floats.**
5. **Sequence numbers at three scopes: client, gateway, global. Global is the truth.**
6. **WAL journal + periodic snapshots. Recovery = load snapshot + replay tail.**
7. **Single goroutine. Hot path is allocation-free. Microsecond latency.**

### Interview Questions to Expect

- "What data structure is the order book? Why not a hash map? Why not a sorted array?"
- "Red-black tree or B-tree for the price levels — which and why?"
- "Walk me through matching a market buy order against the book. What are the data access patterns?"
- "How do you preserve time priority within a price level? What data structure for the FIFO?"
- "If the matching engine crashes at sequence 5,000,000, how does it recover? How long does it take?"
- "How often do you snapshot? What's the trade-off?"
- "Why is it critical that the matching engine be deterministic? What breaks if it's not?"
- "You told me it's single-threaded. How do you scale to 1M TPS aggregate? (Answer: one engine per trading pair.)"

### Drill: Module 3

1. Implement a B-tree-backed order book in Go using `github.com/google/btree`. Support: place limit order, cancel, match market buy/sell, match limit buy/sell. Preserve price-time priority with a FIFO per level. Orders as indices into a pre-allocated pool (Phase 1 Lesson 3).

2. Write a determinism test: feed 10,000 random events, record all emitted trades. Restart from scratch, replay the same events, verify byte-identical trades. Break it on purpose (inject `time.Now()`, map iteration, a second goroutine) and verify the test catches it.

3. Benchmark: how many orders/sec can your engine process? Target: 1M+ on a laptop. If less, profile and fix. Compare p50 and p99 latency.

4. Out loud, whiteboard the full matching flow from "order arrives at risk engine" through "trade emitted downstream." 10 minutes. Pause and record yourself. Play it back. Are there gaps?

---

# Module 4: Post-Trade Settlement & Clearing

> Once a trade matches, the result must be broadcast. Kafka or ring buffers here.
> Downstream consumers update the DB, calculate fees, push WebSocket updates.
> Master the Transactional Outbox pattern.

### The Blackbox Question

"The matching engine emits a trade event. What happens next?"

This is where the beautiful tight-loop matching engine meets the messy real world. State must propagate to: DB, fee accounting, notifications, WebSocket fan-out, analytics, compliance, tax reporting. All of this asynchronously, reliably, without slowing down the engine.

---

### The Post-Trade Flow

```
[Matching Engine]
    │
    ↓ emits TradeEvent to Kafka (or ring buffer)
    │
    ├────────┬────────┬────────┬─────────┬──────────┐
    ↓        ↓        ↓        ↓         ↓          ↓
[Balance  [Fee    [Trade   [Position [WebSocket  [Analytics
 Updater] Calc]   History] Tracker]  Broadcaster] / Tax]
    │        │        │        │         │          │
    ↓        ↓        ↓        ↓         ↓          ↓
[Postgres][Postgres][ClickHouse][Redis/  [Clients] [Data
                              Postgres]           warehouse]
```

Each consumer has an independent job. All read the same event stream. Kafka partitioning + consumer groups handle parallelism (System Design 101 Lesson 8 material).

**Why this fan-out pattern:**
- **Separation of concerns.** The matching engine doesn't know about WebSocket clients or tax reporting. It emits one event.
- **Independent scaling.** Balance updater can be single-instance (serial per user) while WebSocket broadcaster is 10 gateway instances.
- **Failure isolation.** If the analytics consumer is broken, trading is unaffected. Events accumulate in Kafka until it's fixed.
- **Replay-ability.** New consumer added 6 months later can replay from an earlier offset and build its view.

---

### Matching Engine → Downstream: Kafka vs Ring Buffer

Two choices for how the matching engine hands off trade events.

**Ring buffer (Phase 1 Lesson 1 material):**
- In-process shared memory. Sub-microsecond latency.
- Single consumer (or a few) can read directly.
- Non-durable unless paired with WAL.

**Kafka:**
- Out-of-process, durable, horizontally scalable consumers.
- Latency: millisecond or so.
- Network hop + broker ack.

**Production pattern: both.**

```
[Matching Engine]
    │
    ↓ ring buffer (sub-μs)
[Trade Publisher]
    │
    ↓ Kafka produce (async batch, durable)
[Kafka topic: trades]
    │
    ↓ fan-out to consumers
[Downstream]
```

The ring buffer handoff from engine to publisher is sub-microsecond — keeps the engine's hot path fast. The publisher batches and writes to Kafka with `acks=all` for durability. Downstream consumers read from Kafka.

This way, the engine never blocks on network I/O. If Kafka is slow, the ring buffer fills until the publisher catches up. If the ring buffer fills entirely (publisher disaster), the engine has to make a choice: block new orders, or start dropping events (unacceptable). Typically: sound an alarm, reject new orders, do not lose events.

---

### Maker-Taker Fees (Specifics)

**The basic model:**

When two orders match:
- **Taker:** the incoming order that caused the match (consumed liquidity from the book).
- **Maker:** the resting order that was sitting in the book (provided liquidity).

The exchange charges different fees:
- **Taker fee:** 0.05-0.15% of trade value (typical). The user pays.
- **Maker fee:** lower (0.01-0.05%), sometimes zero or negative (rebate paid TO the user).

Why:
- Makers provide liquidity. Deep books are valuable for traders. Reward them.
- Takers consume liquidity. Charge them for the benefit.

**Fee tiers** (real example based on 30-day volume):

| 30-day Volume (USD) | Maker  | Taker  |
|---------------------|--------|--------|
| < $100K             | 0.10%  | 0.10%  |
| $100K - $1M         | 0.08%  | 0.10%  |
| $1M - $10M          | 0.05%  | 0.08%  |
| $10M - $100M        | 0.02%  | 0.05%  |
| > $100M             | 0.00%  | 0.03%  |
| "VIP 9"             | -0.005% (rebate) | 0.02% |

Negative maker fees = exchange pays the maker. Why? Because attracting high-volume market makers makes the book deep → attracts takers → takers pay taker fees → exchange profits overall.

**Calculation in the fee service:**

```go
type FeeCalculator struct {
    tiers map[uint64]FeeTier  // user_id → current tier, refreshed daily
}

func (f *FeeCalculator) Calculate(trade Trade) (makerFee, takerFee int64) {
    makerTier := f.tiers[trade.MakerUserID]
    takerTier := f.tiers[trade.TakerUserID]
    
    notional := trade.Price * trade.Qty / priceScale
    
    makerFee = notional * makerTier.MakerRate / rateScale  // may be negative
    takerFee = notional * takerTier.TakerRate / rateScale
    return
}
```

Then emit ledger transactions (Phase 3 Lesson 7 material):

```
Taker fee:
  +takerFee  User.Taker.USDT.Available
  -takerFee  Equity.Fees.USDT
  Sum: 0 ✓

Maker fee (if positive):
  +makerFee  User.Maker.USDT.Available
  -makerFee  Equity.Fees.USDT

Maker rebate (if negative):
  -|rebate|  User.Maker.USDT.Available  (user receives)
  +|rebate|  Equity.Fees.USDT            (exchange pays)
```

Same double-entry discipline. Sums to zero.

---

### Trade Broadcast to WebSocket (Fast Path)

Traders want to see their fills immediately. Path:

```
Matching Engine → TradeEvent → Kafka
                              → WebSocket Broadcaster consumer
                              → looks up all WS clients subscribed to this symbol's trade feed
                              → serializes trade once
                              → pushes to each client via per-client queue
```

**Latency budget:**
- Engine → Kafka: ~1ms.
- Kafka → broadcaster: ~1ms.
- Broadcaster fan-out: ~10ms for 100K clients (Phase 4 Lesson 10 material).
- Total: ~15ms end-to-end. Acceptable for most traders.

**For HFT-grade latency (~sub-ms):**
- Replace Kafka with UDP multicast or Aeron.
- Co-locate the broadcaster with the engine.
- Bypass the broadcast → private direct push to VIP clients via their own dedicated connections.

Not every user needs sub-ms. Tier your service levels.

---

### Balance Updates: Where Transactional Outbox Comes In

When a trade is filled, balances change for both counterparties. This is a ledger operation (Phase 3 Lesson 7). But the ledger must also emit events (to notify fee service, analytics, etc.).

**The dual-write problem (from the outbox supplement):**

```go
// Broken:
func (s *Settlement) OnTrade(t Trade) error {
    // 1. Update balance in DB
    _, err := db.Exec("UPDATE balances SET ... WHERE user_id = $1", t.TakerUserID)
    if err != nil {
        return err
    }
    // 2. Emit event for downstream
    err = kafka.Produce("balance_updates", buildEvent(t))
    if err != nil {
        // Balance updated but event not published. Analytics is now out of sync. Some systems never know.
        return err
    }
    return nil
}
```

Classic dual-write. If step 2 fails, balance is updated but no event emitted. Downstream systems never know. Fee service doesn't charge. Analytics has wrong volume. Hours later, support ticket.

**The outbox fix:**

```go
func (s *Settlement) OnTrade(t Trade) error {
    tx, _ := db.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    // 1. Update balance in transaction
    tx.Exec("UPDATE balances SET available = available - $1 WHERE user_id = $2", t.Amount, t.TakerUserID)
    tx.Exec("UPDATE balances SET available = available + $1 WHERE user_id = $2", t.Amount, t.MakerUserID)
    
    // 2. Write event to outbox table (same transaction)
    tx.Exec("INSERT INTO outbox (aggregate, event_type, payload) VALUES ($1, $2, $3)",
        fmt.Sprintf("trade:%d", t.ID), "trade_settled", payload)
    
    return tx.Commit()
}

// Separate publisher goroutine:
func (p *OutboxPublisher) Run(ctx context.Context) {
    for {
        rows := queryOutbox("WHERE published = false LIMIT 100 FOR UPDATE SKIP LOCKED")
        for _, row := range rows {
            kafka.Produce(row.Payload)
        }
        markPublished(rows)
        time.Sleep(100 * time.Millisecond)
    }
}
```

Balance change and event are atomic. Publisher is at-least-once. Consumers are idempotent (on event ID). No dual-write.

**Interview essential:** be able to draw the outbox pattern end-to-end, explain why naive dual-write fails, explain the at-least-once + consumer idempotency convergence.

---

### Settlement vs Clearing (Know the Terms)

Terminology interviewers may use:

- **Matching:** pairing a buy order with a sell order to produce a trade.
- **Clearing:** all post-trade processing to assign rights/obligations (who owes what to whom). For a CEX, this is the balance updates, fee settlement, position updates.
- **Settlement:** final transfer of assets (the crypto actually moves to the user's internal balance, or on-chain for withdrawals).

In traditional finance (T+2 equities), matching, clearing, and settlement happen at different times (trade today, settle 2 days later). In crypto CEX, these are effectively simultaneous — balances update within seconds of the match.

For perpetual futures: there's also **funding**, periodic payments between longs and shorts to keep the perpetual's price close to the underlying index. Usually every 8 hours. Settlement service handles it.

---

### Module 4 Summary: The Rules

1. **Engine → ring buffer → publisher → Kafka → consumers.** Keep the engine hot path synchronous-free.
2. **Fan-out to independent consumers:** balance, fees, trade history, position, WebSocket, analytics.
3. **Maker-taker fees.** Rebates for makers at high tiers. Understand the incentive design.
4. **Transactional Outbox pattern** between Postgres-backed services and Kafka. Non-negotiable.
5. **Idempotent consumers on event ID.** At-least-once delivery converges correctly.
6. **Sequence number preserved end-to-end.** A trade's seq tells you exactly where it was in the engine's event stream.
7. **Match / clear / settle — know the vocabulary.** Interviewers test.

### Interview Questions to Expect

- "The matching engine emits a trade. Walk me through everything that happens next."
- "How do you update the user's balance in Postgres without slowing down the engine?"
- "What's the transactional outbox pattern? Why do you need it here?"
- "Explain maker-taker fees. Why would an exchange pay a negative fee to a maker?"
- "How do you ensure a trade notification reaches the user's WebSocket reliably?"
- "The balance updater crashes after processing a trade but before publishing the next event. What happens?"
- "At 1M trades/sec, how does each downstream consumer keep up? What if one is slow?"

### Drill: Module 4

1. Build the post-trade pipeline in Go: matching engine emits TradeEvents into a ring buffer, a publisher goroutine batches to Kafka, three consumers (balance updater, fee calculator, WebSocket broadcaster) each consume and do their job. Use the outbox pattern for the balance updater.

2. Inject failures: kill the publisher mid-batch, kill a consumer, make Kafka unreachable for 30 seconds. Verify the system recovers with no lost events and no duplicates applied (consumer idempotency working).

3. Implement tier-based fee calculation. Run a backtest: simulate 1M trades across 1,000 users at various tiers. Verify ledger invariant `Assets = Liabilities + Equity` holds after every trade.

4. Out loud, answer: "The engine just matched a trade. What happens before the user sees 'Filled' in their app?" Target: 3-5 minutes, end-to-end with no gaps.

---

# Putting the Blackbox Back Together: The 45-Minute Whiteboard

Here's the full flow, compressed. Practice saying this out loud. It should take 5-7 minutes.

1. **Client** submits order (limit buy 1 BTC @ 50,000 USDT) with idempotency key via HTTPS to API gateway.

2. **Cloudflare** drops if DDoS-flagged. **Load balancer** terminates TLS and routes to a gateway instance.

3. **Gateway** authenticates (JWT), checks idempotency (Redis SET NX + pending placeholder), rate-limits (token bucket with variable weight). Passes to sequencer.

4. **Sequencer** assigns global sequence number, writes event to WAL with fsync (group-commit batched), forwards to matching engine.

5. **Risk engine** (sharded by user_id, single-goroutine per shard): in-memory check of user balance, reserve `50,000 × 1.001` (price × qty × fee), emit ledger reservation transaction. Pass to matching engine.

6. **Matching engine** (single goroutine, pure function): walks B-tree of asks, finds any at price ≤ 50,000. Matches price-time priority against FIFO queue at each level. Produces trades. Any unfilled quantity is inserted into the bids B-tree. State change is deterministic.

7. **Engine emits** TradeEvents into ring buffer. Publisher picks up, writes to Kafka with outbox semantics (if going through a relational stage) or direct if going straight to Kafka.

8. **Downstream consumers** (parallel, independent):
   - Balance updater: updates Postgres balances. Uses outbox to emit confirmation events.
   - Fee calculator: computes maker/taker fees per tier, emits ledger transactions.
   - Trade history: writes to ClickHouse for audit/query.
   - WebSocket broadcaster: pushes to all clients subscribed to `trades.BTC-USDT` and `user.123.fills`.
   - Position tracker: updates risk engine's view of user's position for margin/liquidation math.

9. **Client receives** "Filled" notification via WebSocket, near-real-time (~15ms from match).

10. **Persistence and recovery:** the engine's WAL (journaling every event) plus periodic snapshots (Phase 2 Lesson 6) means recovery from any crash is minutes, not hours.

11. **Replication:** a Raft cluster (Phase 2 Lesson 5) runs the same engine on multiple machines. Failover of the primary is sub-second, zero data loss.

**End-to-end latency budget for a typical order:** ~5-20ms from client submit to client "filled" receipt. Matching itself: ~5-50 microseconds.

---

## Final Interview Tips

**1. Name your numbers.** "Postgres can do about 5,000 writes/sec, the engine needs to do 1 million, so we can't put the hot path on Postgres." Specific numbers, not "Postgres is too slow."

**2. Name the trade-off for every decision.** "I'd use a B-tree rather than a red-black tree because the cache behavior is 3-5x better at this depth. But B-tree insertion is slightly more complex code, and at smaller books the difference is invisible."

**3. Know the failure mode for every component.** "If Kafka is down: events pile up in the ring buffer. If the ring buffer fills: the engine has to choose between blocking or dropping. Blocking is correct — sound an alarm, reject new orders, never lose data."

**4. Use the vocabulary.** Price-time priority, maker-taker, notional, mark price, initial margin, funding rate, fund reservation, single-writer principle, event sourcing, determinism, idempotency, outbox, Raft quorum, exactly-once-effective. Sprinkle these naturally, not forcefully.

**5. Be honest about complexity.** "A correct implementation of Raft is about 2,000 lines of careful Go. I'd use `hashicorp/raft` rather than rolling my own. Here's what I'd customize..."

**6. Show you've thought about adversaries.** "A malicious user will try: double-submission (idempotency catches it), sustained order placement (rate limit catches it), quote stuffing (behavioral detection catches it), and an attack on WebSocket connection quota (per-IP connection limit catches it). Defense in depth."

**7. Close with what you'd monitor.** "I'd track: engine latency p99, Kafka consumer lag per consumer, reservation vs fill ratio (fat-finger detection), hot wallet balance vs daily withdrawal volume, DLQ depth. Page on anomalies."

---

*End of CEX Blackbox interview prep. Combined with Phases 1-4 and the Outbox supplement, you now have both the deep technical foundation AND the interview-ready framing. The rest is practice — mock interviews, out-loud explanations, whiteboard sessions. Good luck.*
