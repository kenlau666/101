## drill 1

# q1

1. lock is slow when there is contention. its required for the kernel to make thread sleep and wake. this take 1-10ms.
2. false sharing always happen in lock. lock itself is also a variable. if core A is holding it and core B is trying to acquire it, a false sharing happen cuz core A would update the flag that its holding it and B has to fetch the lock from mem again. this frequently happen
3. i cant think of the third reason

# q2

1. read ringbuffer folder

# q3

1. division require more opeartion then bit operation. only if size is powers of 2, we can do bit operation.
   17 = 10001
   07 = 01001
   10001 & 01001 = 00001 = 1
   dont use 1000 cuz not the power of 2 and the reason above

# q4

1. read queue_bench_test.go

## drill 2

# q1

1.

# q2 (plan)

**Package layout**
- `engine/` — pure library, no I/O, no globals
  - `decimal.go` — string ↔ `int64` at scale 10⁸
  - `types.go` — `Event` (wire format), `Order` (internal), `Trade` (wire format), `Side`
  - `book.go` — sorted-slice order book + FIFO price levels
  - `engine.go` — `Engine` with `Step(Event) ([]Trade, error)`
- `cmd/engine/main.go` — thin NDJSON stdin→stdout driver

Reason for the split: the engine has zero coupling to I/O, so the same `Step` function can be driven by stdin (this drill), the ring buffer from drill 1 (Q3.6), or a Kafka consumer in production.

**Decimal handling — no floats anywhere**
- Scale = 10⁸ (`Scale int64 = 100_000_000`)
- `ParseDecimal(s string) (int64, error)`: split on `.`, parse each half byte-by-byte (no `strconv.ParseFloat`), pad fractional part to 8 digits, combine. Reject >8 fractional digits, multiple dots, negatives, non-digit chars.
- `FormatDecimal(x int64) string`: integer part → `strings.Builder`; if fractional remainder > 0, render 8 digits left-padded then trim trailing zeros. So `5_000_050_000_000` → `"50000.5"`. Canonical and deterministic — `"50000.50"` and `"50000.5"` round-trip to the same output, which is what Q3's diff harness needs.

**Order book — sorted slices, not maps**
- Two `book` structs: bids (sorted *descending* by price, best at index 0) and asks (sorted *ascending*, best at index 0).
- Each `book.levels` is `[]*priceLevel`; each `priceLevel.Orders` is `[]*Order` in arrival order (FIFO = time priority).
- Insert: `sort.Search` to find position, splice the new level in. O(log L) lookup, O(L) shift on insert.
- A `map[string]*Order` exists only for O(1) cancel lookup. **It is never iterated** — matching only walks the sorted slices — so map randomness can't leak into output.

**Matching algorithm (`Step` → `place` path)**
1. Parse side, price, quantity. Reject duplicate IDs.
2. Build a `taker` Order. Pick the opposite book.
3. Loop: while taker has remaining qty AND opposite book has a `best()` level AND `crosses(side, taker.Price, best.Price)`:
   - Walk the level's FIFO front-to-back. For each maker, match `min(taker.Qty, maker.Qty)`, emit a `Trade` (price = maker's resting price, timestamp = taker's), decrement both. Drop fully-filled makers from the FIFO and from the orders map.
   - When the level's FIFO drains, `removeBest()` the level.
4. If taker has qty left after sweeping, insert it into its own side via `findOrInsert(price)` and append to the FIFO. This is what gives **price-time priority** for free: makers always rest, takers always sweep first.

**Cancel path**
- Look up by ID in the orders map. If missing, no-op (orders may have been fully filled — common in real exchanges).
- Otherwise locate the price level via `sort.Search` on the correct book, splice the order out of the FIFO, drop the level if it's now empty.

**Determinism enforcement (the whole point)**
- No `float64`/`float32` anywhere — grep -r returns zero hits in `engine/`.
- No `time.Now()` anywhere — every timestamp comes from `Event.Timestamp`. Trades inherit the taker's timestamp.
- No map iteration in the matching path. The orders map is a lookup table, not a control structure.
- `Step` is single-threaded by contract; no goroutines spawned inside the engine, no shared mutable state.
- `Trade` JSON encoding via `encoding/json` is deterministic (struct field order is declaration order).
- Output `[]Trade` slice is reused across `Step` calls (zero-alloc-friendly for drill 3) but the test helper deep-copies it.

**Tests to write alongside**
- Decimal round-trip + error cases (8-digit cap, multiple dots, negative).
- Match scenarios: rest-no-cross, simple full match, partial fill on taker, partial fill on maker, FIFO time priority at same price, price priority across levels, no-cross on price mismatch, fractional quantities.
- Cancel: resting order, unknown ID no-op, duplicate place rejected.

This plan delivers everything Q3's harness needs to verify (run twice, byte-identical output) and sets up Q4's sabotages to fail the harness in the predicted ways (float drift, map iteration order, clock noise, goroutine race).

# q5

1. **Identical state without coordination.** Kafka partitions give every consumer the same ordered, immutable sequence of events. If both replicas run the same deterministic state machine — same code, same input order, no `time.Now()`, no map iteration, no floats, no goroutine races — then `state = fold(initial, events[0..n])` is a pure function of `n`. Same `n` ⇒ byte-identical state on both replicas. No replication protocol, no consensus, no chatter between A and B; the partition itself is the source of truth.

2. **B is 100 events behind A.** Nothing is wrong — B is just at offset `N-100` while A is at offset `N`. Determinism guarantees that when B processes those 100 events, it will arrive at exactly A's current state. Lag is a latency concern (stale reads from B), not a correctness concern. This is why "catch up by replaying the log" works at all.

3. **B crashes and restarts.** B reads its last committed offset from durable storage (or from a snapshot's offset metadata), then resumes consuming from Kafka at that offset. Because every event from that offset onward produces a deterministic state transition, B replays its way back to consistency with A — same fold, same result. A snapshot just lets it skip ahead instead of replaying from offset 0; the determinism property is what makes both the snapshot and the replay safe.
