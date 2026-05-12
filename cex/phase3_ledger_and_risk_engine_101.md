# CEX Matching Engine — Phase 3 Course (Go)

> The Ledger & Risk Engine
> Three lessons. Accounting, margin, and blockchain primers inline. Harsh drills.

---

## Phase 3 Introduction

Phase 1 made your engine fast. Phase 2 made it reliable. Phase 3 makes it **correct and safe**. A fast, reliable, *incorrect* exchange loses money faster than a slow one. You must be able to prove, at any moment, that every dollar and every satoshi is where it should be — and that no code path can create money from nothing.

Three lessons:

1. **Double-Entry Bookkeeping:** the accounting primitive that makes "infinite money" bugs impossible by construction.
2. **Risk Engine:** the pre-trade check that stops users from taking positions they can't cover — in under 10μs, synchronously, before the order reaches the book.
3. **Hot/Cold Wallet Architecture:** the bridge between your fast in-memory ledger and the slow, secure blockchain. Where most exchange hacks happen.

I'll assume you don't know accounting, don't know margin trading, and don't know blockchain internals beyond "people send coins." Primers are inline. Skip nothing.

---

# Lesson 7: Double-Entry Bookkeeping

### Why This Lesson Exists

Phase 1 and 2 built a matching engine. But who owns the assets being matched? How does the exchange track that user A has 5 BTC and user B has 50,000 USDT? How do you ensure that when they trade, the total supply of BTC and USDT in your system doesn't accidentally change?

Answer: a **ledger**. Specifically, a double-entry ledger. This is the accounting primitive used by every bank, every regulated exchange, every serious business since the 1400s. It's boring. It's also load-bearing. Get it wrong and your exchange prints money (good for no one — the accounting breaks and you're insolvent on paper) or loses users' money (career-ending).

**Real exchange failures from ledger bugs:**

- **Mt. Gox, 2014.** Bug where withdrawals could be initiated but not always properly deducted from the internal ledger. Attackers found it and drained ~850,000 BTC over months. By the time anyone noticed, the exchange was insolvent. Gone.
- **BitGrail, 2018.** Internal ledger bug lost ~17M Nano (~$170M). Users never recovered.
- **Various DeFi exploits (DAO, bZx, etc.)** — same pattern at the smart-contract level: write the state change in the wrong order, attacker drains the pool.

Double-entry bookkeeping prevents this entire class of bugs **by construction**. Done right, money literally cannot appear or disappear in your system — every operation is a zero-sum rearrangement. The mechanical check that enforces it is called the balance equation, and you run it constantly.

---

### Accounting Primer

Five concepts. Learn once, use forever.

**1. Account.** A named bucket that holds a balance. "User 123's BTC balance." "Hot wallet BTC custody." "Exchange fee revenue in USDT." Every distinct bucket of value is an account. Accounts are typed by asset (BTC account, USDT account) and by category (user balance, custody, equity, etc.).

**2. Debit and Credit.** A change to an account. Confusingly, "debit" and "credit" don't just mean "+" and "−"; their meaning depends on account type:

- **Asset** accounts (things the exchange possesses): debit = increase, credit = decrease.
- **Liability** accounts (things the exchange owes to others — *this includes all user balances*): debit = decrease, credit = increase.
- **Equity** accounts (the exchange's own money — fees collected, proprietary trading P&L): debit = decrease, credit = increase.

For our purposes, stop worrying about the words. Think of every entry as a signed amount, and remember: **every transaction has entries that sum to zero, per asset**. That's the only rule that matters.

**3. Transaction (Journal Entry).** A set of entries that together represent one business event. Example: "User deposits 1 BTC" is:

```
Transaction: deposit
  Entry: +1 BTC to Asset account "Hot Wallet BTC Custody"
  Entry: +1 BTC to Liability account "User 123 BTC Balance"
Sum (asset): +1
Sum (liability): +1
Net: asset − liability = 0 ✓
```

Depending on your sign convention, you can represent this as two positive entries (one asset +, one liability +) or as +1 on custody and −1 on a "to-be-owed" side. The important thing is the balance-sheet equation below still holds.

I'll use this simpler convention for the rest of this lesson: **every entry is a signed amount, and entries in a transaction sum to zero per asset.** Asset increases and liability increases are represented as matched +/− pairs from the exchange's balance-sheet perspective.

**4. The Balance Sheet Equation.**

```
Assets = Liabilities + Equity
```

Every business on earth. Rewritten for a CEX:

```
(Total custody of each asset)  =  (Sum of all user balances for that asset)
                                  + (Exchange's own holdings, i.e., equity)
```

This must hold for *every asset*, at *every moment*. If it doesn't, you have a bug or you're insolvent. This is the invariant you defend with your life. Check it after every transaction in development. Check it periodically in production. If it ever fails, stop all trading until you find the cause.

**5. Double-Entry.** Every transaction has entries that sum to zero per asset. This is the mechanical check that makes infinite money bugs impossible: if your transaction doesn't sum to zero, it's rejected. If your transaction *type* can't be expressed with entries summing to zero, you've misunderstood the operation — go back and think again.

---

### The CEX Account Structure

For a CEX, the accounts look like:

```
ASSETS (things the exchange possesses):
  Custody.BTC.HotWallet           ← BTC in hot wallet
  Custody.BTC.ColdWallet          ← BTC in cold storage
  Custody.BTC.DepositAddresses    ← BTC in user deposit addresses (pre-sweep)
  Custody.USDT.HotWallet
  Custody.USDT.ColdWallet
  ...

LIABILITIES (things the exchange owes to users):
  User.123.BTC.Available          ← what User 123 can trade or withdraw
  User.123.BTC.Locked             ← locked in open orders
  User.123.USDT.Available
  User.123.USDT.Locked
  User.456.BTC.Available
  ...

EQUITY (the exchange's own money):
  Equity.BTC.Fees                 ← trading fees collected in BTC
  Equity.USDT.Fees
  Equity.USDT.InsuranceFund       ← reserve for liquidation losses
  ...
```

Note that `User.123.BTC.Available` and `User.123.BTC.Locked` are *distinct* accounts. Locking funds for an open order moves value from one to the other — a liability-to-liability transfer, still a zero-sum transaction.

---

### Every Event Is a Zero-Sum Transaction

Watch how every real-world event decomposes.

**Deposit 1 BTC (after N confirmations):**
```
+1 BTC  Custody.BTC.DepositAddresses   (asset increased)
−1 BTC  User.123.BTC.Available          (liability increased, represented as negative
                                         from exchange's balance-sheet perspective)
Sum: 0 ✓
```

**Sweep from deposit address to hot wallet:**
```
−1 BTC  Custody.BTC.DepositAddresses
+1 BTC  Custody.BTC.HotWallet
Sum: 0 ✓  (asset-to-asset transfer)
```

**Place a limit buy order for 0.1 BTC at 50,000 USDT:**
```
+5,000 USDT  User.123.USDT.Available    (remove from available)
−5,000 USDT  User.123.USDT.Locked       (add to locked)
Sum (USDT): 0 ✓
```

**Fill (User A buys 0.1 BTC from User B @ 50,000):**
```
−0.1 BTC    User.A.BTC.Available        (A receives BTC; liability increase → negative)
+0.1 BTC    User.B.BTC.Available        (B gives up BTC; liability decrease → positive)
+5,000 USDT User.A.USDT.Locked          (A pays from locked)
−5,000 USDT User.B.USDT.Available       (B receives USDT)
Sum (BTC): 0 ✓
Sum (USDT): 0 ✓
```

**Fee (exchange takes 0.1% of USDT side = 5 USDT from taker):**
```
+5 USDT  User.A.USDT.Available
−5 USDT  Equity.USDT.Fees
Sum (USDT): 0 ✓
```

**Withdraw 2 BTC (ledger-side, before on-chain execution):**
```
+2 BTC   User.123.BTC.Available
−2 BTC   User.123.BTC.Withdrawing        (limbo: debited from user, not yet broadcast)
Sum: 0 ✓
```

**Withdrawal confirmed on chain:**
```
+2 BTC   User.123.BTC.Withdrawing        (close the limbo account)
−2 BTC   Custody.BTC.HotWallet           (the exchange paid out)
Sum: 0 ✓  (Liability decrease + Asset decrease: exchange's balance sheet shrinks)
```

Every single action fits the pattern. If you can't express it as a zero-sum transaction, you don't understand the action yet.

---

### Implementation: Transactions and Entries

```go
type AccountID struct {
    Category string  // "User", "Custody", "Equity"
    Scope    uint64  // user ID, 0 for custody/equity
    Asset    string  // "BTC", "USDT", ...
    Kind     string  // "Available", "Locked", "HotWallet", "Fees", ...
}

type Entry struct {
    Account AccountID
    Amount  int64   // fixed-point, signed
}

type Transaction struct {
    ID        uint64
    Timestamp int64
    Type      string            // "deposit", "trade", "fee", "reserve", "withdraw"
    Entries   []Entry
    Metadata  map[string]string // event_id, order_id, trade_id, txid
}

func (t *Transaction) Validate() error {
    // Rule 1: entries per asset must sum to zero.
    totals := map[string]int64{}
    for _, e := range t.Entries {
        totals[e.Account.Asset] += e.Amount
    }
    for asset, sum := range totals {
        if sum != 0 {
            return fmt.Errorf("entries don't sum to zero for %s: got %d", asset, sum)
        }
    }
    return nil
}
```

**Critical property:** `Validate()` is called *before* the transaction is applied. Any code path that constructs a transaction is mechanically forced to account for both sides. Forget the counterparty entry? Validate fails, transaction rejected. Forget the fee counterparty? Validate fails. You cannot accidentally create money if you go through this code.

---

### Atomicity: All or Nothing

Applying a transaction must be **atomic**: every entry applied, or none. If you credit User A but crash before debiting User B, you've created value. After recovery, the invariant check fails, trading halts, everyone panics.

Two approaches:

**1. Database transaction (Postgres).** Wrap entries in a DB transaction. If anything fails, rollback. Slow (~ms per transaction), correct. Use this for off-engine ledgers: deposits/withdrawals, reconciliation.

**2. In-memory single-writer with WAL.** Write the *entire* transaction to the WAL, fsync, then apply all entries in-memory. Because the WAL is durable, recovery can replay. Because the in-memory apply is single-goroutine, it either completes fully or the process crashes (in which case recovery replays from the WAL). Fast (~μs), also correct — but only under single-writer discipline.

For the matching engine's ledger, option 2. For the withdrawal batching and reconciliation layer, option 1.

**The key sequence:**
```
Receive event
  ↓
Construct Transaction (all entries)
  ↓
Validate (sum to zero)
  ↓
WAL write + fsync
  ↓
Apply entries in-memory (one by one, no other goroutines)
  ↓
Acknowledge
```

If you crash anywhere after "WAL + fsync," recovery replays the transaction. If you crash before, the transaction is gone but nothing was applied — acknowledge was never sent, sequencer will re-send. Correct in both cases.

---

### The Fund Reservation Pattern

You've seen this in System Design 101 Lesson 8. At the ledger level it looks like this:

When User A places a limit buy for 1 BTC at 50,000 USDT, the exchange must lock 50,000 USDT immediately. Otherwise:

1. A places order X1 (consumes 50k USDT intention).
2. A places order X2 (exchange naively allows — no lock).
3. A withdraws 50k USDT.
4. Both orders fill. Exchange owes 2 BTC against 0 USDT. Insolvent.

**Fix: reserve at order placement.**

Order placement transaction:
```
+50,000 USDT  User.A.USDT.Available
−50,000 USDT  User.A.USDT.Locked
```

Order cancellation transaction (inverse):
```
−50,000 USDT  User.A.USDT.Locked
+50,000 USDT  User.A.USDT.Available
```

Fill transaction (partial fill 0.3 BTC):
```
−0.3 BTC      User.A.BTC.Available          (A receives 0.3 BTC)
+0.3 BTC      User.B.BTC.Available          (B gives up 0.3 BTC)
+15,000 USDT  User.A.USDT.Locked            (A pays from locked, 0.3 × 50k)
−15,000 USDT  User.B.USDT.Available         (B receives USDT)
```

Remaining 35,000 USDT stays locked against the 0.7 BTC unfilled portion.

**The withdrawal check reads `Available`, not `Available + Locked`.** That's the whole trick. Locked funds can't be withdrawn. Can't be used for another order. Properly accounted for. Cancelation returns them.

---

### The Golden Invariant Check

After every transaction in development, periodically in production, you verify:

```
For each asset:
    (sum of all Custody accounts for this asset)
    == (sum of all User.*.Available and User.*.Locked for this asset)
       + (sum of all Equity accounts for this asset)
```

If this fails, stop trading immediately. You have either a code bug or real insolvency. Neither can be ignored.

```go
func (l *Ledger) CheckInvariant() error {
    byAsset := map[string]int64{}
    for acctID, balance := range l.balances {
        switch acctID.Category {
        case "Custody":
            byAsset[acctID.Asset] += balance   // assets: positive
        case "User":
            byAsset[acctID.Asset] -= balance   // liabilities: subtract
        case "Equity":
            byAsset[acctID.Asset] -= balance   // equity: subtract
        }
    }
    for asset, net := range byAsset {
        if net != 0 {
            return fmt.Errorf("invariant broken for %s: net %d", asset, net)
        }
    }
    return nil
}
```

If this ever returns an error in production, you have seconds to halt trading before the bug drains more money. Build alerting that pages on this, not a Slack notification.

---

### Common Bugs

**1. Forgetting the counterparty leg.** Crediting User A but forgetting to debit User B (or the custody account). `Validate()` catches it if you run it. The failure mode is "engineers bypass `Validate()` for 'performance' reasons." Don't.

**2. Wrong asset type.** Fee transaction specifies BTC for one entry and USDT for another. `Validate()` catches it because sums are computed per-asset.

**3. TOCTOU (time-of-check to time-of-use).** Example: risk check reads balance, decides order is OK. Between that read and the lock transaction, another transaction runs and reduces the balance. Now the lock produces negative. Fix: single-threaded ledger. One goroutine processes all transactions. No interleaving possible.

**4. Reentrancy.** Function A applies a transaction that triggers a callback, which calls A again with a derived transaction, before A's first transaction is committed. The DAO hack in Ethereum. In a CEX ledger: avoid by building the full transaction in memory *first*, validating, *then* applying. No external calls in the middle of apply.

**5. Floating point accumulator.** `float64` sum of 1 million balances drifts by ~1e-10 per sum. Drift accumulates. After months, the invariant starts failing for no apparent reason. Fix: fixed-point `int64` throughout. Same discipline as Phase 1 Lesson 2.

**6. Non-atomic apply after crash.** Crash mid-apply, some entries applied, others not. Recovery replays — but if the WAL only had *individual* entries, not the full transaction, recovery can't tell what was meant to be atomic. Fix: WAL the entire transaction in one entry.

**7. Replay non-idempotency.** WAL replayed twice (e.g., recovery ran, then ran again). Transactions applied twice. Fix: transaction ID check — each transaction has a unique ID, apply only if not already seen.

---

### Go Implementation: A Working Ledger

```go
type Ledger struct {
    balances    map[AccountID]int64   // single-goroutine access only
    appliedIDs  map[uint64]bool        // transaction IDs already applied (for replay idempotency)
    wal         *WAL                   // from Phase 2
    nextTxID    uint64
}

func (l *Ledger) Apply(tx Transaction) error {
    if tx.ID == 0 {
        l.nextTxID++
        tx.ID = l.nextTxID
    }
    if l.appliedIDs[tx.ID] {
        return nil // idempotent: already applied
    }
    
    if err := tx.Validate(); err != nil {
        return err
    }
    
    // Pre-check: accounts that must stay non-negative don't go negative.
    for _, e := range tx.Entries {
        current := l.balances[e.Account]
        if current+e.Amount < 0 && mustBeNonNegative(e.Account) {
            return fmt.Errorf("account %v would go negative", e.Account)
        }
    }
    
    // Durable log before state mutation.
    payload := serializeTx(tx)
    if _, err := l.wal.AppendSync(payload, tx.Timestamp); err != nil {
        return err
    }
    
    // Apply atomically (single goroutine).
    for _, e := range tx.Entries {
        l.balances[e.Account] += e.Amount
    }
    l.appliedIDs[tx.ID] = true
    return nil
}

func mustBeNonNegative(acct AccountID) bool {
    // User available and locked must not go negative.
    // Custody can't go negative (we can't have less than zero BTC on hand).
    // Equity can in principle, but usually you'd halt if that happens.
    return acct.Category != "Equity"
}
```

For the hot path (matching engine), the map is replaced by Phase 1 / Phase 3 Lesson 3 technique: pre-allocated arrays, indexed by internal IDs. Same logic, zero allocation.

---

### Integration with the Matching Engine

The matching engine produces events ("order placed," "order canceled," "fill at X"). Each event becomes one or more ledger transactions:

```go
func (e *Engine) Step(evt Event) {
    switch evt.Type {
    case OrderPlace:
        tx := buildReservationTx(evt)         // reserve funds
        if err := e.ledger.Apply(tx); err != nil {
            e.emitReject(evt, err)
            return
        }
        e.book.Place(evt.Order)
    case OrderFill:
        tx := buildFillTx(evt)                 // move between users + fee
        if err := e.ledger.Apply(tx); err != nil {
            panic("fill transaction failed - ledger is broken")  
            // should never happen in production; fills presume prior reservation
        }
        e.emitFill(evt)
    case OrderCancel:
        tx := buildCancelTx(evt)               // unlock funds
        e.ledger.Apply(tx)
        e.book.Cancel(evt.OrderID)
    }
}
```

**The ledger lives in the same goroutine as the engine.** Single writer. No synchronization cost. Every fill the engine emits is mechanically accompanied by a balance-changing transaction. Total supply is invariant by construction.

---

### Summary: The Rules

1. **Every transaction's entries sum to zero per asset.** `Validate()` enforces it mechanically.
2. **`Assets = Liabilities + Equity`** for every asset, at every moment. Check it. Alert on failure.
3. **Fund reservation at order placement.** Separate Available / Locked accounts.
4. **Fixed-point `int64` arithmetic.** No floats in balances, ever.
5. **WAL the whole transaction before applying.** Atomicity through replay.
6. **Single-goroutine ledger.** No TOCTOU, no interleaving, no races.
7. **Idempotent apply.** Transaction IDs, replay-safe.
8. **Invariant check in production.** Periodic + after every transaction in dev.

---

### Drill 7

**Q1. Construct the entries.**
Write out the full set of entries, with amounts and signs, for each transaction below. Verify each sums to zero per asset.

(a) User 123 deposits 10,000 USDT (after confirmations, before sweep).
(b) Sweep User 123's 10,000 USDT from deposit address to hot wallet.
(c) User 123 places a limit sell order: 0.5 ETH @ 3,000 USDT.
(d) Partial fill: 0.3 ETH of User 123's order fills against User 456's buy at 3,000 USDT.
(e) Exchange takes a 0.1% taker fee in USDT from User 456 on that 0.3 ETH fill.
(f) User 123 cancels the remaining 0.2 ETH of the order.
(g) User 123 withdraws 1,000 USDT. Ledger-side only, not yet on-chain.

**Q2. Ledger implementation.**
Implement a Ledger in Go with:
- `AccountID` struct as shown.
- `Transaction` / `Entry` with `Validate()`.
- `Apply(tx Transaction) error` that integrates with your Phase 2 WAL.
- Idempotent replay (by transaction ID).
- `CheckInvariant()` that verifies `Assets = Liabilities + Equity` per asset.

**Q3. Fuzz the invariant.**
Write a test harness that:
- Creates 1,000 users with random initial deposits.
- Runs 1 million random transactions: deposits, withdrawals, limit order placements, fills, cancels, fees.
- After each transaction, runs `CheckInvariant()`.
- The invariant must hold for all 1M transactions. Every time.

Then introduce a deliberate bug (e.g., drop one entry from the fee transaction's entries). Verify your invariant check catches the bug. How many transactions does it take to catch? Is it deterministic or probabilistic? Explain.

**Q4. Integrate with the engine.**
Wire the ledger into your Phase 1+2 matching engine. Every place/cancel/fill emits a ledger transaction. Benchmark:
- Ledger transactions per second (at matching engine peak throughput).
- ns per `Apply` call (target <1μs on modern hardware).
- Zero allocations in `Apply` (per Lesson 3 discipline).

**Q5. Reading.**
Read an exchange exploit post-mortem — Mt. Gox (2014), BitGrail (2018), or Coincheck (2018) are the classics. Identify which specific ledger / accounting invariant was broken. Could the bug have been prevented with the double-entry discipline from this lesson? If yes, how? If no, why not? Write 2-3 paragraphs.

**Q6. Reconciliation thought experiment.**
How would you reconcile your in-memory ledger against the on-chain state of your hot wallet BTC? Walk through the procedure and identify what edge cases make exact matching hard (pending transactions, mempool, reorgs, sweeps in flight, fees). Lesson 9 will give you the answers.

---

# Lesson 8: Risk Engine

### Why This Lesson Exists

The ledger records what happened. The risk engine stops things that *shouldn't* happen.

Specifically, before every order reaches the matching engine, the risk engine must verify:

1. User has enough balance / margin to cover the order.
2. The resulting position doesn't exceed per-user position limits.
3. Exchange-wide risk limits (open interest caps, concentration) aren't breached.
4. The order isn't a fat-finger mistake (price 100x away from market, size 10000x average).
5. The order isn't self-trading (user trading against themselves).

All of this must happen **synchronously**, **before** the order enters the book, in **under 10μs** — ideally under 1μs. If risk takes 10ms, your engine stalls. If risk runs asynchronously *after* the order hits the book, you have a race: order fills, risk rejects it too late, now you've filled an order that shouldn't have existed.

Pre-trade risk is a tight computational loop. In-process. Allocation-free. Fixed-point. Single-goroutine. The same Phase 1 discipline applied to a different problem.

---

### Margin Trading Primer

If you're building a spot-only exchange, skim this. For futures / margin / perpetuals, you need it.

**Spot trading.** You trade with assets you own. Buy 1 BTC for 50,000 USDT means: you had 50,000 USDT, now you have 1 BTC and 0 USDT. No leverage. No counterparty risk beyond the trade. Our ledger handles this with straightforward available/locked accounts.

**Margin trading.** You trade with borrowed money. "10x leverage" means the exchange lends you 9x what you put up. You control a 50,000 USDT BTC position with only 5,000 USDT of your own money. If BTC moves 1% up, your PnL is 500 USDT on 5,000 collateral = 10% return. If it moves 1% down, same math, you lose 10%. Leverage amplifies both.

**Notional Value.** Total size of the position in quote currency. 1 BTC long at 50,000 USDT = 50,000 notional.

**Initial Margin (IM).** Collateral required to *open* a position. At 10x leverage: IM = 10% of notional. At 100x: 1%. The exchange sets this based on the asset's volatility and the user's tier.

**Maintenance Margin (MM).** Minimum collateral to *keep* a position open. Less than IM. At 10x leverage, MM might be 5% of notional. The gap between IM and MM is the buffer before liquidation.

**PnL (Profit and Loss).**
- Long 1 BTC entry 50,000. Mark at 51,000. PnL = +1,000 USDT.
- Short 1 BTC entry 50,000. Mark at 51,000. PnL = −1,000 USDT.

Unrealized PnL is counted as part of collateral. Realized PnL is settled into the user's balance at close.

**Available Collateral.**
```
Available Collateral = Initial Margin Deposited + Unrealized PnL
```

If Available Collateral drops below Maintenance Margin, you breach MM → liquidation.

**Liquidation Price.** The mark price at which MM is breached. For 1 BTC long at 50,000 with 5,000 IM and 2,500 MM (5%):

```
Loss that breaches MM = 5,000 − 2,500 = 2,500
Liquidation price = 50,000 − 2,500 = 47,500
```

If mark drops to 47,500 or below, the risk engine triggers forced closure.

**Mark Price.** Critical: liquidations are triggered by *mark price*, not *last-traded price*. A malicious trader could place a tiny order at an extreme price, trigger their own liquidations for gain (or others' liquidations to manipulate the market). Mark price is constructed from multiple reference sources — index of external exchanges, time-weighted average, smoothing — to resist manipulation.

---

### Pre-Trade Risk Checks

For every new order, the risk engine computes:

**1. Balance check (spot).**
```
user.available[quote_currency] >= order.price × order.qty
```
Straightforward. Read the available balance from the risk engine's cached view, compare.

**2. Margin check (derivative).**
```
new_position = current_position + order_signed_qty
required_im = abs(new_position) × mark_price × im_rate
user.available_collateral >= required_im
```
Tricky part: "available collateral" includes unrealized PnL on existing positions, which means it depends on the mark price, which is moving. Risk engine maintains live view.

**3. Position limit check.**
```
abs(new_position) <= user_position_limit[symbol]
```
Per-user, per-symbol cap to limit risk concentration.

**4. Exchange-wide limits.**
- Total open interest cap per symbol.
- Insurance fund ratio.
- Concentration (no single user > N% of symbol's open interest).

**5. Sanity checks (fat-finger protection).**
- Price within ±X% of mark.
- Size ≤ Y× average fill size.
- For market orders: estimated fill doesn't empty the book.

All of these are O(1) lookups and a couple of arithmetic ops. Microseconds on modern hardware.

---

### Architecture: Risk In-Line

```
[Gateway]
    ↓
[Risk Engine]   ← reads user state, balances, positions from in-memory views
    ↓ (accept / reject)
[Sequencer]
    ↓
[Matching Engine + Ledger]
    ↓
[Fill Events]
    ↓
  ┌──→ [Mark Price Updater] (async, separate)
  │
  └──→ [Risk State Updater] (applies fills to risk engine's view)
         │
         ↓
      [Risk Engine]   ← closes the feedback loop
```

The risk engine maintains its own state: per-user balances, positions, collateral. This state is updated by consuming:

- **Fill events** from the matching engine → update positions and realized PnL.
- **Ledger events** from deposits / withdrawals → update balances.
- **Mark price updates** → recompute unrealized PnL.

Why separate from the ledger? Two reasons:

1. **Read optimization.** The ledger stores raw entries for correctness. The risk engine needs aggregates (total position per symbol, collateral per user) at every check. Pre-computed, cache-aligned, tight loop.
2. **Throughput.** Risk checks run on every order (millions/sec). The ledger handles only successful fills (a subset). Different load profiles, different data structures.

The two are eventually consistent: a fill updates the ledger and the risk view in the same event-processing step, but from the risk engine's perspective, it's consuming the fill event to update its view. If they ever disagree, the ledger is truth.

---

### The Tight Loop

```go
type UserRisk struct {
    // 64 bytes exactly = 1 cache line
    AvailableUSD     int64   // fixed-point, scale = 1e8
    LockedUSD        int64
    Position         int64   // signed; + = long, − = short
    EntryPrice       int64
    UnrealizedPnL    int64
    PositionLimit    int64
    _                [16]byte // padding
}

type RiskEngine struct {
    users [1 << 20]UserRisk        // pre-allocated 1M users
    
    // Parameters (read-mostly)
    initialMarginRate     int64    // fixed-point
    maintenanceMarginRate int64
    markPrices            [NumSymbols]int64  // atomic updates
}

func (r *RiskEngine) CheckLimitBuy(userID uint32, symbol uint16, price, qty int64) error {
    u := &r.users[userID]
    mark := atomic.LoadInt64(&r.markPrices[symbol])
    
    notional := (price * qty) / priceScale
    requiredIM := (notional * r.initialMarginRate) / marginScale
    
    if u.AvailableUSD < requiredIM {
        return ErrInsufficientMargin
    }
    
    newPosition := u.Position + qty
    if abs(newPosition) > u.PositionLimit {
        return ErrPositionLimit
    }
    
    // Fat-finger: price not more than 10% off mark
    if price < mark*90/100 || price > mark*110/100 {
        return ErrPriceOutOfRange
    }
    
    return nil
}
```

- Array index, not map lookup.
- Fixed-point arithmetic throughout.
- No allocation.
- No string compares.
- Errors are sentinel values, allocated at package init.
- Atomic load for mark price (separate goroutine updates mark).

Benchmark target on modern hardware: < 300ns per check. 3M+ checks/sec on a single core.

---

### The Liquidation Engine

Parallel problem: detect users in breach of maintenance margin and force-close them.

Liquidations are triggered by **mark price changes**, not orders. Each time the mark price ticks:

```go
func (r *RiskEngine) OnMarkPriceUpdate(symbol uint16, newMark int64) {
    atomic.StoreInt64(&r.markPrices[symbol], newMark)
    
    for userID := 0; userID < r.numUsers; userID++ {
        u := &r.users[userID]
        if u.Position == 0 {
            continue
        }
        
        // Recompute unrealized PnL
        u.UnrealizedPnL = (newMark - u.EntryPrice) * u.Position / priceScale
        
        // Check maintenance margin
        availableCollateral := u.AvailableUSD + u.UnrealizedPnL
        notional := abs(u.Position) * newMark / priceScale
        requiredMM := notional * r.maintenanceMarginRate / marginScale
        
        if availableCollateral < requiredMM {
            r.queueLiquidation(uint32(userID), u.Position)
        }
    }
}
```

Scanning 1M users takes ~1ms on a single core. Fast enough if mark updates every 100ms. If you need faster, parallelize (each goroutine scans a shard, or use SIMD via cgo).

**The liquidation queue** is separate: it receives user IDs and positions to liquidate, generates market orders, submits them to the matching engine. Priority: users most deeply under MM first (worst collateralized = most urgent).

---

### The Liquidation Cascade Problem

In a fast crash, many users hit liquidation simultaneously. All their positions get force-closed, adding selling pressure, driving the price down further, triggering more liquidations. Positive feedback loop. Markets crash beyond fundamentals.

**May 2021 Bitcoin flash crash:** BTC fell from ~$58,000 to ~$30,000 in hours. Most of the drop was liquidation-driven.
**LUNA/UST collapse, May 2022:** liquidation cascades across multiple lending protocols compounded the algorithmic de-peg.

**Mitigations:**

1. **Liquidation queue prioritization.** Worst collateralized first. If the market can only absorb so many liquidations per second, take the most urgent first.
2. **Partial liquidations.** Close only enough to bring the user back above MM, not the full position. Less selling pressure.
3. **Rate limiting.** Cap liquidations per second to what the order book can absorb without crashing.
4. **Insurance fund.** The exchange maintains a reserve (funded by fees and prior liquidation windfalls) to absorb losses when the liquidated position's close price is worse than MM. Without this, the exchange bears the loss directly.
5. **Auto-Deleveraging (ADL).** If the insurance fund can't absorb, the exchange claws back PnL from the most-profitable opposite-side traders, pro-rata. Users hate this but it's better than exchange insolvency.
6. **Circuit breakers.** If volatility exceeds a threshold, halt trading for a cooldown window.

---

### Circuit Breakers

Exchange-level safety valves, orthogonal to individual user risk:

- **Price band.** Orders > X% away from mark rejected. Prevents fat-finger from crashing the book.
- **Volatility halt.** If price moves > Y% in Z seconds, halt new orders for a cooldown. Traditional equities use this (NYSE, etc.).
- **Position concentration.** If any user's open interest exceeds W% of the symbol's total OI, block them from adding.
- **Funding rate clamp.** For perpetuals, cap the funding rate to prevent runaway shorts/longs.
- **Order rate limit.** Per-user and exchange-wide.

These are tuning knobs on top of the risk engine's core checks.

---

### Mark Price Construction

A typical mark price formula:

```
mark_price = median(
    external_exchange_1_last_price,
    external_exchange_2_last_price,
    external_exchange_3_last_price,
    our_exchange_last_price  // weighted lower
)
```

Smoothed with exponential moving average over a short window (e.g., 10 seconds). The median makes it resistant to a single external exchange being manipulated. The EMA smooths out single-tick spikes.

For perpetuals, the mark price might also include a *funding* component based on the futures-spot basis. Details depend on contract specification.

---

### Integration with the Ledger

Fund reservation (from Lesson 7) and margin requirements interact. For a margin order:

1. User places a leveraged order.
2. Risk engine computes required IM.
3. Ledger transaction: move required IM from `Available` to `Locked`.
4. Order enters book.
5. On fill: position opens. `Locked` stays locked, now backing the position.
6. On close (by user or liquidation): PnL realized. `Locked` moves back to `Available` ± PnL.

Every step is a ledger transaction. Sum-to-zero. Invariant check catches anything that goes wrong.

---

### Go Implementation: Zero-Allocation Discipline

Reuse Phase 1 Lesson 3 techniques:

- Pre-allocated user array.
- Fixed-point `int64` throughout.
- Sentinel errors: `var ErrInsufficientMargin = errors.New(...)` at init, never `fmt.Errorf` in hot path.
- No `interface{}`.
- No map lookup.
- Atomic operations for mark prices (single-writer: mark updater goroutine).
- User state struct sized to a cache line (or a few) to optimize the tight loop.

Benchmark with `-benchmem`. Target 0 allocs/op. If the mark-update scan allocates, you've done it wrong — the loop should be tight enough to compile to a flat series of memory loads and arithmetic.

---

### Summary: The Rules

1. **Risk is synchronous, in-process, pre-trade.** Never after-the-fact.
2. **Pre-allocate everything. Fixed-point int64. Zero allocation.** Phase 1 Lesson 3 discipline.
3. **Mark price, not last-trade price, for liquidations.** Prevents manipulation.
4. **Separate liquidation queue from main engine flow.** Priority + rate-limit.
5. **Insurance fund + ADL + circuit breakers** for cascade mitigation.
6. **Risk engine has its own state view, synchronized with ledger events.**
7. **Every margin flow integrates with the ledger's reservation mechanism.**

---

### Drill 8

**Q1. Margin math.**
For each scenario, compute liquidation price. Show your work.

(a) Long 1 BTC at entry 50,000 USDT, 10x leverage, MM = 5%.
(b) Short 2 ETH at entry 3,000 USDT, 20x leverage, MM = 3%.
(c) Long 10 BTC at entry 50,000 USDT, 100x leverage, MM = 0.5%. Why is this dangerous even if the math checks out?

**Q2. Risk engine implementation.**
Implement a risk engine in Go with:
- Pre-allocated `[1 << 20]UserRisk` array.
- `CheckLimitBuy`, `CheckLimitSell`, `CheckMarketBuy`, `CheckMarketSell`.
- `OnFill(userID, symbol, side, qty, price)` that updates position and balance.
- `OnMarkPriceUpdate(symbol, newMark)` that recomputes unrealized PnL and queues liquidations.
- Sentinel errors pre-allocated at init.

**Q3. Benchmark.**
Target: `CheckLimitBuy` at < 300ns/op, 0 allocations, 3M+ ops/sec on a single core. Run `go test -bench=. -benchmem`. Report numbers.

**Q4. Liquidation scan.**
Generate a test with 1M users holding random leveraged positions. Call `OnMarkPriceUpdate` with a 5% adverse move. Measure:
- Scan time (target: < 2ms).
- Number of liquidations triggered.
- Memory allocations during scan (must be 0).

Now simulate a flash crash: price drops 10% in 1 second (100 mark-price ticks of -0.1% each). Count cumulative liquidations. Is the cascade behavior visible? What would partial liquidations change?

**Q5. Integration.**
Wire the risk engine into your Phase 1+2+Lesson 7 system:
```
Order → Risk → Sequencer → WAL → Matching Engine → Ledger → Fill → Risk state update
```
Benchmark end-to-end throughput and p50/p99/p99.9 latency. Compare to pre-risk (Lesson 7 end) numbers. How much did risk cost you?

**Q6. Reading.**
Read "The LUNA Collapse" write-ups (any of the quality post-mortems from May 2022). Identify:
- Where the liquidation cascade started.
- What on-chain vs off-chain components interacted.
- Which risk mitigations listed in this lesson would have helped.
- Which wouldn't have, because the mechanism was algorithmic stablecoin design, not ordinary leverage.

Write 3-4 paragraphs.

---

# Lesson 9: Hot/Cold Wallet Architecture

### Why This Lesson Exists

Everything so far has been about the exchange's *internal* ledger — fast, in-memory, WAL-backed, correctness-enforced. But users deposit real crypto (Bitcoin, Ethereum, etc.) and withdraw real crypto. The exchange has to hold that crypto *somewhere*. And there's a fundamental tension:

- **Fast withdrawals** require private keys online. Keys online = attacker who compromises the server steals everything.
- **Maximum security** requires private keys offline (air-gapped, in vaults, behind multi-sig). Offline keys = no automated withdrawals.

The resolution is architectural: split custody into **hot** (small, online, fast) and **cold** (large, offline, secure). Most of the funds live in cold. Only a small working fraction lives in hot to service daily withdrawal volume. When hot runs low, a *ceremony* moves funds from cold to hot with human approval.

Exchanges that got this wrong include Mt. Gox (2014, ~850K BTC lost), QuadrigaCX (2019, ~$190M inaccessible after the CEO died holding all the keys), FTX (2022, customer funds comingled with operational wallets, then lost), BitGrail (2018), Bitfinex (2016, ~120K BTC stolen from multi-sig hot wallet), Coincheck (2018, $530M from hot wallet), KuCoin (2020, $280M from hot wallet), and many more. **This is where most exchange money disappears.**

Exchanges that got it right (or at least better): Coinbase, Kraken, Gemini, Binance (though Binance has had hot-wallet hacks — the cold holdings prevented catastrophe). The common pattern: most value in cold, segregated custody, regular proof-of-reserves, rigorous operational procedures.

---

### Blockchain Primer

Six concepts.

**1. Transaction.** A signed message that moves value. "From address A, send X coins to address B, with fee F." Broadcast to the network. Included in a block if miners / validators accept it. Once in a block, public and immutable.

**2. Private key and signature.** An address is derived from a public key. The private key, held by the owner, signs transactions. **Whoever holds the private key can spend the address's funds.** No customer service, no reset-password. Lose the key = lose the funds. Leak the key = attacker empties the address.

**3. Nonce.** A counter that prevents replay attacks and ensures transaction ordering. On Ethereum, every transaction from an address must have a strictly increasing nonce (0, 1, 2, ...). Miss nonce 5, and nonces 6, 7, 8 get stuck in the mempool. Bitcoin doesn't have per-address nonces but uses UTXO references (each coin spent only once).

**4. Confirmations.** When a transaction is included in a block, it has 1 confirmation. Each block built on top adds 1. A transaction with 6 confirmations has 5 blocks stacked on top of it. More confirmations = more certainty the transaction is permanent. Typical conservatism: Bitcoin 3-6, Ethereum 12-30 (post-merge: finality after ~12.8 minutes), Solana 1.

**5. Reorg.** Short for "chain reorganization." When two blocks get mined simultaneously, the chain forks. Eventually one fork becomes longer; the shorter fork's blocks are orphaned. Transactions in orphaned blocks are un-done. **If you credit a deposit after 1 confirmation and a 2-block reorg happens, the deposit disappears.** You've already given the user internal credit. They withdraw. Now you're short. This is why you wait. The reorg depth that's "safe" depends on the chain — Bitcoin reorgs of > 2 blocks are rare but have happened; Ethereum post-merge has strong finality after ~2 epochs.

**6. Gas / Fee.** The cost to include a transaction in a block. Pay too little and your transaction waits in mempool (or gets dropped). Pay too much and you overpaid. Modern exchanges have fee-estimation algorithms that target inclusion within N blocks.

**Multi-sig.** An address that requires M-of-N signatures to spend. "3-of-5" means any 3 of the 5 designated keys, acting together, can sign. Used for cold storage and corporate treasuries: no single person can move funds; collusion of at least M is required. Different chains implement this differently (Bitcoin: native multisig scripts; Ethereum: smart contract wallets like Gnosis Safe).

**HSM (Hardware Security Module).** A tamper-resistant hardware device that stores private keys and performs signing operations. Keys never leave the HSM — you send a message to be signed, the HSM returns the signature. Enterprise-grade security. Products: AWS CloudHSM, YubiHSM, Thales Luna. Essential for serious custody.

**TEE / Enclave.** Trusted Execution Environment (Intel SGX, ARM TrustZone, AWS Nitro Enclaves). Software-level equivalent of HSM — a protected region of memory the OS can't read. Cheaper than HSMs, less battle-tested, known side-channel attacks. Used by some newer custody stacks.

**HD Wallet (BIP32 / BIP44).** Hierarchical Deterministic wallet. A single master seed deterministically generates a tree of private keys. Practically: you can derive a unique deposit address for every user from one cold-stored master seed, without revealing the seed. When you need to spend from a derived address, you re-derive the private key from the seed (inside the HSM / cold vault). This is how exchanges assign unique deposit addresses to every user without generating millions of independent keys.

---

### The Split: Hot, Warm, Cold

Three tiers is the modern standard.

```
                    ┌─────────────────────┐
                    │     Blockchain      │
                    └──────┬──────┬───────┘
                           ↓      ↑
             deposits      │      │      withdrawals
                           ↓      ↑
                    ┌──────────────────────┐
                    │     Hot Wallet       │  ← 1-3% of funds
                    │  Online, HSM-signed, │
                    │  rate-limited        │
                    └──────┬───────────────┘
                           ↑
                           │ periodic,
                           │ semi-automated
                           ↓
                    ┌──────────────────────┐
                    │    Warm Wallet       │  ← 5-10% of funds
                    │  Offline most of     │
                    │  time, ceremonial    │
                    │  withdraw approvals  │
                    └──────┬───────────────┘
                           ↑
                           │ rare,
                           │ full ceremony
                           ↓
                    ┌──────────────────────┐
                    │    Cold Wallet       │  ← 85%+ of funds
                    │  Air-gapped, multi-  │
                    │  sig, geographically │
                    │  distributed keys    │
                    └──────────────────────┘
```

**Hot wallet:** online, reachable by the withdrawal service. Signs automatically (via HSM, rate-limited). Holds just enough to service typical daily withdrawal flow. If compromised, loss is bounded to whatever's in hot.

**Warm wallet:** mostly offline, occasionally brought online to refill hot. Requires approval from multiple officers but without full ceremony. Bridges speed (hot refill) and security (not always reachable).

**Cold wallet:** fully air-gapped, multi-sig (often 3-of-5 or 5-of-7), keys distributed geographically across employees and jurisdictions. Requires a *ceremony*: officers physically gather, review the transaction, sign with their respective keys (each possibly an HSM or hardware wallet), broadcast. Hours to days per operation.

The ratio drifts based on the exchange's size and withdrawal patterns. Binance has disclosed roughly 2-5% hot over time. Coinbase has historically claimed 98%+ cold. Adjust based on your volume.

---

### Deposit Flow

User wants to deposit. They need a destination address.

**Step 1: Assign a deposit address.**
The exchange derives a unique address for the user from its HD wallet master seed. `BIP44: m / 44' / coin' / account' / change / user_index`. The master seed is cold. The derivation is deterministic — the exchange can generate the public address at any time without the seed (using an extended public key, `xpub`), but signing from the derived address requires the seed (done in a cold ceremony when sweeping).

**Step 2: User sends funds.**
They send from their wallet to the assigned address. Transaction broadcast to the network.

**Step 3: Exchange blockchain node sees the transaction.**
Either in the mempool (unconfirmed) or included in a block.

**Step 4: Wait for N confirmations.**
Depends on chain and deposit size. Typical:
- BTC: 3-6 confirmations. Huge deposits: 6+.
- ETH: 12-30 post-merge (or wait for "finalized" block status).
- Stablecoins on Tron/BSC: often 20-30.
- SOL: 1 (high finality).

**Step 5: Credit the user.**
Ledger transaction:
```
+amount  Custody.{coin}.DepositAddresses
−amount  User.{id}.{coin}.Available
```
(Note: sign convention depends on your accounting framework; either way it's zero-sum with both sides representing the asset arriving and the liability to the user arising.)

**Step 6: Sweep (periodic).**
The deposit address holds the funds. You don't want money scattered across millions of addresses. Periodically, the exchange sweeps user deposit addresses into the hot wallet. Batch many sweeps into one transaction to save fees.

Ledger transaction for the sweep:
```
−amount  Custody.{coin}.DepositAddresses
+amount  Custody.{coin}.HotWallet
```
Plus a fee entry:
```
−fee_amount  Equity.{coin}.OperationalCosts  (or another appropriate account)
```

The sweep itself requires signing from the deposit address's private key — which you derive from the cold seed, meaning sweeps are a "warm" operation at best, often a scheduled ceremony.

Some exchanges take a different approach: use a single shared deposit address and rely on memo/tag fields to identify the user (e.g., XRP, XLM, BNB). Simpler sweep story (no sweep needed) but requires users to include the memo correctly — easy to mess up.

**Never credit before N confirmations.** This is the most common mistake that costs exchanges money. A user deposits, the exchange credits after 1 confirmation (to be "fast"), user withdraws, reorg happens, the deposit is reversed, exchange is short. Don't be fast here. Be right.

---

### Withdrawal Flow

User wants to withdraw.

**Step 1: Pre-checks.**
- User has sufficient `Available` balance.
- Destination address format is valid.
- Destination isn't blacklisted (OFAC sanctions, etc.).
- 2FA verified.
- Withdrawal within daily / per-transaction limits.
- Cooldown period from last login / password change has elapsed (optional security measure).

**Step 2: Ledger debit (internal).**
```
+amount  User.{id}.{coin}.Available
−amount  User.{id}.{coin}.Withdrawing   (limbo account)
```

The user's internal balance drops. The withdrawal is now "queued," sitting in the Withdrawing limbo account.

**Step 3: Queue for on-chain processing.**
Withdrawal service picks up the queued withdrawals. Usually batched: combine many user withdrawals into one on-chain transaction per batch window (every 10 minutes, say), saving fees.

**Step 4: Construct and sign.**
- Build the transaction: inputs from hot wallet, outputs to each user's destination.
- Compute fee based on current network conditions.
- Submit to HSM for signing. HSM enforces policy: rate limits, per-transaction caps, whitelisted destinations (if applicable).
- Private key never leaves HSM. The HSM returns a signed transaction.

**Step 5: Broadcast.**
Send to the blockchain network. Track txid.

**Step 6: Confirm.**
Wait for confirmations. Update internal status.

**Step 7: Finalize ledger.**
Once confirmed:
```
+amount  User.{id}.{coin}.Withdrawing   (close limbo)
−amount  Custody.{coin}.HotWallet        (hot wallet drained)
```

**If withdrawal fails on-chain** (rejected, stuck, double-spent, wrong nonce), reverse:
```
+amount  User.{id}.{coin}.Withdrawing
−amount  User.{id}.{coin}.Available
```
User balance is credited back.

**Gotchas:**

- **Nonce management (Ethereum).** If you have multiple withdrawal workers and they race to assign nonces, you'll have gaps, stuck transactions. Solution: a single nonce-assigner, or strict serialization.
- **RBF (Replace-By-Fee).** Bitcoin allows replacing an unconfirmed transaction with a higher-fee version. Good for speeding up stuck withdrawals; also a double-spend vector. Track RBF carefully.
- **Dust.** Tiny balances in a UTXO or account can be uneconomical to move (fee exceeds value). Aggregate or ignore.
- **Stuck transactions.** Fee too low, transaction sits. Either wait, or use CPFP (child-pays-for-parent) / RBF. Monitor mempool and automate fee adjustments.

---

### Rebalancing Hot ↔ Cold

Hot wallet balance drifts over time:

- Deposits get swept in (refilling hot, but slowly since deposits go to sweep-stage addresses first).
- Withdrawals drain hot.

Two scenarios:

**Hot too low:** Need to refill from warm or cold. Trigger a transfer ceremony — officers authorize, multi-sig from cold, send to hot. This is the highest-risk moment (funds briefly traverse online channels) — timing should align with expected demand, and amounts should be minimized.

**Hot too high:** Excess should go to cold. This is *safer* (signing from hot to your own cold address is low-risk). Can be automated with HSM policy or done by a regular schedule.

Target policy:

```
If hot_balance < 1 day of average withdrawal volume:
    Trigger refill ceremony (pulls from warm/cold).

If hot_balance > 5 days of average withdrawal volume:
    Auto-send excess to warm/cold.
```

Never let hot exceed what you're willing to lose in a single compromise.

---

### Security Model: Threats and Mitigations

**Threat 1: Server compromise.** Attacker has root on the exchange's infrastructure.

- **Mitigation:** keys in HSM. Attacker can request signatures but can't exfiltrate keys. All signing requests logged and subject to HSM-enforced policy.
- **Failure mode:** if the HSM policy is "sign anything," attacker requests N withdrawals to their own address and drains the hot wallet. Coincheck (2018) lost $530M this way.
- **Better mitigation:** destination allowlisting, rate limiting at the HSM level, anomaly detection at the application level.

**Threat 2: Key leakage.**
Developer accidentally commits a key to GitHub. Employee exfiltrates a key shard. Backup tape leaked.

- **Mitigation:** air gap for cold. Multi-sig for cold. Keys split (Shamir secret sharing or MPC) so no single copy reveals the key.
- **QuadrigaCX (2019):** founder Gerald Cotten died holding all the keys. Multiple hundreds of millions inaccessible. Single point of failure in key management.

**Threat 3: Reorg / double-spend.**
Attacker reverses a deposit after you've credited it.

- **Mitigation:** sufficient confirmations. Scale conservatively with deposit size. Monitor mempool for RBF.

**Threat 4: Replay attack.**
Same signed transaction submitted twice on different chains (e.g., Ethereum vs ETH Classic post-fork), or before/after a hard fork.

- **Mitigation:** chain IDs, replay protection built into modern chains. Monitor.

**Threat 5: Malicious insider.**
An employee with signing authority moves funds to their address.

- **Mitigation:** multi-sig. No single insider can move significant funds alone. Background checks. Segregation of duties (the person who approves withdrawals is not the person who has signing access).

**Threat 6: Social engineering.**
Attackers convince support or an officer to sign a malicious transaction.

- **Mitigation:** out-of-band verification of all significant operations. Standardized withdrawal ceremonies that can't be short-circuited.

---

### Reconciliation

Daily, the exchange compares:

1. **Internal ledger total custody for each coin.**
   From Lesson 7's invariant check.

2. **Actual on-chain balance at all custody addresses for each coin.**
   From blockchain node queries.

These must match (within a tolerance for pending transactions). Mismatches are investigated *immediately*. Any unexplained discrepancy, trading halts.

```go
func (r *Reconciler) CheckBTC() error {
    // Internal: sum of all Custody.BTC.* accounts in the ledger
    internalTotal := r.ledger.TotalCustody("BTC")
    
    // External: sum over all custody addresses
    externalTotal := int64(0)
    for _, addr := range r.custodyAddresses["BTC"] {
        balance, err := r.btcNode.GetAddressBalance(addr)
        if err != nil {
            return err
        }
        externalTotal += balance
    }
    
    diff := internalTotal - externalTotal
    if abs(diff) > r.tolerance["BTC"] {
        return fmt.Errorf("BTC reconciliation failed: internal %d, external %d, diff %d",
            internalTotal, externalTotal, diff)
    }
    return nil
}
```

**Proof-of-Reserves (PoR).** Post-FTX, users expect cryptographic proof. Standard approach:

1. Exchange constructs a Merkle tree of user balance snapshots.
2. Publishes the root. Each user can verify their own leaf is in the tree.
3. Exchange signs messages from custody addresses proving control (not just knowledge of the address).
4. Total in Merkle tree ≤ on-chain balance proven.

PoR is imperfect (doesn't prove liabilities beyond user balances — e.g., loans, hidden debts) but it's the minimum bar now.

---

### Go Implementation Considerations

The on-chain layer is *not* performance-critical in the HFT sense. Seconds-per-transaction is fine. What matters:

- **Correctness.** Don't lose or duplicate funds.
- **Idempotency.** Same blockchain txid must map to exactly one ledger transaction. Use txid as primary key; skip if already seen.
- **Auditability.** Every on-chain operation logged, matched to a ledger transaction, reconcilable.
- **Security.** HSM for signing. Never hold private keys in application memory. Even ephemerally.
- **Fee management.** Fee estimation, RBF / CPFP handling.
- **Retry logic.** On-chain operations fail, get stuck, need re-broadcast. Idempotent retries with proper nonce/UTXO handling.

**Libraries:**

- Bitcoin: `btcsuite/btcd`, `btcsuite/btcutil`, `btcsuite/btcwallet`. Or run `bitcoind` and talk to it via RPC.
- Ethereum: `go-ethereum` (geth). Run your own node or use a service like Infura / Alchemy (with HSM signing still local).
- HSM: PKCS#11 library (`github.com/miekg/pkcs11`), AWS CloudHSM SDK, vendor-specific.

**Multi-chain complexity.** Every chain has its quirks. Dedicated per-chain services (one each for BTC, ETH, SOL, etc.) are the norm, not a single omni-chain service.

---

### Summary: The Rules

1. **Split custody: 1-3% hot, 5-10% warm, 85%+ cold.** Tune by volume.
2. **Never credit deposits before N confirmations.** N conservative with deposit size.
3. **HSM for all signing. Keys never in application memory.**
4. **Every on-chain event is an idempotent ledger transaction (keyed by txid).**
5. **Multi-sig for cold. Single-sig is a disaster waiting.**
6. **Daily reconciliation: internal ledger totals vs on-chain balances.**
7. **Rebalance hot to cap exposure at ~1 day of typical withdrawals.**
8. **Proof-of-reserves at least quarterly. Expected by serious users post-FTX.**
9. **Rate-limit and policy-enforce at the HSM, not only at the application.**

---

### Drill 9

**Q1. Confirmations policy.**
For your exchange, choose and justify N-confirmation policies for: BTC, ETH, USDT-on-Ethereum, USDT-on-Tron, SOL. Consider reorg history, typical deposit size bands (small / medium / large / whale), and time tolerance. Write a short policy document.

**Q2. Deposit flow (pseudocode).**
Write out the full deposit flow in pseudocode with proper idempotency:
- Subscribe to blockchain events.
- Track confirmations.
- Handle reorgs that orphan previously-seen transactions.
- Credit on finality.
- Sweep deposit addresses periodically.

Key questions to answer in the pseudocode: what happens if the node restarts mid-flow? What if a transaction is seen twice (once in mempool, once in block)? What if a block reorg rolls back a transaction you thought was final?

**Q3. Mock hot wallet in Go.**
Using Ethereum testnet (Sepolia or similar) and go-ethereum:
- Generate an HD wallet from a seed.
- Derive unique deposit addresses for 100 mock users.
- Listen for incoming transactions; credit after 12 confirmations.
- Implement a sweep that consolidates into a single hot wallet address.
- Implement a withdrawal flow: debit ledger, sign with a local key (stand-in for HSM), broadcast, track confirmations, finalize ledger.
- Integrate with your Lesson 7 ledger.

Put everything on testnet. Real money stays with grown-ups.

**Q4. Reconciliation.**
Implement the daily reconciliation job. Compare internal ledger totals with on-chain balances of all custody addresses. Detect discrepancies. Build an alert that fires if diff exceeds a tolerance.

Inject a bug (e.g., double-credit a deposit) and verify reconciliation catches it.

**Q5. Attack simulation.**
Write a thought experiment: you've been given access to the exchange's hot wallet signing server. What can you steal? What policies would have prevented it? Walk through:
(a) HSM with no destination allowlist, no rate limit.
(b) HSM with rate limit (N withdrawals/hour, max amount each).
(c) HSM with destination allowlist (pre-approved addresses only).
(d) Multi-sig at the hot-wallet level (2-of-3 approval for every withdrawal).

At which level does the attacker stop being able to drain meaningful funds?

**Q6. Reading.**
Read one of these post-mortems in depth:
- Mt. Gox collapse (Protos and other investigative journalism).
- Coincheck $530M NEM hack (2018).
- Bitfinex $72M BTC hack (2016) — note: multi-sig was breached, understand why.
- FTX collapse (official bankruptcy filings / John Ray III's statements).

Identify:
- Which boundary in the architecture from this lesson was crossed.
- What specific mitigation from this lesson would have prevented or limited the loss.
- What *wouldn't* have helped (because the failure was elsewhere — governance, fraud, etc., not the custody architecture).

Write 4-5 paragraphs.

---

## Phase 3 Master Rules

### Double-Entry Ledger
- Every transaction sums to zero per asset. Mechanical, enforced.
- `Assets = Liabilities + Equity` per asset, at all times. Check it.
- Fund reservation at order placement (Available / Locked accounts).
- Fixed-point `int64` for all balances. No floats.
- WAL the transaction before applying. Atomicity through replay.
- Single-goroutine ledger. Idempotent by transaction ID.
- Invariant check in production: periodic + after every transaction in dev.

### Risk Engine
- Synchronous, in-process, pre-trade. Under 10μs.
- Pre-allocated user state array. Zero allocations in hot path.
- Mark price (not last-trade) for liquidations.
- Liquidation queue prioritized by collateral deficit.
- Partial liquidations + insurance fund + ADL for cascade resilience.
- Circuit breakers on volatility, concentration, order rate.

### Hot/Cold Custody
- ~1-3% hot, 5-10% warm, 85%+ cold. Tune by volume.
- Cold = air-gapped, multi-sig, ceremony-only, geographically distributed keys.
- HSM for all signing. Keys never in app memory.
- N confirmations before crediting deposits. Scale with size.
- Every on-chain event = idempotent ledger transaction (keyed by txid).
- Daily reconciliation: internal ledger ↔ on-chain balances.
- Rate-limit + allowlist + multi-sig layered at HSM and at policy level.

### If You Do Phase 3 Right
- No code path can create or destroy money. Provable at any moment.
- Every user order passes risk in < 10μs before entering the book.
- A full-machine compromise loses at most the hot wallet's contents.
- Daily reconciliation matches, cryptographically.
- You can stand in front of regulators and auditors.

---

*Phase 3 complete. Phase 4 (if you want one): Gateway protocols (FIX, WebSocket streaming), market data distribution, cross-venue arbitrage prevention, and operational practice (deployment, disaster recovery drills, regulatory reporting).*
