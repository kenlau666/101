# CEX Matching Engine — Supplement: The Transactional Outbox Pattern (Go)

> Dual-Write Problem & Its Fix
> One lesson. Postgres + Kafka primitives inline. Harsh drill at the end.

---

## Where This Lesson Fits

This is a supplement sitting between Phase 2 (WAL + Raft) and Phase 3 (Ledger + Wallets). It's not about the matching engine's hot path — the matching engine is already event-sourced through Kafka and doesn't have the problem this pattern solves.

This lesson is about everything *else* in your exchange that uses Postgres and also needs to publish events: the withdrawal service, the notification service, the saga orchestrator, the audit logger, the deposit crediter. All of these are "boring" services in the sense that they run at hundreds or thousands of ops/sec, not millions. But they're not boring in the sense that bugs here lose real money and corrupt real state.

The outbox pattern is the standard solution to the dual-write problem. Know it. Use it. The one-hour investment pays off forever.

---

## Why This Lesson Exists

### The Dual-Write Problem (Concrete)

User 123 requests a withdrawal of 1,000 USDT. Your withdrawal service must do two things:

1. Debit the user's balance in Postgres.
2. Publish a `withdrawal_requested` event to Kafka, which the on-chain processor consumes and broadcasts the transaction.

The naive implementation:

```go
func ProcessWithdrawal(userID uint64, amount int64) error {
    // Step 1: debit the user
    _, err := db.Exec("UPDATE balances SET amount = amount - $1 WHERE user_id = $2", amount, userID)
    if err != nil {
        return err
    }

    // Step 2: publish event
    err = kafka.Produce("withdrawals", buildEvent(userID, amount))
    if err != nil {
        return err
    }

    return nil
}
```

This looks fine. It's catastrophically broken.

### The Four Failure Modes

There are exactly four ways this can crash, and the three bad ones kill you:

**Case 1: Both succeed.** Happy path. Balance debited, event published. On-chain processor picks it up, processes the withdrawal. Correct.

**Case 2: DB fails.** The `UPDATE` returns an error. Function returns the error. No event published. User balance unchanged. User retries. Correct.

**Case 3: DB succeeds, Kafka fails.** `UPDATE` committed. Process crashes (or Kafka is down) before `Produce` returns success. **User balance debited, no event published.** The on-chain processor never sees the withdrawal. User's 1,000 USDT is gone from their visible balance but never gets sent to their wallet. They open a support ticket. Support has to manually reconcile. If you have thousands of users, this happens daily and the support backlog becomes the new bottleneck.

**Case 4: DB succeeds, Kafka succeeds, but process crashes between.** Depending on Kafka client configuration, the produce may have already succeeded on the broker but your code thinks it failed and retries. Duplicate event published. On-chain processor sees two withdrawal requests. If the consumer isn't idempotent (Phase 2 Lesson 5 discipline), it processes both. **User's 1,000 USDT is sent twice. You just paid an attacker to find this bug.**

**Reversing the order is not a fix.** If you publish to Kafka first, then debit the DB:

- Case 5: Kafka publishes, DB debit fails. On-chain processor sends 1,000 USDT to the user, but their balance was never deducted. They can withdraw again. Infinite money bug.

**There is no order that's safe without a protocol.** The two writes, to two independent systems, cannot both succeed or both fail without coordination. This is the *dual-write problem*, and it's one of the most common bugs in distributed systems.

### Real Exchange Failures

This pattern of failure has hit real exchanges. The post-mortems are often vague ("internal accounting discrepancy") because nobody wants to admit they had a dual-write bug, but the symptoms are telltale:

- Users reporting "my balance shows debited but the withdrawal never arrived" (Case 3).
- Users receiving double withdrawals during incidents (Case 4).
- Manual reconciliation teams being a permanent part of exchange operations.

Every exchange that uses Postgres and Kafka has had at least one Case 3 incident. Most have had multiple. The fix is always the same: the outbox pattern. They just hadn't implemented it yet when the incident happened.

---

## Postgres and Kafka Primer (Just Enough)

### Postgres transactions

A Postgres transaction wraps multiple statements in atomicity:

```sql
BEGIN;
UPDATE balances SET amount = amount - 1000 WHERE user_id = 123;
INSERT INTO transactions (user_id, type, amount) VALUES (123, 'withdrawal', 1000);
COMMIT;
```

Either both statements succeed (after `COMMIT`) or neither does (if you `ROLLBACK` or crash). This is ACID's atomicity property. Postgres enforces it by writing to its WAL (same idea as Phase 2 Lesson 4, but for the DB) and only flushing the transaction's visibility to other readers after `COMMIT`.

**Critical property for outbox:** inserts in a transaction are not visible to other transactions until `COMMIT`. This is what makes the outbox pattern work — you can insert the event in the same transaction as the state change, and a reader won't see the event until the state change is committed.

### Kafka producer semantics

Kafka producers have configurable guarantees:

- `acks=0`: fire and forget. Fastest, zero durability. Never use for correctness.
- `acks=1`: wait for leader to ack. Survives leader write but not leader failure before replication.
- `acks=all`: wait for all in-sync replicas to ack. Durable. Slower. **Use this for anything that matters.**

With `enable.idempotence=true`, the producer attaches sequence numbers so retries are deduplicated at the broker. With `acks=all` + idempotent producer, a successful return really means the message is on the log durably.

The outbox publisher uses `acks=all`, idempotent producer, and handles the "partial success" ambiguity via the outbox's own state tracking.

---

## The Outbox Pattern

### The core idea

**Write the DB state change AND the event to publish in a single DB transaction.** The event goes into an `outbox` table in the same database. A separate process (the "publisher") reads the outbox table and publishes to Kafka, marking rows as published once Kafka confirms.

```sql
BEGIN;
UPDATE balances SET amount = amount - 1000 WHERE user_id = 123;
INSERT INTO outbox (id, event_type, payload, created_at, published)
       VALUES (gen_random_uuid(), 'withdrawal_requested',
               '{"user_id": 123, "amount": 1000, ...}',
               NOW(), false);
COMMIT;
```

Now either both succeed or neither does. The outbox row exists if and only if the balance was actually debited. If the service crashes mid-transaction, Postgres rolls back — no debit, no outbox row.

### The architecture

```
┌────────────────────────────────┐
│ Withdrawal Service             │
│                                │
│  BEGIN;                        │
│    UPDATE balances;            │
│    INSERT INTO outbox;         │
│  COMMIT;                       │
└─────────┬──────────────────────┘
          │
          ↓
┌────────────────────────────────┐
│ Postgres                       │
│                                │
│  balances table                │
│  outbox table                  │
└─────────┬──────────────────────┘
          │ (polled / LISTEN)
          ↓
┌────────────────────────────────┐
│ Outbox Publisher (separate     │
│ process or goroutine)          │
│                                │
│  SELECT * FROM outbox          │
│      WHERE published = false   │
│      ORDER BY id               │
│      LIMIT 100;                │
│  → produce to Kafka            │
│  → UPDATE outbox SET published │
└─────────┬──────────────────────┘
          │
          ↓
┌────────────────────────────────┐
│ Kafka                          │
└────────────────────────────────┘
```

The publisher is **idempotent and restart-safe.** If it crashes mid-batch, on restart it re-reads the same rows (still `published = false`) and re-publishes. Downstream consumers must be idempotent (see Phase 2 Lesson 5, SD101 Lesson 3) — they dedupe by the outbox row's UUID.

### Delivery guarantee

This is **at-least-once delivery.** The publisher may publish a message, crash before marking `published = true`, and on restart publish the same message again. That's fine if downstream is idempotent.

Combined with consumer idempotency, this achieves **effectively-once processing** — the same logical effect as exactly-once semantics, without the distributed-transaction complexity.

### Why this works

The key property: **the outbox row and the state change are in the same database.** The atomicity guarantee of Postgres transactions covers both. There's no dual-write anymore — there's a single write (DB transaction) and a separate downstream replication (DB → Kafka) that's idempotent and retryable.

You've turned a distributed-atomicity problem into a local-atomicity problem plus a replication problem. Both are solvable.

---

## Implementation in Go

### The outbox table schema

```sql
CREATE TABLE outbox (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate   TEXT NOT NULL,           -- e.g., "user:123" — for ordering per aggregate
    event_type  TEXT NOT NULL,           -- e.g., "withdrawal_requested"
    payload     JSONB NOT NULL,          -- or BYTEA for binary
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published   BOOLEAN NOT NULL DEFAULT false,
    published_at TIMESTAMPTZ
);

CREATE INDEX idx_outbox_unpublished
    ON outbox (created_at)
    WHERE published = false;
```

Notes:
- `id` is the idempotency key. Downstream consumers dedupe on this.
- `aggregate` is used if you need per-entity ordering (all events for `user:123` in order).
- The partial index on `published = false` keeps lookups fast as the outbox grows.

### The write path

Wrap your DB transaction to also write the outbox row:

```go
type OutboxEvent struct {
    Aggregate string
    EventType string
    Payload   []byte
}

func (s *WithdrawalService) Withdraw(ctx context.Context, userID uint64, amount int64) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // State change
    _, err = tx.ExecContext(ctx,
        "UPDATE balances SET amount = amount - $1 WHERE user_id = $2 AND amount >= $1",
        amount, userID)
    if err != nil {
        return err
    }
    // Check affected rows to enforce sufficient balance (not shown)

    // Outbox insert in the SAME transaction
    payload, _ := json.Marshal(map[string]any{
        "user_id":   userID,
        "amount":    amount,
        "timestamp": time.Now().UnixNano(),
    })
    _, err = tx.ExecContext(ctx, `
        INSERT INTO outbox (aggregate, event_type, payload)
        VALUES ($1, $2, $3)`,
        fmt.Sprintf("user:%d", userID),
        "withdrawal_requested",
        payload,
    )
    if err != nil {
        return err
    }

    return tx.Commit()
}
```

### The publisher

The publisher is a separate goroutine or process that polls the outbox and publishes:

```go
type Publisher struct {
    db       *sql.DB
    producer *kafka.Producer
    topic    string
}

func (p *Publisher) Run(ctx context.Context) error {
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := p.publishBatch(ctx); err != nil {
                log.Printf("outbox publish error: %v", err)
                // Continue — transient errors are expected
            }
        }
    }
}

func (p *Publisher) publishBatch(ctx context.Context) error {
    // 1. Read a batch of unpublished events, ordered by creation time
    rows, err := p.db.QueryContext(ctx, `
        SELECT id, aggregate, event_type, payload
        FROM outbox
        WHERE published = false
        ORDER BY created_at
        LIMIT 100
        FOR UPDATE SKIP LOCKED`)
    if err != nil {
        return err
    }
    defer rows.Close()

    var events []outboxRow
    for rows.Next() {
        var r outboxRow
        if err := rows.Scan(&r.id, &r.aggregate, &r.eventType, &r.payload); err != nil {
            return err
        }
        events = append(events, r)
    }

    // 2. Publish each to Kafka, using the outbox UUID as the idempotency key
    for _, e := range events {
        err := p.producer.Produce(&kafka.Message{
            TopicPartition: kafka.TopicPartition{Topic: &p.topic},
            Key:            []byte(e.aggregate),    // partition key = aggregate for ordering
            Value:          e.payload,
            Headers: []kafka.Header{
                {Key: "event_id", Value: []byte(e.id.String())},
                {Key: "event_type", Value: []byte(e.eventType)},
            },
        }, nil)
        if err != nil {
            return err
        }
    }

    // Wait for all produces to be acked
    if remaining := p.producer.Flush(5000); remaining > 0 {
        return fmt.Errorf("%d messages failed to flush", remaining)
    }

    // 3. Mark as published
    ids := make([]string, len(events))
    for i, e := range events {
        ids[i] = e.id.String()
    }
    _, err = p.db.ExecContext(ctx, `
        UPDATE outbox SET published = true, published_at = NOW()
        WHERE id = ANY($1)`, pq.Array(ids))
    return err
}
```

Key details:

- **`FOR UPDATE SKIP LOCKED`** — allows multiple publisher instances to run concurrently. Each grabs a different batch. No locking conflicts.
- **Kafka partition key = aggregate ID.** All events for `user:123` go to the same Kafka partition, preserving order (Kafka guarantees ordering within a partition, not across).
- **`event_id` header = outbox UUID.** Downstream consumer stores processed IDs in Redis or Postgres and dedupes on it.
- **Flush before marking published.** Never mark a row as published until Kafka has durably acked it. Otherwise, on crash, you lose the message.

### Consumer idempotency

The consumer must be idempotent, as covered in Phase 2 Lesson 5 and SD101 Lesson 3. Concrete pattern:

```go
func (c *OnChainProcessor) Consume(msg *kafka.Message) error {
    eventID := getHeader(msg, "event_id")

    // Check if we've already processed this
    alreadyProcessed, err := c.redis.Get(ctx, "processed:"+eventID).Bool()
    if err != nil && err != redis.Nil {
        return err
    }
    if alreadyProcessed {
        c.consumer.CommitMessage(msg)
        return nil  // skip
    }

    // Process
    if err := c.processWithdrawal(msg.Value); err != nil {
        return err
    }

    // Mark processed, with TTL longer than outbox retention
    if err := c.redis.Set(ctx, "processed:"+eventID, true, 7*24*time.Hour).Err(); err != nil {
        return err
    }

    c.consumer.CommitMessage(msg)
    return nil
}
```

With this pattern:
- Outbox republishes on crash → consumer sees duplicate event_id → skips it.
- Consumer crashes after processing but before committing → Kafka redelivers → consumer sees duplicate event_id → skips it.

Everything converges.

---

## Publisher Strategies: Polling vs LISTEN/NOTIFY vs CDC

Three ways the publisher can detect new outbox rows.

### 1. Polling (what I showed above)

Publisher runs a loop with a timer, queries every N ms. Simple, robust, well-understood.

- Latency: N ms (your poll interval). 100ms is common; shorter if you need lower latency.
- DB load: one query every N ms per publisher instance. Cheap with the partial index.
- Works with any SQL database.

**Downside:** at high rates, polling becomes wasteful. Better: use LISTEN/NOTIFY to avoid idle polling.

### 2. LISTEN/NOTIFY (Postgres-specific)

Postgres has a pub-sub mechanism. Publisher issues `LISTEN outbox_new`, holds an idle connection. The write path adds `NOTIFY outbox_new` to the transaction. Publisher wakes up immediately when a new event is committed.

```sql
-- In the write path:
INSERT INTO outbox ...;
NOTIFY outbox_new;
```

```go
// In the publisher:
listener := pq.NewListener(connStr, ...)
listener.Listen("outbox_new")

for {
    select {
    case <-listener.Notify:
        p.publishBatch(ctx)
    case <-time.After(30 * time.Second):
        // Fallback poll in case we miss a notification
        p.publishBatch(ctx)
    }
}
```

Latency: sub-millisecond. Still need fallback polling because `NOTIFY` isn't durable (if the listener reconnects, notifications emitted during disconnection are lost).

### 3. CDC (Change Data Capture)

Instead of an outbox table, use a tool like **Debezium** or **pg_logical** that reads the Postgres WAL directly and publishes each transaction's changes to Kafka automatically. No application code writes to an outbox. The tool watches specific tables and publishes each change.

Pros: zero application-side code. Works for any table change. High-throughput capable.
Cons: operationally heavy (Debezium needs Kafka Connect, Zookeeper historically, schema management). Another system to operate. Event format is tied to DB schema (every column change creates an event).

Used by: Shopify, Netflix, large shops that have DBA + platform teams.

For a CEX starting out: polling or LISTEN/NOTIFY is the right choice. CDC is premature unless you're at serious scale and already have the infrastructure team.

---

## Ordering Guarantees

This is where people get tripped up.

**Strict global ordering is hard.** Events in the outbox are inserted in transaction-commit order, but multiple transactions can commit concurrently, and their outbox rows get different IDs. If you process them in `created_at` order, two events committed within the same clock tick could be in either order.

**Per-aggregate ordering is achievable.** All events for `user:123` can be ordered: they come from transactions that touched user 123's row, and row-level locking means those transactions don't commit concurrently. Use the aggregate ID as the Kafka partition key — all events for the same aggregate go to the same partition, which Kafka orders.

**What you actually need:**

- For user-level operations: per-user ordering. Use `user:123` as the aggregate / Kafka key.
- For system-wide events: usually doesn't matter; if it does, you need a single partition (and thus a single consumer) for that topic, accepting the throughput limit.

**What you should not do:** try to achieve strict global ordering across all outbox events. It's expensive, error-prone, and usually unnecessary.

---

## Outbox Growth and Cleanup

The outbox table grows forever unless you clean it up. At 1,000 events/sec, it grows by ~86M rows/day. You need a retention policy.

Two approaches:

**Soft delete + archival.** Published rows stay in the table but are marked `published = true`. A separate job deletes rows older than N days (or moves them to an archive table / S3 for audit purposes).

```sql
DELETE FROM outbox
 WHERE published = true
   AND published_at < NOW() - INTERVAL '7 days';
```

Run this on a schedule (every hour, say). Keeps the table bounded.

**Partitioned outbox.** For very high volumes, partition the outbox table by day. Drop old partitions instead of deleting rows (much faster).

```sql
CREATE TABLE outbox (
    -- columns
) PARTITION BY RANGE (created_at);

CREATE TABLE outbox_2026_04_23 PARTITION OF outbox
    FOR VALUES FROM ('2026-04-23') TO ('2026-04-24');

-- Next day:
DROP TABLE outbox_2026_04_16;  -- drop 7-day-old partition
```

Postgres native partitioning. Covered in the System Design 101 file if you need a refresher.

**Retention = max(downstream consumer lag you tolerate + safety margin, audit window).** If your on-chain processor is usually < 1 minute behind but could fall 6 hours behind during incidents, keep at least 24 hours. If regulatory audit needs 30 days, keep 30 days in the table or archive.

---

## Common Bugs

**1. Publisher reads uncommitted data.** Default Postgres isolation (Read Committed) means the publisher's query only sees committed rows — this is what you want. But if you're using a weaker isolation setting or doing something weird with autocommit, you might accidentally read rows from an in-flight transaction. Always verify: publisher's read isolation is Read Committed or stricter.

**2. Duplicate publishing without idempotency downstream.** Publisher publishes, crashes, republishes. Downstream processes the withdrawal twice. **This is the most common outbox bug, and it's not in the outbox — it's in the consumer.** Always enforce consumer-side idempotency on the event ID.

**3. Out-of-order publishing.** Multiple publisher instances, no aggregate-based partitioning. Event A (at T=1) and event B (at T=2) for user 123 end up in different Kafka partitions, consumed in either order. Fix: aggregate-based partitioning.

**4. Outbox table growing forever.** No cleanup job. Table reaches hundreds of GB, indexes rot, queries slow down, publisher falls behind, more rows accumulate. Positive feedback loop. Run a cleanup job from day one.

**5. Poison messages blocking the queue.** An event payload is malformed (bug in the write path before validation was added). The publisher retries forever, never advancing. Fix: after N retry failures, mark the row as `failed` (add a status column: `pending / published / failed`), alert, continue. Ops investigates manually. Similar to DLQ for Kafka consumers (Phase 2 Lesson 5).

**6. Publisher downtime not alerting.** Publisher dies, outbox fills up silently. No one notices for hours. Always monitor: unpublished row count, oldest unpublished row age. Alert when either exceeds threshold.

**7. Writer forgets to include the outbox insert.** Developer adds a new state-changing endpoint, writes DB updates, forgets outbox. Silent dual-write bug. Mitigation: code review discipline + architectural conventions (e.g., all state-changing operations go through a common `Transaction` helper that requires at least one outbox event).

**8. Schema skew between writer and consumer.** Writer adds a new field to the event payload. Consumer doesn't know about it. Depending on parser strictness, consumer either ignores it (fine) or errors (outage). Use a schema registry or versioned event envelopes.

---

## Where to Use It (and Where Not)

### Use outbox when:

- You have a service with a Postgres database that also publishes events.
- You cannot afford dual-write inconsistency (money, user balances, reservations, account state changes).
- Event volume is moderate (thousands per second per service).
- You want a clear audit trail of emitted events per business operation.

### Don't bother with outbox when:

- **The matching engine.** Already event-sourced through Kafka. The WAL / Kafka log is the source of truth; there's no separate DB state to keep in sync.
- **Pure read services.** No state changes, no events to publish.
- **Single-system operations.** If the operation only touches Postgres, you don't need outbox — DB transactions alone suffice.
- **Extremely high throughput.** At millions of events/sec, even a well-tuned outbox publisher may struggle. Consider CDC or event-sourced architectures instead.

### Outbox vs Event Sourcing vs CDC (Decision Framework)

**Event sourcing:** the event log *is* the source of truth. State is derived by replaying events. Use for the matching engine, accounting ledger, anywhere correctness depends on the exact order of events. (Phase 2 Lesson 4, Phase 3 Lesson 7.)

**Outbox:** the DB is the source of truth. Events are a derived reliable side-channel. Use for service-level state changes that need to notify downstream systems. The pattern of this lesson.

**CDC:** the DB is the source of truth, but event emission is handled by infrastructure (Debezium) reading the Postgres WAL. Use at very high scale when you don't want application code writing outbox rows. Operationally heavier.

**Rule of thumb:**
- Matching engine / ledger hot paths → event sourcing.
- Withdrawal, notification, saga orchestration, non-core services → outbox.
- Entire organization replicating all DB changes to a data warehouse → CDC.

---

## Integration with the CEX (What To Use It For)

Concrete places outbox applies in your exchange:

**Withdrawal service.** Debit balance + publish `withdrawal_requested` to the on-chain processor. Exactly the example from this lesson. Non-negotiable.

**Deposit credit.** On-chain processor sees confirmed deposit, the deposit service updates the user balance (Phase 3 Lesson 7) + emits `deposit_credited` for the notification service (send email) and analytics. Outbox.

**Saga orchestrator (Lesson 4 of SD101).** Multi-step vault deposit: reserve funds → mint shares → confirm. Each step is a local DB transaction + outbox event → next service. If any step fails, compensating transactions (also outbox-emitted) reverse earlier steps.

**Audit log.** Every sensitive operation (withdrawal approval, API key creation, admin action) writes to Postgres + emits an audit event to a long-term store. Outbox ensures no audit gap.

**Fee collection.** Trading fee collected in DB + emit `fee_collected` for analytics and accounting. Outbox.

**User settings changes.** Password change, 2FA toggle, whitelisted withdrawal addresses. DB update + emit event for security monitoring. Outbox.

**Order cancellation cascade.** User disconnects. Auth service emits `user_disconnected`. Subscriber service cancels all WebSocket subscriptions + orders. Each step is DB + outbox.

Basically: any time a service-level operation both changes DB state and needs to notify another service, outbox is the default.

---

## Summary: The Rules

1. **Any time code writes to two systems, you have a dual-write problem.** Outbox is the fix.
2. **Write state change + event in the same DB transaction.** Atomicity guaranteed by Postgres.
3. **Publisher runs separately, polls outbox, publishes to Kafka, marks as published.**
4. **Delivery is at-least-once.** Consumers must dedupe on event ID.
5. **Partition Kafka by aggregate ID for per-entity ordering.** Don't try for strict global order.
6. **Clean up the outbox.** Delete or partition-drop published rows after retention window.
7. **Monitor unpublished row count and age.** Alert on growth.
8. **Handle poison messages.** Mark failed rows, alert, don't block the queue.
9. **Event sourcing for the matching engine. Outbox for everything else that publishes.**

---

## Drill: The Outbox

**Q1. Failure enumeration.**
Without looking back at the lesson, write out every failure mode of naive dual-write between Postgres and Kafka. For each, state whether the system ends up in a correct state, an inconsistent state with money missing, or an inconsistent state with money duplicated. Minimum five distinct scenarios.

**Q2. Basic implementation.**
In Go with Postgres and any Kafka client:

- Create the `outbox` table with the partial index.
- Implement a `WithdrawalService.Withdraw(userID, amount)` that atomically debits balance and inserts an outbox row.
- Implement a `Publisher` goroutine that polls every 100ms, publishes to Kafka with the outbox UUID as event ID header, then marks rows as published.
- Implement a consumer that reads from Kafka, stores processed event IDs in Redis with 7-day TTL, and skips duplicates.

Integration test: submit 1,000 withdrawals. Verify all 1,000 Kafka messages delivered, zero duplicates processed by the consumer.

**Q3. Crash injection.**
Extend your harness to randomly kill the publisher mid-batch (use `os.Exit` or SIGKILL in a subprocess). After 1,000 withdrawals with 10 publisher crashes during the run, verify:

- All 1,000 outbox rows eventually marked `published = true`.
- Downstream processed exactly 1,000 distinct event IDs (dedup working).
- No withdrawal was lost.

If any of these fail, diagnose and fix.

**Q4. Duplicate detection.**
Disable consumer idempotency (skip the Redis dedup check). Run Q3's test again. Observe how many duplicates are processed. This is the attack window if you deploy without consumer idempotency. Report the count and compare to the number of publisher crashes.

**Q5. Ordering.**
Create a test where a single user issues 100 withdrawals quickly (for amounts 1, 2, 3, ..., 100). Use aggregate-partitioned Kafka keys. Verify the consumer processes them in order (1, 2, 3, ..., 100).

Now change the aggregate key to something random per event (breaking per-user ordering). Re-run. Observe out-of-order processing. Explain why.

**Q6. LISTEN/NOTIFY.**
Replace the polling publisher with a LISTEN/NOTIFY-based publisher. Add `NOTIFY outbox_new` to the write transaction. Measure end-to-end latency (outbox insert → Kafka ack → consumer process) for both approaches. Report the numbers. When would you choose polling over LISTEN/NOTIFY, and vice versa?

**Q7. Cleanup.**
Write the cleanup job that deletes outbox rows older than N days and `published = true`. Write a monitoring query that returns (oldest unpublished row age, total unpublished count, total published count). Set up a simple alert rule: fire if oldest unpublished > 60 seconds or unpublished count > 10,000.

Simulate a publisher outage (stop the publisher goroutine for 5 minutes while the write path keeps running). Verify the alert fires. Restart the publisher. Verify the alert clears as the backlog drains.

**Q8. Poison message.**
Introduce a bug: occasionally the write path inserts an outbox row with a malformed payload (e.g., truncated JSON). The publisher crashes when trying to marshal it. Verify that:

- Without mitigation, the publisher gets stuck on the poison row and all subsequent rows never publish.
- With mitigation (status column: `pending / published / failed`, retry limit), the poison row is marked `failed` after N attempts and the publisher continues with later rows.

**Q9. Reading.**
Read two sources:

- The "Microservices.io" page on the Transactional Outbox pattern (Chris Richardson's canonical write-up).
- A post-mortem of any distributed-system incident involving dual-write inconsistency (Shopify's blog on eventual consistency, Uber's engineering blog on consistency patterns, or any similar).

Answer:
- What trade-offs does Chris's description highlight that weren't in this lesson?
- In the post-mortem, was outbox the right fix? If it was used, what specifically went wrong? If it wasn't used, would it have prevented the incident?

Write 3-4 paragraphs.

---

*End of supplement. With the four phases plus this pattern, you have the core toolkit for building a correct, fast, reliable, safe centralized exchange in Go. The rest is operational maturity — which is its own multi-year apprenticeship. Build well.*
