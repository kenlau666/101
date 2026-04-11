# System Design Course — Full Recap
> Lessons 1–9 with Drills and Key Rules

---

## Lesson 1: The Only Reason to Scale

### Core Idea
Scale to solve a specific bottleneck. Not because it's trendy. Not because Netflix does it.

### The Three Bottlenecks

| Bottleneck | Symptom | Example |
|---|---|---|
| Compute | CPU pegged, slow processing | Query aggregations, ML inference |
| Disk I/O | CPU idle but latency high, high disk wait | Full table scan, no index |
| Network | Latency high, bandwidth saturated | Large payloads, geographic distance |

**Disk I/O vs Network:**
- Disk I/O = data moving between disk and memory, inside one machine
- Network = data moving between machines over a wire
  - Latency subproblem: distance, routing hops → fix with CDN, co-location
  - Bandwidth subproblem: large payloads, high volume → fix with compression, pagination

### The Capacity Equation
1. What is my current capacity?
2. What is my actual or projected load?
3. What breaks first, and when?

### Vertical vs Horizontal
- **Vertical** — bigger machine. Fast, zero code changes, hard ceiling. **Use this first. Always.**
- **Horizontal** — more machines. Complex, requires stateless design. Use when vertical hits ceiling or fault tolerance is needed.

### Diagnostic Cheat Sheet
```
Is CPU high?           → Compute bottleneck
CPU low but slow?      → I/O bottleneck
Disk wait high?        → Disk I/O
Disk fine?             → Network I/O
```

### Drill 1 — Key Lesson
- Always name the specific metric misbehaving, not just the category
- Project capacity mathematically. Don't feel it — calculate it.
- After every scaling action, reset baseline and find the next ceiling.

**Rule: Every scaling decision buys runway. Know how much, and what breaks next.**

---

## Lesson 2: When Vertical Stops Working

### The Decision Tree
```
Is load read-heavy or write-heavy?
├── Read-heavy → Try cache first
│     └── Cache miss rate still high? → Read replicas
└── Write-heavy → Sharding (last resort)
```

### Cache Value Formula
```
Cache value = Access frequency × Cost to recompute
Hidden third variable: Cost of serving stale data
```

### What To Cache
**Best candidates:**
- Reference/config data (product catalog, feature flags) — high frequency, low stale cost
- Expensive aggregations (leaderboards, totals) — costly to compute, acceptable staleness

**Dangerous to cache:**
- Financial balances — high stale cost, requires strict invalidation or write-through

### The Three Cache Questions
Before caching anything:
1. How often is it read?
2. How expensive to recompute?
3. What is the cost of serving stale data?

### Exchange Caching Patterns
| Data | Mechanism | Why |
|---|---|---|
| Order book | In-memory + push (not traditional cache) | Changes too fast for TTL |
| Past trades | Redis, TTL 1-2 seconds | Immutable, high read, low stale cost |
| Current price | Redis, background refresh | Key-value, read millions/sec |
| Video files | CDN | Network latency problem, not cache |

**Rule: When data changes too fast to cache with TTL, stop pulling and start pushing.**

---

## Lesson 3: Horizontal Scaling

### Stateless vs Stateful
- **Stateless** — no memory between requests, any instance handles any request → scale freely with load balancer
- **Stateful** — owns specific data or connections → scaling is hard

**Requirements for truly stateless API:**
- Sessions in Redis (not local memory)
- Files in object storage (not local disk)
- Cache in Redis (not in-process)
- Auth via JWT (not server-side session)

### Scaling Stateful Services
Three strategies:
1. **Externalize state** → instances become stateless (sessions in Redis, files in S3)
2. **Partition state** → each instance owns a slice (matching engine per trading pair)
3. **Replicate state** → all instances have a copy (read replicas, CDN)

### Kafka-Based Horizontal Scaling
- Partitions = unit of parallelism
- Max useful consumers = number of partitions
- Consumer group = the scaling group
- **Kafka is the load balancer** — no separate LB needed for Kafka consumers

### Offset Management
- Kafka stores committed offsets in `__consumer_offsets` internal topic
- Heartbeat mechanism: consumer sends heartbeat every ~3 seconds
- If no heartbeat within `session.timeout.ms` → broker declares consumer dead → rebalance
- **Always use manual commit** — auto-commit can commit before processing completes → data loss

### Delivery Semantics
- **At-least-once**: process before commit. Duplicates possible. Handle with idempotency.
- **At-most-once**: commit before process. Messages can be lost. Never for financial data.
- **Exactly-once**: Kafka transactions + idempotent producer. Most complex. Use for balance updates.

### Idempotency Pattern
```
Read event
  → Check Redis/DB: have I processed event_id X?
  → Yes: skip, commit offset
  → No: process, mark seen, commit offset
```

**Order of operations:** process → mark seen → commit offset. Never commit before processing.

### Redis INCR Atomicity
- Single Redis commands (INCR, SET, GET) are atomic — Redis is single-threaded
- Multiple commands together are NOT atomic — use Lua scripts
- Lua scripts execute as one indivisible unit — no interleaving possible

### Drill 3 — Key Lessons
- Notification service is I/O bound (waiting on Apple/Google API), not CPU bound
- Scale with Kafka consumer groups, not load balancers
- Crash before offset commit → Kafka redelivers → idempotency prevents duplicate
- Redis idempotency key must be set AFTER push succeeds but BEFORE offset commit

---

## Lesson 4: Breaking a Monolith

### The Unpopular Truth
Microservices are not an upgrade. They are a tradeoff. Only split when you can name the specific problem you're solving.

### Three Valid Reasons to Split
1. **Scale mismatch** — one component's resource needs force over-provisioning of everything else
2. **Deployment coupling** — one service's deploy requires testing another, slowing velocity
3. **Technology mismatch** — component needs a different language or runtime

### The Seam Principle
Split along natural seams where:
- Communication across boundary is infrequent
- Data on each side is owned exclusively by one side
- Failure on one side doesn't require rollback on the other

### Costs of Splitting
- Function call (ns) → network call (ms)
- Shared DB transaction → distributed transaction problem
- One log file → logs across N services
- One stack trace → distributed trace

### The Distributed Transaction Problem
In a monolith: wrap in a DB transaction. Simple.
Across services: **no shared transaction**. If step 2 of 3 fails, step 1 already committed. Data inconsistency.

### Saga Pattern
Break distributed transaction into local transactions with compensating actions.

**Choreography:** each service publishes events and listens for others. Decoupled but hard to debug.

**Orchestration:** central coordinator tells each service what to do. Easier to debug, single point of visibility. Better for complex financial flows.

```
Vault deposit saga (orchestration):
  Step 1: tell Account Service → deduct balance
    fail → abort, nothing to compensate
  Step 2: tell Vault Service → increase TVL, mint shares
    fail → tell Account Service → refund balance
  Step 3: tell Notification Service → confirm
    fail → retry only, no financial rollback needed
```

### Strangler Fig Pattern
Extract one service at a time. Validate in production. Never extract everything at once.

### Decision Framework
```
1. Scale mismatch?           No → don't split
2. Deployment coupling?      No → don't split
3. Technology mismatch?      No → don't split
4. Clean seam, no shared tx? No → don't split
All yes → split, strangler fig, one at a time
```

### DeltaDeFi Service Map
```
Matching Engine    → Go, separate (scale + tech mismatch)
Price Feed         → Go, separate (scale + tech mismatch)
WebSocket Server   → separate (scales by connections, not pairs)
Account Service    → Postgres, owns balances and positions exclusively
Trade History      → ClickHouse, append-only time series
Vault Service      → separate, orchestrates deposit/withdraw saga
Auth Service       → separate, low traffic, owns identity
Notification       → stateless, Kafka consumer
```

---

## Lesson 5: Databases

### The Four Questions Before Choosing
1. What is the shape of my data?
2. How do I access it?
3. What are my consistency requirements?
4. What is my read/write ratio?

### Database Decision Tree
```
Timestamp data, append-only?        → Time series (ClickHouse, TimescaleDB)
Full text search needed?            → Elasticsearch (secondary index, not primary)
Deep relationship traversal?        → Graph DB
Flexible queries, strong consistency? → Relational (Postgres)
Simple key lookup, extreme speed?   → Key-value (Redis, DynamoDB)
Unsure?                             → Postgres. Always start here.
```

### DeltaDeFi Database Map
| Service | Database | Why |
|---|---|---|
| Account Service | Postgres | ACID, financial data |
| Matching Engine | In-memory + Postgres for persistence | Never hits disk during matching |
| Trade History | ClickHouse | Append-only, time series, heavy aggregations |
| Price Feed | Redis | Key-value, read millions/sec |
| Vault Service | Postgres | Financial, ACID required |
| Auth Service | Postgres + Redis for sessions | Identity + speed |

### Indexing
- **Always index:** foreign keys, WHERE clause columns, ORDER BY columns on large tables
- **Never blindly index:** boolean columns, columns never queried
- **Composite index rule:** most selective column first, range query columns last
- Index benefit: faster reads. Index cost: slower writes, more storage.

### Pre-Sharding Checklist (exhaust in order)
```
1. Add indexes
2. Optimize slow queries (EXPLAIN ANALYZE)
3. Add read replicas
4. Vertical scale
5. Caching layer
6. Time partitioning / archiving
7. Shard  ← only here
```

### Sharding
- **Good shard key:** high cardinality, even distribution, appears in most queries
- **Bad shard key:** low cardinality (status), time-based (hot spot on writes), rarely in queries
- Cross-shard queries hit all shards — expensive. Route to different data store instead.
- DeltaDeFi: shard orders by `user_id`. Pair-based queries go to ClickHouse.

### Table Statistics and Seq Scan
**Table statistics:** Postgres maintains statistical summaries (row count, distinct values, distribution) used by the query planner to choose execution strategy. Updated by autovacuum. Run `ANALYZE table_name` after bulk operations.

**Seq Scan:** Postgres reads every row to find matches. Correct when returning >10% of rows or table is tiny. Catastrophic on large tables with selective queries — fix with correct index.

**EXPLAIN ANALYZE signals:**
- `Seq Scan` + `Rows Removed by Filter: 7999996` → missing index
- `Index Scan using idx_name` + low execution time → correct

---

## Lesson 6: Caching In Depth

### Cache Topologies
| Topology | Where | Pros | Cons |
|---|---|---|---|
| Client-side | App process memory | Zero network hop | Instances diverge, inconsistent |
| Distributed (Redis) | Shared external | Consistent across instances | Network hop (~0.1ms) |
| CDN | Network edge | Geographic speed, massive scale | Static/semi-static only |

### Eviction Policies
- **LRU** (Least Recently Used) — evict oldest accessed. General purpose default.
- **LFU** (Least Frequently Used) — evict least accessed total. Better for permanently hot data. New keys vulnerable to early eviction.
- **TTL** — expire after fixed time. For known staleness tolerance.
- **No eviction** — error when full. For Redis as primary store, not cache.

### Cache Stampede
When a popular key expires, many requests miss simultaneously and hammer the DB.

**Fix 1: Mutex lock** — only one request recomputes, others wait.
**Fix 2: Probabilistic early expiration** — randomly recompute before TTL expires.
**Fix 3: Background refresh** — background job refreshes before expiry. Cache never expires in foreground. Best for DeltaDeFi hot data.

### Write Strategies
| Strategy | How | Use When |
|---|---|---|
| Cache-aside | Write DB, invalidate cache. Miss → read DB, populate cache. | General purpose, read-heavy |
| Write-through | Write DB and cache simultaneously | Frequent re-reads of recently written data |
| Write-behind | Write cache immediately, DB async | Non-critical, can tolerate data loss. Never financial. |

### Redis Memory Isolation Rule
**Never share Redis between infrastructure data (prices, order book) and application cache (portfolio, sessions).** Use separate instances with separate `maxmemory` limits. Portfolio analytics filling Redis must never evict order book snapshots.

### Portfolio Cache Strategy
- Store pre-computed projection per user: `portfolio:{user_id}`
- TTL: 24 hours, reset on every event and every read
- Inactive user (>24h): cold start from Postgres snapshot + event replay, then write to Redis
- Financial decisions (withdrawal, margin): always Postgres, never Redis

### Drill 6 — Key Lessons
- LRU blind spot: tracks reads not writes. Background-refreshed data looks cold to LRU.
- 3am fix: flush the offending key pattern immediately (`redis-cli --scan --pattern "portfolio:*" | xargs redis-cli del`)
- Real fix: separate Redis instances for infra vs application cache
- Before merging any cache feature: calculate `expected_keys × avg_value_size`. Can Redis absorb it?

---

## Lesson 7: API Design Under Scale

### Protocol Decision Framework
```
External client (browser, mobile, third party)?  → REST
  Complex frontend, many data shapes?            → GraphQL on top of REST
Internal service to service?                     → gRPC
Real-time streaming, internal?                   → gRPC streaming
Real-time streaming, browser?                    → WebSocket
```

### DeltaDeFi Protocol Map
| Connection | Protocol | Reason |
|---|---|---|
| Browser/mobile → API | REST | Universal, CDN cacheable |
| Matching engine → Account service | gRPC | Internal, high throughput, typed |
| Price feed → WebSocket server | gRPC internally, WS to browser | Browser can't speak gRPC |
| Third party integrations | REST | Simple, documented |

### Rate Limiting Algorithms
| Algorithm | How | Best For |
|---|---|---|
| Fixed window | Count per window, reset at boundary | Simple, non-critical limits |
| Sliding window | Count requests in last N seconds | Accurate, memory expensive |
| Token bucket | Bucket refills at fixed rate, burst allowed | **Exchanges** — legitimate burst behavior |
| Leaky bucket | Queue drains at fixed rate, smooth output | Protecting downstream services |

**Rate limit state always in Redis.** All API instances read/write the same key. Use Lua scripts for atomic check-and-decrement.

**Why token bucket for exchanges:** traders place many orders rapidly during volatile markets. Fixed window blocks legitimate behavior. Token bucket allows burst as long as bucket has tokens.

### Race Condition in Rate Limiting
Multiple Redis commands (GET + check + INCR) are not atomic together. Fix: Lua script executes entire check-and-decrement as one indivisible operation.

### Pagination
**Offset pagination:** `LIMIT 20 OFFSET 40`. Simple. Expensive at high page numbers — must skip N rows. Duplicates/misses when data changes between pages. Use for small datasets.

**Cursor pagination:** pointer to last seen record. Constant performance regardless of position. No duplicates. Cannot jump to arbitrary page. **Default for DeltaDeFi.**

```sql
-- Cursor: {id: 123, created_at: "2024-01-15T10:23:01Z"}
SELECT id, pair, side, size, price, created_at
FROM trades
WHERE user_id = {user_id}  -- always scope to authenticated user
AND (
  created_at < '2024-01-15T10:23:01Z'
  OR (created_at = '2024-01-15T10:23:01Z' AND id < 123)
)
ORDER BY created_at DESC, id DESC
LIMIT 21  -- fetch one extra to determine has_more
```

**LIMIT 21 trick:** fetch N+1 to determine `has_more` without a separate COUNT query.

### Broken Object Level Authorization
Never accept `user_id` from client request for data queries. Derive it from the authenticated JWT. Otherwise any user can read any other user's data.

### API Gateway
Single entry point for all clients. Handles: JWT verification, rate limiting, routing, SSL termination, request logging. Internal services are invisible to clients. Restructure services without breaking client code.

### JWT Authentication
- **Issuance:** Auth service signs token with private key. Happens once at login.
- **Verification:** Gateway verifies signature with public key. Local, no network call, ~1ms.
- **Revocation:** Auth service writes token ID to Redis blocklist. Gateway checks blocklist on every request (one Redis read).
- **Token TTL:** Access token 15 minutes. Refresh token 7 days.

### WebSocket Architecture
```
Client → API Gateway (auth + rate limit)
       → WebSocket Server Pool
           all instances subscribe to Kafka order-book-updates topic
           each instance pushes deltas to its own connected clients

On connect:
  1. Client gets snapshot from Redis (current state)
  2. Client subscribes to live delta stream
  3. Client applies deltas on top of snapshot using sequence numbers
  4. Discard any delta with sequence number older than snapshot
```

**Why Kafka not Redis pub/sub for WebSocket fan-out:** Kafka is persistent. Instance reconnects and replays from last offset. Redis pub/sub is fire-and-forget — missed messages during downtime are gone.

### WebSocket Token Auth
Issue short-lived WS token (TTL 60s) via REST. Client uses it to open WebSocket. No ongoing re-auth needed. What Binance does.

---

## Lesson 8: Kafka Deep Dive — Exactly-Once, Event Sourcing, CQRS

### Partition Key Rule
Kafka guarantees ordering within a partition only. Partition by the entity whose state depends on order.

```
User balance events  → partition by user_id
Order book events    → partition by trading pair
Vault events         → partition by vault_id
```

Wrong partition key → events processed out of order → balance inconsistency.

### Consumer Lag
```
Lag = latest partition offset − last committed consumer offset
```
Monitor lag per consumer group. Alert when lag grows faster than it shrinks.

```
Account service lag > 1,000    → page immediately
Trade history lag > 10,000     → warning
Notification lag > 50,000      → warning (lower priority)
```

### Delivery Semantics
| Semantic | When | Use For |
|---|---|---|
| At-most-once | Commit before process | Logs, non-critical metrics |
| At-least-once | Process before commit + idempotency | Notifications, emails |
| Exactly-once | Kafka transactions + idempotent producer | Balance updates, position changes |

### Exactly-Once Implementation
1. **Idempotent producer:** sequence numbers on messages, broker deduplicates retries
2. **Transactional API:** consume + process + produce in one atomic transaction. Offset commit and message produce are one operation. Either all commit or all roll back.

### Event ID Generation
| Strategy | When | How |
|---|---|---|
| UUID v7 | User-initiated actions (idempotency key) | Random + time-ordered, generated once per action |
| Deterministic hash | Automated retry paths (fill events) | sha256(content + source sequence number) |
| Snowflake ID | High-throughput single-process (matching engine) | timestamp + machine ID + sequence |

**Idempotency key (client):** generated once before first attempt, reused on every retry. Server stores in Redis with TTL 24h. Prevents duplicate submissions.

**Event ID (server):** derived deterministically from content + idempotency key. Same logical event always same ID. Prevents duplicate Kafka processing.

### Idempotency Layers
```
Layer 1: API Gateway idempotency key check (Redis)
  → prevents duplicate client submissions

Layer 2: Kafka consumer processed_events check (Postgres)
  → prevents duplicate processing after consumer crash
```

Both needed. Each solves a different duplicate problem.

### Poison Pill and DLQ
If offset N always fails, it blocks the entire partition. No messages after N are processed (ordering guarantee).

**Strategy 1: Retry with exponential backoff** — handles transient failures.
**Strategy 2: Dead Letter Queue** — after N retries, publish to DLQ topic, commit offset, continue. DLQ is a critical alert for financial consumers.
**Strategy 3: Circuit breaker** — if failure rate exceeds threshold, stop consuming entirely. Let downstream system recover. Prevents retry storms.

**DLQ messages for financial events = critical page. Manual reconciliation required.**

### Event Sourcing
Store every event that led to state, not current state itself.

```
balance_events table:
  deposit     +5000
  trade_fee   -50
  fill_credit +500
  withdrawal  -1200

Current balance = replay all events = 4250
```

**Pros:** complete audit trail, time travel, bug recovery by replaying with fixed logic, regulatory compliance.
**Cons:** read complexity, performance (use snapshots), storage growth.

**Snapshots:** periodically snapshot state. On read, load latest snapshot + replay only events after it.

**DeltaDeFi event sourcing scope:** user balances, positions, vault share issuance. Not: user profiles, sessions, order book.

### CQRS
Separate write model (normalized for correctness) from read model (denormalized for speed).

```
Write side:  Postgres, event sourced, ACID, source of truth
Read side:   Redis/ClickHouse, pre-computed projections, eventually consistent

Events flow through Kafka to update both sides asynchronously.
```

**Always use write model for:** balance checks before withdrawal, liquidation price, margin checks.
**Use read model for:** portfolio display, trade history, leaderboards.

### DeltaDeFi CQRS Design
```
Commands → Kafka → Write side consumers (Account Service, Trade History)
                → Read side consumers (Portfolio Projector → Redis,
                                       Analytics Projector → ClickHouse,
                                       Leaderboard Projector → Redis sorted set)
```

### Fund Reservation Pattern
Place order → **reserve** funds (not debit).
Fill → convert reserved to position.
Cancel → release reservation back to available.

```
User balance:
  available: 5,000  ← can trade or withdraw
  reserved:  5,000  ← locked against open orders
  total:     10,000
```

Withdrawal check must verify `available`, not `total`.

---

## Lesson 9: Observability

### The Three Pillars
```
Metrics → what is happening  (numbers over time)
Logs    → what happened      (discrete events)
Traces  → why it happened    (causality across services)
```

### The Four Golden Signals
1. **Latency** — p50, p95, p99, p999. Never use average — it hides the tail.
2. **Traffic** — requests per second, orders per minute, active connections.
3. **Errors** — 4xx (client errors) vs 5xx (your fault). 5xx rate is most critical.
4. **Saturation** — CPU%, memory%, connection pool%, Kafka consumer lag, disk%.

### Metric Types
- **Counter:** only goes up. Total count of events.
- **Gauge:** goes up and down. Current state.
- **Histogram:** distribution of values. Used for latency percentiles.

### Structured Logging
Always log as structured key-value pairs, never plain strings. Enables machine querying.

```
logger.info("order_placed", {
  user_id: 123, order_id: "ord_789",
  pair: "BTC/ADA", trace_id: "abc123"
});
```

**Log levels:**
- ERROR: failed, requires attention, always with stack trace
- WARN: unexpected but recovered
- INFO: normal business events worth recording
- DEBUG: never in production

**Never log:** passwords, API keys, private keys, full JWT tokens, PII in plain text.

### Distributed Tracing
Every request gets a trace ID at entry point. Propagated to every downstream service via headers/gRPC metadata. Each service creates spans — timed segments of work.

```
Trace: abc123 (800ms total)
├── API Gateway: 5ms
└── Order Service: 780ms
      ├── Postgres balance_check: 750ms  ← bottleneck
      ├── Kafka publish: 25ms
      └── Redis write: 5ms
```

**Sampling strategy for DeltaDeFi:**
- Always trace: all errors, all requests >500ms, all fill events, all withdrawals
- Sample 1%: normal successful requests

### Alert Design Principles
- **Alert on symptoms, not causes.** p99 latency > 500ms, not CPU > 80%.
- **Every alert must be actionable.** If engineer can't do anything — remove it.
- **Avoid alert fatigue.** Fires twice per week without action → fix root cause or remove.

### DeltaDeFi Alert Tiers
```
Page immediately:
  Matching engine down
  Account service Kafka lag > 5,000
  Balance update failures > 0 in 5 minutes
  API 5xx rate > 1% for 2 minutes
  Redis memory > 90%
  Any DLQ message in financial topics

Slack notification:
  Kafka lag > 1,000
  API p99 > 500ms for 5 minutes
  Redis memory > 75%
  Cache hit rate < 80%
  DB connection pool > 80%
```

### Observability Stack
```
Metrics:  Prometheus → Grafana → AlertManager → PagerDuty
Logs:     Fluent Bit → Elasticsearch/Loki → Kibana/Grafana
Traces:   OpenTelemetry SDK → OTel Collector → Jaeger/Tempo → Grafana
```

Use OpenTelemetry — vendor neutral, instrument once, swap backend freely.

### Production Index Fix (CONCURRENTLY)
```sql
-- Never do this in production (locks table):
CREATE INDEX ON orders (user_id, created_at DESC);

-- Always do this:
CREATE INDEX CONCURRENTLY idx_orders_user_created
ON orders (user_id, created_at DESC);
```

Monitor progress: `SELECT phase, blocks_done, blocks_total FROM pg_stat_progress_create_index`.
Verify valid: `SELECT indexname, valid FROM pg_indexes WHERE tablename = 'orders'`.

### Incident Investigation Flow
```
1. Metrics    → confirm which endpoint, when did it start,
                correlate with recent deployments
2. Traces     → find slow trace, identify slowest span
3. EXPLAIN ANALYZE → if DB span is slow, run query plan
4. Fix        → CONCURRENTLY index, ANALYZE table, or code fix
5. Monitor    → watch metrics recover in Grafana
```

---

## Master Rules Summary

```
Scaling:
  Vertical first. Horizontal when forced.
  Name the bottleneck before scaling anything.
  Every scaling decision buys runway. Know how much.
  After fixing, reset baseline and find the next ceiling.

Caching:
  Cache value = frequency × recompute cost. Minus stale cost.
  Separate Redis instances for infra vs application cache.
  Background refresh for hot data. Never let it expire in foreground.
  Financial decisions always from write model, never cache.

Kafka:
  Partition by the entity whose state depends on ordering.
  Process → mark seen → commit offset. Never reverse.
  Every financial consumer needs DLQ + circuit breaker.
  Single Redis commands atomic. Multiple commands need Lua script.

Databases:
  Exhaust indexes, replicas, cache, archiving before sharding.
  Never SELECT *. Never accept user_id from client. Scope to JWT identity.
  EXPLAIN ANALYZE before touching infrastructure.
  CREATE INDEX CONCURRENTLY in production. Always.

Services:
  Split for a specific reason. Name it.
  Every service owns its data exclusively.
  Every distributed state change needs a saga.
  Financial flows need orchestration saga, not choreography.

APIs:
  REST external. gRPC internal. WebSocket real-time.
  Token bucket for exchange rate limiting.
  Cursor pagination as default. Offset only for small datasets.
  Idempotency key from client. Event ID deterministic from server.

Observability:
  Alert on symptoms not causes.
  Traces tell you why. Metrics tell you what. Logs tell you what happened.
  Every alert must be actionable.
  Correlate latency spikes with recent deployments first.
```

---

*Course by Claude — Lessons 1–9 complete. Lesson 10: End-to-end system design simulation pending.*
