# System Design Course — Deep Recap with Full Drill Q&A
> Lessons 1–9 | Every concept, every correction, every rule

---

## Lesson 1: The Only Reason to Scale

### Core Philosophy
Scale to solve a specific bottleneck. Not because it is trendy. Not because Netflix does it. If you cannot name the specific problem you are solving by scaling, you are guessing.

### The Three Bottlenecks

**Compute (CPU)**
The machine's ability to execute instructions. Processing logic, running queries, encrypting data, serializing JSON.

What consumes CPU:
- Query execution (joins, aggregations, sorting)
- Business logic in application server
- Compression and decompression
- Encryption (TLS handshakes, hashing)
- Image and video processing
- JSON serialization

How you detect it:
- CPU utilization sustained above 70%
- Latency grows under concurrency even with small data size
- Adding more requests makes everything slower

Example: API doing GROUP BY aggregation across 10M rows. Data fits in memory. Disk is idle. CPU pegged at 90%. That is a compute bottleneck — the machine is burning cycles calculating, not fetching.

**Disk I/O**
Data moving between disk and memory inside a single machine.

What consumes disk I/O:
- Database fetching rows from storage
- Full table scans (no index — reads entire disk)
- High write throughput (logs, bulk inserts)

How you detect it:
- CPU is low but latency is high — machine is waiting, not working
- High disk wait time (iowait in Linux)
- Query slow but CPU is fine — seq scan

Example: Query a table with no index on filter column. Postgres reads every row off disk to find 10 matching ones. CPU is 5%. Latency is 2 seconds. That is disk I/O — paying the cost of fetching, not processing.

Fix: add an index. Postgres jumps directly to the rows. Disk reads drop 99%. Latency drops to 10ms.

**Network**
Data moving between machines over a wire. Two sub-problems:

- Latency: how long does one trip take? Caused by geographic distance, routing hops. Fix: CDN, co-locate services.
- Bandwidth: how much data per second? Caused by large payloads, high volume. Fix: compression, pagination, sparse fieldsets.

How you detect it:
- CPU and disk are both fine but latency is high
- Bandwidth utilization near ceiling
- Latency varies by user geography
- One slow downstream call makes everything slow

Example: API returns 200-field JSON payload but client uses 5 fields. At 10,000 req/sec that is 400MB/s of egress instead of 20MB/s. Network bottleneck caused by bad API design.

**Key distinction — Disk I/O vs Network:**
- Disk I/O: data moving inside one machine (disk to memory)
- Network: data moving between machines (wire)
- When engineers say "I/O bottleneck" they almost always mean disk
- When they say "network bottleneck" they mean wire between machines

**Bottlenecks cascade.** Fix one and another surfaces. Add index → CPU becomes bottleneck because queries run faster and you process more. Add cache → now cache layer becomes network bottleneck. Add app servers → single DB becomes I/O bottleneck.

### The Capacity Equation
Before any scaling decision, answer three questions:
1. What is my current capacity?
2. What is my actual or projected load?
3. What breaks first, and when?

Only after answering these do you decide how to scale.

### Vertical vs Horizontal Scaling

**Vertical scaling** — make the machine bigger (more CPU, RAM, faster disk)
- Fast, cheap, zero code changes
- Hard ceiling, single point of failure
- **Use this first. Always.**

**Horizontal scaling** — add more machines, distribute load
- Complex, requires stateless design or shared state
- Theoretically unbounded
- Use when vertical hits ceiling or fault tolerance is needed

Most engineers jump to horizontal too early. That is premature complexity.

### Diagnostic Cheat Sheet
```
Where is the system waiting?

Inside one machine:
  CPU busy?             → Compute bottleneck
  CPU idle, disk busy?  → Disk I/O bottleneck

Between machines:
  Data volume too large? → Network bandwidth
  Distance too far?      → Network latency
  Too many round trips?  → Network chattiness (design problem)
```

---

### Drill 1

**Scenario:** REST API (Node.js) backed by single PostgreSQL on 4-core/16GB machine. Serving 500 req/sec. DB CPU at 40% average, spiking to 80% at peak. Expect 3x growth in 6 months.

**Q1: What is the bottleneck?**

Your answer: I/O
Correct answer: **Compute (CPU)**

Correction: I/O bottlenecks show up as CPU low but latency high, high disk wait time, slow queries despite low CPU. CPU bottleneck shows up as CPU pegged under load, query processing slowing under concurrency. The symptom here is "DB CPU at 40% average, spiking to 80%" — that is CPU, not I/O. Always name the specific metric misbehaving.

**Q2: What breaks first and when?**

Your answer: DB CPU, when 20% growth
Correct answer: **Breaks at ~2x load, roughly 3–4 months**

Correction: "When 20% growth" tells nothing. Show the math:
- Today: 500 req/sec, DB CPU 40% average / 80% peak
- CPU scales roughly linearly with load
- At 3x load: 40% → ~120% average. Already dead.
- Peak hits ceiling at ~2x load
- At 20% MoM growth: 2x is roughly 3–4 months

Always project mathematically. Do not feel it — calculate it.

**Q3: Vertical or horizontal first?**

Your answer: Vertical, still a lot of room to scale
Correct answer: **Vertical first — correct. But reasoning needs to be sharper.**

Full correct answer: Upgrade to 8-core/32GB. Zero code changes. Buys runway. Then monitor whether CPU or something else becomes the next ceiling after scaling.

Key lesson added: After every scaling action, reset your baseline and ask the three capacity questions again. Scaling is iterative, not a one-time fix.

**Rule: Every scaling decision buys you runway. Your job is to know how much runway, and what breaks next.**

---

## Lesson 2: When Vertical Stops Working

### The Decision Tree
```
Is load read-heavy or write-heavy?
├── Read-heavy → Try cache first
│     └── Cache miss rate still high? → Read replicas
└── Write-heavy → Sharding (last resort, expensive)
```

**Cache vs Read Replicas distinction:**
- Read replica still hits disk — distributes load but does not eliminate work
- Cache serves from memory — 100x faster than SSD, 1000x faster than spinning disk
- Even 50% cache hit rate cuts disk I/O in half instantly
- **Cache first because it eliminates work. Replicas only distribute work.**

### Cache Value Formula
```
Cache value = Access frequency × Cost to recompute
Hidden third variable: Cost of serving stale data
```

High frequency + high cost to recompute = cache this first
Low frequency + low cost = do not bother

### The Three Cache Questions
Before caching anything ask:
1. How often is it read? (High = good candidate)
2. How expensive to recompute? (High = good candidate)
3. What is the cost of stale data? (High = cache carefully or not at all)

### Best Cache Candidates
**Reference / configuration data:**
- Product catalog, exchange rates, feature flags, country list
- Read millions of times, changes rarely, stale for 60 seconds is fine
- Perfect candidate: high frequency, high compute cost relative to value, low stale cost

**Expensive aggregations:**
- Top 10 trending, total orders this month, leaderboard
- Computed from millions of rows, used constantly, slightly stale acceptable
- Without cache: full aggregation on every request. With cache: one query per TTL window.

### Dangerous Cache: Financial Balances
User balance scores high on frequency but has **extremely high stale cost**. Serving stale balance can allow overdrafts — not a display bug but a financial integrity problem.

Options:
- Cache with strict invalidation — every transaction immediately invalidates or updates cache entry
- Very short TTL or zero — never serve balance from cache without write-through strategy
- Many banking systems do not cache balances at all and accept the DB cost for correctness

### Exchange Caching Patterns

**Order book — should you cache it?**
- Read: extremely high. Recompute cost: low (maintained in memory already). Stale cost: extremely high.
- Do not use traditional TTL cache. Order book lives in memory permanently, mutated on every event, pushed to clients via WebSocket.
- This is in-memory state + push architecture, not traditional caching.

**Past trades — should you cache it?**
- Read: high. Recompute cost: medium (DB query). Stale cost: low (completed trades never change).
- Perfect cache candidate. TTL 1-2 seconds. At 10,000 req/sec, even 1 second TTL serves 9,999 from memory.

```
Data velocity determines architecture:
  Slow changing    → Cache with TTL         (past trades, user profile)
  Fast changing    → In-memory + push       (order book, ticker price)
  Never changes    → Cache indefinitely     (trade history older than 1hr)
```

---

### Drill 2

**Scenario:** Vertically scaled twice, now on 16-core/64GB near cost ceiling. 90% reads, read latency P99 800ms, writes fine at P99 12ms. Cache hit rate 0% (no cache exists).

**Q1: What is the bottleneck now?**

Your answer: Not CPU, it is I/O now
Correct answer: **Disk I/O — correct**

Full explanation: Vertical scaled to near ceiling. Reads are slow. Writes are fine because they are sequential — append to disk. Reads are slow because they are random — jumping around disk to fetch rows. At high read concurrency, the disk is getting hammered with random access patterns. CPU idle + latency high = I/O. Specifically disk I/O because it is a single machine.

**Q2: Cache or read replicas first?**

Your answer: Cache first, mostly read and no cache exists
Correct answer: **Cache first — correct**

Full explanation: Cache eliminates work. Read replicas only distribute work. 0% cache hit rate means every read goes to disk. Even 50% hit rate cuts disk I/O in half instantly. Cache before replicas.

**Q3: What specific reads benefit most from caching?**

Your answer (after prompting for specifics): Video content and user balance

Corrections on each:

Video content: Right instinct, wrong mechanism. You do not cache video files in Redis — gigabytes vs kilobytes. What you described is a CDN (network latency solution). Correct cache story for video: cache video metadata in Redis (title, duration, thumbnail URL — read millions of times, never changes per upload). Video bytes go to CDN edge nodes.

User balance: Correctly identified as high frequency. But you missed the hidden third variable — stale cost is extremely high in banking. User sees $5,000, spends $4,000, real balance was $1,000 due to pending transaction. You allowed an overdraft. This is the cache invalidation problem — one of two hard problems in computer science.

**Correct best cache candidates:**
1. Reference/config data — high frequency, low stale cost, high compute cost relative to value
2. Expensive aggregations — computed from millions of rows, acceptable staleness, costly without cache

---

## Lesson 3: Horizontal Scaling

### Stateless vs Stateful — Precise Definition

**Stateless:** service holds no memory of previous requests. Any instance can handle any request. Add or remove instances freely.

**Stateful:** service remembers something between requests. Specific instance owns specific data or connection. Cannot freely route requests to any instance.

```
Stateless examples:           Stateful examples:
REST API with JWT auth         WebSocket server
Image resize worker            Order matching engine
Email sender                   In-memory order book
Report generator               Shopping cart in local memory
```

### Requirements for Truly Stateless API
- Sessions in Redis (not local memory)
- Files in S3 (not local disk)
- Cache in Redis (not in-process)
- Auth via JWT (not server-side session)

Violate any of these and you get sticky sessions — requests must go to same server. Load balancer now does routing logic. Scaling becomes harder.

### Scaling Stateful Services — Three Strategies

**1. Externalize state** → instances become stateless → scale freely
- Sessions in Redis, files in S3
- Instances themselves hold nothing

**2. Partition state** → each instance owns a slice → scale by adding partitions
- Matching engine per trading pair
- DB sharding by user ID

**3. Replicate state** → all instances have a copy → scale reads, not writes
- Read replicas, CDN

### WebSocket Fan-Out Problem
Multiple WS instances each hold their own connected clients. One instance cannot push to another's clients.

Solution: shared pub/sub layer (Redis or Kafka). Every WS instance subscribes to shared topic. Event published once. All instances receive it and push to their own clients.

**Why Kafka over Redis pub/sub for order book:**
- Redis pub/sub: fire and forget. Instance down during message = message lost.
- Kafka: persistent log. Instance reconnects, replays from last committed offset. No missed updates.
- For order book where missing an update means stale prices: Kafka is correct.

### Matching Engine Scaling
Cannot run two matching engines for same trading pair — race condition causes double fills.

Solution: partition by trading pair. Each engine is single-threaded, sequential. Scale horizontally by adding pairs, not parallelizing one pair. This is partitioned stateful scaling.

### Kafka-Based Horizontal Scaling
For Kafka consumers: no load balancer needed. Kafka is the load balancer.

```
Kafka Topic: notifications (6 partitions)
P0, P1 → Consumer Instance A
P2, P3 → Consumer Instance B
P4, P5 → Consumer Instance C
```

Add instance → Kafka rebalances partitions automatically.
Max useful consumers = number of partitions.

### Kafka Offset Management — Full Internals

**What Kafka is:** append-only log. Messages never deleted on consumption. Each consumer tracks its own position (offset).

**Where offsets are stored:** internal topic `__consumer_offsets`. A commit is just a write to this topic: `{group_id, topic, partition} → offset`.

**Crash detection:** heartbeat mechanism. Consumer sends heartbeat every 3 seconds. If broker receives no heartbeat within `session.timeout.ms` (default 45s) → consumer declared dead → rebalance → partitions reassigned.

**After rebalance:** new consumer asks "where should I start?" Kafka checks `__consumer_offsets` → resumes from last committed offset. Unprocessed message not lost.

**Auto-commit trap:**
```
Auto-commit (wrong):
  Read message
  [timer fires] → commit ← committed before processing
  Start processing
  CRASH → message lost

Manual commit (correct):
  Read message
  Process fully
  Commit ← only after success
```

Always use manual commit for critical systems.

**The duplicate problem with manual commit:**
```
Read message
Process successfully (balance updated)
CRASH before commit
→ Kafka redelivers
→ Balance updated twice
→ Financial error
```

This is why idempotency is needed alongside offset management.

### Idempotency Pattern

**Order of operations (critical):**
```
Read event
  → Check processed_events: event_id seen?
  → YES: skip, commit offset
  → NO: process, mark event as seen in DB (same transaction), commit offset
```

**Mark seen AFTER processing, BEFORE offset commit.**

If Redis SET fails during idempotency check:
- Option 1: Accept it (low stakes — notifications, analytics)
- Option 2: Retry SET (handles transient failures only)
- Option 3: Push idempotency to the receiver (Apple/Google push APIs support idempotency keys natively)
- Option 4: Transactional outbox pattern (financial systems — write intent to DB, separate worker executes, all in same DB transaction)

**Rule: Every time you write to two systems, you have a consistency gap. Match solution complexity to stakes.**

### Redis Atomicity
- Single Redis command (INCR, SET, GET): atomic. Redis is single-threaded.
- Multiple commands together: NOT atomic. Other clients can interleave.
- Fix: Lua script. Entire script executes as one indivisible operation.

```
Wrong (race condition):
  GET ratelimit → read 99
  [another instance reads 99 here]
  INCR → write 100
  [other instance writes 100 too — both allowed, limit broken]

Right (Lua script):
  Entire GET + check + INCR executes atomically
  No interleaving possible
```

---

### Drill 3

**Scenario:** Notification service. Node.js, receives events from Kafka, calls Apple/Google push APIs. 50,000 notifications/sec at peak. Users tolerate 2-3 second delay.

**Q1: Stateless or stateful?**

Your answer: Stateless, holds no memory
Correct answer: **Stateless — correct**

**Q2: Bottleneck at 50,000/sec?**

Your first answer: CPU because single Node.js
Corrected to: **Network I/O — correct after hint**

Explanation: The service receives event, calls Apple/Google API, **waits for response**, receives next event. During the wait, CPU is idle. This is I/O bound — waiting on external network call, not computing. Node.js handles this well with async/await — one thread can fire thousands of concurrent API calls without blocking. But single instance still has limits on concurrent connections.

This is the distinction between:
- CPU bound: computation is bottleneck → add more cores
- I/O bound: waiting is bottleneck → add more concurrency

**Q3: How to scale horizontally?**

Your first answer: Load balancer with instances A, B, C
Corrected to: **Kafka consumer group scaling**

Explanation: Events come from Kafka, not HTTP clients. Load balancers sit in front of HTTP servers. For Kafka: same consumer group, multiple instances, Kafka automatically assigns partitions.

```
Kafka Topic: notifications (6 partitions)
    │
┌───┼───┐
▼   ▼   ▼
[A][B][C]  ← same consumer group
    │
Apple/Google Push APIs
```

**Q4: What breaks in your design?**

Your answer: Instance crashes, notification missed. Better do retry.

Correct answer: **Not missed — duplicated. And "do a retry" is not an architecture.**

Full explanation:
- Kafka offset management guarantees at-least-once delivery — no permanent loss
- The real risk is duplicate notification when crash occurs after push but before offset commit
- Fix: Redis idempotency key per notification ID with TTL 24h
- Order: send push → SET Redis → commit Kafka offset
- If SET Redis fails: accept for notifications (low stakes), or use Apple/Google native idempotency keys

**Rule: At-least-once (Kafka) + idempotency (Redis) = every message processed, every notification sent exactly once.**

---

## Lesson 4: Breaking a Monolith

### The Unpopular Truth
Most teams break up their monolith too early, for the wrong reasons, and make their system worse. Microservices are a tradeoff, not an upgrade. You trade simplicity for independent scalability. That trade only makes sense under specific conditions.

### Three Valid Reasons to Split

**1. Scale mismatch**
Different components have wildly different load profiles. In a monolith, you scale everything together.

```
Reports module:    10 requests/day
Auth module:       10,000 req/sec
Order matching:    50,000 events/sec
```

To handle matching engine load, you scale the whole monolith including the reports module that gets 10 requests a day. Wasteful and expensive.

**Split when:** one component's resource requirements force you to over-provision everything else.

**2. Deployment coupling**
Team wants to deploy notification service. It shares codebase with payments. Deploy requires testing payments. Bug in notifications can take down payments.

**Split when:** deployment coupling is measurably slowing down your team.

**3. Technology mismatch**
Report generation needs Python. API is Node.js. Matching engine needs Go for performance.

**Split when:** component has genuine technology requirement conflicting with the rest.

### Costs of Every Split
- Function call (nanoseconds) → network call (milliseconds)
- Shared DB transaction → distributed transaction problem
- One log file → logs across N services requiring distributed tracing
- One stack trace → trace across services
- Separate deployment, monitoring, scaling, infrastructure per service

These costs are real and ongoing. Every service added multiplies them.

### The Distributed Transaction Problem
In monolith: trivial. Wrap in DB transaction. Either all succeed or all roll back.

Across services: no shared database. If step 2 of 3 succeeds and step 3 crashes, step 2 already committed. No way to roll back. Data inconsistency.

**Rule: Before splitting, identify every transaction boundary that crosses the split. If you cannot handle that transaction failing halfway, do not split there.**

### The Seam Principle
Split along natural seams where:
- Communication across boundary is already infrequent
- Data on each side is owned exclusively by one side
- Failure on one side does not require rollback on the other

```
Good seam:                          Bad seam:
Notifications from Orders           Orders from Payments
Orders tell notifications to send.  Share a transaction boundary.
Fire and forget.                    Splitting creates distributed
Notification failure does not       transaction problem.
affect order state.
```

### Saga Pattern

**The problem:** distributed transaction across multiple services. Need all-or-nothing guarantee without shared DB.

**Solution:** break into local transactions. Each step has a forward action and a compensating action that undoes it if later steps fail.

**Choreography Saga:**
Each service publishes events and listens for others. Decoupled. No central coordinator. Hard to debug — full flow is invisible. Hard to reason about complex failure paths.

```
User deposits into vault:
  Account Service: deducts balance → publishes BalanceDeducted
  Vault Service: consumes BalanceDeducted → mints shares → publishes SharesMinted
  If Vault Service fails:
    → publishes SharesMintFailed
    → Account Service consumes → refunds balance (compensating action)
```

**Orchestration Saga:**
Central coordinator tells each service what to do. Knows full state. Easier to debug. Single point of visibility. Better for complex financial flows.

```
Vault Saga Orchestrator:
  Step 1: tell Account Service → deduct balance
    fail → abort, nothing to compensate
  Step 2: tell Vault Service → increase TVL, mint shares
    fail → tell Account Service → refund balance
  Step 3: tell Notification Service → confirm deposit
    fail → retry only, no financial rollback needed
```

**Which to use for DeltaDeFi vault deposits:** Orchestration. User funds involved. Need single place that knows exactly where saga is at any point. Easier to audit and debug.

### Strangler Fig Pattern
Do not rewrite everything at once. Extract one service at a time. Validate in production. Then extract next. Never extract everything simultaneously.

### Decision Framework
```
1. Scale mismatch?              No → do not split
2. Deployment coupling slowing  No → do not split
   team measurably?
3. Technology mismatch?         No → do not split
4. Clean seam, no shared        No → do not split
   transactions?
All yes → split, strangler fig, one service at a time
```

### DeltaDeFi Service Map

**Matching Engine:** separate. Scale mismatch + tech mismatch (Go). Clean seam — orders in, fills out.

**Price Feed:** separate. Scale mismatch + Go. Subscribes to matching engine fills via Kafka. Clean seam — no shared transactions. Do not merge with matching engine — different scale axes.

**WebSocket Server:** separate. Scales by connected clients, not by trading pairs. Consumes Kafka events and pushes to clients.

**Account Service:** separate. Owns balances and positions exclusively. Nobody else writes to its database. Consumes fill events from Kafka and updates state atomically within one service.

**Trade History:** separate. ClickHouse. Append-only time series. Different technology requirement.

**Vault Service:** separate. Orchestrates deposit/withdraw saga. Owns TVL and share calculations. Most dangerous transaction boundary in DeltaDeFi — vault deposit touches user balance + vault TVL + share issuance across services.

**Auth Service:** separate. Low traffic, owns identity. Deployment independence.

**Notification Service:** stateless Kafka consumer. Clean seam — fire and forget from all other services.

---

### Drill 4

**Question:** For each DeltaDeFi component, should it be separate or monolith? What specific reason? Where are dangerous transaction boundaries?

**Your answers:**
- All separate: scale mismatch for matching engine vs auth/reporting, tech mismatch (Go vs TypeScript), need to decouple
- Dangerous boundary: orders filled but balance/history not updated

**What you missed:**

You grouped everything instead of reasoning per component. You rebuilt a monolith by putting matching engine + trade history + price feed together (related ≠ same service).

You correctly identified the dangerous transaction boundary but did not propose a solution.

**Dangerous transaction: fill event must atomically update balance AND trade history.**

Solution: do not update both in same operation. Account Service owns balance. Trade History Service owns fills. Both consume the same Kafka fill event independently. Each updates its own data store atomically. Kafka event is the bridge — no distributed transaction needed.

**Vault deposit boundary** (most dangerous): deposit touches user balance (Account Service), vault TVL (Vault Service), share issuance (Vault Service). Use orchestration saga. If balance deducted but shares not minted → saga compensates by refunding balance.

---

## Lesson 5: Databases

### The Four Questions Before Choosing
1. What is the shape of my data? (Rows? Documents? Graph? Time series?)
2. How do I access it? (By key? Range? Relationship? Full text?)
3. What are my consistency requirements? (Strong? Eventually consistent?)
4. What is my read/write ratio? (Read-heavy? Write-heavy?)

### Database Landscape

**Relational (PostgreSQL, MySQL)**
- Structured, tabular, relationships between entities
- Flexible querying — filter by any column, join, aggregate, sort
- ACID transactions, strong consistency
- Best for: financial data, complex relationships, when correctness matters more than speed
- The trap: abandoned too early. Well-tuned Postgres handles millions of rows and thousands of QPS.

**Document (MongoDB, DynamoDB)**
- Nested, variable structure per record
- Access primarily by document ID or small set of known fields
- Best for: content with variable structure, whole-document access, frequently changing schema
- The trap: using to avoid thinking about schema. Then you need to query across documents — no joins. Joins in application code are slower, harder to maintain, impossible to index properly.

**Key-Value (Redis, DynamoDB)**
- Flat. Key maps to value. No structure beyond that.
- Always by exact key. No range queries, no filters, no joins.
- Redis is single-threaded — strongly consistent within one instance
- Best for: cache, sessions, rate limiting counters, leaderboards (sorted sets)
- The trap: using Redis as primary database. Redis is in-memory. Restart without persistence = data gone.

**Time Series (ClickHouse, TimescaleDB, InfluxDB)**
- Events with timestamps. Append only. Never updated.
- Access by time range, aggregations over windows, latest N records.
- Columnar storage — reads only columns you query, not entire rows
- Best for: trade history, price history, metrics and monitoring
- Why not Postgres for this: at scale, ClickHouse handles 1M inserts/sec vs Postgres struggling. Aggregations across 500M rows in seconds vs minutes.

**Search (Elasticsearch)**
- Full text search, fuzzy matching, relevance ranking
- Eventually consistent. Not suitable for financial data.
- Best for: search boxes, log analysis, free text queries
- The trap: using as primary database. It is a search index. Source of truth lives in Postgres. Sync to ES for search.

**Graph (Neo4j, Neptune)**
- Nodes and edges. Relationships are first-class.
- Access by traversal: find all nodes connected to X within 3 hops.
- Best for: social networks, fraud detection, recommendation engines
- The trap: using because data "kind of looks like a graph." Postgres handles relationship queries fine up to moderate complexity.

### Database Decision Tree
```
Timestamp data, never updated?        → Time series (ClickHouse)
Full text search needed?              → Elasticsearch (secondary, not primary)
Deep relationship traversal?          → Graph DB
Flexible queries, strong consistency? → Relational (Postgres)
Simple key lookup, extreme speed?     → Key-value (Redis)
Unsure?                               → Postgres. Always start here.
```

### DeltaDeFi Database Map
| Service | Database | Reason |
|---|---|---|
| Account Service | Postgres | ACID, balances, positions, strong consistency |
| Matching Engine | In-memory + Postgres persistence | Never hits disk during matching |
| Trade History | ClickHouse | Append-only, time series, heavy aggregations |
| Price Feed | Redis | Key-value, read millions/sec, TTL |
| Vault Service | Postgres | Financial, ACID required, share calculations |
| Auth Service | Postgres + Redis for sessions | Identity + speed |
| Notification Service | No primary DB. Redis for idempotency. | Stateless |

### Table Statistics
Postgres maintains statistical summaries of data used by query planner to choose execution strategy.

For each column: n_distinct (unique value count), most_common values, value distribution histogram, null fraction.
For each table: estimated row count, page count.

Updated by AUTOVACUUM background process. Can become stale after bulk operations.

**When statistics go wrong:** bulk load 8M rows. Statistics still say 100K rows. Query planner thinks seq scan is cheap. Actually catastrophic. Fix: `ANALYZE orders;` — immediately recomputes statistics.

**Run ANALYZE after:** bulk inserts, mass deletes, migrations that change data distribution. Do not wait for autovacuum.

### Sequential Scan (Seq Scan)
Postgres reads every row from start to finish, checking each against WHERE clause filter.

```
Table: 8,000,000 rows
Query: WHERE user_id = 123

Seq Scan: read all 8M rows, discard 7,999,996, return 4
Cost: catastrophic
```

**When Seq Scan is actually correct:**
- Returning more than ~10% of table rows (sequential reads faster than random)
- Table is tiny (overhead not worth it)
- No useful index exists (index on low-cardinality column barely helps)

**EXPLAIN ANALYZE signals:**
```
Bad:  Seq Scan on orders
      Rows Removed by Filter: 7999996
      Execution Time: 2342ms

Good: Index Scan using idx_orders_user_created
      Rows examined: 4
      Execution Time: 0.4ms
```

### Indexing

**What an index is:** separate sorted data structure with pointers back to full rows. Like alphabetical index at back of a book.

**Tradeoff:** faster reads vs slower writes (index must be updated on every write) + more storage.

**Always index:** foreign keys (user_id, order_id), WHERE clause columns, ORDER BY columns on large tables.

**Never blindly index:** boolean columns (low selectivity), columns never queried, every column "just in case."

**Composite index rule:** most selective column first, range query columns last.

```sql
-- Query: WHERE user_id = 123 AND created_at > '2024-01-01'
CREATE INDEX ON orders (user_id, created_at DESC);
-- user_id first (high cardinality, equality filter)
-- created_at last (range filter)

-- This index helps: WHERE user_id = 123
-- This index helps: WHERE user_id = 123 AND created_at > ...
-- This index does NOT help: WHERE created_at > ... (missing leading column)
```

### Pre-Sharding Checklist (exhaust in order)
```
1. EXPLAIN ANALYZE slow queries     ← always first
2. Add correct composite indexes    ← almost certainly fixes it
3. Optimize slow queries            ← query plan analysis
4. Add read replicas                ← scale reads cheaply
5. Vertical scale                   ← bigger machine
6. Caching layer                    ← eliminate repeated DB reads
7. Time partitioning / archiving    ← reduce active table size
8. Shard                            ← only here, after all else exhausted
```

Most systems never need to go past step 6.

### Time Partitioning / Archiving
Active table holds last 90 days. Archive table holds everything older. Background job moves old records nightly.

```sql
-- Postgres native partitioning
CREATE TABLE orders (id BIGINT, created_at TIMESTAMP)
PARTITION BY RANGE (created_at);

CREATE TABLE orders_2024_q1 PARTITION OF orders
FOR VALUES FROM ('2024-01-01') TO ('2024-04-01');
```

Queries for last 30 days only touch recent partition. Old partitions can be moved to cold storage or dropped. This alone often eliminates need to shard.

### Sharding
Split data across multiple DB instances. Each owns a subset.

**Good shard key:** high cardinality, even distribution, appears in most queries.
**Bad shard key:** low cardinality (status), time-based without user component (hot spot on current partition), rarely in queries.

**Cross-shard queries hit all shards — expensive.** Route to ClickHouse or dual-write instead.

**DeltaDeFi:** shard by user_id. Pair-based queries (no user_id) route to ClickHouse trade history, not the sharded orders table.

### CREATE INDEX CONCURRENTLY
Never run plain CREATE INDEX in production — locks the table, blocks all reads and writes during build.

```sql
-- Production-safe:
CREATE INDEX CONCURRENTLY idx_orders_user_created
ON orders (user_id, created_at DESC);

-- Monitor progress:
SELECT phase, blocks_done, blocks_total
FROM pg_stat_progress_create_index
WHERE relid = 'orders'::regclass;

-- Verify valid:
SELECT indexname, valid FROM pg_indexes WHERE tablename = 'orders';
```

Tradeoffs: no table lock, zero downtime, takes 2-3x longer, cannot run inside a transaction, failed build leaves invalid index (must clean up manually).

---

### Drill 5

**Scenario:** 2M users, 50K daily traders, 500M order records in Postgres, P99 query time degrading to 3 seconds. Common queries: open orders for user, order history last 30 days, orders for trading pair last 1 hour.

**Q1: Before touching infrastructure, what do you check first?**

Your answer: CPU usage and disk I/O, check if vertical scale can help
Correct answer: **EXPLAIN ANALYZE the slow queries first**

Correction: Slow queries are not automatically a hardware problem. Before looking at CPU or disk, look at WHY the query is slow. EXPLAIN ANALYZE shows exactly what Postgres is doing. Most likely a seq scan on 500M rows due to missing index. Vertical scaling a machine to fix a missing index is like buying a faster car because you forgot to fill the tank.

**Q2: What indexes would you add?**

Your answer: user_id in user table, account_id in account table, order_id in order table
Correct answer: **Composite indexes matching exact query patterns**

Correction: Primary keys are already indexed automatically — you added nothing. Indexes must match actual query patterns:

```sql
-- Query 1: open orders for user
CREATE INDEX ON orders (user_id, status);

-- Query 2: order history, last 30 days
CREATE INDEX ON orders (user_id, created_at DESC);

-- Query 3: orders for trading pair, last 1 hour
CREATE INDEX ON orders (pair, created_at DESC);
```

Rule: equality filters first, range query columns last.

**Q3: Old data queried almost never — what do you do?**

Your answer: Cache recent 90 days or archive old history

Correct full answer:
- Caching: valid but does not solve underlying table size problem. 500M rows still exist. Index maintenance on 500M rows is slow.
- Archiving is the more complete solution: move data older than 90 days to `orders_archive` table via nightly background job. Active table drops from 500M to ~50M rows. P99 drops dramatically.
- Use Postgres native partitioning — queries for last 30 days automatically touch only recent partition.

**Q4: After all optimizations, still hitting limits — shard?**

Your answer: Yes, shard by user_id. High cardinality, even distribution.
Correct answer: **Correct shard key. But you missed the cross-shard problem.**

Problem: Query 3 (orders for trading pair) has no user_id. With user_id sharding, this query hits ALL shards. You made it more expensive, not less.

Solutions:
- Option A: Route pair-based queries to ClickHouse trade history (different data store, different access pattern)
- Option B: Dual write — `orders_by_user` (sharded by user_id) and `orders_by_pair` (sharded by pair). More storage, more write complexity, both patterns fast.

---

## Lesson 6: Caching In Depth

### Cache Topologies

**Client-side cache**
Lives in application process memory.
- Pros: zero network hop, fastest possible read
- Cons: each instance has own cache, instances diverge, inconsistency across instances
- Use when: data that almost never changes — feature flags, static config. Never for financial data.

**Distributed cache (Redis)**
Lives in shared external layer. All instances read from same place.
- Pros: consistent across all instances, one miss populates for everyone
- Cons: network hop to Redis (~0.1-1ms), Redis itself can become bottleneck
- Use when: shared mutable data — sessions, rate limits, order book snapshots, current prices. Default cache layer.

**CDN cache**
Lives at network edge, geographically close to users.
- Pros: eliminates geographic latency, scales to enormous read volumes
- Cons: only for static or semi-static content, not personalized or real-time
- Use when: static assets (JS, CSS, images), public API responses same for all users, market data snapshots with acceptable staleness

### Eviction Policies

**LRU (Least Recently Used)**
Evict item not accessed for longest time. General purpose. Redis default `allkeys-lru`.
- LRU blind spot: tracks reads not writes. Background-refreshed data looks cold to LRU even though it is maintained constantly.

**LFU (Least Frequently Used)**
Evict item with fewest total accesses. Better for permanently hot data (BTC price stays hot).
- Trap: new items start at 0 accesses, immediately vulnerable before accumulating hits.

**TTL (Time To Live)**
Expire after fixed time regardless of access. Eviction by expiry, not pressure.
- Trap: too long → stale data. Too short → too many misses, DB hammered.

**No Eviction**
Error when memory full. For Redis as primary data store where data loss is unacceptable.

### Cache Stampede

When popular key expires, many simultaneous requests miss and all query DB — death spiral.

```
10,000 requests/sec hit expired key
All miss → all query DB simultaneously
DB receives 10,000 identical queries
DB falls over
All subsequent requests also miss
System cannot recover
```

**Fix 1: Mutex Lock**
Only one request recomputes. Others wait or return stale.
```javascript
const lock = await redis.set('lock:key', '1', 'NX', 'EX', 2);
if (lock) {
  const data = await db.query(...);
  await redis.set('key', data, 'EX', ttl);
  await redis.del('lock:key');
} else {
  await sleep(50);
  return redis.get('key');
}
```

**Fix 2: Probabilistic Early Expiration**
Randomly recompute slightly before expiry. One request recomputes in background while cache is still warm. Stampede never happens.

**Fix 3: Background Refresh (Best for DeltaDeFi)**
Never let TTL expire in foreground. Background job refreshes before expiry. All read requests always hit cache. DB only touched by background refresher. Zero stampede risk.

### Write Strategies

**Cache-Aside (Lazy Loading)**
- Read: check cache → miss → read DB → write to cache
- Write: update DB → invalidate (delete) cache entry
- Pros: simple, cache only contains actually-read data
- Cons: first read after invalidation hits DB, potential mini-stampede
- Use: general purpose, read-heavy with occasional writes

**Write-Through**
- Write DB and cache simultaneously. Cache always warm.
- Pros: always fresh data, no cold reads after writes
- Cons: every write pays cache write cost, cache fills with never-re-read data
- Use: frequent re-reads of recently written data (sessions, real-time positions)

**Write-Behind (Write-Back)**
- Write cache immediately. Write DB asynchronously later.
- Pros: very low write latency
- Cons: cache dies before async write → data loss
- Use: analytics counters, view counts. **Never for financial data.**

### DeltaDeFi Write Strategy Map
| Data | Strategy | Reason |
|---|---|---|
| Order book snapshot | Background refresh | Never expires, always warm |
| Current price | Background refresh | Matching engine pushes |
| User balance | Write-through | Financial, must be fresh |
| Order status | Cache-aside | Invalidate on fill, lazy reload |
| Trade history | Cache-aside, long TTL | Immutable after write |
| Session tokens | Write-through | Must exist on every auth check |

### Redis Memory Isolation — Critical Rule
**Never share Redis between infrastructure data and application cache.**

Infrastructure data (prices, order book snapshots) and application features (portfolio analytics, user sessions) must be in separate Redis instances with separate `maxmemory` limits.

Portfolio analytics filling Redis must never be able to evict order book snapshots.

### Portfolio Cache Storage Strategy
- Key: `portfolio:{user_id}` — pre-computed projection
- TTL: 24 hours, reset on every event write and every read
- Active user: always served from Redis
- Inactive user (>24h): cold start — rebuild from Postgres snapshot + event replay → write to Redis
- Financial decisions (withdrawal, margin): always Postgres, never Redis

**Before caching any new feature:**
```
Expected cache size = avg value size × expected peak keys
If > 20% of available Redis memory → compress, reduce TTL, or cache summary only
```

---

### Drill 6

**Scenario:** 3am alert. Redis memory 98%. DB query rate 10x normal. API P99 8 seconds. LRU eviction policy. Portfolio analytics feature launched yesterday with 24h TTL. 80% of users loaded it. Redis full of portfolio data. Hot trading data being evicted.

**Q1: What is causing the DB spike?**

Your answer: Cache stampede. Hot trading data keys missing due to portfolio analytics filling Redis. Other features reading from DB directly.
Correct answer: **Correct**

Full chain:
```
Portfolio analytics fills Redis (80% users × large payload = Redis full)
→ LRU evicts hot trading data (looks cold — background refresh writes but nobody reads it directly)
→ Price feed, order book requests → cache miss
→ All hit DB simultaneously → stampede
```

**Q2: Why is LRU failing specifically?**

Your answer: Hot trading data becomes least recently used because Redis is full of portfolio data
Correct answer: **Correct but needed one more level of precision**

The precise reason LRU fails here: LRU tracks reads not writes. Background refresher writes `price:BTC` to Redis every 800ms. But nobody reads `price:BTC` directly between refreshes — WebSocket server pushes to clients, reads happen differently. From LRU's perspective, `price:BTC` has not been read recently → looks cold → gets evicted. LRU cannot distinguish between genuinely cold data and data maintained by a writer rather than a reader.

**Q3: Immediate 3am fix?**

Your answer: Switch to LFU instead
Correct answer: **Wrong. LFU alone does not fix it at 3am.**

Redis is at 98% memory. Portfolio analytics keys have high frequency counts after 24 hours. LFU would not evict them either. Memory is still full.

Correct immediate fix: **Flush the portfolio analytics keys right now.**
```bash
redis-cli --scan --pattern "portfolio:*" | xargs redis-cli del
```
This immediately frees memory. Hot trading data stops being evicted. DB spike stops. Latency recovers. Incident over.

**Q4: Proper long-term fix?**

Your answer: Background refresh for hot trading data
Correct answer: **Correct direction but incomplete. Three parts needed.**

Full answer:

Part 1 (most important): **Separate Redis instances**
- Redis Instance A: hot trading data, `maxmemory` 4GB, isolated
- Redis Instance B: application cache, `maxmemory` 8GB
- Portfolio analytics eating Instance B cannot affect Instance A

Part 2: **TTL discipline on expensive features**
- Calculate before shipping: `users × payload_size = memory footprint`
- Portfolio analytics at 24h TTL with 80% of 2M users = 400MB minimum
- Should be TTL 1 hour maximum, not 24 hours

Part 3: **Redis memory monitoring per key pattern**
- Schedule `redis-cli --bigkeys` to run and alert when any pattern exceeds 30% of memory
- Would have caught portfolio:* growing hours before the incident

---

## Lesson 7: API Design Under Scale

### Why API Design Is a Scaling Problem
Bad API design creates network bottlenecks, over-fetching, chatty clients, and systems that cannot be changed without breaking everything.

### Protocol Comparison

**REST**
- Resources identified by URLs, operations mapped to HTTP verbs
- JSON over HTTP. Human readable. Easy to debug.
- Strengths: universal (every client speaks HTTP), CDN cacheable, simple mental model
- Weaknesses: over-fetching (endpoint returns 50 fields, client needs 3), under-fetching (3 round trips for 3 endpoints), no contract enforcement, JSON parsing overhead at extreme scale
- Use when: public APIs, browser clients, mobile apps, anything needing CDN, simplicity matters

**gRPC**
- Functions not resources. Binary Protobuf over HTTP/2.
- Schema defined in .proto files. Code generated automatically.
- Strengths: strongly typed contract (client and server must agree), binary encoding (~5-10x smaller than JSON, ~5-10x faster parse), streaming built in, code generation
- Weaknesses: not human readable, poor browser support (gRPC-Web needed), cannot CDN cache, schema changes require coordination
- Use when: internal service-to-service, high throughput, streaming, anywhere browsers not involved

**GraphQL**
- Client specifies exactly what data it needs. Server returns only that.
- Strengths: no over/under-fetching, one request for deeply nested data, self-documenting schema
- Weaknesses: N+1 query problem (one query can trigger hundreds of DB queries), hard to cache, hard to rate limit (one query can be cheap or catastrophically expensive), overkill for simple APIs
- Use when: complex frontends with many different data needs, BFF layer, many client types with different data requirements

### Protocol Decision Framework
```
External (browser, mobile, third party)?
  → REST. Universal, cacheable, simple.
  → Complex frontend, many data shapes?
    → GraphQL on top of REST.

Internal service to service?
  → gRPC. Fast, typed, efficient.

Real-time streaming?
  → gRPC streaming (internal) or WebSocket (browser clients)
```

### Rate Limiting Algorithms

**Fixed Window:** count requests per window, reset at boundary.
- Problem: boundary attack — 100 requests at 00:59 + 100 at 01:00 = 200 in 2 seconds. Effective limit doubled.
- Use: simple non-critical limits only.

**Sliding Window:** track exact timestamps of each request, count how many fall in last N seconds.
- No boundary attack. Accurate. Memory expensive at scale (store timestamps for every request).

**Token Bucket:** bucket of tokens, each request consumes one, refills at fixed rate.
- Allows legitimate bursting — burst up to bucket size if tokens available
- **Best for exchanges** — traders place many orders rapidly during volatile markets
- DeltaDeFi limits: place order 10 tokens/sec refill, 50 token bucket; read order book 100 tokens/sec refill, 500 token bucket

**Leaky Bucket:** requests enter queue, queue drains at fixed rate, full queue = drop.
- Smooth predictable output regardless of input burst
- Good for protecting downstream services that cannot handle bursts

### Rate Limit State in Redis
All API instances must read/write same counter. Use Redis atomic operations.

**Single INCR is atomic** (single command, Redis single-threaded).
**GET + check + INCR is not atomic** — race condition. Instance A reads 99, Instance B reads 99, both increment to 100. Both allowed. Limit broken.

**Fix: Lua script** — executes entire check-and-decrement as one indivisible operation.

### JWT Authentication Flow
- **Issuance:** auth service signs with private key at login. Happens once.
- **Verification:** gateway verifies with public key. Local, no network call, ~1ms. Auth service not involved.
- **Revocation:** auth service writes token ID to Redis blocklist. Gateway checks blocklist (one Redis read per request).
- **Token TTL:** access token 15 minutes, refresh token 7 days.

Auth service is only in hot path at login and refresh. Never on regular API calls.

### Pagination

**Offset pagination:**
```sql
SELECT * FROM orders ORDER BY created_at DESC LIMIT 20 OFFSET 40;
```
- Problem: OFFSET is expensive. Page 1000 requires skipping 19,980 rows.
- Problem: new records inserted between pages cause duplicates or misses.
- Use: small datasets, admin UIs, when total count display required.

**Cursor pagination (default for DeltaDeFi):**
```sql
-- Cursor: {id: 123, created_at: "2024-01-15T10:23:01Z"} (base64 encoded)
SELECT id, pair, side, size, price, created_at
FROM trades
WHERE user_id = {from_jwt}  -- never from client
AND (
  created_at < '2024-01-15T10:23:01Z'
  OR (created_at = '2024-01-15T10:23:01Z' AND id < 123)  -- handles ties
)
ORDER BY created_at DESC, id DESC
LIMIT 21;  -- fetch one extra to determine has_more
```

- Constant performance regardless of position
- No duplicates or misses when data changes
- Cannot jump to arbitrary page, no total count
- Cursor must be base64 encoded — never expose raw IDs (leaks schema, prevents forging)

**LIMIT 21 trick:** fetch N+1. Got 21 = has_more true, return first 20. Got ≤20 = last page, has_more false. One query, no COUNT.

**Broken Object Level Authorization:** never accept user_id from client request. Derive from JWT. Otherwise any user can read any other user's data. One of most common API security vulnerabilities.

### Idempotency Key Pattern
Client generates UUID v7 once before first attempt. Reuses same key on every retry. Server stores in Redis with TTL 24h.

```
Client generates: idempotency_key = uuid_v7()
Attempt 1: POST /orders, Idempotency-Key: 018e8f3a-abc123
Network timeout.
Attempt 2: POST /orders, Idempotency-Key: 018e8f3a-abc123  ← SAME
Server checks Redis: already processed → return cached response
```

Event ID on server is deterministic: `sha256(user_id + pair + side + qty + price + idempotency_key)` — same logical action always same event ID.

### API Gateway
Single entry point. Handles: JWT verification, rate limiting (Lua + Redis), routing, SSL termination, logging. Internal services invisible to clients.

### WebSocket Architecture
```
Client → API Gateway (JWT verify at connection, rate limit)
       → WebSocket Server Pool
           ↓ all instances subscribe to Kafka: order-book-updates
           ↓ each instance pushes deltas to own connected clients

Connection flow:
  1. Client requests short-lived WS token via REST (TTL 60s)
  2. Client opens WS: wss://api.deltadefi.com/stream?token=xyz
  3. Server sends current snapshot from Redis (with sequence number)
  4. Server subscribes client to live delta stream
  5. Client applies deltas: snapshot + delta1 + delta2 = current state
  6. Discard deltas with sequence number older than snapshot
```

**Redis staleness gap (5-10ms):** acceptable for display. Never use Redis snapshot for financial decisions. Matching engine in-memory is source of truth. Redis is read-optimized copy. Kafka stream is real-time nervous system.

---

### Drill 7

**Scenario:** Public REST API for algo traders. Free tier 100 req/min. Pro tier 2,000 req/min. Rogue trader sent 50,000 requests in 10 seconds last week and degraded system. Trade history endpoint returns all trades with no pagination.

**Q1: Rate limiting algorithm for order placement?**

Your answer: Token bucket. Window would block users in volatile market.
Correct answer: **Token bucket — correct**

Full reasoning: fixed window blocks legitimate burst behavior. During volatile market, trader placing 20 orders in 2 seconds is legitimate. Token bucket allows burst as long as bucket has tokens. Different limits per endpoint based on cost: place order 10 tokens/sec refill / 50 bucket, read order book 100 tokens/sec refill / 500 bucket.

**Q2: Where does rate limit state live?**

Your answer: Redis so all instances read/write same key with atomic operations
Correct answer: **Correct**

**Q3: Rate limit did not stop rogue trader. Why?**

Your first answer: API limit does not work, no idea why
After hint: Race condition in Redis — Instance A writing while B reading old value

Correct full answer: Rate limit logic requires multiple Redis operations (GET + check + INCR) that are not atomic together. Instance A reads 99, Instance B reads 99 before A writes. Both increment to 100. Both allowed. At 5,000 req/sec this happens constantly.

Fix: Lua script makes the entire check-and-decrement atomic. No interleaving possible. Single indivisible operation.

Note: single Redis commands like INCR are already atomic (single-threaded). The problem is only when combining multiple commands.

**Q4: Fix trade history endpoint with cursor pagination**

Your first answer: `{order_id: "", created_at: ""}`
Corrected to full contract:

First request: no cursor, `SELECT ... WHERE user_id = {jwt_user_id} ORDER BY created_at DESC, id DESC LIMIT 21`

Response: `{ trades: [...20], next_cursor: "base64encoded", has_more: true }`

Subsequent request: decode cursor, use WHERE clause with tie-handling: `WHERE (created_at < ts) OR (created_at = ts AND id < id_value)`

End of records: `next_cursor: null, has_more: false`

Key corrections: cursor must be base64 encoded (not raw IDs), WHERE clause must handle same-timestamp ties (exchange fills multiple orders per millisecond), always scope to JWT user_id.

**Q5: Full architecture from trader to internal services**

Your answer omitted WebSocket path. Corrected full architecture:

```
Trader Client
  │ REST: orders, account, auth
  │ WS: real-time order book stream
  ▼
API Gateway
  │ JWT verify (local, no auth service call)
  │ Rate limit (Lua script + Redis)
  │ Route by path
  │
  ├── REST ─────────────────────────────────────────┐
  │   /orders/* ─gRPC→ Order Service                │
  │   /accounts/* ─gRPC→ Account Service            │
  │   /auth/* ─gRPC→ Auth Service                   │
  │   /trades/* ─gRPC→ Trade History Service         │
  │   /ws/token ─gRPC→ Auth Service (WS token)      │
  │                                                  │
  └── WebSocket ─────────────────────────────────── │
        wss://api.deltadefi.com/stream?token=xyz     │
        ↓                                            │
  WebSocket Server Pool                              │
  [WS-A][WS-B][WS-C] all subscribe to Kafka         │
  order-book-updates topic                           │
  each pushes deltas to own connected clients        │
                                                     │
Internal Services ←──────────────────────────────────┘
  All communicate via gRPC
  All publish/consume via Kafka
```

**Consolidation: pro trader, 1999th request, GET /trades/history?limit=20**

Full correct flow:
1. Client sends request with `Authorization: Bearer {jwt}`
2. Gateway decodes and verifies JWT signature locally (public key, ~1ms, no network call)
3. Gateway Lua script in Redis: GET ratelimit counter → if ≥2000 return 429 → else INCR + EXPIRE (all atomic)
4. Gateway routes to Trade History Service via gRPC, passes user_id from JWT in gRPC metadata
5. Trade History Service: no cursor → first page, query ClickHouse: `SELECT id, pair, side, size, price, created_at FROM trades WHERE user_id = {from_metadata} ORDER BY created_at DESC, id DESC LIMIT 21`
6. Got 21 results → has_more true, encode cursor from 20th record as base64
7. Return `{ trades: [...20], next_cursor: "eyJ...", has_more: true }`

---

## Lesson 8: Kafka Deep Dive

### Partition Key Rule
Kafka guarantees ordering within a partition only. Not across partitions.

**If event ordering matters for correctness — partition by the entity whose state depends on ordering.**

```
User balance events:    partition by user_id
  (deposit must be processed before withdraw)
Order book events:      partition by trading pair
  (order placed must be processed before order filled)
Vault events:           partition by vault_id
```

Wrong partition key causes out-of-order processing → balance inconsistency → financial error.

### Consumer Lag
```
Lag = latest partition offset − consumer last committed offset
```

Monitors health of consumers. Lag growing faster than it shrinks = system falling behind = critical.

```
DeltaDeFi alert thresholds:
  Account service lag > 1,000      → page immediately (financial data)
  Trade history lag > 10,000       → warning
  Notification lag > 50,000        → warning (lower priority)
```

### Delivery Semantics

**At-most-once:** commit before process. Crash = message lost. Use: non-critical logs, metrics.

**At-least-once:** process before commit. Crash = reprocessed. Duplicates possible. Use: notifications, emails (with idempotency).

**Exactly-once:** Kafka transactions + idempotent producer. Hardest. Use: balance updates, position changes.

### Exactly-Once Implementation
1. **Idempotent producer:** every message gets sequence number. Broker deduplicates retries at producer level. Enable: `idempotent: true`.

2. **Transactional API:** wrap consume + process + produce in one atomic transaction. Offset commit and message produce are one operation. Either all commit or nothing does.

```javascript
await producer.transaction(async (txn) => {
  const messages = await consumer.poll();
  const results = process(messages);
  await txn.send({ topic: 'output', messages: results });
  await txn.sendOffsets({ consumerGroupId, topics });
  // offset commit and produce are atomic
});
```

### Poison Pill Problem
If offset N always fails, it blocks entire partition — Kafka ordering guarantee means cannot skip.

**Strategy 1: Retry with exponential backoff** — handles transient failures (DB briefly unavailable, network blip).

**Strategy 2: Dead Letter Queue (DLQ)** — after N retries, publish to DLQ topic, commit offset, continue pipeline. DLQ = critical alert for financial consumers. Manual reconciliation required.

**Strategy 3: Circuit breaker** — failure rate exceeds threshold → stop consuming entirely → let downstream recover → test with one message. Prevents retry storms from making recovering system worse.

```
Three states:
  Closed:    normal operation, consuming
  Open:      downstream down, stop consuming
  Half-open: test with one message → success = close, fail = stay open
```

**DLQ for financial events = page immediately. Represents unprocessed financial state changes requiring human resolution.**

### Event Sourcing
Store every event that led to state, not current state.

```
balance_events table (append-only):
  deposit     +5000   09:00
  trade_fee   -50     09:15
  fill_credit +500    09:30
  withdrawal  -1200   10:00

Current balance = replay all events = 4250
```

**Pros:** complete audit trail, time travel (reconstruct state at any point), bug recovery (replay with fixed logic), regulatory compliance.

**Cons:** read complexity, performance on large history (use snapshots), storage growth.

**Snapshots:** periodically snapshot state. On read: load latest snapshot + replay only events after it. Fast and auditable.

**DeltaDeFi event sourcing scope:** user balances, positions, vault share issuance. Not: user profiles, sessions, order book state.

### CQRS (Command Query Responsibility Segregation)
Separate write model (normalized for correctness) from read model (denormalized for read speed).

```
Write side:   Postgres, event sourced, ACID, source of truth
Read side:    Redis, ClickHouse — pre-computed projections, eventually consistent

Events flow through Kafka to update both sides asynchronously.
```

**Always use write model for:** balance check before withdrawal, liquidation price, margin check, any financial decision.

**Use read model for:** portfolio display, trade history UI, leaderboards.

### Fund Reservation Pattern
Place order → **reserve** funds (not debit).

```
User balance:
  available: 5,000 ADA  ← can trade or withdraw
  reserved:  5,000 ADA  ← locked against open orders
  total:     10,000 ADA

Fill → convert reserved to position
Cancel → release reserved back to available
Withdrawal check → must verify available, not total
```

### Event ID Generation

**UUID v7 (client-generated idempotency keys):** time-ordered, globally unique, no coordination. Generated once per intended action, reused on retries. Never generates same ID on retry — correct for idempotency keys.

**Deterministic hash (server-generated event IDs for automated retry paths):**
```javascript
const eventId = sha256(user_id + pair + side + qty + price + idempotency_key);
```
Same logical event always same ID. Retry of same fill produces same event ID. Consumer deduplication catches it.

**Snowflake ID (high-throughput single-process like matching engine):** 41-bit timestamp + 10-bit machine ID + 12-bit sequence. Compact, sortable, fast, 4096 IDs/ms/machine. Requires machine ID assignment at startup.

### Idempotency Layers (Both Required)
```
Layer 1: API Gateway (Redis idempotency key check)
  → prevents duplicate client submissions
  → client reuses same key on retry → server returns cached response

Layer 2: Kafka consumer (Postgres processed_events table)
  → prevents duplicate processing after consumer crash
  → event_id check before processing → skip if already seen
```

### DeltaDeFi Consumer Strategy Map
| Consumer | Deduplication | Semantic |
|---|---|---|
| Account Service (balance) | Postgres processed_events + Kafka transactions | Exactly-once |
| Trade History (ClickHouse) | ClickHouse insert idempotent by event_id PK | At-least-once + natural idempotency |
| Portfolio Projector (Redis) | Redis SET is idempotent — overwrite safe | At-least-once + natural idempotency |
| Notification Service | Redis dedup key TTL 24h | At-least-once + Redis idempotency |
| Price Feed (Redis) | Last-write-wins overwrite safe | At-least-once naturally idempotent |

---

### Drill 8

**Scenario:** Trader deposits 10,000 ADA → places limit buy 5,000 ADA → partial fill 2,000 ADA → checks portfolio → withdraws 3,000 ADA.

**Q1-Q5 compiled answers (with corrections):**

**Events published:**

Step 1 — Deposit:
```
topic: account-events, key: user_id
{ type: "deposit", user_id, amount: 10000, event_id: uuid_v7, timestamp }
```
Consumers: Account Service (append to balance_events, available +10,000), Portfolio Projector (update Redis)

Step 2 — Place order:
```
topic: account-events, key: user_id
{ type: "funds_reserved", user_id, order_id, amount: 5000, timestamp }

topic: order-events, key: "BTC/ADA"
{ type: "order_placed", order_id, user_id, side: "buy", quantity: 5000, price, type: "limit" }
```
Correction: do NOT debit balance on place order. Reserve funds. Do NOT write trade history — trade history only records fills, not placed orders.

Account Service consumers: available -5,000, reserved +5,000.
Order Book Service: insert order.

Step 3 — Partial fill:
```
topic: order-events, key: "BTC/ADA"
{ type: "order_filled", taker_order_id, maker_order_ids, fill_quantity: 2000,
  fill_price, taker_user_id, maker_user_ids, fees, timestamp }
```
Consumers: Order Book (update book, remove filled qty), Account Service (taker: reserved -2000, position +BTC; maker: available +2000), Trade History (append to ClickHouse), Price Feed (update Redis price), Portfolio Projector (update Redis for both), Notification (push "partially filled")

Step 4 — Portfolio check:
Data from Redis read model (portfolio projector's projection). Eventually consistent — acceptable for display. Never use for financial decisions.

Step 5 — Withdrawal:
```
topic: account-events, key: user_id
{ type: "withdrawal", user_id, amount: 3000, timestamp }
```

**Before processing withdrawal:** check available balance from Postgres (write model, source of truth). NOT Redis. NOT total balance.
```
available = total - reserved
10,000 - 5,000 = 5,000 available
Withdrawal 3,000 ≤ 5,000 → allowed
```

**If Portfolio Projector crashes after step 3:**
- Kafka retains events at last committed offset
- Projector restarts, replays from last committed offset
- Redis portfolio shows stale data during downtime
- Option A: show staleness indicator in UI ("updating...")
- Option B: fall back to write model query if Redis timestamp exceeds staleness threshold
- DeltaDeFi: Option B for balance display (must be accurate), Option A for PnL metrics (staleness acceptable)

---

## Lesson 9: Observability

### The Three Pillars
```
Metrics  → what is happening      numbers over time, trends, baselines
Logs     → what happened          discrete events, errors, audit trail
Traces   → why it happened        causality across services, find the bottleneck
```

None replaces the others. All three needed. Metrics tell you something is wrong. Logs tell you what happened in one service. Traces tell you which service caused it and why.

### The Four Golden Signals

**1. Latency**
How long does a request take?

Always measure percentiles, never averages. Average hides the tail.
- p50: median, what most users experience
- p95: 95% of requests faster than this
- p99: 99% of requests faster than this (1 in 100 is slow)
- p999: 99.9% faster (1 in 1000 is very slow)

Example: p50=12ms, p99=850ms, p999=4200ms, average=24ms. Average looks fine. Hides that 1 in 100 traders experiences 850ms — they think the system is broken.

**2. Traffic**
How much demand is the system receiving?

Requests per second, orders per minute, WebSocket connections, Kafka messages consumed/sec. Traffic gives context — latency spike at 10 req/sec is different from same spike at 10,000 req/sec.

**3. Errors**
What fraction of requests are failing?

Measure 4xx (client errors) and 5xx (server errors) separately. 5xx rate is most important — it is your fault. 1% 5xx at 50,000 daily traders = 500 failed experiences per day.

**4. Saturation**
How full is your system?

CPU%, memory%, DB connection pool%, Kafka consumer lag, Redis memory%, disk%. Saturation predicts problems before they become failures. CPU 90% is a warning. CPU 100% is an incident.

### Metric Types
- **Counter:** only goes up. Total count. `total_orders_placed`, `total_errors`
- **Gauge:** goes up and down. Current state. `active_ws_connections`, `redis_memory_bytes`
- **Histogram:** distribution of values. Used for latency. Compute p50/p95/p99 accurately.

### Structured Logging
Always structured key-value, never plain strings. Enables machine querying.

```javascript
// Wrong:
console.log("Order placed for user 123, order ID ord_789");

// Right:
logger.info("order_placed", {
  user_id: 123, order_id: "ord_789",
  pair: "BTC/ADA", trace_id: "abc123def456"
});
```

With structured logs: query all errors for user 123 in last hour, all events with trace_id abc123. With plain strings: grep and regex, slow and miserable at 3am.

**Log levels:**
- ERROR: failed, requires attention, always with stack trace
- WARN: unexpected but system recovered
- INFO: normal business events worth recording
- DEBUG: never in production (too much volume)

**Never log:** passwords, API keys, private keys, full JWT tokens, PII in plain text.

**Always log for DeltaDeFi:** order lifecycle (placed/cancelled/filled/rejected), balance changes, authentication events, every ERROR with full context, every DLQ message.

**Never log:** order book updates (too frequent), every Kafka message consumed, every Redis read.

### Distributed Tracing
Every request gets unique trace ID at entry point. Propagated to every downstream service via HTTP headers or gRPC metadata. Each service creates spans — timed segments of work with start time, duration, success/failure.

**Trace ID propagation:**
```
Client → Gateway: gateway generates trace_id "abc123"
Gateway → Order Service: gRPC metadata { trace-id: "abc123" }
Order Service → Postgres: comment in query /* trace_id=abc123 */
Order Service → Kafka: event header { trace-id: "abc123" }
Kafka → Account Service: reads trace_id from header, tags all logs and spans
```

Without propagation: disconnected spans, cannot reconstruct full picture.

**Sample trace for 800ms request:**
```
Trace: abc123 (800ms)
├── API Gateway: 5ms
└── Order Service: 780ms
      ├── Postgres balance_check: 750ms  ← bottleneck immediately visible
      ├── Kafka publish: 25ms
      └── Redis write: 5ms
```

**Sampling strategy:**
- Always trace: all errors, all requests >500ms, all fill events, all withdrawal requests
- Sample 1%: normal successful requests, health checks, WebSocket heartbeats

Cannot trace everything at 50,000 req/sec — too expensive. Tail sampling captures interesting cases (errors, slow requests) without tracing everything.

### Alert Design Principles

**Alert on symptoms, not causes.**
```
Wrong: CPU > 80%                → engineer wakes up, does nothing
Right: p99 latency > 500ms      → engineer wakes up, traders are affected
```

**Every alert must be actionable.** If engineer cannot do something about it → fix root cause or remove alert.

**Avoid alert fatigue.** Alert fires twice per week without action → fix root cause or remove. If every alert is noise, the real incident gets missed.

### DeltaDeFi Alert Tiers
```
Page immediately (wake someone up):
  Matching engine down
  Account service Kafka consumer lag > 5,000
  Balance update failures > 0 in 5 minutes
  API 5xx rate > 1% for 2 consecutive minutes
  Redis memory > 90%
  Any DLQ message in financial topics
  WebSocket server completely down

Slack notification (next business hour):
  Kafka consumer lag > 1,000
  API p99 latency > 500ms for 5 minutes
  Redis memory > 75%
  Cache hit rate < 80%
  DB connection pool > 80% used

Log only (weekly review):
  Individual retry successes
  Cache misses on cold start
  Rate limited requests (expected behavior)
```

### Runbook
Every page-worthy alert must have a runbook. Step-by-step instructions for on-call engineer. Allows junior engineer to resolve most incidents at 3am without escalating.

### Observability Stack for DeltaDeFi
```
Metrics:  Prometheus scrapes all services
          → Grafana dashboards
          → AlertManager → PagerDuty

Logs:     Fluent Bit (lightweight agent on each node)
          → Elasticsearch or ClickHouse
          → Kibana or Grafana Loki

Traces:   OpenTelemetry SDK (instrument once, vendor-neutral)
          → OpenTelemetry Collector
          → Jaeger or Grafana Tempo

Unified:  Grafana — metrics + logs + traces in one place
```

Use OpenTelemetry — instrument once, swap backend freely without changing application code.

---

### Drill 9

**Scenario:** Tuesday 2pm. Alert: POST /orders p99 > 800ms for 5 minutes. Normal p99: 45ms. Current p99: 850ms. Error rate: normal (0.1%). Traffic: normal (500 req/sec).

**Q1: First action — what do you look at first?**

Your answer: Go to logs, find trace ID with long latency, look at which service has longest span
Correct answer: **Metrics first, then traces. Not logs.**

You already have the alert telling you which endpoint and what latency. Logs are unstructured and slow to search.

Faster path:
1. Metrics: confirm endpoint, check all instances vs one, correlate spike time with recent deployments
2. Traces: filter POST /orders with duration >500ms, find slow trace, identify slowest span
3. Logs: filter by trace_id for deep investigation of specific slow service

The trace ID approach is correct — you just reach it via traces, not logs.

**Q2: High latency but normal error rate — what does it tell you?**

Your answer: Some caching might not be working
Correct answer: **Requests are succeeding but something changed that made processing slower**

Cache miss causes latency increase but also typically causes error rate increase as DB gets hammered. Error rate is normal here. This tells you:
- Not a hard failure (service up, DB up)
- Not a cache stampede (error rate would spike)
- System is working but slower than usual
- Something changed: missing index, slow query, lock contention, recent deployment

Most important signal: was 45ms, now 850ms, errors unchanged. Something specific changed. Find what changed — correlate with recent deployments.

**Q3: Trace shows Order Service 840ms, balance check inside 820ms. What next?**

Your answer: Check CPU/network/I/O on Order Service instance and cache memory
Correct answer: **Go directly to the Postgres query. Run EXPLAIN ANALYZE.**

Trace already identified precisely: balance check 820ms. CPU, network, cache are all irrelevant. The bottleneck is identified. Use the information.

Next action: run EXPLAIN ANALYZE on the specific balance check query. That is it.

**Q4: EXPLAIN ANALYZE shows Seq Scan, 8M rows examined. What happened, how to fix?**

Your answer: DB storing too many records. Quick fix: add index. Long term: shard or archive.
Correct answer: **Correct conclusion, incomplete reasoning. Wrong about scale solution.**

Seq Scan does not mean too many records — it means no index (or wrong index). At 8M rows with a correct index, this query should run in <1ms. 8M rows is not large.

Two reasons for Seq Scan:
1. Index does not exist → add it
2. Index exists but Postgres chose not to use it → run ANALYZE orders to refresh statistics, or check if index covers the right columns

Quick fix: add correct composite index. Almost certainly fixes it without sharding.

Long term: verify all query patterns have matching indexes, add archiving policy as table grows. Sharding at 8M rows is premature. Exhaust the checklist: indexes → cache → archive → replicas → vertical → shard.

**Q5: How to add index in production without downtime?**

Your answer: Not sure
Correct answer: **CREATE INDEX CONCURRENTLY**

```sql
-- NEVER in production (locks table, blocks all reads/writes during build):
CREATE INDEX ON orders (user_id, created_at DESC);

-- Always in production (zero downtime):
CREATE INDEX CONCURRENTLY idx_orders_user_created
ON orders (user_id, created_at DESC);

-- Monitor build progress:
SELECT phase, blocks_done, blocks_total
FROM pg_stat_progress_create_index;

-- Verify index is valid before declaring fix:
SELECT indexname, valid FROM pg_indexes WHERE tablename = 'orders';
-- valid must be true

-- Confirm query now uses index:
EXPLAIN ANALYZE SELECT ...;
-- Should show Index Scan, not Seq Scan

-- Watch latency recover in Grafana
```

Tradeoffs: no table lock (zero downtime), takes 2-3x longer, cannot run inside transaction, failed build leaves invalid index (check `valid = false` and drop before retrying).

---

## Master Rules

### Scaling
- Vertical first. Horizontal when forced.
- Name the specific bottleneck before scaling anything.
- Project capacity mathematically. Do not feel it — calculate it.
- Every scaling decision buys runway. Know how much and what breaks next.
- After fixing, reset baseline and find the next ceiling. Scaling is iterative.

### Caching
- Cache value = frequency × recompute cost. Minus cost of stale data.
- Cache eliminates work. Replicas only distribute work. Cache before replicas.
- Separate Redis instances for infrastructure data vs application cache. Never share.
- Background refresh for hot data. Never let it expire in foreground.
- Before caching any feature: calculate expected_keys × avg_value_size. Can Redis absorb it?
- Financial decisions always from write model (Postgres). Never from cache.

### Kafka
- Partition by the entity whose state depends on ordering.
- Process → mark seen → commit offset. Never reverse this order.
- Every financial consumer needs DLQ + circuit breaker.
- Single Redis command = atomic. Multiple commands = Lua script.
- Consumer lag is your health metric. Monitor per consumer group.

### Databases
- Exhaust in order: indexes → query optimization → replicas → cache → archive → vertical → shard.
- EXPLAIN ANALYZE before touching infrastructure. Slow query ≠ hardware problem.
- Never SELECT *. Never accept user_id from client. Always scope to JWT identity.
- CREATE INDEX CONCURRENTLY in production. Always. Never plain CREATE INDEX.
- Run ANALYZE after bulk operations. Stale statistics cause wrong query plans.

### Services
- Split for a specific reason. Name it: scale mismatch, deployment coupling, or tech mismatch.
- Every service owns its data exclusively. Nobody else writes to another service's database.
- Every distributed state change needs a saga with compensating actions.
- Financial flows with user funds: orchestration saga, not choreography.
- Strangler fig: extract one service at a time.

### APIs
- REST external. gRPC internal. WebSocket real-time browser.
- Token bucket for exchange rate limiting. Allows legitimate burst.
- Cursor pagination as default. Offset only for small datasets.
- Idempotency key from client (UUID v7, reused on retry). Event ID deterministic from server.
- Alert on symptoms not causes.

### Observability
- Metrics → what is happening. Traces → why. Logs → what happened.
- p99 latency, not average. Average hides the tail.
- Correlate latency spikes with recent deployments first.
- Every alert must be actionable. Alert fatigue kills incident response.
- Traces: find slowest span first. Then investigate that specific service.

---

*Lessons 1–9 complete. Lesson 10: End-to-end system design simulation pending.*
