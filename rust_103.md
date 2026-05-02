# Rust — Phase 3 Course (Low-Latency Edition)

> Microseconds, Determinism, and the Discipline of Going Fast.
> Three lessons. Same teaching style as before: gentle prose, harsh drills.
> **Assumes Phases 1 and 2.** This phase is about pushing Rust to the limits prop trading firms care about.

---

## Before You Start: What Low-Latency Really Means

Phase 1 taught you Rust. Phase 2 made you idiomatic. Phase 3 is about a specific industry — proprietary trading firms — and the very specific kind of code they write. You will build **matching engines**, **market data handlers**, **order gateways**, and **strategy runtimes**. The thing they all share: they are extremely sensitive to latency.

A few numbers to anchor you. When you click a link in your browser, the page renders in roughly 100 milliseconds. That feels instant. A web service that handles "fast" requests responds in 10–50 milliseconds. A high-quality online game targets 16ms per frame. None of this is the world we're in.

A prop trading firm's matching-engine response latency is in **microseconds**. A strategy that reacts to a market event has roughly **5–20 microseconds** to make and send its decision before someone else has already done it. A market data parser that takes 500 nanoseconds *per message* is considered slow. A `printf` is unthinkable in the hot path. A heap allocation in the hot path is a fireable mistake.

The reason is brutal economics. Two firms see the same market event at the same time. The one whose order arrives at the exchange first wins the trade. Whichever firm is 5 microseconds slower walks away with nothing — every time. Over a year, this compounds into hundreds of millions of dollars. Speed is not a vanity metric; it's the entire business.

To work at these speeds you have to think about things that web developers never consider:

* CPU pipeline stalls and cache misses.
* The TLB and how virtual-to-physical address translation hits performance.
* Branch prediction.
* The kernel and why every syscall is a problem.
* Network card NICs and how to talk to them without going through the OS.
* NUMA topology — which RAM is "near" which CPU core.
* Jitter sources: GC pauses (Rust avoids these, mostly), interrupt handling, page faults, hyperthreading.
* Allocator behaviour, memory fragmentation.
* Atomic memory ordering at the level individual machine instructions.

This phase teaches the conceptual core, with Rust as the vehicle. The same ideas apply to C++ low-latency code, but Rust gives us additional tools (and constraints) that change how you write the patterns. You'll build smaller pieces here than in Phase 2 — a single SPSC queue, a single message parser, a single hot loop — but you'll measure them obsessively. The discipline of "measure first, decide later" is what separates professional low-latency engineers from people who *think* their code is fast.

This lesson is also brutally honest. Most "performance tips" you'll see online (use `Vec::with_capacity`, prefer `&str` to `String`) are real but small. The interesting wins are in the architecture, the data layout, and the ten-line hot loop you've measured to death. We'll work in that register.

If you've been through Phases 1 and 2 and the drills, you have the prerequisites. If you haven't, the language details below will whoosh past you. Go back and fight the borrow checker more.

Plan on 12–20 hours per lesson with drills. The drills involve real benchmarking; setting up the measurement environment alone is half the work the first time you do it.

---

## A Phase 3 Glossary

* **Latency** — time per operation. Usually measured in nanoseconds (ns) or microseconds (μs).
* **Throughput** — operations per second. The number you brag about.
* **p50, p99, p99.9** — percentile latencies. p99 = 99% of requests complete faster than this. The tail (p99.9, p99.99) is where the bodies are buried.
* **Jitter** — variance in latency. Two systems with identical median latency can have wildly different jitter. Trading firms care about jitter as much as median.
* **Tail latency** — the slowest 0.1% or 0.01% of requests. Your competitor's worst is often better than your average.
* **Hot path** — the code that runs on every market event. Optimised obsessively.
* **Cold path** — startup, error handling, configuration. Optimised for clarity, not speed.
* **Cache line** — the unit of memory the CPU loads at once. 64 bytes on modern x86 and ARM.
* **L1, L2, L3** — three levels of CPU cache, fastest to slowest.
* **Cache miss** — the data you wanted wasn't in cache; CPU stalls waiting for RAM.
* **TLB** — Translation Lookaside Buffer. Caches virtual-to-physical address mappings. TLB miss = page table walk = nanoseconds wasted.
* **Branch predictor** — CPU hardware that guesses which way an `if` will go. Correct guesses are free; wrong guesses cost 10-20 cycles.
* **Pipeline stall** — the CPU pipeline empties because an instruction can't proceed (cache miss, branch mispredict, dependency).
* **Out-of-order execution** — modern CPUs reorder instructions for throughput. The architectural state always *appears* in-order; the physical execution doesn't have to be.
* **NUMA** — Non-Uniform Memory Access. Multi-socket servers have different RAM attached to different CPU sockets. Accessing your local NUMA node is fast; the remote one is much slower.
* **Hyperthreading / SMT** — one physical core runs as two logical cores. They share execution units. In low-latency code this is usually disabled.
* **Atomic operation** — a CPU instruction that completes indivisibly. The basis of lock-free programming.
* **Memory ordering** — the rules about what operations on different memory locations can be reordered. Rust exposes this through `Ordering::Relaxed`, `Acquire`, `Release`, `AcqRel`, `SeqCst`.
* **Lock-free** — an algorithm where no thread can block another indefinitely. Stronger: **wait-free**, where every thread completes in a bounded number of steps.
* **SPSC, MPSC, MPMC** — Single/Multi Producer, Single/Multi Consumer. Queue topologies, each with different guarantees and costs.
* **Kernel bypass** — networking that talks directly to the NIC, skipping the OS network stack. DPDK, Solarflare/AOE, RDMA.
* **Userspace networking** — same idea, more general term.
* **PTP / hardware timestamping** — measuring time-of-flight at sub-microsecond precision.
* **Tick** — the smallest price increment on an instrument. Internally, prices are integer ticks, never floats.
* **Order book** — the data structure tracking all open orders for an instrument, sorted by price.
* **Matching engine** — the program that pairs bids with asks and produces trades.
* **FIX** — Financial Information eXchange. A text-based wire protocol for orders. Verbose; the firms that care about latency use binary alternatives (ITCH, OUCH, SBE, custom).

---

# Lesson 7: The Performance Mental Model

## 7.1 Why This Lesson Exists

A junior engineer reading their first low-latency codebase has a predictable reaction: "Why is this code so weird? Why does it not use HashMap? Why are there fixed-size arrays everywhere? Why is there a 200-byte struct with 100 bytes of explicit padding? Why does this function avoid a perfectly good iterator?"

The answer is always the same: someone benchmarked, found the obvious version was 5x slower than the weird version, and the weird version shipped. The "weirdness" is the cumulative residue of years of measurement. None of it is theoretical. None of it is taste. It's all empirical responses to data the team gathered.

To learn this domain, you have to learn the *why* behind the weirdness. Three things have to become reflexes:

1. **A working mental model of CPU performance.** Where does time go in a hot loop? What is the CPU actually doing per nanosecond? You can't optimise what you can't reason about.
2. **The discipline of measurement.** Profilers, micro-benchmarks, percentiles, statistical noise. "I think this is faster" is worthless; "I have a 10,000-iteration measurement with confidence interval ±5ns" is the unit of useful conversation.
3. **A taste for what's fast and what isn't.** When you read a function, you should be estimating its cost in nanoseconds before you measure it. After enough measurement, your estimates get reliable.

This lesson covers all three. It does not cover lock-free concurrency or kernel bypass — those are Lessons 8 and 9. The point of this lesson is to build the mental machinery you'll use throughout Phase 3 and your career in low-latency engineering.

## 7.2 What a CPU Is Actually Doing

You probably learned in school that a CPU executes one instruction at a time. This is wrong in three important ways. Every modern CPU:

1. **Pipelines** instructions: at any moment, 10-20 instructions are in flight at different stages. Fetch, decode, register-rename, schedule, execute, retire — each in a different cycle.
2. **Executes out-of-order**: it'll complete a later instruction before an earlier one if the data is ready, as long as the architectural state ends up correct.
3. **Speculates**: when the CPU sees a branch (`if`, function call, return), it predicts which way the branch will go and starts executing the predicted side. If the prediction is wrong, it throws away that work.

The implication: a hot loop's speed is mostly determined not by the *number* of instructions, but by:

* **How well the CPU can pipeline them.** Independent instructions pipeline well. Long chains of dependencies (A depends on B depends on C) don't.
* **Whether the data is in cache.** A cache miss to main memory stalls the pipeline for ~100 cycles, during which the CPU might do nothing useful.
* **Whether branches predict well.** A consistently-taken branch (always-true) costs zero. A 50/50 random branch costs ~10 cycles per misprediction.

A 1-million-iteration loop where every iteration takes a cache miss runs in ~100ms. The same loop where everything's in cache runs in <1ms. Same code, 100x difference, no source-level change.

### The latency numbers, again, with feeling

Phase 1 / the LMAX reference covered these. They're so important they need repetition. Memorise them. Test yourself.

```
Operation                                 Approx. Latency
─────────────────────────────────────────────────────────
Single CPU instruction (best case)        0.3 ns
Branch mispredict                         5 ns
L1 cache hit                              1 ns
L2 cache hit                              4 ns
L3 cache hit                              12 ns
Main memory access (L3 miss)              80-100 ns
Atomic operation, uncontended             5-15 ns
Atomic operation, contended               40-200 ns
Mutex lock (uncontended)                  20-25 ns
Mutex lock (contended, kernel involved)   1-10 μs
Context switch (kernel)                   1-10 μs
Syscall (cheap, e.g. clock_gettime VDSO)  20 ns (vDSO) or 100-200 ns
Syscall (real, e.g. write)                500 ns - 2 μs
Network: RTT same data center             100-500 μs
Network: RTT same continent               20-80 ms
Disk: NVMe random read                    20 μs
Disk: spinning random read                5-10 ms
```

Two big takeaways:

**RAM is shockingly slow.** L1 to RAM is 100x. If your code makes the CPU go to RAM all the time, you've lost. Most low-latency optimisation is fundamentally about keeping data in cache.

**Syscalls and kernel involvement are catastrophes.** A single `write()` to a socket can take longer than 1000 cache hits. Lesson 9 will get into kernel-bypass; in this lesson, just internalise that "going through the kernel" is one of the worst things you can do in a hot path.

### Cache lines and the layout of your data

A cache line is 64 bytes. The CPU never reads less than that from RAM at a time. When your code touches one byte at memory address X, the CPU loads the 64-byte chunk containing X into cache. If you read the next 63 bytes, they're already there — free.

This has *enormous* consequences for data layout. Three rules of thumb:

**Rule 1: Pack the things you read together.** If a function reads only `order.price` and `order.quantity`, those two fields should be adjacent in memory. The struct should be small enough that the working set fits in L1.

**Rule 2: Separate the things written from different threads.** If thread A writes `producer_seq` and thread B writes `consumer_seq`, those two values should be on different cache lines. Otherwise every write by A invalidates B's cache line, and vice versa. This is **false sharing**, and it's a 10x performance killer in concurrent code.

**Rule 3: Sequential access beats random access.** Iterating an array of 1M integers reads 1M / 16 = 62,500 cache lines (each holding 16 ints), with the hardware prefetcher loading them in parallel. Walking a linked list of 1M nodes incurs 1M cache misses because the next node is at a hardware-unpredictable address. Same data, 10x difference.

Visually, what a struct's memory layout looks like:

```
struct Order { id: u64, price: i64, qty: i64, side: u8, /* padding */ }
                    8         8         8       1     7
└──────────────────────────────────────────────────────┘
                  32 bytes total

This fits half of one cache line. Two of them fit per cache line.
Reading 4 of these in sequence: 2 cache lines = 2 misses worst case.
```

In Rust, you control layout via `#[repr(C)]` or `#[repr(packed)]` (don't use packed casually; it loses the compiler's alignment optimisations). Default Rust layout is `#[repr(Rust)]`, which lets the compiler reorder fields for size — usually fine, but for performance-critical layouts you want `#[repr(C)]` and explicit field order.

```rust
#[repr(C)]
struct Order {
    id: u64,        // offset 0
    price: i64,     // offset 8
    qty: i64,       // offset 16
    side: u8,       // offset 24
    _pad: [u8; 7],  // offset 25-31 — explicit padding to 32 bytes
}
```

Now you can rely on `mem::size_of::<Order>() == 32` across compilers, platforms, and Rust versions.

## 7.3 The Discipline of Measurement

You will be wrong about performance more often than you're right. So will I. So will every senior engineer at every prop firm. Performance is empirical: measure, don't argue.

### Tools

* **`criterion`** — the standard Rust benchmarking crate. Statistical rigor, automatic regression detection, multi-sample, confidence intervals. Use this for micro-benchmarks.
* **`perf`** — Linux's profiler. `perf stat` shows cycles, instructions, cache misses, branch misses. `perf record` + `perf report` shows where time is spent.
* **`flamegraph`** (the `cargo-flamegraph` crate) — generates a visual breakdown of CPU time. Indispensable for "where is my program *actually* spending time?"
* **`hyperfine`** — wall-clock benchmarks for whole programs.
* **`hdrhistogram`** — record latencies in a histogram and report percentiles. The right way to measure tail latency.

You should install all of these now, before going further. The drills assume you have them.

### A tiny criterion benchmark

```rust
// In Cargo.toml:
// [dev-dependencies]
// criterion = "0.5"
// [[bench]]
// name = "my_bench"
// harness = false

use criterion::{black_box, criterion_group, criterion_main, Criterion};

fn fibonacci(n: u64) -> u64 {
    if n < 2 { n } else { fibonacci(n-1) + fibonacci(n-2) }
}

fn bench_fib(c: &mut Criterion) {
    c.bench_function("fib 20", |b| b.iter(|| fibonacci(black_box(20))));
}

criterion_group!(benches, bench_fib);
criterion_main!(benches);
```

Run with `cargo bench`. Criterion will:

* Run a warmup phase.
* Run hundreds of samples, each measuring many iterations.
* Compute mean, standard deviation, outliers.
* Compare to previous runs (it stores history).
* Report something like `fib 20 time: [21.234 µs 21.301 µs 21.370 µs]` — that's [lower CI, point estimate, upper CI].

`black_box` is critical. It tells the compiler "I might be using this value, don't optimise it away." Without `black_box`, the compiler will see your benchmark calls a pure function whose result is discarded, and elide the whole thing. Your bench will report 0.5 nanoseconds per call. You'll get excited. You'll be measuring nothing. Always wrap inputs (and sometimes outputs) in `black_box`.

### Latency vs throughput

Two different questions, two different measurements:

* **Throughput**: how many ops/sec can I do? Drive the system at full rate, measure aggregate completion. `criterion` does this naturally.
* **Latency**: how long does *one* op take, given everything else is idle? Different measurement: dispatch one op, time until response, repeat at controlled intervals.

A common mistake: measuring throughput and reporting it as latency. If 1M ops takes 10ms, throughput is 100M/sec, but the per-op latency under that load is *not* 10ns — that's an average, and the system's percentiles can be much worse, especially under contention.

For any real low-latency benchmark, you want a histogram of per-op latencies, with reported percentiles:

```rust
use hdrhistogram::Histogram;
use std::time::Instant;

let mut hist = Histogram::<u64>::new(3).unwrap();   // 3 = significant figures

for _ in 0..1_000_000 {
    let start = Instant::now();
    perform_one_op();
    let elapsed_ns = start.elapsed().as_nanos() as u64;
    hist.record(elapsed_ns).unwrap();
}

println!("p50: {} ns", hist.value_at_quantile(0.50));
println!("p99: {} ns", hist.value_at_quantile(0.99));
println!("p99.9: {} ns", hist.value_at_quantile(0.999));
println!("p99.99: {} ns", hist.value_at_quantile(0.9999));
println!("max: {} ns", hist.max());
```

This is the format every serious latency report takes. p50 alone is not enough; p99.9 is what gets you fired in trading.

### Coordinated omission

The single biggest pitfall in latency measurement, and almost everyone makes it the first time.

Suppose you measure 1M operations at 1 microsecond per op, in a tight loop. Wall clock: 1 second. Latency p50: ~1 μs. Latency p99.9: also ~1 μs.

Now suppose during that second, the system stalled for 100ms. What do you record? The classic naive approach: you take a timestamp before each op, do the op, take a timestamp after, record the difference. During the 100ms stall, *no measurements happen* — you're stuck inside one op. You record 999,999 fast samples plus one 100ms sample. Your p99.9 looks great because the bad outliers are missing.

This is **coordinated omission**: the measurement omitted the very samples the user would care about. The fix is to schedule operations on a fixed cadence (e.g. one every 1 μs), measure each from its *intended* start time, not its actual start time. If the system stalls for 100ms, all the operations that *should* have started during that stall now contribute their (tens of milliseconds) of latency to the histogram.

Tools that do this correctly: hdrhistogram (with `record_corrected`), wrk2 (load testing tool by Gil Tene who coined the term). If you're rolling your own measurement, read Gil's article ("How NOT to Measure Latency") — it's the Rosetta Stone.

### Microbenchmark traps

The list of ways to make a microbenchmark lie is long:

* **Compiler elides your code** because the result is unused → use `black_box`.
* **CPU caches everything** because you're calling the same function with the same input on the same data → vary inputs, dirty caches between iterations.
* **Branch predictor learns the pattern** → use random or anti-patterned inputs.
* **CPU frequency scaling** boosts during benchmarks then drops → pin frequency (`cpupower frequency-set`).
* **Power saving** parks idle cores → run on a busy-pinned core.
* **Other processes** steal cycles → idle the box, or pin to an isolated core.
* **Hyperthreading** shares execution units between siblings → pin to physical cores, disable HT in BIOS for serious work.
* **NUMA** — your memory might be on a remote node → control allocation with `numactl`.

A typical professional setup runs benchmarks on a dedicated Linux box with HT off, frequency pinned, the trading process pinned to a specific core (with `taskset`), interrupts moved to other cores, the kernel built with `CONFIG_NO_HZ_FULL`, etc. You don't need all of this for a learning project, but you should know what's going on at the high end.

## 7.4 Reading Assembly: Less Scary Than You Think

This is a skill every low-latency engineer eventually needs. Not all the time, just for the moments when "this looks like 4 instructions in source but the benchmark shows 200 cycles, what's actually happening."

`cargo asm` is the go-to tool. Install with `cargo install cargo-show-asm`. Given:

```rust
pub fn add(a: i64, b: i64) -> i64 { a + b }
```

Run `cargo asm --rust mycrate::add`:

```
mycrate::add:
    lea rax, [rdi + rsi]
    ret
```

That's it. The function's two parameters arrive in `rdi` and `rsi` (System V calling convention on Linux x86-64). `lea rax, [rdi + rsi]` puts their sum into `rax` (the return register). `ret` returns. Two instructions, ~0.5ns when called, often inlined.

A more interesting one:

```rust
pub fn sum(v: &[u64]) -> u64 {
    v.iter().sum()
}
```

The assembly is dozens of instructions long, but the inner loop typically becomes 2-4 instructions of vectorised AVX additions, processing 4 or 8 u64s at a time. You don't need to read every line; you need to be able to answer "did the compiler vectorise this? did it inline? did it eliminate this bounds check?"

### Bounds checks

In safe Rust, every array index does a bounds check: "is `i < arr.len()`?" If yes, proceed; if no, panic. This is a branch, and the branch can prevent vectorisation.

For most code, the cost is invisible — the branch predictor nails it 100% of the time, and the optimiser sometimes proves the bound away entirely. For tight numeric loops, you might want to confirm. Two tricks:

**Trick 1: iterate, don't index.** `for x in arr.iter()` does no bounds checks; the iterator's design proves them away. Idiomatic Rust naturally avoids the issue.

**Trick 2: prove the bound to the compiler.** If you write `assert!(i < arr.len())` once before the loop, the compiler can sometimes carry that proof into the loop body and elide subsequent checks. This is a finicky optimisation and you confirm it in the assembly.

**Trick 3 (escape hatch): `get_unchecked`.** `arr.get_unchecked(i)` is unsafe and skips the check. Use only after you've confirmed the cost matters and you're upholding the invariant manually. Don't reach for it casually.

## 7.5 Where Time Actually Goes

Where do nanoseconds go in a typical hot path? In rough order of frequency:

1. **Cache misses.** By far the biggest. Reading data that wasn't in cache, especially when the data structure isn't designed for locality.
2. **Branch mispredicts.** Common in code with data-dependent branching: `if order.side == Side::Buy { ... } else { ... }` where buys and sells alternate randomly.
3. **Hashmap operations.** A `HashMap` lookup is "fast" by application standards (~30-50ns) but glacial by HFT standards. Replace with arrays-indexed-by-id where possible.
4. **Allocation.** Even a fast allocation is 50-100ns. In a 5-microsecond budget, that's 1-2% of your time. Pre-allocate.
5. **Atomic operations.** Each is 5-15ns uncontended. Cheap individually; expensive in bulk.
6. **System calls.** Avoid in the hot path entirely.
7. **Arithmetic and logic.** Almost free. Stop worrying about whether `x*2` or `x<<1` is faster.

Notice what's *not* high on this list: integer arithmetic, function call overhead (after inlining), iteration. Beginners obsess over these and gain nothing. Senior engineers obsess over data layout and cache behaviour, where the wins actually are.

### A worked optimisation

Let's see this in practice. Suppose we have:

```rust
struct Order {
    id: u64,
    price: i64,
    quantity: i64,
    side: Side,
    user_id: u64,
    metadata: String,    // <-- heap allocation
    notes: Vec<u8>,      // <-- heap allocation
    timestamp: u64,
}

fn total_value(orders: &[Order]) -> i64 {
    orders.iter().map(|o| o.price * o.quantity).sum()
}
```

Looks fine. Looks idiomatic. Why is it slow?

* `Order` is ~80 bytes (the two heap allocations are 24 bytes each on the stack).
* `total_value` reads only `price` and `quantity` — 16 bytes per order.
* Per cache line (64 bytes), we fit *less than one Order*.
* So iterating 10,000 orders is ~10,000 cache lines = ~1 megabyte of data fetched. Most of which we don't use.

A "data-oriented" rewrite separates the hot fields:

```rust
struct OrderHot {
    id: u64,
    price: i64,
    quantity: i64,
    side: Side,
    timestamp: u64,
    _pad: [u8; 7],     // total: 32 bytes, exactly half a cache line
}

struct OrderCold {
    user_id: u64,
    metadata: String,
    notes: Vec<u8>,
}

struct Orders {
    hot: Vec<OrderHot>,
    cold: Vec<OrderCold>,
}

fn total_value(orders: &Orders) -> i64 {
    orders.hot.iter().map(|o| o.price * o.quantity).sum()
}
```

Now `total_value` walks `hot` only, two orders per cache line. 10,000 orders is 5,000 cache lines = ~320 KB, fits comfortably in L2. The benchmark gets 3-5x faster, depending on input size. Same algorithm, same correctness, just a layout change.

This pattern — hot/cold split — is foundational. Find the fields used in the hot path, put them in a tight struct, put the rest somewhere else. Keep them parallel-indexed if you need to correlate.

## 7.6 Fixed-Point Arithmetic

We mentioned in Phase 1 / the LMAX reference: never use floats for money. Repeat with feeling: never use floats for money. Or for prices. Or for quantities.

Why: `f64` arithmetic is non-associative (`(a+b)+c != a+(b+c)` for some values), platform-dependent in subtle ways, and accumulates rounding errors. Two replicas computing the same trade can diverge by a single bit, which over time turns into "your books don't balance" — a regulatory disaster.

Solution: integer arithmetic with an explicit scale. Pick a scale (typically 10⁶, 10⁸, or 10⁹) and represent prices in that scale. `$50,000.12345678` becomes `5000012345678` as an `i64` with scale 10⁸.

```rust
const PRICE_SCALE: i64 = 100_000_000;   // 10^8

fn from_decimal(d: &str) -> i64 {
    // parse "50000.12345678" into 5000012345678. Real impl needs care
    // around precision, signs, etc. Use a crate (rust_decimal, fixed) for production.
    todo!()
}

fn to_decimal(ticks: i64) -> String {
    let whole = ticks / PRICE_SCALE;
    let frac = ticks.abs() % PRICE_SCALE;
    format!("{}.{:08}", whole, frac)
}
```

Multiplications need care to avoid overflow:

```rust
// price * quantity, both with PRICE_SCALE.
// Naive: result is in PRICE_SCALE^2. Divide by PRICE_SCALE to fix.
// But PRICE_SCALE * PRICE_SCALE overflows i64 — use i128 intermediate.

fn notional(price: i64, quantity: i64) -> i64 {
    let prod = (price as i128) * (quantity as i128);
    (prod / PRICE_SCALE as i128) as i64
}
```

For libraries with this done well, see `rust_decimal` (arbitrary-precision decimal) and `fixed` (typed fixed-point). For matching engines that need to be deterministic across replicas, you typically write your own simple `i64` math because you control every operation — which is more auditable than depending on a library.

## 7.7 Allocation Discipline

In the hot path: zero allocations. Period.

This is the single biggest cultural gap between application Rust and HFT Rust. Application Rust freely allocates — every `String`, every `Vec`, every `Box::new`. This is fine and idiomatic, and any GC-language refugee finds it natural. In HFT code, every allocation is a problem because:

* Each allocation takes 30-100ns even at the fast path.
* Allocations cause cache pollution.
* Allocations stress the allocator's internal locks.
* Long-running programs fragment memory; allocation latency drifts up over time.
* Even Rust's lack of GC doesn't save you — allocators have their own latency variability.

Techniques:

**Pre-allocate at startup.** Decide your maximum order book size, your maximum number of in-flight orders, your maximum message buffer. Allocate it all on day zero. Never grow.

```rust
struct Engine {
    orders: Vec<Order>,           // preallocated to MAX_ORDERS
    free_slots: Vec<u32>,         // free list of indices
    book: OrderBook,
    output_buffer: Vec<u8>,       // preallocated send buffer
}

impl Engine {
    fn new() -> Self {
        let mut e = Engine {
            orders: Vec::with_capacity(MAX_ORDERS),
            free_slots: (0..MAX_ORDERS as u32).collect(),
            book: OrderBook::new(MAX_PRICE_LEVELS),
            output_buffer: Vec::with_capacity(64 * 1024),
        };
        // Pre-fill orders with default values so the Vec has the same
        // backing memory throughout.
        for _ in 0..MAX_ORDERS { e.orders.push(Order::default()); }
        e
    }
}
```

**Use indices, not pointers.** A `u32` index into a pre-allocated array is 4 bytes, cache-friendly, and the GC ignores the array (well, Rust has no GC, but reborrowing rules are simpler). A `Box<Order>` is 8 bytes, cache-unfriendly, and each one is a separate allocation. Always prefer the index.

**Reuse buffers.** If you build a message in a `Vec<u8>` and send it, don't drop the Vec — clear it (`vec.clear()` keeps the capacity) and reuse it next message.

**Don't trust the standard library.** `format!("{:?}", x)` allocates. `String::new()` doesn't, but `string.push_str(other)` might. `HashMap::insert` might trigger a rehash, which allocates. Profile, measure, replace.

**Fixed-size buffers everywhere.** `[u8; 1024]` instead of `Vec<u8>` for known-bounded data. `arrayvec::ArrayVec` for "Vec but stack-allocated with a fixed cap" — common in HFT codebases.

## 7.8 The "Why Are You Using HashMap" Conversation

Junior engineer writes:

```rust
let mut orders_by_id: HashMap<u64, Order> = HashMap::new();
// ... insert, lookup, etc. ...
```

Senior reviewer says: don't.

Why? HashMap operations are 30-50ns each. In a 5-microsecond budget, that's 1% per operation. If you do 10 of them per event, you've spent 10% of your budget on hash table lookups.

Alternatives:

**For dense ID spaces (small range of IDs, all used):** an array indexed by ID.

```rust
const MAX_ORDER_ID: usize = 1_000_000;
let mut orders: Vec<Option<Order>> = vec![None; MAX_ORDER_ID];
orders[id as usize] = Some(order);
let o = orders[id as usize].as_ref();
```

This is one cache line away. ~3-5ns per access, including the cache miss.

**For sparse IDs (large ID range, few used):** swap-back vec + a small index map. Or accept the HashMap cost and use a faster hash:

```rust
use ahash::AHashMap;   // significantly faster than std HashMap
```

Or `FxHashMap` from the `rustc-hash` crate, which is even faster but with weaker hash quality (fine for trusted internal IDs; not for adversarial inputs).

**For sorted lookups (price levels in an order book):** sorted Vec with binary search, or B-tree. Definitely not HashMap.

**For "I just need to remember a few things":** a tiny array with linear scan beats a HashMap up to ~10-20 entries because of cache and branch prediction.

The point isn't "HashMap is bad." It's that HashMap is the universal answer in application code, and HFT code lives in a different cost regime. Always ask: is there a domain-specific structure that does better?

## 7.9 Determinism, Once More

Phase 1's LMAX reference covered this in depth. Quick recap because it's central to HFT:

A matching engine must be deterministic — same input events, same output trades, byte-for-byte, every time. This enables:

* Replay for debugging.
* Hot-standby replicas in lockstep.
* Audit logs.

In Rust, the same gotchas apply as in any language:

* Don't use `std::time::Instant` in the engine — time comes from input events.
* Don't iterate `HashMap` in output-affecting code (`HashMap` iteration order is randomised).
* Use `BTreeMap` or sorted vecs for any iteration that affects output.
* Avoid floats for money.
* Single-threaded for the matching core.

Rust adds a few:

* `HashMap`'s default hasher uses random seeds per process for DoS resistance — even insertion order is non-deterministic across runs. Use `BTreeMap`, or supply a fixed seed.
* Some Vec methods (`sort_unstable_by`) don't guarantee tie-breaking order. Use `sort_by` for stable order.
* iterator adapters that are documented as "no order guarantee" — don't use them in deterministic contexts.

A test you should always have: feed the engine the same input twice, diff the outputs byte-for-byte. They must match. Run this in CI on every commit. The day you have to debug a non-determinism bug after the fact is the day you're already in trouble.

## 7.10 Summary: The Rules

1. **Memory hierarchy dominates everything.** RAM is 100x slower than L1. Optimise for cache, not instruction count.
2. **Cache lines are 64 bytes.** Pack hot fields together. Separate cross-thread writers.
3. **Measure before optimising.** Use `criterion` for micro-benchmarks, `perf`/`flamegraph` for whole-program.
4. **Always `black_box` benchmark inputs.** Otherwise the compiler elides your work.
5. **Report percentiles, not means.** p99.9 is what gets you fired.
6. **Watch for coordinated omission.** Naive timing under-reports tail latency.
7. **Fixed-point integers for money.** Never `f64`. Never.
8. **Zero allocations in the hot path.** Pre-allocate everything at startup.
9. **Indices beat pointers.** `u32` into a pool, not `Box<T>` per object.
10. **Hot/cold split your structs.** Pack the read-in-the-hot-path fields tightly; relegate the rest.
11. **Reading assembly is a skill, not a magic art.** Use `cargo asm`. Confirm vectorisation. Confirm bounds-check elision.
12. **HashMap is too slow for the hot path.** Arrays indexed by ID, sorted vecs, BTreeMap, or specialised hashers.
13. **Determinism is a property of the design, not an accident.** Test it byte-for-byte in CI.

## 7.11 Drill 7

The drills in this phase require setup. You need a Linux box (or a Mac, with caveats — perf and frequency-pinning are Linux-specific) where you can run benchmarks reliably. WSL2 works for early drills; for the latency-sensitive ones, get to bare metal.

Install: `criterion` (cargo dep), `cargo-show-asm`, `cargo-flamegraph`, `hyperfine`, `perf` (on Linux: usually `linux-tools-common` or your distro equivalent), `hdrhistogram-rs`.

**Q1. The cache-line proof.**

Implement two functions over a million `u64`s:

```rust
fn sum_array(v: &[u64]) -> u64 {
    v.iter().sum()
}

fn sum_linked_list(head: &Option<Box<Node>>) -> u64 { ... }   // a linked list, same data
```

Build both with the same 1M values. Benchmark each with `criterion`. Report ns per element for each, and the ratio.

Then run `cargo asm` on each and report:

* What does the array sum compile to? Did the compiler vectorise?
* What does the linked list sum compile to? Why can't the compiler vectorise?
* Predict the ratio of cache misses you'd see. Verify with `perf stat -e cache-misses` on a binary that runs each.

You should see roughly 5-10x difference. If you don't, your linked list nodes are accidentally allocated contiguously (which can happen with a fresh allocator) — re-allocate in random order to force scattering.

**Q2. The hot/cold split benchmark.**

Take the `Order` struct from section 7.5. Implement `total_value(orders: &[Order]) -> i64` two ways:

* (a) Single-struct version, as written.
* (b) Hot/cold split, with `OrderHot` packed.

Benchmark both for 10,000, 100,000, and 1,000,000 orders. Report ns/order at each size. Explain the curve. Where does the difference come from in absolute numbers?

Now profile the slow version with `perf stat -e cache-misses,cache-references,instructions,cycles`. Compute "instructions per cycle" (IPC) for each version. The hot/cold version should have noticeably higher IPC. Why?

**Q3. Coordinated omission.**

Implement a tiny system that processes "events" — each event is just `std::hint::spin_loop()` repeated some number of times to consume ~500ns. Process 100,000 events.

Inject a stall: after every 10,000 events, do a `std::thread::sleep(Duration::from_millis(50))`.

Measure latency two ways:

* **Method A (naive):** time each event individually.
* **Method B (corrected):** schedule events at fixed cadence (one per microsecond), measure each from intended start.

Report p50, p99, p99.9, and max for each method. The two methods should give wildly different tail percentiles. Explain why.

Bonus: read Gil Tene's article ("How NOT to Measure Latency") and explain in your own words what "coordinated omission" means.

**Q4. Allocation cost.**

Build a tiny "message processor" with three implementations:

* **(a) Allocating:** for each input message, allocate a `Vec<u8>`, do some work, drop.
* **(b) Reused buffer:** keep one `Vec<u8>` around, `.clear()` and reuse each iteration.
* **(c) Stack buffer:** use `[u8; 1024]` on the stack.

Benchmark each at 1M messages. Report ns/message and total allocations (use `jemalloc-ctl` or just count with `Drop` instrumentation in a debug build).

How does the gap between (a) and (b) compare to the gap between (b) and (c)? Why does (c) help even though (b) already eliminated allocator calls?

**Q5. HashMap vs array indexed.**

Implement a function `lookup_orders` that, given a slice of order IDs, returns the corresponding orders. Two versions:

* (a) Storage is `HashMap<u64, Order>` with default hasher.
* (b) Storage is `HashMap<u64, Order>` with `ahash` or `rustc-hash`.
* (c) Storage is `Vec<Option<Order>>` indexed by ID, where IDs are dense in `0..N`.

Set up a workload of 1M orders with sequential IDs. Bench all three at 100K random lookups. Report ns/lookup.

Now what if the IDs are *sparse* — 1M orders with IDs randomly chosen from `0..10^9`? Re-bench. Which approach wins now? What does that tell you about choosing data structures based on distribution, not just operation?

**Q6. Reading assembly.**

For each of these functions, predict (in advance) what the assembly will look like, then check with `cargo asm`. For each, answer: (i) how many instructions in the inner loop? (ii) is there a bounds check? (iii) did the compiler vectorise? (iv) is the function inlined into typical callers?

```rust
pub fn sum_safe(v: &[u64]) -> u64 {
    let mut total = 0u64;
    for i in 0..v.len() {
        total = total.wrapping_add(v[i]);
    }
    total
}

pub fn sum_iter(v: &[u64]) -> u64 {
    v.iter().fold(0u64, |a, &x| a.wrapping_add(x))
}

pub fn sum_unchecked(v: &[u64]) -> u64 {
    let mut total = 0u64;
    let n = v.len();
    for i in 0..n {
        // SAFETY: i < n, and v.len() == n.
        total = total.wrapping_add(unsafe { *v.get_unchecked(i) });
    }
    total
}
```

If `sum_iter` and `sum_safe` produce identical assembly, that confirms iterators are zero-cost over indexed access. They probably do. Verify.

**Q7. A determinism harness.**

Implement a minimal matching engine in Rust (single trading pair, limit orders, price-time priority). It should:

* Read events from stdin as newline-delimited JSON.
* Process events sequentially.
* Emit trades to stdout as newline-delimited JSON.
* Use fixed-point i64 prices, never floats.
* Use BTreeMap or sorted Vec for the book; never HashMap iteration.
* Take timestamps from input events, never `Instant::now()`.

Generate a test input of 10,000 events (mix of placements and cancellations). Run the engine on it twice. Diff the two outputs. They must be byte-identical.

Now deliberately introduce a non-determinism: add `let now = Instant::now();` somewhere and put `now` into an output. Re-run and confirm the diff fails. Revert.

Add this diff test to a `cargo test` that runs in CI.

**Q8. Reading.**

Read these:

* Ulrich Drepper's *What Every Programmer Should Know About Memory*. Sections 2-3 are the relevant ones; 4-5 if you have time. It's long; budget 4 hours.
* Aleksey Shipilёv's "JMM Pragmatics" (Java but the cache and ordering content is universal).
* Gil Tene, "How NOT to Measure Latency" — there's a slide deck and a talk on YouTube.

After reading, write up:

* The three biggest things from Drepper's paper that affect Rust code.
* What does Shipilёv mean by "the hardware doesn't actually run your program; it runs a simulation that is observably equivalent"?
* Tene's three favorite mistakes engineers make in latency measurement.

---

# Lesson 8: Lock-Free and Low-Latency Concurrency

## 8.1 Why This Lesson Exists

Most concurrent code in the world is wrong. Not buggy in the obvious sense — the test cases pass, the code "works" — but subtly wrong in ways that show up at 3 AM in production, occasionally, on a Tuesday. The kind of bugs where you stare at the code, swear it's correct, and then discover that the CPU was reordering memory operations in a way you didn't realise was permitted.

In application Rust, you avoid this entire category by using `Arc<Mutex<T>>` for shared state, or channels, and never thinking about it again. The mutex provides a memory barrier; the channel synchronises producer and consumer. Phase 1 covered these patterns. They are the right answer for 95% of code.

For the remaining 5% — the matching engine's hot path, the market data parser's per-message dispatch, the strategy runtime's event loop — locks are too slow. An uncontended mutex acquisition is 20-25ns. A *contended* one, where another thread already holds the lock, is 1000+ ns because of the kernel involvement. In a system with a 5-microsecond budget, you cannot afford even one contended lock per event.

So you go lock-free. The basic idea: instead of synchronising with a mutex, use atomic operations directly, with carefully chosen memory orderings to ensure visibility between threads. Done right, a single-producer single-consumer (SPSC) queue can pass 200 million messages per second, with sub-100ns per-message latency. Done wrong, you get use-after-free or torn reads or your program just produces nonsense outputs.

This lesson covers:

* The atomic operations Rust gives you, and what they each guarantee.
* The C++/Rust memory model — what reordering is permitted, and what `Acquire`/`Release`/`SeqCst` actually mean.
* The single-producer single-consumer ring buffer (the "SPSC" of LMAX Disruptor fame), built up rigorously.
* The pitfalls: ABA, hazard pointers (in passing), epoch-based reclamation.
* When to reach for lock-free — and when not to.

It's the densest lesson in this phase. There is genuinely no way to make the memory model intuitive in 30 pages. Go through it slowly. Build the example. Run the drills. Read the references at the end. After three months you'll be comfortable; after a year you'll be one of the people in your firm who actually understands what's happening.

## 8.2 The Refresher: Why Locks Are Slow

Phase 1's reference document had this. We need it again because it motivates everything below.

A `std::sync::Mutex` in Rust is implemented (on Linux) using futex syscalls. The fast path is purely userspace:

* `lock()`: atomic compare-and-swap (CAS) on the mutex's state. If the lock was free (state was 0), set it to 1 and return. ~25ns.
* `unlock()`: atomic store of 0. ~5ns.

The slow path happens under contention. When `lock()` finds the state is 1 (someone else holds it):

* The thread CASes the state from 1 to 2 ("contended").
* It calls `futex(FUTEX_WAIT, ...)` — a syscall that puts the thread to sleep.
* The kernel schedules another thread on this CPU.
* When the holder finally unlocks, they see state == 2, do the regular store, then call `futex(FUTEX_WAKE, ...)` to wake one waiter.
* That syscall, the context switch, and the wakeup take 1-10 *microseconds*, depending on system load.

Now imagine your matching engine processes events under a mutex. Most of the time the mutex is uncontended (one thread is doing all the work) — fine. But when contention occurs even briefly — the kernel handles a network interrupt and pre-empts your producer thread, the consumer hits a contended lock, suddenly there's a 5μs stall — your latency tail explodes.

Locks are tools for correctness, not performance. They serialise threads safely; they do not serialise them quickly. For the trading engine's hot path, you need something else.

Two paths forward:

1. **Lock-free data structures** — algorithms where progress is guaranteed without locks, using atomic operations. Subject of this lesson.
2. **Single-threaded execution + message passing** — a single thread owns the engine state; other threads send it messages via lock-free queues. Often the better answer.

In practice, professional matching engines combine both: each engine instance runs on a single thread (no internal synchronisation needed), and inter-thread communication uses lock-free queues. The lock-free part is the queues, not the engine logic itself.

## 8.3 Atomic Operations in Rust

Rust exposes atomics through `std::sync::atomic`:

```rust
use std::sync::atomic::{AtomicU64, AtomicI64, AtomicUsize, AtomicBool, Ordering};

let counter = AtomicU64::new(0);

// Basic operations
let _v = counter.load(Ordering::Acquire);
counter.store(42, Ordering::Release);
let _old = counter.fetch_add(1, Ordering::AcqRel);
let _old = counter.swap(99, Ordering::AcqRel);

// Compare-and-swap (CAS)
let result = counter.compare_exchange(
    99,                  // expected current value
    100,                 // new value
    Ordering::AcqRel,    // success ordering
    Ordering::Acquire,   // failure ordering
);
match result {
    Ok(prev) => println!("CAS succeeded, prev was {}", prev),
    Err(actual) => println!("CAS failed, actual was {}", actual),
}
```

Each of these is a *single CPU instruction* on x86, plus possibly a memory-fence instruction. Costs:

* `load`/`store` with `Relaxed` or `Acquire`/`Release`: ~1ns (basically free on x86).
* `fetch_add`, `swap`, `compare_exchange`: 5-15ns uncontended.
* Same operations under heavy contention from another core: 40-200ns due to cache-line bouncing.

The first thing to understand: these are not just "thread-safe versions of integer ops." They have memory ordering implications that ripple through your entire program's memory accesses. The `Ordering` parameter is the most important part.

## 8.4 Memory Ordering: The Hardest Part

I'll be honest: this is where most engineers' eyes glaze over. Power through; you cannot write correct lock-free code without it.

### The problem: reordering

Both compilers and CPUs reorder your memory operations. They do this within strict rules, the central one being **the program must appear to execute in source order from the perspective of the thread doing the executing**. You'll never observe your own thread "skipping ahead." But other threads can see *your* writes in a different order than you wrote them.

A classic example. Two threads, four shared variables (all start at 0):

```
Thread A:        Thread B:
x = 1;           y = 1;
r1 = y;          r2 = x;
```

Naive intuition: at least one of r1, r2 must be 1 at the end. Either A's write to x happens before B reads it, or B's write to y happens before A reads it.

Reality: both r1 and r2 can be 0. The CPU is allowed to reorder each thread's "store, then load to a different address" because they're independent. Both threads do their loads first (which see 0), then their stores. Both threads see 0. This is permitted on x86 (subject to specific rules) and on ARM (much more aggressively), even though every individual line of code looks innocent.

This is the deep weirdness of multithreaded memory. To make it tractable, the C++ committee defined a *memory model* — a formal set of rules saying which reorderings are allowed and how to prevent the ones you don't want. Rust borrowed this model. C, Java, and recent versions of Go have similar models.

### The orderings, in order

Rust offers five orderings (`std::sync::atomic::Ordering`):

* `Relaxed`
* `Acquire`
* `Release`
* `AcqRel`
* `SeqCst`

Increasing in strength and cost. From weakest to strongest:

#### `Relaxed`

No ordering guarantees beyond atomicity. The operation is atomic — no torn reads, no torn writes, no lost updates — but other operations around it can be reordered freely. Use for counters where you don't need to coordinate with other variables:

```rust
static REQUESTS: AtomicU64 = AtomicU64::new(0);
fn handle_request() {
    REQUESTS.fetch_add(1, Ordering::Relaxed);
    // ... handle ...
}
```

Cheapest. On x86 a relaxed load is just a `MOV`; a relaxed `fetch_add` is a `lock add`.

#### `Acquire` (loads only) and `Release` (stores only)

The pair you'll use most. The mental model:

* A `Release` store **publishes** all the memory writes that happened before it in this thread.
* An `Acquire` load **synchronises with** any `Release` store of the same value, and sees all the writes that the releaser published.

So if thread A writes data and then does `flag.store(true, Release)`, and thread B does `flag.load(Acquire) == true` and then reads the data, B is guaranteed to see what A wrote. The compiler and CPU may not reorder writes from before the Release across it, nor reads from after the Acquire to before it.

This is exactly the SPSC ring buffer's protocol:

```rust
// Producer
self.ring[idx] = event;                              // 1. write data
self.producer_seq.store(seq, Ordering::Release);     // 2. publish

// Consumer
while self.producer_seq.load(Ordering::Acquire) < seq {} // 3. wait
let event = self.ring[idx];                              // 4. read data
```

Step 2's `Release` synchronises with step 3's `Acquire`. Once 3 sees the published seq, 4 is guaranteed to see what 1 wrote. The pair makes the data transfer safe.

If you used `Relaxed` instead, it would fail. Step 1's write could be reordered by the CPU to *after* step 2, so the consumer would observe an updated seq with stale data underneath.

#### `AcqRel`

For a single operation that is both a load and a store (like `fetch_add` or `compare_exchange`), `AcqRel` makes the load Acquire and the store Release. Most read-modify-write atomic operations use `AcqRel`.

#### `SeqCst`

The strongest ordering: sequential consistency. All `SeqCst` operations across all threads have a single global total order, consistent with each thread's program order. Java's `volatile` is approximately SeqCst.

`SeqCst` is the safest default — if you don't know what ordering you need, use this. It's also the slowest (typically requires an `mfence` on x86, which serialises the pipeline). You use it when Acquire/Release pairs aren't sufficient — typically because you need to coordinate operations across multiple atomic variables.

### The simplification (for x86)

x86 has a strong memory model. On x86:

* `Relaxed` and `Acquire` loads compile to a regular `MOV`. Same instruction. Same cost.
* `Relaxed` and `Release` stores also compile to a regular `MOV`.
* `SeqCst` operations require an additional fence; they're more expensive.

So on x86, the difference between `Relaxed` and `Acquire`/`Release` is mostly for the *compiler*, not the CPU. The compiler is forbidden from reordering across Acquire/Release boundaries; the CPU mostly wouldn't anyway.

This is a curse for learning. Code with insufficient ordering often runs correctly on x86, then breaks when you ship to an ARM server. Always test on ARM if your production hardware might be ARM (Apple Silicon counts; AWS Graviton counts).

### Recommended practice

* Use `Relaxed` only when you have a clear reason (e.g., simple counters where no other state depends on the ordering).
* Use `Acquire`/`Release` for the common producer-consumer / publish-subscribe patterns.
* Use `SeqCst` if you're unsure or if you're coordinating multiple atomics. Pay the small perf cost; it's much better than a memory-model bug.

A simpler heuristic from the LMAX paper: pretty much every Disruptor implementation uses Acquire/Release pairs. Read examples. Internalise the pattern.

## 8.5 The SPSC Ring Buffer, Built From Scratch

Now we put it together. Here's a single-producer single-consumer ring buffer, the foundation of Disruptor-style designs.

```rust
use std::sync::atomic::{AtomicU64, Ordering};
use std::cell::UnsafeCell;
use std::mem::MaybeUninit;

const CAPACITY: usize = 1024;            // Must be power of 2.
const MASK: u64 = (CAPACITY as u64) - 1;

#[repr(C, align(64))]                     // align to cache line
struct CachelinePadded<T>(T);

pub struct SpscRing<T> {
    // Producer-only state, on its own cache line.
    producer_seq: CachelinePadded<AtomicU64>,
    // Consumer-only state, on its own cache line.
    consumer_seq: CachelinePadded<AtomicU64>,
    // The ring itself. Each slot is wrapped in UnsafeCell because we need
    // to mutate it through shared references.
    slots: Box<[UnsafeCell<MaybeUninit<T>>]>,
}

// Tell the compiler this struct is safe to share across threads. We're
// taking responsibility for that ourselves — the access pattern is what
// makes it safe, not the type.
unsafe impl<T: Send> Send for SpscRing<T> {}
unsafe impl<T: Send> Sync for SpscRing<T> {}

impl<T> SpscRing<T> {
    pub fn new() -> Self {
        let mut v = Vec::with_capacity(CAPACITY);
        for _ in 0..CAPACITY {
            v.push(UnsafeCell::new(MaybeUninit::uninit()));
        }
        SpscRing {
            producer_seq: CachelinePadded(AtomicU64::new(0)),
            consumer_seq: CachelinePadded(AtomicU64::new(0)),
            slots: v.into_boxed_slice(),
        }
    }

    /// Called by the producer thread only.
    /// Returns Err if the buffer is full; caller can retry or yield.
    pub fn try_push(&self, value: T) -> Result<(), T> {
        let head = self.producer_seq.0.load(Ordering::Relaxed);  // we wrote it last
        let tail = self.consumer_seq.0.load(Ordering::Acquire);  // sync with consumer

        if head.wrapping_sub(tail) >= CAPACITY as u64 {
            // Buffer is full.
            return Err(value);
        }

        let idx = (head & MASK) as usize;

        // SAFETY: We are the only producer; this slot is exclusively ours
        // until we publish the new sequence. The consumer cannot touch it
        // because head is past the consumer's tail.
        unsafe {
            (*self.slots[idx].get()).write(value);
        }

        // Publish: store the new producer_seq with Release. This synchronises
        // with the consumer's Acquire load below.
        self.producer_seq.0.store(head + 1, Ordering::Release);

        Ok(())
    }

    /// Called by the consumer thread only.
    /// Returns None if the buffer is empty; caller can retry or yield.
    pub fn try_pop(&self) -> Option<T> {
        let tail = self.consumer_seq.0.load(Ordering::Relaxed);  // we wrote it last
        let head = self.producer_seq.0.load(Ordering::Acquire);  // sync with producer

        if tail == head {
            // Buffer is empty.
            return None;
        }

        let idx = (tail & MASK) as usize;

        // SAFETY: We are the only consumer. The slot was written by the producer
        // before they incremented producer_seq past `tail`. The producer cannot
        // touch this slot again until we advance consumer_seq past it.
        let value = unsafe {
            (*self.slots[idx].get()).assume_init_read()
        };

        // Publish: store the new consumer_seq with Release. The producer's
        // Acquire load above (in the next try_push) will see it.
        self.consumer_seq.0.store(tail + 1, Ordering::Release);

        Some(value)
    }
}

impl<T> Drop for SpscRing<T> {
    fn drop(&mut self) {
        // Drop any unread values.
        while let Some(_) = self.try_pop() {}
    }
}
```

Let me walk through this slowly because every detail matters.

### Why `UnsafeCell<MaybeUninit<T>>`?

We need to *mutate* slots through `&self` (a shared reference), because both producer and consumer hold `&self` to the same ring. Safe Rust forbids this — `&self` gives only shared access. `UnsafeCell` is the one safe way to opt out: it's the only type for which the compiler permits mutation through `&self`.

`MaybeUninit<T>` represents "memory that might or might not contain a valid `T`." We need it because the slots start uninitialised — we haven't written real data into them yet. Reading uninitialised memory through a `T` reference is undefined behavior, even if you never use the value. `MaybeUninit` carries the proof that "this might not be init."

Together: each slot is a piece of memory, of size `T`, that we can mutate (UnsafeCell) and that might be uninitialised (MaybeUninit). Producer initializes; consumer reads-and-takes; producer can re-initialize after consumer is done.

### Why `unsafe impl Send + Sync`?

The compiler can't prove that `SpscRing<T>` is safe to share between threads — it sees `UnsafeCell` and assumes the worst. We override that with `unsafe impl`, taking responsibility for the safety ourselves. The justification: the producer-only and consumer-only access pattern, enforced by sequence numbers, ensures no two threads ever access the same slot simultaneously.

### Why `#[repr(C, align(64))]`?

Cache-line alignment. Without it, `producer_seq` and `consumer_seq` might end up on the same cache line (they're each 8 bytes; two fit in 64). They're written by different threads. Sharing a cache line means every producer write invalidates the consumer's cache, and vice versa — false sharing. Cost: 100x slowdown.

With alignment, each is on its own line. No interference.

### The orderings, justified

Producer's `try_push`:

* `producer_seq.load(Relaxed)` — we (the producer) wrote it last. No need to synchronise with anyone; we know its current value.
* `consumer_seq.load(Acquire)` — synchronises with the consumer's `Release` store in `try_pop`. Once we see a new consumer_seq, we know all the consumer's reads of the old slot are complete.
* `producer_seq.store(Release)` — synchronises with the consumer's `Acquire` load. Once they see the new value, they're guaranteed to see our write to the slot.

Consumer's `try_pop` is the symmetric mirror.

If you're suspicious — "is `Relaxed` for our own writes really safe?" — yes, because the only thread that reads producer_seq (other than the producer's own self-loads) is the consumer, which loads with Acquire from the producer's Release store. The producer's reads of its own atomic don't need ordering against itself; program order takes care of that.

### Power-of-two trick

`head & MASK` is the bitmask trick from Phase 1's reference: it computes `head mod CAPACITY` in one CPU cycle, but only if CAPACITY is a power of 2. Always pick a power of 2. "I want exactly 1000" is wrong; pick 1024.

### Wrapping arithmetic

`head.wrapping_sub(tail)` does subtraction with two's-complement wraparound — no overflow check, no panic. We use `wrapping_sub` because if `head` ever wraps around `u64::MAX` (after 18 quintillion enqueues — won't happen, but the optimiser shouldn't have to prove that), the math still gives the right "how full is the buffer" number. Always use wrapping arithmetic on counters that can in principle overflow.

### What's missing

This is a real working SPSC, but it lacks features you'd want in production:

* No batch operations (push N, pop N).
* No notification when items become available — caller has to poll.
* No backpressure feedback richer than "full" / "not full."
* Memory barrier overhead per operation; could be reduced by batching.

The LMAX Disruptor adds all of these and a multi-consumer extension. After understanding this SPSC, look at the [`crossbeam`](https://docs.rs/crossbeam) and [`disruptor-rs`](https://docs.rs/disruptor) crates for production-ready versions. Don't ship your own SPSC unless you've measured against these and have specific reasons — they've absorbed a decade of subtle bug fixes you'd otherwise reproduce.

## 8.6 MPSC and MPMC Queues

The SPSC above is the easy case. Multi-producer or multi-consumer queues are dramatically harder.

### Why MPSC is harder

With multiple producers:

* You can no longer load `producer_seq` with `Relaxed` and trust it; another producer might have updated it.
* Reserving a slot requires a CAS loop: read producer_seq, compute slot, CAS to claim it; if another producer beat you, retry.
* Publishing the slot is also harder: you can't just store the new producer_seq, because another producer might have a slot in flight with a smaller index that hasn't been written yet.

A typical MPSC uses a "sequence per slot" trick: each slot has its own sequence number. Producers CAS to claim; once the slot is written, they bump the slot's sequence to "ready." Consumer reads slots in order, waiting for each slot's sequence to indicate "ready."

### Why MPMC is even harder

Now the consumer side has the same problem: multiple consumers all want a slot, they have to coordinate without locks. Ditto contention.

Typical implementations:

* **`crossbeam-queue::SegQueue`**: unbounded, MPMC. Internally a linked list of bounded segments; allocates as needed (so unsuitable for "zero alloc in hot path").
* **`crossbeam-queue::ArrayQueue`**: bounded, MPMC. Fixed-size, no allocation in the hot path. Slower than SPSC but works for many threads.

Production HFT systems mostly use SPSC for the hot path and split work across cores with a fanout/fanin pattern, rather than reaching for MPMC. The scaling penalty of MPMC isn't usually worth it.

### Recommended pattern: SPSC + sharding

Instead of one MPMC queue feeding one consumer, have N consumer threads, each with their own SPSC queue from a single producer. Distribute work by sharding (e.g., by symbol). Each consumer is single-threaded and uses SPSC for its input. No MPMC needed.

For matching engines: each trading pair gets its own engine, on its own thread, with an SPSC inbound queue. To scale, add more pairs / more threads. Each thread is a sealed kingdom. This pattern is shockingly successful in production — you avoid the entire MPMC complexity and your latency is much more predictable.

## 8.7 ABA, Hazard Pointers, and Epochs

For pointer-based lock-free data structures (lock-free linked lists, queues with linked nodes), there's a notorious problem called **ABA**.

### The ABA problem

You're building a lock-free stack with a head pointer. You want to pop:

1. Read head pointer, value is A.
2. Read A.next, value is B.
3. CAS(&head, expected: A, new: B). If head is still A, swap to B and return A.

Looks correct. But here's the disaster scenario:

1. Thread X reads head, sees A.
2. Thread X reads A.next, sees B.
3. Thread X gets pre-empted.
4. Thread Y pops A. Then pops B. Then frees both.
5. Thread Y allocates a new node, gets back A's old memory, now containing C. C.next is something else.
6. Thread Y pushes this new "A" onto the stack. Head is now (the new) A.
7. Thread X resumes. CAS(&head, expected: A, new: B). The CAS *succeeds* because head's pointer value matches. But it's a different A!
8. Stack now has B on top, but B was freed in step 4. Use-after-free.

The CAS detected the pointer value, not the underlying object. The "A" came back and fooled it. This is ABA.

### Solutions

* **Tagged pointers**: encode a counter in unused bits of the pointer; increment on every modification. The CAS now detects "same A" only if the counter matches too. Works but limited (depends on having spare bits).
* **Hazard pointers**: a thread-local indicator saying "I'm currently using this address; don't reclaim it." Reclamation logic checks all threads' hazard lists before freeing. Standard in C++; less ergonomic in Rust.
* **Epoch-based reclamation (EBR)**: organize work into epochs. A node freed in epoch N is only physically reclaimed once all threads have advanced past epoch N. Used by `crossbeam-epoch`.
* **Reference counting** (e.g. `Arc` with atomic count): works, but adds atomic ops on every access. Often too slow.

In Rust, the standard library is `crossbeam-epoch`. If you build a lock-free data structure that uses pointers, this is your starting point. The crate handles epoch tracking, garbage collection, and provides safe APIs over the unsafe primitives.

For HFT, you mostly avoid this entire mess by using arrays (no pointer recycling) instead of linked structures. The SPSC ring buffer above never reclaims memory — slots are reused by index, which doesn't have the ABA problem because the producer/consumer protocol ensures only one writer at a time.

## 8.8 Atomic Wait/Notify (briefly)

Modern Rust (1.59+) exposes platform-level futex primitives:

```rust
use std::sync::atomic::AtomicU32;

let a = AtomicU32::new(0);

// One thread does:
a.store(1, Ordering::Release);
unsafe { atomic_wait::wake_one(&a as *const _ as *const u32); }   // some crate

// Other thread does:
unsafe { atomic_wait::wait(&a as *const _ as *const u32, 0) };
```

(Real syntax via crates like `atomic-wait`; std's APIs were stabilised more recently.)

These let you build "atomic flag with notification" without using a full Mutex. Useful when you want a sleeping consumer rather than a busy-spinning one, but you don't want the full overhead of a `Condvar`.

For HFT-specific use cases, you mostly want the busy-spinning version (commit a CPU core to the consumer, never sleep). The atomic-wait pattern is for "I need a low-latency notification but I don't want to burn a core" cases.

## 8.9 When NOT to Reach for Lock-Free

A lot of "lock-free" code in the wild is slower and buggier than the equivalent locked code. Reasons:

1. Lock-free contention causes cache-line ping-pong just like locks do; sometimes worse, because lock-free code often has *more* atomic ops per logical operation.
2. Lock-free retries (CAS loops) get worse under contention — each retry is wasted work.
3. Lock-free correctness is genuinely harder, and most engineers shipping "lock-free" code haven't actually proven their algorithms.

When you should *not* reach for lock-free:

* **Cold paths.** Configuration, startup, error handling. Use a Mutex.
* **Coarse-grained synchronisation that's rarely contended.** A Mutex held for microseconds, contended once a minute, is fine.
* **You're not sure.** Use a Mutex + benchmark + see if it's actually a problem.
* **Low contention.** Mutexes' uncontended fast path is fast. If you see no contention in profiling, the Mutex isn't your bottleneck.

When you should:

* **You've measured.** A specific lock is showing up as 5%+ of your hot path.
* **Single-threaded won't work** for the data flow.
* **You can isolate the lock-free section** and audit it formally.
* **You're using a battle-tested library** (`crossbeam`, `disruptor-rs`, etc.), not rolling your own.

The senior engineer's instinct, even in HFT: prefer single-threaded with message passing over multi-threaded with locks, prefer locks over hand-rolled lock-free, prefer libraries over hand-rolled libraries.

## 8.10 Summary: The Rules

1. **Locks aren't slow when uncontended; they're slow when they wake the kernel.** 25ns vs 5000ns.
2. **Atomics are not "thread-safe operations."** They have memory ordering implications that affect all your code, not just the atomic line.
3. **The five orderings, in increasing strength:** `Relaxed` < `Acquire` (loads) ≈ `Release` (stores) < `AcqRel` (RMW) < `SeqCst`.
4. **Acquire/Release pairs are the workhorse pattern.** Producer does data writes + Release store; consumer does Acquire load + reads. Synchronises across threads.
5. **`SeqCst` if unsure.** Better safe and slightly slower.
6. **SPSC with cache-line-padded sequence numbers is the foundation pattern.** Sub-100ns per message.
7. **MPMC is much harder than SPSC.** Prefer SPSC + sharding architectures over MPMC.
8. **Pointer-based lock-free needs epoch reclamation.** Use `crossbeam-epoch`. Or avoid pointer recycling by using array indices.
9. **Always cache-line-pad data written by different threads.** False sharing is a 10x silent killer.
10. **Test on x86 and ARM both.** x86 forgives memory-ordering bugs that ARM doesn't.
11. **Don't roll your own.** `crossbeam`, `disruptor-rs`, `loom` (for testing) are battle-tested. Use them unless you measured them and they're insufficient.
12. **Lock-free is not always the answer.** Single-threaded + SPSC is faster than multi-threaded + lock-free for many workloads.

## 8.11 Drill 8

**Q1. The mutex vs SPSC throughput showdown.**

Build three queue implementations and benchmark each at passing 10 million `u64` from a producer thread to a consumer thread:

* (a) `std::sync::mpsc::channel` (the standard Rust unbounded MPSC).
* (b) A `Vec<u64>` protected by `Arc<Mutex<...>>` with capacity 1024 (block on full / empty with `Condvar`).
* (c) The SPSC ring buffer from section 8.5.

For each, report:

* Total wall-clock time.
* Throughput (messages/sec).
* p50, p99, p99.9 latency per message (use coordinated-omission-corrected measurement).

The SPSC should be at least 5x faster in throughput and 10x better in p99 latency. If it isn't, find the bug. Common causes: missing cache-line padding (false sharing), wrong memory ordering (you accidentally used `SeqCst` everywhere — measure the perf cost!), the producer outpacing the consumer (try a slower producer to confirm).

**Q2. The memory ordering bug.**

The following SPSC implementation compiles and works on x86. It's broken on ARM. Find the bug, explain it, and fix it.

```rust
pub fn try_push(&self, value: T) -> Result<(), T> {
    let head = self.producer_seq.0.load(Ordering::Relaxed);
    let tail = self.consumer_seq.0.load(Ordering::Relaxed);   // <-- HERE

    if head.wrapping_sub(tail) >= CAPACITY as u64 { return Err(value); }

    let idx = (head & MASK) as usize;
    unsafe { (*self.slots[idx].get()).write(value); }

    self.producer_seq.0.store(head + 1, Ordering::Relaxed);   // <-- AND HERE
    Ok(())
}
```

For each `Relaxed`, say what could go wrong if it isn't strengthened, and on what kind of hardware.

If you have access to an ARM machine (Apple Silicon Mac, AWS Graviton instance, Raspberry Pi), test the bug. Run a tight producer-consumer loop with the broken ordering for 10M iterations. Check that the consumer's read values are actually what the producer wrote. On x86 you'll see no issue; on ARM you may see torn or stale reads.

**Q3. False sharing benchmark.**

Build a struct:

```rust
struct Counters {
    a: AtomicU64,
    b: AtomicU64,
}
```

Spawn two threads. Thread A increments `counters.a` 100M times with `Relaxed`. Thread B increments `counters.b` 100M times. They never touch each other's counter. Measure total wall-clock time.

Now add cache-line padding:

```rust
#[repr(C)]
struct Counters {
    a: AtomicU64,
    _pad: [u8; 56],
    b: AtomicU64,
}
```

Re-run. Report the speedup. Should be 5-10x. If you don't see one, your two threads are accidentally on the same physical core (HT siblings count as one); pin them to different physical cores with `core_affinity` crate.

**Q4. Build a benchmark for ABA.**

This is harder. Implement a lock-free Treiber stack — the classic lock-free linked list stack:

```rust
struct Stack<T> {
    head: AtomicPtr<Node<T>>,
}

struct Node<T> {
    value: T,
    next: *mut Node<T>,
}
```

`push` and `pop` use CAS on `head`. Don't use any reclamation (just leak memory) for the first version.

Now implement a stress test that triggers ABA. Two consumers and one producer, repeatedly pushing and popping the same few values, with sleeps and yields to maximize interleaving. You're trying to get into the situation where a popped node is freed and re-pushed before another popper notices.

(Caveat: this is hard to trigger reliably even when broken. The point is to build the test rig and measure how often it fails over many iterations.)

Then fix the ABA either:

* (a) Using `crossbeam-epoch`'s `Atomic<T>` and `Guard`, OR
* (b) Using a tagged pointer (encode a 16-bit counter in unused upper bits).

Re-run the stress test. Confirm no failures.

**Q5. The SeqCst tax.**

Take the SPSC from section 8.5. Make a copy where every `Acquire`/`Release` is replaced with `SeqCst`. Benchmark both. Report ns/message difference.

Then run with `RUSTFLAGS="-C target-cpu=native"` and re-benchmark both. Does the gap change?

What about on an ARM machine? Run there too if you can. The relative cost of SeqCst vs Acquire/Release is much higher on ARM than x86.

**Q6. Read with `loom`.**

Install the `loom` crate. Write a tiny test for the SPSC: one producer thread sends `[1, 2, 3]` through the queue; one consumer receives them in order. Use `loom::sync::atomic` and `loom::thread::spawn` instead of std versions. Run with `cargo test --release` (in `loom` mode, default behavior is to model-check all possible interleavings).

Loom will exhaustively explore all the orderings the memory model permits and verify your code is correct under all of them. It will catch bugs that don't trigger on real hardware in a million runs.

Now deliberately introduce a memory-ordering bug (e.g., change `Release` to `Relaxed` on the producer's store). Re-run loom. Does it catch it?

This is the gold standard for testing lock-free Rust. Real production lock-free code at trading firms is loom-tested.

**Q7. Real-world reading.**

Read the source of two of these:

* `crossbeam-channel` (the unbounded version): https://github.com/crossbeam-rs/crossbeam/tree/master/crossbeam-channel
* `disruptor-rs`: https://github.com/nicholassm/disruptor-rs
* `tokio::sync::mpsc`: https://github.com/tokio-rs/tokio/blob/master/tokio/src/sync/mpsc

For each, describe in 100 words:

* The data structure used.
* The queue topology (SPSC, MPSC, MPMC).
* How they handle the case where the queue is full or empty.
* What memory orderings they use, and why.

Don't aim for full understanding — aim for the ability to navigate and recognise the patterns from this lesson.

**Q8. Reading.**

Read these:

* "The C++ memory model and concurrency" series by Mara Bos — author of the Rust atomics book. The book itself, *Rust Atomics and Locks*, is the single best resource for this material; if you can carve out the time, read the whole thing. Free online: https://marabos.nl/atomics/
* The original LMAX Disruptor paper (Thompson et al.): https://lmax-exchange.github.io/disruptor/files/Disruptor-1.0.pdf
* "Linearizability: A Correctness Condition for Concurrent Objects" — Herlihy and Wing, 1990. The formal definition of "what does it mean for a concurrent data structure to behave as if operations happened atomically?"

Write 200 words on:

* Why the C++/Rust memory model exists. What was the alternative the language designers were trying to avoid?
* The difference between linearizability and sequential consistency. Why does a queue need linearizability?
* Mara Bos's recommendation on memory ordering choices: when does she suggest reaching for SeqCst vs Acquire/Release?

---

# Lesson 9: The HFT Toolkit

## 9.1 Why This Lesson Exists

You now know how to make a hot loop fast (Lesson 7) and how to coordinate threads without locks (Lesson 8). That's the *language* layer of low-latency engineering. There's a layer below — the *system* layer — that's just as important, and that no Rust book covers, because it's not about Rust. It's about Linux, network cards, CPU topology, and decades of systems-engineering folklore.

When a prop trading firm builds a matching engine, the Rust code is maybe 30% of the project. The other 70% is:

* Configuring the kernel to not interfere.
* Configuring the network card to deliver packets directly to userspace, bypassing the OS.
* Pinning threads to specific CPUs and ensuring no other process can pre-empt them.
* Synchronising hardware clocks across machines to nanoseconds.
* Designing the wire protocol to be parseable in 50 nanoseconds.
* Logging without allocating, without blocking, without making the GC do work (Rust again helps here, but you still have to be careful).
* Testing the whole stack end-to-end with deterministic replay of historical market data.

This lesson is a tour of those topics. It's the broadest of the three lessons in Phase 3, by necessity — each one of these subjects could be its own course. But you need to know they exist, what problems they solve, and where to look when you need to learn one in depth. After this lesson, when a senior engineer at a prop firm says "we need to move that thread off CCX 0 because it's sharing L3 with the kernel housekeeping core," you'll have a fighting chance of understanding.

We'll also touch on **what the actual codebase of a real low-latency Rust system looks like** — the conventions, the architecture, the things they do differently from web Rust. The goal is to make the unfamiliar familiar.

## 9.2 The Operating System Is Mostly the Enemy

When you write web Rust, the OS is your friend: it manages memory, schedules threads, handles networking, abstracts hardware. You barely notice it.

When you write low-latency Rust, the OS is *mostly* an obstacle. Every interaction with the kernel is a syscall, taking 100ns to 2μs. Every interrupt the kernel handles steals CPU time from your threads. Every page fault means walking page tables. The OS scheduler will happily migrate your hot thread from one core to another because it thought another core was warmer (it wasn't). The Linux kernel, for all its quality, is built for general-purpose computing — your trading process is a *very* unusual workload, and you have to work hard to get the kernel to leave you alone.

The general strategy at the high end:

1. **Reserve cores for trading processes.** Tell the kernel "these CPU cores are off-limits to all other processes."
2. **Pin your trading threads to those cores.** Each thread sticks to one specific CPU.
3. **Move all interrupts off those cores.** The NIC interrupts, the timer interrupts, all routed elsewhere.
4. **Disable hyperthreading on those cores.** Or use only one of the two HT siblings.
5. **Use kernel-bypass networking.** Packets go from NIC to userspace directly, no OS involvement.
6. **Pre-fault all memory.** No page faults during operation.
7. **Use huge pages.** 2MB pages instead of 4KB to reduce TLB pressure.

Each of these is a technique we'll cover. None is Rust-specific, but Rust gives you good tools to use them.

### CPU pinning in Rust

Use the `core_affinity` crate (or call `sched_setaffinity` directly via `libc`):

```rust
use core_affinity::CoreId;

fn run_engine() {
    let cores = core_affinity::get_core_ids().unwrap();
    // Pin to physical core 4 (avoid HT sibling on 4+N).
    core_affinity::set_for_current(cores[4]);

    // ... engine main loop ...
}
```

A typical setup:

* Cores 0-1: kernel housekeeping.
* Cores 2-3: market data receiver threads.
* Cores 4-5: matching engine.
* Cores 6-7: order gateway / response.

Each thread has one core. The kernel can't migrate them. Cache stays warm.

### Isolating CPUs from the kernel scheduler

Boot Linux with kernel parameter `isolcpus=4-7`. Now the kernel scheduler won't schedule any other process on cores 4-7. Your `core_affinity::set_for_current(4)` will be the only thing running on core 4.

Plus `nohz_full=4-7` to disable the periodic timer tick on those cores (one less interrupt per millisecond), and `rcu_nocbs=4-7` to move RCU callback processing off them.

This level of tuning is operations work, not Rust code. But the Rust process needs to be aware that it's running in this environment — it shouldn't, for example, spawn a thread pool that lands on the wrong cores.

### Pre-faulting memory

When you allocate memory, Linux usually doesn't actually give you physical pages — it gives you virtual address space, and only allocates the physical page when you first touch it. The first touch takes a page fault, which is a syscall, which is microseconds.

To avoid this in the hot path: write to every page at startup. Either touch it explicitly, or use `mlock()` to lock memory into RAM and pre-fault it.

```rust
use libc::{mlockall, MCL_CURRENT, MCL_FUTURE};

fn pre_fault() {
    unsafe {
        mlockall(MCL_CURRENT | MCL_FUTURE);
    }
    // All current and future memory is now locked into RAM.
    // Any virtual pages are pre-faulted.
}
```

Note: `mlockall` requires `CAP_IPC_LOCK` capability or root. In production, the trading process runs with this capability granted.

### Huge pages

The TLB (Translation Lookaside Buffer) caches recent virtual-to-physical address translations. With 4KB pages, a 1GB working set requires 256K page table entries; the TLB has 1500-ish entries; you miss the TLB constantly, which means a page-table walk (200+ cycles) per miss.

With 2MB huge pages, the same 1GB needs only 512 entries, all of which fit in the TLB. TLB miss rate drops by 500x.

Configuring this is a sysadmin job — `sysctl vm.nr_hugepages`, `mmap` with `MAP_HUGETLB`. Many production setups also use *transparent huge pages*, but the predictability is worse. Serious low-latency setups use explicit huge page allocation.

In Rust: `mmap` with the `MAP_HUGETLB` flag, or use a crate like `huge_alloc` that abstracts this.

## 9.3 Kernel Bypass Networking

The OS network stack is incredible. It implements TCP, IPv4, IPv6, DNS, ARP, dozens of protocols. It's also slow for HFT purposes. A typical syscall to send one packet (`send()` on a socket) is:

* Copy data from user buffer to kernel buffer.
* TCP/IP stack processing: header construction, checksums, routing.
* Hand to NIC driver.
* Eventually NIC sends.

Total: 1-5 microseconds. For receiving, similar overhead, plus an interrupt to wake your process up.

For HFT, this is the *whole* latency budget eaten by one packet send. Solution: bypass the OS entirely. The NIC delivers packets directly to userspace; your application reads them from the NIC's ring buffers without any kernel involvement. Same for sending: write directly to the NIC's TX ring, the NIC sends.

Standard kernel-bypass technologies:

* **DPDK** (Data Plane Development Kit): the open-source standard. Drivers for major NICs, library APIs in C. Rust bindings exist. Steep learning curve.
* **AF_XDP**: Linux's newer "give me raw packets fast" interface. Less performant than DPDK but better integrated with the kernel.
* **Solarflare ef_vi / OpenOnload**: vendor-specific, used heavily in finance because Solarflare/Xilinx NICs were the de-facto standard for years.
* **RDMA**: a different paradigm — Remote Direct Memory Access — letting one machine read/write another machine's memory directly over Infiniband or RoCE. Used for very-low-latency intra-datacenter messaging.

A typical kernel-bypass setup:

1. NIC dedicated to the trading process. Other traffic goes to a separate NIC.
2. NIC driver is taken out of the kernel's hands.
3. NIC's RX queues are mapped into the trading process's memory.
4. The trading process polls the queue (busy-waits) for new packets.
5. New packets are processed in-place, no copies.
6. Outbound packets are written directly to the TX queue.

Latency for a packet round-trip on the wire: ~3-5 microseconds total (NIC to NIC, end-to-end), of which less than 1 microsecond is the userspace processing.

In Rust, the popular crates are:

* `dpdk` and `dpdk-rs` — DPDK bindings.
* `xdp-rs` and `aya` — AF_XDP and the more general eBPF ecosystem.
* `rdma-core` — InfiniBand / RoCE.

Most of this is complex and platform-specific. You'd typically not implement kernel bypass yourself unless joining a team that already does — instead, you'd be the application engineer using their abstractions.

## 9.4 Hardware Timestamping and Time

For HFT, "what time is it" is harder than you think.

The CPU's `RDTSC` instruction returns a counter that increments at the CPU's nominal frequency. Reading it takes ~20 cycles (~6ns). That's the cheapest, highest-resolution timer you have.

```rust
fn rdtsc() -> u64 {
    unsafe { core::arch::x86_64::_rdtsc() }
}

let start = rdtsc();
// ... do work ...
let end = rdtsc();
let cycles = end - start;
let nanos = cycles * 1_000_000_000 / cpu_frequency_hz;
```

Caveats:

* `RDTSC` is per-core. Two cores' counters might not be synchronised.
* Modern Intel/AMD CPUs synchronise across cores in the same socket; cross-socket may drift.
* Frequency scaling can make RDTSC inaccurate on older CPUs. Modern CPUs use a constant-rate TSC (`invariant TSC`).

For wall-clock time at high precision, your options:

* **Hardware-timestamped network packets**: the NIC stamps each packet on arrival/departure with sub-microsecond accuracy. The kernel exposes this via `SO_TIMESTAMPING` socket option.
* **PTP (Precision Time Protocol)**: synchronises clocks across machines to <1μs. The standard for serious HFT setups.
* **GPS-disciplined clocks**: gold standard, used by exchanges. Sub-100ns accuracy.

For most application code, the question isn't "what time is it really" but "did event A happen before event B." For that, monotonic timers (`std::time::Instant` in Rust, `clock_gettime(CLOCK_MONOTONIC)` in C) are sufficient — they don't drift backwards, which is what you actually need for ordering.

```rust
use std::time::Instant;

let start = Instant::now();
do_work();
let elapsed = start.elapsed();   // a Duration
println!("{} ns", elapsed.as_nanos());
```

`Instant::now()` is ~25ns on Linux x86_64 (uses the vDSO so no full syscall).

## 9.5 Wire Protocols

Real exchange protocols are designed to be parseable fast. The differences from a typical web protocol are stark.

### What HFT-friendly protocols look like

Properties:

* **Binary, fixed-width fields.** Not text. Not JSON. Not Protobuf with varints.
* **Predictable layout.** You can compute offsets at compile time.
* **No dynamic-length fields in the hot path** (or at most one, at the end).
* **Little-endian** (because most current CPUs are LE).
* **No CRCs/checksums in the parser** (the NIC handles this).

Real examples:

* **NASDAQ ITCH**: market data feed. Each message is 36 bytes or fewer. Fixed layouts per message type.
* **NASDAQ OUCH**: order entry. Similarly fixed.
* **CME MDP 3.0**: market data, uses Simple Binary Encoding (SBE).
* **SBE (Simple Binary Encoding)**: a generic schema-driven binary protocol. Code generators produce Rust/C++/Java parsers that compile to ~10ns per message.

### Parsing in Rust, fast

A fixed-layout binary message is a Rust struct with `#[repr(C, packed)]`:

```rust
#[repr(C, packed)]
struct AddOrderMsg {
    msg_type: u8,           // = 'A'
    timestamp: u64,         // little-endian
    order_id: u64,
    side: u8,               // 'B' or 'S'
    quantity: u32,
    symbol: [u8; 8],
    price: u32,             // tick * 10000 say
    // ... 36 bytes total or whatever
}
```

Parsing a packet is then a memory transmute:

```rust
unsafe fn parse(buf: &[u8]) -> &AddOrderMsg {
    debug_assert!(buf.len() >= core::mem::size_of::<AddOrderMsg>());
    &*(buf.as_ptr() as *const AddOrderMsg)
}
```

This is unsafe but straightforward. Caveats:

* `#[repr(C, packed)]` says "no padding." But unaligned access is slower on some architectures (and undefined behavior in C on most). On x86 it's fine; on ARM beware.
* If endianness doesn't match, you have to byte-swap. `u32::from_le_bytes(...)` is the safe path.
* The `as *const _` cast is unsafe: you're trusting that `buf` is correctly aligned and at least the right size.

For production HFT, look at the `zerocopy` crate. It provides safe wrappers for this pattern, with traits ensuring alignment is correct and the type really is "plain old data."

### Avoiding `serde` in the hot path

Phase 2 lavished praise on serde. For ordinary code it's perfect. For HFT it's too slow — the trait dispatch, the format-agnostic abstractions, the per-field method calls all add up. A binary message that takes 2ns to memcpy takes 50ns to deserialise via serde-bincode.

For HFT message parsing:

* Roll your own parser on `#[repr(C, packed)]` structs with `zerocopy`.
* Or use a code generator like `sbe-tool` for the protocol's schema.
* Use serde for cold paths (config files, status reports, logs).

## 9.6 The Allocator

In Phase 1 we said: avoid heap allocation in the hot path. In HFT we say it harder: avoid the *system allocator entirely* in the hot path.

Linux's default allocator (typically `glibc`'s `ptmalloc` or `jemalloc`) is fast — call it 100ns for a small alloc — but it's not deterministic. It has internal locks, internal data structures that can fragment, and the latency varies. The 99.99th percentile allocation can be tens of microseconds.

For HFT:

1. **No heap allocations in the hot path, full stop.** Pre-allocate at startup.
2. **Use a fast allocator for cold paths.** `mimalloc` or `jemalloc` instead of glibc.
3. **For data structures that need to grow** (rare in hot paths), use a custom arena or pool.

In Rust:

```rust
// In Cargo.toml:
// [dependencies]
// mimalloc = { version = "0.1", default-features = false }

#[global_allocator]
static GLOBAL: mimalloc::MiMalloc = mimalloc::MiMalloc;

fn main() {
    // All allocations now go through mimalloc.
}
```

This is a one-line change that often shaves a few percent off application latency. Then you obsess about eliminating allocations in the inner loop, which buys the orders-of-magnitude wins.

A typical HFT process at steady state allocates *zero bytes per second*. You can verify this by observing `/proc/<pid>/statm`'s RSS over time — if it's not changing, you're not allocating.

## 9.7 The Architecture of a Trading System

Let me sketch what a real trading system looks like, end to end, so you understand where Rust fits.

```
       Exchange
        ──┬──
          │ (10 Gbps Ethernet)
          ▼
      ┌───────────┐
      │   NIC     │
      └─────┬─────┘
            │ DMA via DPDK
            ▼
   ┌─────────────────┐
   │ Market Data     │    Pinned to core 2
   │ Receiver        │
   │ (Rust, hot)     │
   └────────┬────────┘
            │ SPSC queue
            ▼
   ┌─────────────────┐
   │ Strategy        │    Pinned to core 4
   │ Engine          │
   │ (Rust, hot)     │
   └────────┬────────┘
            │ SPSC queue
            ▼
   ┌─────────────────┐
   │ Order Gateway   │    Pinned to core 6
   │ (Rust, hot)     │
   └────────┬────────┘
            │ DMA via DPDK
            ▼
        ┌───────┐
        │  NIC  │ (separate NIC, dedicated to outbound orders)
        └───────┘
            │ (10 Gbps Ethernet)
            ▼
       Exchange
```

Off to the side, in non-hot-path-critical processes:

* **Risk system**: pre-trade risk checks (position limits, capital checks). Has to be fast (<5μs added latency) but not on the same thread.
* **Logger**: per-event logger. Writes to a memory-mapped file. The hot threads emit events to a logger thread via SPSC; the logger thread persists.
* **Config / Control**: external API for changing parameters mid-run. HTTP server (axum) on a non-hot-path core.
* **Metrics**: counters for everything. Aggregated and sent to a monitoring system every second.

The hot path is dedicated, pinned, kernel-bypassed, allocation-free. Everything else lives in the same process but on different cores, with messages passed via SPSC queues so the hot threads never block.

Each component is a few hundred to a few thousand lines of Rust. The whole system might be 50K-200K lines. A typical team owning the matching engine: 5-10 engineers.

## 9.8 Logging Without Hurting

Logging in HFT is paradoxical: you need extensive logs for debugging and compliance, but writing logs from the hot path is unacceptable (it allocates, blocks on I/O, calls into the kernel).

The standard pattern: **deferred / asynchronous logging**.

```rust
struct LogEvent {
    timestamp: u64,
    level: u8,
    code: u16,           // Pre-defined event code, not a string.
    payload: [u8; 32],   // Inline data.
}

// Hot path:
let event = LogEvent {
    timestamp: rdtsc(),
    level: LEVEL_INFO,
    code: EVENT_ORDER_FILLED,
    payload: serialize_order_id(order_id),
};
log_queue.try_push(event).ok();   // SPSC queue, never blocks the hot path

// Logger thread (cold):
loop {
    if let Some(event) = log_queue.try_pop() {
        format_and_write_to_file(event);   // Slow ops happen here.
    } else {
        std::thread::yield_now();
    }
}
```

Properties:

* **No string formatting in the hot path.** Events are integer codes; formatting happens in the logger thread.
* **No allocations in the hot path.** The event struct is fixed-size on the stack.
* **No kernel calls in the hot path.** The SPSC queue is pure userspace.

Production crates that do this: `tracing` with the `tracing-subscriber` chrome layer, or specialised crates like `quanta` for timestamping. For really custom needs, you write your own — it's not much code.

## 9.9 Benchmarking the Whole System

Microbenchmarks (Lesson 7) are necessary but insufficient. The whole-system question is: "from market-event-on-the-wire to order-on-the-wire, what's the p99.9 latency?"

This requires:

1. A traffic generator that produces realistic market data at production rates.
2. Hardware timestamping of packets in and out.
3. A way to correlate inbound events with outbound orders (typically by event ID).
4. End-to-end measurement of the full pipeline.

Many shops build internal "shadow exchanges" — programs that simulate the exchange's behavior, replay historical data at controlled speeds, and provide hardware timestamps. The trading system is run against the shadow, latency is measured, regressions caught.

A quick approximation in Rust: `tcpdump` your inbound and outbound NIC, correlate by event ID in the payload, compute end-to-end latency. Crude but works.

The business question is always: "is our p99.9 lower than competitors'?" There's no competition for first place if everyone's at the same physical speed; the differentiator is who has the lowest tail.

## 9.10 The Codebase Conventions

When you join a low-latency Rust team, the code looks different from app Rust. Conventions:

**Module boundaries match thread boundaries.** The market data receiver is one module, the strategy engine is another, the order gateway is a third. Each module has a clear input queue and output queue. State is private. Cross-thread communication is explicit through queues, not shared state.

**Heavy use of `unsafe`.** More than typical Rust. Mostly in narrow, well-audited spots: SPSC queue internals, packet parsers, allocator interfaces. The team has a culture of code review and testing for unsafe code.

**`#[inline(always)]` is common.** On the perf-critical functions. The default heuristic compiler is conservative; you sometimes need to override it. Verify with `cargo asm` afterwards.

**Const generics everywhere.** Buffer sizes, queue depths, lookup tables — all parameterised at compile time so the compiler can specialise. `struct OrderBook<const N: usize> { levels: [Level; N] }`.

**Custom error types but no anyhow.** The hot path doesn't `Err(anyhow!(...))` because allocating an error message is slow. Errors are integer codes or small enums. `Result<T, ErrCode>` with ErrCode being a small repr-u8 enum.

**Profiling-driven development.** Every PR includes benchmark numbers. "I reduced p99 from 850ns to 720ns" is the unit of progress.

**Minimal dependencies.** Adding a crate adds compilation time, code size, surface area. The hot path has near-zero dependencies. Cold paths can be more liberal.

**Tons of testing infrastructure.** Determinism replay tests, microbenchmarks per component, end-to-end shadow tests, fuzz tests for parsers, loom tests for lock-free code, soak tests running for days to find leaks. The CI pipeline is more elaborate than the application code.

**Comprehensive metrics.** Every queue's depth, every component's processing latency, every allocator's bytes/sec. Visible in real-time. When latency spikes, you can pinpoint within minutes.

## 9.11 What This Career Looks Like

You came here asking about prop trading firms. Some context.

Roles in HFT engineering:

* **Trading engineers**: build the matching engine, market data, order gateway. Primarily systems work. Rust, C++, sometimes Java (but Java is fading for HFT).
* **Strategy engineers**: build trading models. Mix of systems and statistics. Often Python for research, Rust/C++ for production.
* **Infrastructure engineers**: kernel tuning, networking, hardware. Mostly Linux/C, increasingly Rust.
* **Quantitative researchers**: statistical models, backtesting. Mostly Python/Julia/R; not your job after this course unless that's your specific bent.

Pay is high. Hours are intense (markets are open NY/London/Tokyo time, you support production). The work is technically demanding and intellectually narrow — you'll get extremely good at one thing, and you might or might not enjoy that.

If you want to break in:

* **Build a personal project**: a matching engine, or a market-data parser for a public protocol like NASDAQ ITCH. Get it benchmarked and on GitHub.
* **Read the public domain literature**: Carrie Solinger's writing on exchange systems, the LMAX papers, Mara Bos's *Rust Atomics and Locks*, Drepper's memory paper.
* **Apply to firms with strong Rust adoption**: Jane Street is OCaml mostly, but DRW, Optiver, IMC, Hudson River Trading, Citadel Securities, Two Sigma, Jump Trading all have Rust. As of 2026 the trend is clearly toward Rust over C++ for new systems.
* **Pass technical interviews**: heavy on systems-level reasoning, latency questions, lock-free design, debugging at the assembly level. The Phase 1-3 material plus the drills are roughly the floor of what's expected.

The skills generalize. Game engines, real-time control, embedded systems, low-latency databases, browser engines, search engines — all need similar discipline. HFT is a high-paying specialty among many.

## 9.12 Summary: The Rules

1. **The OS is the enemy in the hot path.** Pin threads, isolate cores, bypass the kernel where possible.
2. **`isolcpus`, `nohz_full`, `core_affinity`, `mlockall`, huge pages.** Memorise these names; you'll need them all.
3. **Kernel-bypass networking is the standard.** DPDK is the open option; vendor SDKs (Solarflare) are common in established shops.
4. **Hardware timestamping for measurement.** PTP for inter-machine clock sync. RDTSC for cheap intra-thread timing.
5. **Wire protocols are binary, fixed-width, predictable.** ITCH, OUCH, SBE. Not JSON, not Protobuf with varints.
6. **`zerocopy` for parsing.** Mark structs `#[repr(C)]`, transmute from byte buffer, validate with traits.
7. **No system allocator in the hot path.** Use mimalloc/jemalloc for cold; pre-allocate everything for hot.
8. **Architecture: one thread per stage, SPSC queues between, all pinned, all bypass.**
9. **Logging via deferred/async pattern.** Hot path enqueues compact events; cold thread formats.
10. **Whole-system benchmarking with hardware timestamps.** Microbenchmarks lie about end-to-end behavior.
11. **Code conventions favour explicit unsafe, const generics, integer error codes, comprehensive testing.**
12. **The career is technically narrow and well-compensated.** Match it to your interests and your willingness to specialise.

## 9.13 Drill 9

These drills assume you have a Linux machine with sudo access (a VM is fine for most; a dedicated box helps for the latency-sensitive ones). Some drills require multiple machines with networking; if you can't, do the analysis without the measurement.

**Q1. CPU pinning effects.**

Build a tight benchmark: a single-threaded loop that increments a counter 1 billion times. Time it three ways:

* (a) With no pinning, default settings.
* (b) With `core_affinity` pinning to core 0.
* (c) With `core_affinity` pinning to core 0, and the system isolated as much as possible (close other apps, disable browser, disable Spotify, etc.).

Repeat each run 10 times. Report mean and standard deviation. The pinned version should have lower variance. The pinned-and-isolated version should have lower variance still.

If you have access to a Linux machine where you can boot with `isolcpus=N`: rerun (b) with the pinned core in the isolated set. Variance should drop dramatically.

**Q2. The mlockall experiment.**

Allocate a 1GB Vec<u8>. Time how long it takes to zero out the entire thing.

Now allocate the same Vec, immediately after allocation call `mlockall(MCL_CURRENT | MCL_FUTURE)`, then time the zeroing.

The first run should be much slower because it's page-faulting as you write. The second should be uniform. Report the difference.

(You may need to run as root or grant `CAP_IPC_LOCK` to make `mlockall` work.)

**Q3. Wire protocol parser.**

Implement a parser for NASDAQ ITCH 5.0's "Add Order" message. Specification: https://www.nasdaqtrader.com/content/technicalsupport/specifications/dataproducts/NQTVITCHspecification.pdf (yes, look it up).

Two implementations:

* (a) Idiomatic: bytes via `Cursor`, individual reads with `ReadBytesExt`. Allocations.
* (b) Zero-copy: `#[repr(C, packed)]` struct, `zerocopy::FromBytes`, single transmute.

Generate 1M synthetic Add Order messages. Benchmark each parser. Report ns/message.

The zero-copy version should be 5-20x faster. If it isn't, check for: bounds checks not eliminated, byte-swapping costs (ITCH is big-endian; on x86 little-endian you have to swap), unaligned access penalties.

**Q4. SPSC log pipeline.**

Build a logger:

* Hot path: emits 1M events, each ~32 bytes, into an SPSC queue. Measure per-emit latency (use the corrected histogram from Lesson 7).
* Cold path: dequeues events and writes them to a file as JSON.

Three configurations:

* (a) Hot path does `println!` directly.
* (b) Hot path emits an event into a `tokio::sync::mpsc::channel`.
* (c) Hot path emits via your SPSC; cold-path thread serialises.

Report p50, p99, p99.9 for the hot path in each. Report total throughput (events/sec) too.

You should see the SPSC version with sub-100ns hot-path latency at 99th percentile. The println version will be 100-1000x slower.

**Q5. End-to-end measurement on loopback.**

Spin up two threads on the same machine, communicating via TCP loopback. The "client" thread sends a 64-byte message, waits for a response. The "server" thread reads, processes (just echoes), responds. Measure round-trip time per request.

Use `std::net::TcpStream` first. Report p50, p99, p99.9, max latency for 100,000 requests.

Now use UDP. Repeat. Report.

Now (advanced) try `AF_XDP` via the `xsk-rs` crate or similar. Repeat. The latency should drop from ~10-20μs (TCP) to ~5-10μs (UDP) to ~2-3μs (XDP).

(Real production setups go further with kernel bypass, achieving <1μs RTT on dedicated hardware. We can't reach that in a generic Linux environment without specific NIC support.)

**Q6. The matching engine, fully.**

This is the capstone drill. Build a complete matching engine for one trading pair, with all the lessons applied:

* **Wire input**: read newline-delimited JSON events from stdin (or a file). For bonus realism, parse a real binary protocol (a subset of ITCH).
* **Engine core**: limit-order book, price-time priority, deterministic. Single-threaded. Zero allocations after warmup.
* **Wire output**: emit trades to stdout / a file in the same format as input.
* **Threading**: one thread per stage (input parser, engine, output formatter). SPSC between them.
* **Pinning**: each thread on its own core via `core_affinity`.
* **Determinism harness**: run twice with same input, diff outputs, must be byte-identical.
* **Benchmarking**: report throughput (events/sec) and per-event p50/p99/p99.9 latency from input parse to output emit.

Targets on a modern Linux laptop:

* 5M+ events/sec sustained.
* p99 latency < 5 μs.
* p99.9 latency < 50 μs.
* Zero bytes/sec allocated after warmup (verify with `/proc/<pid>/statm`).

This is the kind of project that gets you a serious interview at a prop trading firm. Put it on GitHub. Write a clear README explaining your design choices and showing the benchmark output.

**Q7. Reading.**

Read these:

* The LMAX Disruptor paper, again. Now you have the prerequisites to follow every detail. https://lmax-exchange.github.io/disruptor/files/Disruptor-1.0.pdf
* Mara Bos, *Rust Atomics and Locks*, the entire book. Free online. https://marabos.nl/atomics/
* Carrie Solinger, "What's in a Matching Engine?" series of blog posts. https://web.archive.org/web/...solinger... (search; she's published widely)
* The DPDK documentation, at least the introduction and architecture pages. https://doc.dpdk.org/guides/

After reading, write 500 words on a topic of your choice from this lesson, going deeper than the lesson did. Suggested topics:

* The full memory-ordering analysis of an MPSC queue.
* How DPDK actually delivers packets to userspace, mechanically.
* The tradeoffs between full kernel bypass (DPDK) and partial (AF_XDP).
* What happens, in detail, in the 100ns of an L3 cache miss.

This essay is partly for your understanding and partly for the interview portfolio — being able to write coherently about systems is itself a job skill.

---

## Phase 3 Master Rules

A condensed reference, organised the way a senior HFT engineer would think.

### Mental model

* The latency hierarchy: L1 (1ns) << RAM (100ns) << syscall (μs) << network (ms).
* Cache lines are 64 bytes. Pack hot data; separate cross-thread writers.
* Branch predictors love consistency. Data-dependent branches kill performance.
* RAM is the bottleneck more often than ALU. Optimise memory access patterns.
* p50 is comfort; p99.9 is the truth.

### Measurement

* Always `criterion` for micros, `perf`/`flamegraph` for whole programs.
* Always `black_box` benchmark inputs.
* Always histograms, percentiles, never just means.
* Always corrected for coordinated omission.
* Test on production-like hardware, not your dev laptop, for tail-latency claims.

### Hot path discipline

* Zero allocations. Pre-allocate everything at startup.
* Fixed-point integers for prices. Never floats.
* No HashMap. Arrays indexed by ID, sorted Vec, BTreeMap if you must iterate.
* No format!, no Vec growth, no String operations.
* No syscalls. No locks. No blocking.
* Single-threaded engines, with SPSC queues between threads.

### Lock-free

* Locks are fast uncontended (25ns), catastrophic when waking the kernel.
* Atomic operations have memory ordering implications. The five orderings: Relaxed < Acquire/Release < AcqRel < SeqCst.
* Acquire/Release pairs are the workhorse pattern.
* SeqCst when in doubt.
* Always cache-line-pad cross-thread atomic data.
* Test on x86 AND ARM. ARM exposes bugs that x86 hides.
* Use loom for testing lock-free code.
* SPSC > MPMC. Shard problems to avoid MPMC.
* For pointer-based lock-free: `crossbeam-epoch`, or use array indices to avoid the problem.

### System level

* Pin threads to cores with `core_affinity`. No exceptions.
* Isolate cores with `isolcpus`, `nohz_full`.
* Pre-fault memory with `mlockall`.
* Use huge pages for large working sets.
* Kernel-bypass networking (DPDK, AF_XDP, vendor SDKs) for serious latency.
* Hardware timestamping for measurement; PTP for cross-machine clock sync.
* RDTSC for cheap intra-thread timing.

### Wire and storage

* Binary, fixed-width protocols. ITCH, OUCH, SBE. Not JSON, not text.
* `#[repr(C, packed)]` + `zerocopy` for safe transmute parsing.
* No serde in the hot path.
* Custom allocators (mimalloc) for cold paths; pre-allocation for hot.
* Deferred/async logging via SPSC; never log directly from the hot path.

### Architecture

* One thread per stage. SPSC queues between.
* Each stage pinned to a dedicated core.
* State is thread-local. Cross-thread communication is explicit.
* Risk, logging, control APIs on different cores from the hot path.
* Determinism replay test in CI on every commit.

### Code conventions

* `#[inline(always)]` for hot functions. Verify with `cargo asm`.
* Const generics for compile-time-known sizes.
* Integer error codes, not allocated error messages.
* Heavy use of `unsafe` in narrow, well-audited spots.
* Profiling-driven PRs: every change includes benchmark numbers.
* Minimal dependencies in the hot path.
* Comprehensive metrics for every queue and every stage.

### Career advice

* Build a real project. Open-source matching engines, market data parsers, lock-free queues all impress.
* Learn to read assembly, even if you don't write it.
* Study the public-domain literature: LMAX, Mara Bos, Drepper, exchange specs.
* The skills generalize: game engines, kernel work, real-time systems, embedded, browser engines.
* The work is intense and narrow. Make sure the substance interests you, not just the compensation.

### Success criteria

If after Phase 3 you can:

* Explain why a hot loop with cache misses runs 100x slower than the same loop in cache.
* Read an SPSC queue implementation and audit the memory ordering.
* Spot a coordinated-omission bug in a latency benchmark.
* Implement a binary protocol parser at <50ns per message.
* Pin threads, lock memory, route interrupts, and articulate why each helps.
* Build a deterministic matching engine that hits 5M events/sec with sub-5μs p99 latency.
* Pass a technical interview at a prop trading firm by demonstrating these skills.

Then you've completed the foundation of low-latency engineering. Most of the rest is experience: years of benchmarking, debugging, watching production. The patterns are countable; the discipline of applying them consistently is what separates levels of seniority.

---

*Phase 3 complete. There isn't a Phase 4 from me; what's left is specialisation by sub-domain (FPGA acceleration, custom NIC firmware, exchange-specific protocols, market microstructure) and time on the job. Read source code from real production systems. Build things, measure them, fix them. The community is small; the people in it are mostly happy to talk to anyone serious enough to do the work.*
