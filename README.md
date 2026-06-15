# 101

Long-form self-study courses — gentle teaching prose followed by deliberately
harsh drills. Languages (Rust, Go), system design, and a build-it-yourself tour
of how a crypto exchange (CEX) matching engine actually works. Plus one working
Go code project.

## How to use this repo

- New to the material? Start with `go/go_101.md` if you want a language first,
  or `general/system_design_101.md` if you want the big picture; then work the
  `cex/` phases in order.
- Each course is the same shape: gentle teaching prose first, then drills that
  are deliberately much harsher than the prose.
- `go/go-matching-engine/` is the one runnable code project — clone, `go run`,
  and read its own `README.md`.

---

## `cex/` — CEX Matching Engine course (Go)

A four-phase course on building a centralized-exchange matching engine, from the
microsecond-latency core out to the public gateway, plus a supplement and
interview prep. Read the phases in order; the outbox supplement fits after
Phase 2; the blackbox doc is best last.

| Doc | What it covers |
| --- | --- |
| `phase1_lmax_disruptor_101.md` | **Phase 1 — Low latency & determinism.** LMAX Disruptor / ring buffers, mechanical sympathy, why ordinary queues and databases are too slow for a 10–100µs hot path. Assumes Go experience, zero low-level systems knowledge. |
| `phase2_persistence_and_ha_101.md` | **Phase 2 — Reliability.** Event sourcing + write-ahead log, Raft/Aeron high availability, copy-on-write snapshotting with `fork()`. Linux primitives taught inline. |
| `phase3_ledger_and_risk_engine_101.md` | **Phase 3 — Correctness & safety.** Double-entry bookkeeping (so "infinite money" bugs are impossible by construction), sub-10µs pre-trade risk checks, hot/cold wallet architecture. Accounting/margin/blockchain primers inline. |
| `phase4_api_and_gateway_101.md` | **Phase 4 — Exposing the engine.** WebSocket fan-out to 100k+ concurrent connections (where goroutine-per-connection dies and `epoll` enters), layered rate limiting and DDoS defense from edge to core. |
| `supplement_transactional_outbox_101.md` | **Supplement (after Phase 2).** The dual-write problem and the transactional outbox pattern (Postgres + Kafka) — for the "boring" services around the engine: withdrawals, notifications, sagas, audit. |
| `blackbox_interview_101.md` | **Interview prep.** How the whole exchange fits together end to end; CEX system-design interview structure, talking points, and the trade-off probes interviewers push on. |

## `general/`

| Doc | What it covers |
| --- | --- |
| `system_design_101.md` | A 9-lesson system design course — when (and when not) to scale, the compute/memory/IO bottlenecks, caching, replication, partitioning — with full drill Q&A and corrections. |

## `go/`

| Item | What it covers |
| --- | --- |
| `go_101.md` | Go from zero — concurrency (goroutines/channels, CSP), simplicity, and the discipline of "boring" code. Assumes ~1 year of programming in any language, zero Go. |
| `go-matching-engine/` | Working Go project: a lock-free SPSC ring buffer (cache-line padding, bitmask indexing, atomic sequences — no mutexes, no channels) plus a toy matching engine on top. Has its own `README.md`; `answer.md` holds the drill answers. |

## `rust/`

A three-phase Rust course, same teaching style. Do them in order.

| Doc | What it covers |
| --- | --- |
| `rust_101.md` | **Phase 1 — Memory safety without a GC.** Ownership, borrowing, lifetimes; the type system (enums, traits, generics, `Result`); concurrency basics (`Arc<Mutex<T>>`, channels). Assumes ~1 year of programming, zero Rust. |
| `rust_102.md` | **Phase 2 — Idiomatic Rust.** `impl Trait`, the `'static` lifetime, async/futures, macros (what `#[derive]` does), the small `unsafe` core, and the day-one crate ecosystem (serde, tokio, axum, reqwest). Assumes Phase 1. |
| `rust_103.md` | **Phase 3 — Low-latency Rust.** Microseconds and determinism for prop-trading systems: cache misses, branch prediction, the TLB, kernel-bypass NICs, no-allocation hot paths. Assumes Phases 1–2. |

---

Naming: a `_101 / _102 / _103` suffix is the phase/difficulty within one topic,
not a separate topic. Every course is structured as gentle explanation first,
then drills that are intentionally much harder than the prose.

## Contributing

This is a personal study repo, so contributions are handled through the automated agency pipeline.
Issues and pull requests are managed there, and changes flow through pull requests.
