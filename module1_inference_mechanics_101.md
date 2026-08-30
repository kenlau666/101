# LLM Inference Mechanics — Module 1 Course (Beginner Edition)

> Statelessness, Caching, Cost, and Decoding.
> Three lessons. Teaching mode is gentle and explains every term. Drills are harsh.
> Assumes programming experience but **zero** knowledge of transformer internals.

---

## Before You Start: What You're Building Toward

Every serious LLM application — a chatbot, an agent, a document pipeline, a code assistant — is built on top of an **inference API**. You send text in, you get text out, you pay by the token. That sounds simple. It is not.

Underneath that API call is a GPU cluster doing one of the most memory-hungry computations in all of computing, and the way that computation works leaks directly into everything you care about as an application builder:

1. **Why does my bill grow quadratically as conversations get longer?** (Statelessness.)
2. **Why is the first token slow but the rest stream out fast?** (Prefill vs decode.)
3. **Why did changing one word in my system prompt make everything slower and more expensive?** (Prefix cache invalidation.)
4. **Why does output cost 5x more than input?** (Memory bandwidth.)
5. **Why does the same prompt give different answers, and how do I stop it?** (Sampling.)
6. **Why did my "always return JSON" prompt return broken JSON, and what actually fixes it?** (Constrained decoding.)
7. **Why did my response cut off mid-sentence?** (Max tokens and truncation.)

People who don't understand inference mechanics build applications that are 10x more expensive and 5x slower than they need to be, and then debug them by superstition. People who do understand them can look at a latency graph or an invoice and know exactly what happened.

By the end of this module you will be able to compute — with a calculator, from first principles — how much memory a request uses, why it costs what it costs, roughly how long it will take, and how to control what comes out the other end.

Don't worry if you've never looked inside a transformer. Lesson 1 starts from "what is a token" and builds up. Every term gets explained before it's used.

---

## A Small Glossary You'll See A Lot

I'll explain these properly as they come up, but bookmark this for quick reference:

- **Token** = the unit models read and write. Roughly ¾ of an English word. "Hamburger" might be 3 tokens.
- **Context / context window** = everything the model can see for one request: system prompt + conversation + documents. Measured in tokens. Has a hard maximum (e.g., 200K).
- **Inference** = running a trained model to get outputs. (As opposed to *training*, which creates the model.)
- **Autoregressive** = generating one token at a time, where each new token depends on all previous ones.
- **Logits** = the raw scores the model assigns to every possible next token, before they're turned into probabilities.
- **Sampling** = picking the next token from those probabilities. Temperature and top-p are sampling knobs.
- **Prefill** = the phase where the model processes your entire input prompt. Happens once per request.
- **Decode** = the phase where the model generates output tokens, one at a time.
- **TTFT** = Time To First Token. How long before the first output token arrives. Dominated by prefill.
- **TPOT / ITL** = Time Per Output Token / Inter-Token Latency. The gap between streamed tokens. Dominated by decode.
- **KV cache** = per-request GPU memory holding the attention Keys and Values of every token seen so far. The reason decode is fast.
- **Prefix caching** = reusing the KV cache of a previous request's prompt when a new request starts with the exact same tokens.
- **HBM** = High Bandwidth Memory. The fast memory soldered onto a GPU. 40–192 GB per chip. Scarce and expensive.
- **Memory bandwidth** = how many bytes/second can move between HBM and the GPU's compute units. The real bottleneck of decode.
- **FLOPs** = floating point operations. The unit of "compute work."
- **Batching** = running many users' requests through the GPU at the same time to share the cost of reading the model weights.
- **Greedy decoding** = always pick the single highest-probability token. Temperature 0.
- **Constrained decoding** = mechanically forbidding tokens that would violate a format (like a JSON schema) at each generation step.
- **Stop sequence** = a string that, when generated, immediately ends generation.

---

# Lesson 1: Statelessness and the KV Cache

## 1.1 Why This Lesson Exists

Here's a conversation that confuses every newcomer to LLM APIs.

You build a chatbot. Turn 1, the user says "Hi, I'm Dana." The model replies. Turn 2, the user asks "What's my name?" — and the model answers "Dana." Clearly the model *remembered*.

Then you look at your API logs and discover that on turn 2, your client library sent the **entire conversation** — system prompt, turn 1, the model's reply, and turn 2 — as one big blob of input. And on turn 50, it sent all fifty turns. Your input token count grows every single turn, and you're paying for the same "Hi, I'm Dana" over and over, fifty times.

The model never remembered anything. **The API is stateless.** Every call starts from a blank slate and re-reads the whole history you hand it. "Memory" is an illusion your client constructs by resending the transcript.

This design sounds insane until you understand what's happening on the GPU — specifically, a structure called the **KV cache**, which is the single most important object in all of inference. This lesson explains what it is, why it exists, why it *doesn't* survive between API calls, and how **prefix caching** gives you back most of the benefit anyway — if you structure your prompts correctly.

## 1.2 What a Transformer Actually Does, Per Token (Just Enough)

You don't need to be able to implement a transformer. You need exactly this much:

### Tokens

The model doesn't read characters. Text is first chopped into **tokens** by a tokenizer using an algorithm called BPE (byte-pair encoding). Common words are one token ("the", " and"), rarer words are several ("indefatigable" → maybe 4), and code/JSON is token-dense (every `{`, `":`, newline can be its own token). Rules of thumb for English:

```
1 token ≈ 4 characters ≈ 0.75 words
100 tokens ≈ 75 words ≈ one solid paragraph
1,000 tokens ≈ 750 words ≈ 1.5 pages
100,000 tokens ≈ a 300-page novel
```

The model's vocabulary is fixed — typically 32,000 to 260,000 distinct tokens. Every piece of text becomes a sequence of integers, each an index into that vocabulary.

### The autoregressive loop

Generation works like this, and only like this:

```
1. Take the full sequence of tokens so far (prompt + anything generated).
2. Run it through the model. Out comes a score (a "logit") for EVERY token
   in the vocabulary: "how good would each of these be as the next token?"
3. Turn scores into probabilities. Pick one token (Lesson 3 is about how).
4. Append it to the sequence.
5. Go to step 1. Repeat until a stop condition fires.
```

One forward pass, one token. A 500-token answer means 500 sequential trips through the entire model. Keep this loop in your head for the whole course — everything else is a consequence of it.

### Attention (the 30-second version)

Inside the model, each layer runs an operation called **attention**. For the token currently being processed, attention lets it "look back" at every previous token in the sequence and pull in relevant information. ("What's my name?" attends strongly back to "Dana.")

Mechanically, every token gets three vectors computed from it:

- a **Query** (Q): "what am I looking for?"
- a **Key** (K): "what do I contain, as an address?"
- a **Value** (V): "what do I contain, as content?"

The current token's Q is compared against the K of *every previous token*; the match scores decide how much of each previous token's V gets blended in. This happens in every layer, in every attention head, for every token.

One sentence to remember: **to generate a new token, the model needs the K and V vectors of every token that came before it.**

That sentence is why the KV cache exists.

## 1.3 The Two Phases: Prefill and Decode

Every request the server handles has two very different phases.

**Prefill.** Your prompt arrives — say, 10,000 tokens. The model must compute the internal state (including all those K and V vectors) for all 10,000 tokens. The good news: your prompt is already fully known, so all 10,000 tokens can be processed **in parallel**, as one giant matrix multiplication. GPUs love giant parallel matrix multiplications. Prefill saturates the GPU's compute units. It's a lot of work, but it's *efficient* work, and it happens once.

**Decode.** Now the model generates the answer, one token at a time, using the loop from 1.2. Each step depends on the previous step's output, so there is **no parallelism across time**. Step 501 cannot start before step 500 finishes. Each step does relatively little math but — as you'll see in Lesson 2 — has to drag an enormous amount of data through memory.

This split is visible from the outside:

```
TTFT  (time to first token)      ≈ queueing + prefill time.
                                   Grows with PROMPT length.
TPOT  (time per output token)    ≈ one decode step.
                                   Grows with... mostly nothing you control,
                                   plus slowly with total sequence length.

Total latency ≈ TTFT + (output_tokens × TPOT)
```

A 50K-token prompt with a 100-token answer: you wait seconds for the first token, then the answer streams out fast. A 100-token prompt asking for a 4,000-token essay: first token nearly instant, then a long stream. Two completely different latency profiles, same model.

## 1.4 The Problem the KV Cache Solves

Look at the autoregressive loop again. Step 2 says "run the full sequence through the model." Taken literally, that's a disaster:

```
Generating token 1001: process 1000 tokens of history.
Generating token 1002: process 1001 tokens of history.
Generating token 1003: process 1002 tokens of history.
...
```

To generate N tokens on top of a P-token prompt, you'd redo work proportional to (P+N)² — and almost all of it is *identical* work you already did. Recomputing the K and V vectors for "Hi, I'm Dana" on every single step is pure waste: those tokens haven't changed, so their K and V vectors haven't changed either.

**The KV cache is exactly this observation turned into a data structure:**

> The first time we compute a token's K and V vectors (in every layer), store them in GPU memory. On every future decode step, don't recompute them — just read them.

With the cache, each decode step only computes Q/K/V for the **one new token**, appends its K and V to the cache, and runs attention between the new token's Q and the *cached* K/V of everything before it. Per-step compute drops from "reprocess the whole sequence" to "process one token + read the cache." This is the difference between generation being unusable and generation being real-time. Every production inference server on earth does this.

## 1.5 How Big Is the KV Cache? (Do This Math Once, Remember It Forever)

The cache stores 2 vectors (K and V) per token, per layer, per KV head. The formula:

```
bytes per token = 2 (K and V)
               × n_layers
               × n_kv_heads × head_dim
               × bytes_per_number (2 for fp16/bf16)
```

Two worked examples with real architectures:

**Llama-2-7B** (old-style: every attention head has its own K/V — "multi-head attention," MHA). 32 layers, 32 KV heads, head_dim 128, fp16:

```
2 × 32 × (32 × 128) × 2 bytes = 524,288 bytes ≈ 512 KB per token
```

Half a megabyte *per token*. A 4,096-token context costs **2 GB** of GPU memory — for ONE user's ONE request. The 7B model's weights themselves are 14 GB in fp16. On a 24 GB GPU, five concurrent 4K conversations of KV cache would exceed the memory left over after weights. This is why MHA died.

**Llama-3-70B** (modern: "grouped-query attention," GQA — 64 query heads *share* just 8 KV heads, shrinking the cache 8x). 80 layers, 8 KV heads, head_dim 128, fp16:

```
2 × 80 × (8 × 128) × 2 bytes = 327,680 bytes ≈ 320 KB per token
```

Better — but a 128K-token context is still **~40 GB** of cache. That's an entire second GPU's worth of memory, per request, just to remember the conversation.

Three conclusions you should now feel in your bones:

1. **KV cache memory, not compute, is what limits how many users a GPU can serve at once.** Serving capacity ≈ (HBM − weights) ÷ (KV bytes per token × avg context length).
2. **Long contexts are expensive in a way that has nothing to do with token pricing.** They physically occupy scarce HBM for the duration of the request.
3. **Architecture choices you may have seen in model release notes — GQA, MQA, MLA (DeepSeek's latent compression), sliding-window attention — are all attacks on KV cache size.** Now you know why labs brag about them.

## 1.6 Why the KV Cache Doesn't Persist Across API Calls

So here's the obvious question. If the server built a beautiful KV cache of my whole conversation during my last request... why does my next request have to resend everything? Why not keep my cache warm and let me send only the new message?

Because at the scale of a public API, keeping per-user caches alive is somewhere between uneconomical and impossible:

**Reason 1: HBM is the scarcest resource in the building.** That 40 GB cache from section 1.5 is sitting in memory that could serve *other paying requests right now*. Between your turn 7 and turn 8, a human is typing for 30 seconds — an eternity. Reserving tens of GB of HBM for a user who might never come back is like holding a hospital operating room open in case a discharged patient returns.

**Reason 2: routing.** A provider runs thousands of GPU nodes behind load balancers. Your turn-8 request will, by default, land on a different machine than turn 7 did. Your cache is on the wrong computer. Making requests "sticky" to specific machines fights load balancing, breaks when machines fail, and creates hot spots.

**Reason 3: statelessness is an engineering superpower.** Stateless servers can be added, removed, restarted, and upgraded freely; any request can go anywhere; a crashed node loses nothing durable. Every lesson the web learned in 30 years of scaling (why REST beat stateful sessions) applies directly.

So the contract is: **the client owns the state** (the transcript), and resends it. The server's KV cache is a *per-request scratch space*, built during prefill, used during decode, and freed the moment your request completes.

That freeing is aggressive and immediate — and it created an opportunity. If the server is going to rebuild the same cache from the same bytes tomorrow... why not save a copy?

## 1.7 Prefix Caching: Recycling Prefill

**Prefix caching** (Anthropic and OpenAI call it "prompt caching") is the fix, and it's beautifully simple in concept:

> After prefill, keep the KV cache of the prompt around for a few minutes, indexed by the *exact token content* of the prompt. If a new request arrives whose prompt **starts with the exact same tokens**, skip prefill for the matching part — load the saved KV blocks and only prefill the new tail.

Mechanically, implementations (vLLM, SGLang, and the big providers) chop the KV cache into fixed-size **blocks** (e.g., 16–64 tokens each) and key each block by a hash of *all tokens from position 0 up through that block*. A new request's tokens are hashed block by block; the longest chain of matching blocks is a **cache hit** and gets reused; everything after the first mismatch is computed fresh.

Why this is a big deal, with our chatbot from 1.1:

```
Turn 8 of a conversation. Prompt = 20,000 tokens of history + 50 new tokens.

WITHOUT prefix caching:
  prefill 20,050 tokens.  TTFT: several seconds. Full input price.

WITH prefix caching (turn 7's prompt is a prefix of turn 8's):
  cache hit on ~20,000 tokens, prefill only ~50.
  TTFT: often 5-10x better. Cached tokens billed at ~10% of input price.
```

The same trick powers agents (the system prompt + tool definitions + scratchpad so far is a stable prefix across dozens of tool-call steps) and document Q&A (one big document, many questions — put the document first).

### The invalidation rules (this is where people get burned)

The match is **exact, on tokens, from position zero**. Internalize these rules:

**Rule 1: One changed byte invalidates everything after it.** The cache matches prefixes. If byte 500 of your system prompt differs, tokens 0–499 can still hit, but *everything downstream* — the other 19,500 tokens — is a miss. Change something near the front, lose nearly the whole cache.

**Rule 2: Therefore, order your prompt from most-stable to least-stable.**

```
[system prompt]          ← never changes        (cacheable forever)
[tool definitions]       ← changes on deploys   (cacheable)
[large documents]        ← per-session          (cacheable within session)
[conversation history]   ← append-only          (cacheable — old turns are a prefix!)
[current user message]   ← always new           (never cacheable; that's fine)
```

Append-only is the magic property: if you only ever *add to the end*, every request is an extension of the last one, and hit rates approach 100%.

**Rule 3: Dynamic content at the top is cache poison.** The classic self-inflicted wound:

```
System prompt: "You are a helpful assistant. Current time: 2026-08-26 14:03:11. ..."
```

That timestamp changes every second → every request misses → you pay full prefill and full input price on 20K tokens forever, and never find out why the app is slow. Same for request IDs, random example ordering, "user's current battery level," A/B-test strings. If you must include volatile data, put it at the **bottom**, after all stable content.

**Rule 4: Everything in the prefix counts, not just visible text.** Tool/function definitions, images, document attachments — they're all tokens in the sequence. Reordering your tool list, or regenerating JSON for it with different key ordering or whitespace, silently changes tokens and busts the cache. Serialize deterministically.

**Rule 5: Caches expire.** Provider prompt caches have a TTL — typically ~5 minutes since last use (extendable, e.g. to 1 hour, for extra cost on some providers). An overnight gap = cold cache. Cache reads *refresh* the TTL, so an active agent loop keeps itself warm.

**Rule 6: Writes can cost extra; reads are the payoff.** Typical pricing shape (check your provider's current numbers): cache *write* ≈ 1.25× the normal input price for those tokens; cache *read* ≈ 0.1×. Break-even is fast: write once, read twice, and you're already ahead. But caching a 20K prefix you'll use once is strictly a loss.

**Rule 7 (subtle): sampling parameters don't invalidate; the token stream does.** Temperature, top-p, max_tokens live outside the sequence — changing them doesn't affect caching. Anything that changes the *tokens fed to the model* does.

## 1.8 Aside: PagedAttention (Why Any of This Is Manageable)

One implementation detail worth knowing by name. Early servers allocated each request's KV cache as one giant contiguous buffer sized for the *maximum possible* length — like booking a 200-seat theater because you might have 200 guests. Most memory sat reserved-but-unused; measured waste was 60–80%.

**vLLM's PagedAttention** (2023) applied the oldest idea in operating systems — virtual memory paging — to the KV cache: chop it into small blocks, allocate on demand, keep a per-request block table mapping logical positions to physical blocks. Waste dropped to a few percent, concurrent batch sizes jumped, and — bonus — block-granular storage is exactly what makes prefix caching (shared blocks between requests!) cheap to implement. If you read one systems paper from this course's reading list, make it this one.

## 1.9 Summary: The Rules

1. **The API is stateless.** "Memory" = the client resending the transcript. You pay to re-read history every turn.
2. **One forward pass per output token.** Generation is inherently sequential.
3. **Prefill is parallel and compute-heavy; decode is sequential.** TTFT ← prompt length; TPOT ← decode step time.
4. **The KV cache exists so decode doesn't recompute the past.** It stores K/V per token, per layer.
5. **Know the size formula:** `2 × layers × kv_heads × head_dim × 2 bytes` per token. Cache memory limits concurrency, and motivates GQA/MQA/MLA.
6. **Per-request caches die at request end; prefix caching resurrects them** for exact-token prefix matches, for a few minutes.
7. **Structure prompts stable-first, append-only, nothing volatile at the top.** One early changed byte kills everything after it.
8. **Cache reads ≈ 10% of input price; writes ≈ 125%.** Cache things you'll reuse.

## 1.10 Drill 1

Rules: show mechanism and arithmetic, not vibes. "Caching makes it faster" with no numbers gets zero credit. Reply with your answers and I'll tear them apart.

**Q1. Explain the mechanism.**
In your own words (≥200 words), explain why generating token 2,000 of a response would be catastrophically slow without a KV cache, and what exactly the cache stores to fix it. Your answer must correctly use: autoregressive, Key, Value, Query, layer, prefill, decode. Then explain why the *Q* vectors of past tokens are NOT cached. (If you can't answer that last part, you don't understand attention yet — go back to 1.2.)

**Q2. Do the memory math.**
A model has 61 layers, GQA with 8 KV heads, head_dim 128, served in fp16.
(a) Compute KV cache bytes per token.
(b) A user sends a 150,000-token prompt. How many GB of cache is that?
(c) The GPU node has 8×80 GB HBM and the weights take 220 GB across it. Roughly how many *concurrent* 150K-token requests fit? What are two techniques (from this lesson or release notes you've seen) that would raise that number, and what does each trade away?

**Q3. The invalidation audit.**
Here is a (bad) prompt assembly function:

```python
def build_prompt(user_msg, history, tools):
    now = datetime.utcnow().isoformat()
    random.shuffle(FEW_SHOT_EXAMPLES)          # "for diversity"
    return (
        f"[req:{uuid4()}] You are AcmeBot. Time: {now}\n"
        + "".join(FEW_SHOT_EXAMPLES)
        + json.dumps({t.name: t.schema for t in tools})   # dict ordering!
        + render(history)
        + user_msg
    )
```

Find **every** cache-hostile line. For each: name the line, the mechanism by which it causes misses, and the fix. There are at least five. Then rewrite the function in cache-optimal order.

**Q4. Break-even arithmetic.**
Input price $3/M tokens; cache write $3.75/M; cache read $0.30/M. Your agent has a 30,000-token stable prefix and runs a loop of N model calls, all within the TTL.
(a) Write the total prefix-cost formula with and without caching, as a function of N.
(b) At what N does caching break even?
(c) Your agent averages N=25 calls per task, 10,000 tasks/month. Dollars saved per month (prefix cost only)?

**Q5. Design question.**
Your product has 5,000 daily users, each chatting ~10 turns within a session, sessions ~20 minutes, a shared 4K system prompt, and per-user histories averaging 8K tokens by session end. The provider's cache TTL is 5 minutes, refreshed on use. Predict the cache hit pattern: which parts of which requests hit, which miss, and where the TTL bites. What one product-level change most improves the hit rate?

**Q6. Reading.**
Read "Efficient Memory Management for Large Language Model Serving with PagedAttention" (Kwon et al., 2023 — the vLLM paper), at least sections 1–4. Answer: (a) What are the three kinds of KV memory waste they identify in pre-paging systems? (b) What OS concept is the block table analogous to? (c) How does paging make *sharing* KV blocks across requests (the basis of prefix caching) natural?

---
# Lesson 2: The Token Cost Model and Latency

## 2.1 Why This Lesson Exists

Open any provider's pricing page and you'll see something like this:

```
Input tokens:   $3.00  per million
Output tokens:  $15.00 per million
```

Output is 5x the price of input. Why? Is it greed? Marketing? No — it's physics, specifically **memory bandwidth**, and once you see it you'll be able to predict latency and cost for any workload before you run it.

Here's a puzzle to motivate the lesson. Take a model with 70 billion parameters. Standard result (we'll justify it below): processing one token — whether reading it or writing it — costs about **2 FLOPs per parameter**, so ~140 GFLOPs per token *either way*. Reading a token and writing a token are, in raw arithmetic, the *same amount of math*.

Same math. 5x price difference. Something other than arithmetic must dominate the cost. This lesson is about what that something is, and about the three latency numbers (TTFT, TPOT, total) that every LLM application lives and dies by.

## 2.2 Where the Time Actually Goes: Compute vs Memory

A GPU has two relevant speed limits:

```
NVIDIA A100 (80GB), the workhorse card:
  Compute:  ~312 TFLOP/s (fp16, dense)     = 312 × 10¹² operations/sec
  Memory:   ~2 TB/s HBM bandwidth          = 2 × 10¹² bytes/sec

Ratio: ~156 FLOPs of compute available per byte moved.
```

That ratio is the key. For any workload, ask: per byte of data I move from memory, how many FLOPs of useful math do I do? This is called **arithmetic intensity**.

- If your intensity is **above** ~156 FLOPs/byte, the GPU's arithmetic units are the bottleneck: you are **compute-bound**. The GPU is earning its keep.
- If it's **below**, the arithmetic units sit idle waiting for data: you are **memory-bandwidth-bound**. You bought a Ferrari and you're stuck in traffic.

Now apply this to our two phases.

**Prefill:** the model's weights are loaded from HBM once, and used against *thousands of prompt tokens at once* (one big matrix multiply per layer). Loading 2 bytes of weight buys you ~2 FLOPs × thousands of tokens of work. Intensity: thousands of FLOPs/byte. **Compute-bound. Efficient.**

**Decode, single request:** to produce ONE token, the model must still stream **every single weight** through the compute units (each token passes through every layer, every matrix). For a 70B model in fp16:

```
Bytes moved per decode step ≈ all weights ≈ 140 GB
Time floor per token ≈ 140 GB ÷ 2 TB/s = 70 ms
```

70 milliseconds per token, and the FLOPs performed in that time (~140 GFLOPs) are a rounding error against what the card could do (312 TFLOP/s × 70 ms ≈ 21,800 GFLOPs). **The GPU is >99% idle during single-stream decode.** Intensity ≈ 2 FLOPs/byte, about 78x below the compute-bound threshold.

That is the entire secret of inference economics: **decode is a memory-bandwidth problem wearing a compute costume.** (In practice a 70B model is sharded across multiple GPUs, which multiplies available bandwidth and adds interconnect overhead — but the bandwidth-bound conclusion survives.)

### The numbers every LLM engineer should know

Memorize the shape of this table the way systems people memorize the latency table:

```
Quantity                                        Typical value
──────────────────────────────────────────────────────────────
FLOPs per token (dense model, N params)          ~2N
GPU fp16 compute (A100 / H100)                   312 / ~1000 TFLOP/s
GPU HBM bandwidth (A100 / H100)                  2 / 3.35 TB/s
Compute-bound threshold (A100)                   ~156 FLOPs per byte
Weights, 70B fp16 / 8-bit / 4-bit                140 / 70 / 35 GB
KV cache per token, 70B-class GQA                ~320 KB
Single-stream decode floor, 70B fp16, 1×A100*    ~70 ms/token
Good production TPOT                             10–50 ms/token
Human reading speed                              ~4-5 words/sec ≈ ~6 tokens/sec ≈ 170 ms/token
Prefill throughput, order of magnitude           1,000s–10,000s of tokens/sec/GPU
──────────────────────────────────────────────────────────────
* hypothetical; the model doesn't fit on one card — illustrative floor.
```

Notice: quantizing weights to 4-bit doesn't just shrink the download — it *quarters the bytes per decode step*, which (when bandwidth-bound) roughly quarters TPOT. Now you know why the open-model community is obsessed with quantization.

## 2.3 Batching: How Providers Escape the Bandwidth Trap

If one decode step wastes 99% of the compute while streaming 140 GB of weights... what if 64 users' requests each take one decode step *during the same stream*? The weights are read from HBM **once** and applied to all 64 sequences. Arithmetic intensity goes up ~64x. Cost per token goes down almost 64x.

This is **batching**, and it's why serving is a business at all. Two refinements matter:

**Continuous batching** (a.k.a. in-flight batching): naive batching waits for every request in a batch to finish before admitting new ones — one user asking for a 4,000-token essay holds 63 finished requests hostage. Modern servers (Orca introduced it; vLLM, TensorRT-LLM, everyone uses it) operate at *step* granularity: after every single decode step, finished sequences exit the batch and queued ones join. The GPU batch is a revolving door, not a bus.

**The throughput–latency trade.** Bigger batches = more tokens/sec/GPU = cheaper tokens, but each user's individual TPOT gets a bit worse and queueing appears at admission. Providers pick an operating point; "batch" discount API tiers (50% off for hours-long turnaround) are them running the same hardware at a fat-batch, high-utilization operating point on off-peak capacity. Cheap tokens and instant tokens are physically different products.

Also note what batching does to our compute/memory picture: with a big enough batch, decode stops being weight-bandwidth-bound and the *KV cache* becomes the thing being streamed per step (each sequence's cache is private — batching doesn't share it!). At long context lengths, KV reads dominate weight reads. Long-context serving is expensive twice: once in HBM occupancy (Lesson 1), once in bandwidth per step (here).

## 2.4 The Latency Model You Should Carry Around

```
total_latency ≈ queue_wait
             + prefill_time            (≈ prompt_tokens ÷ prefill_throughput)
             + output_tokens × TPOT

TTFT = queue_wait + prefill_time (+ scheduler admission)
```

Consequences worth stating explicitly:

1. **TTFT scales with prompt length; a prefix-cache hit collapses it.** The single biggest TTFT lever you control is Lesson 1's prompt structure. (Attention's cost grows superlinearly with length, so very long prompts hurt more than linearly.)
2. **Total latency usually scales with OUTPUT length more than anything else.** 500 output tokens at 25 ms/token is 12.5 s of decode. If you need less latency, the first question is always "can the model say less?" — tighter instructions, structured output instead of prose, lower max_tokens.
3. **Streaming doesn't reduce total latency; it reduces *perceived* latency.** Showing tokens at 30 ms intervals beats a 12-second spinner. Humans read at ~170 ms/token; any TPOT under that *feels* instant in a chat UI.
4. **Variance comes from the shared batch.** Your request's TPOT wobbles with who else is on the GPU. p99 planning matters: a step that's occasionally 3x slower is normal, not a bug in your code.
5. **Reasoning/thinking models spend invisible output tokens before your visible ones.** Their TTFT-to-visible-text includes a hidden decode phase. Same mechanics, hidden budget — and yes, you're typically billed for those tokens as output.

## 2.5 The Conversation Cost Spiral (Statelessness Meets Pricing)

Now combine Lesson 1's statelessness with per-token pricing. Every turn resends all prior turns as input. Suppose every turn adds ~500 tokens (user + assistant) on top of a 2,000-token system prompt:

```
Turn  Input tokens sent       Cumulative input paid for
 1        2,500                     2,500
 5        4,500                    17,500
10        7,000                    47,500
20       12,000                   145,000
40       22,000                   490,000
```

Per-turn input grows **linearly**; cumulative input across the conversation grows **quadratically** (it's the sum of a growing series). A conversation twice as long costs roughly 4x in input. This is the mechanism behind every "why is our LLM bill exploding" postmortem.

The standard defenses, in the order you should reach for them:

1. **Prefix caching** (Lesson 1): the history *is* a stable prefix; pay ~10% for the re-reads. This alone tames most of the spiral — the quadratic term is still there, but with a small constant.
2. **Truncation / windowing:** drop or summarize old turns past a budget. Trade memory of the conversation for cost. (Careful: *editing* old turns breaks the append-only property and busts the cache — summarize into a new stable block, don't rewrite history every turn.)
3. **Right-size the model:** route easy turns to a small cheap model, hard ones to the big model.
4. **Cap outputs:** output tokens are 5x input; verbose answers on every turn also *become input* on every subsequent turn. Verbosity compounds. A 500-token answer at turn 3 gets re-read (as input) on turns 4 through 40.

Work one full example, because you'll do this constantly in real life:

```
Pricing: $3/M input, $15/M output. 20-turn conversation,
2,000-token system prompt, ~250 new user tokens/turn, ~250 output tokens/turn.

Output cost: 20 × 250 × $15/M                            = $0.075
Input cost (no caching): Σ over turns ≈ 145,000 × $3/M  = $0.435
Total ≈ $0.51 per conversation.  Input is 85% of the bill.

Same conversation with prefix caching (reads at $0.30/M, ignore write premium):
cached re-reads ≈ 135,500 × $0.30/M ≈ $0.041; fresh input ≈ 9,500 × $3/M ≈ $0.029
Input cost ≈ $0.07  → total ≈ $0.145.  ~3.5x cheaper, one engineering change.
```

## 2.6 Summary: The Rules

1. **~2 FLOPs per parameter per token**, input or output. Arithmetic is not why output costs more.
2. **Prefill is compute-bound and parallel; decode is memory-bandwidth-bound and sequential.** Output price ≈ the cost of streaming the weights (and KV) per token, amortized over a batch.
3. **Arithmetic intensity vs the GPU's FLOPs-per-byte ratio tells you the bottleneck.** Below the line: bandwidth-bound.
4. **Batching is the business model.** Weights are read once per step for the whole batch; KV caches are not shared.
5. **TTFT ← queue + prefill(prompt length, cache hits). Total ← output length × TPOT.** Control latency by controlling output length first.
6. **Streaming fixes feelings, not physics.** TPOT under ~170 ms/token out-reads a human.
7. **Conversation input cost grows quadratically.** Cache the prefix, window the history, cap the outputs, route to smaller models.
8. **Quantization cuts decode time roughly in proportion to weight bytes** — a bandwidth fact, not just a storage fact.

## 2.7 Drill 2

Rules: every answer needs arithmetic. Round freely, state assumptions, show units. "It depends" without a formula is a zero.

**Q1. The 5x question.**
Explain, in ≥200 of your own words, why output tokens are priced ~5x input tokens even though per-token FLOPs are symmetric. Your answer must correctly use: arithmetic intensity, memory-bandwidth-bound, batching, prefill parallelism. Then explain why a provider *can* profitably sell a batch tier at 50% off with hours of latency — what physical operating point makes those tokens cheaper?

**Q2. Back-of-envelope TPOT.**
A 8B-parameter model, 4-bit quantized (~0.5 bytes/param + ~10% overhead), runs single-stream on a card with 1 TB/s bandwidth. (a) Estimate the decode floor in ms/token and tokens/sec. (b) Same model in fp16 — new floor? (c) The measured speed is 40% of your floor estimate. Name three real-world costs your floor model ignored.

**Q3. Latency budget.**
Product requirement: perceived response starts < 1.2 s; full answer < 10 s. Prompt: 12K tokens (9K of it a stable, cacheable prefix). Provider measured at: prefill 5K tok/s (cache miss), TTFT floor 300 ms, TPOT 30 ms.
(a) TTFT on cold cache vs warm cache? Does each meet the 1.2 s bar?
(b) What's the max output length that fits the 10 s total on a warm cache?
(c) You need 800-token answers. List every lever from Lessons 1–2 that could make that fit, and what each trades away.

**Q4. The bill autopsy.**
A team's invoice: 40M input tokens, 2M output tokens, $150/month, at $3/$15 per M. Their app is a 30-turn average support chatbot with a 3K system prompt.
(a) Verify the bill from the numbers.
(b) What is the input:output token ratio, and why is it so lopsided? Derive the expected ratio for a T-turn chat with system prompt S, per-turn user u and output o tokens.
(c) Estimate the new bill if they adopt prefix caching (reads 0.1x, writes 1.25x) with a realistic hit pattern. State your assumptions.

**Q5. KV bandwidth strikes back.**
Batch of 64 sequences, each at 60K context, on the 70B-class model from Lesson 1 (~320 KB KV per token per sequence). Per decode step: (a) bytes of KV read for the whole batch? (b) Compare to the 140 GB of fp16 weights read once per step. Which dominates? (c) Explain why long-context requests degrade *everyone's* TPOT on a shared batch, and name one architectural and one serving-level mitigation.

**Q6. Reading.**
Read kipply's blog post "Transformer Inference Arithmetic" and skim Pope et al., "Efficiently Scaling Transformer Inference" (2022). Answer: (a) reproduce the 2·N FLOPs-per-token argument in your own words; (b) what batch size roughly moves decode from bandwidth-bound to compute-bound on an A100, and what ratio determines it; (c) per Pope et al., what changes about the trade-offs when you shard across many chips?

---
# Lesson 3: Controlling the Output — Sampling, Structure, Stopping

## 3.1 Why This Lesson Exists

Three bug reports, all real patterns:

1. *"The model gives a different answer every time we run the same prompt. QA can't write tests."*
2. *"We told it ALWAYS RESPOND IN VALID JSON, in caps, three times. 2% of responses still have a trailing comma or a chatty preamble, and 2% of a million requests a day is 20,000 crashed parses."*
3. *"Responses randomly end mid-sentence. Sometimes mid-word."*

All three are the same subject: what happens in **step 3 of the autoregressive loop** — the moment the model's raw scores become an actual chosen token — and in the machinery that decides when the loop stops. This is the part of inference you control *per request*, with parameters, and it's where prompt-level superstition ("say it in caps three times") gets replaced by mechanical guarantees.

## 3.2 Logits and Softmax: The Model Proposes, the Sampler Disposes

The forward pass does not output a token. It outputs a **logit** — an unnormalized real-valued score — for *every token in the vocabulary*. For a 128K-token vocabulary, that's 128,000 numbers, every step.

To turn scores into probabilities, apply **softmax**:

```
P(token_i) = exp(logit_i) / Σ_j exp(logit_j)
```

Exponentiate everything (making all values positive, amplifying gaps), then normalize to sum to 1. Now you have a probability distribution over the whole vocabulary:

```
Prompt: "The capital of France is"
  " Paris"   0.92
  " the"     0.03
  " located" 0.01
  " Lyon"    0.004
  ... 127,996 tokens sharing the remaining ~0.03
```

Everything in this lesson is a transformation applied to this distribution *before* one token is drawn from it. The model is frozen; the sampler is yours.

## 3.3 Temperature

Temperature rescales logits before softmax:

```
P(token_i) = softmax(logit_i / T)
```

Divide by T, then softmax. The effect:

- **T < 1** (e.g., 0.2): gaps between logits get *magnified* → distribution sharpens → probability mass piles onto the top few tokens. Output is more predictable, conservative, repetitive.
- **T = 1**: the model's learned distribution, untouched.
- **T > 1** (e.g., 1.5): gaps shrink → distribution flattens → tail tokens become live options. More diverse, more surprising, and past a point, incoherent — at very high T you're approaching uniform-random tokens.

**T = 0 is a special case, not a limit you plug in:** it means skip sampling entirely and take the argmax — the single highest-logit token. This is **greedy decoding**.

A concrete feel, same three logits [5.0, 4.0, 2.0]:

```
T=0.5 →  [0.87, 0.12, 0.002]     sharp
T=1.0 →  [0.70, 0.26, 0.03]      as learned
T=2.0 →  [0.51, 0.31, 0.11]      flat(ter)
T=0   →  pick index 0, always
```

### The greedy determinism footnote (read this before promising reproducibility)

Greedy decoding makes the *sampler* deterministic. It does not make the *system* deterministic. In production you may still see run-to-run variation at T=0 because floating-point addition is non-associative and GPU kernels sum things in different orders depending on **batch composition** — who else is on the GPU with you changes matrix shapes, changes reduction order, changes the 12th decimal of a logit, occasionally flips a near-tie argmax, and every token after that divergence compounds. (MoE models add another wrinkle: expert routing under batching.) T=0 gives you *mostly* stable outputs, not a hash function. If your test suite requires byte-identical LLM output, your test suite is wrong.

## 3.4 Top-p and Top-k: Truncating the Tail

Temperature reshapes the distribution but never removes options — even at T=0.7, thousands of garbage tokens retain tiny probabilities, and over a long generation, one of them eventually gets drawn (a one-in-ten-thousand event happens reliably when you sample 500 tokens × thousands of requests). Truncation fixes this by deleting the tail before sampling:

**Top-k:** keep only the k highest-probability tokens (say k=40), renormalize, sample among them. Blunt: the "right" number of candidates varies wildly by position — after `"The capital of France is"` there's ~1 good option; mid-story there might be 300.

**Top-p (nucleus sampling):** keep the smallest set of top tokens whose probabilities sum to ≥ p (say 0.9), renormalize, sample. Adaptive where top-k is blunt: confident positions → nucleus of 1–2 tokens; open positions → nucleus of hundreds. This adaptivity is why top-p (from Holtzman et al., "The Curious Case of Neural Text Degeneration" — this course's most readable paper) became the default.

Order of operations in most stacks: temperature → top-k → top-p → sample.

### What to actually set

```
Task                          Settings that make sense
──────────────────────────────────────────────────────────────
Code generation               T=0–0.3          wrong-but-creative is worthless
Data extraction, classification  T=0            you want the modal answer
Factual Q&A                   T=0–0.5
General chat                  T≈0.7, p≈0.9     the industry default
Creative writing              T=0.9–1.2, p≈0.95
Brainstorming / N diverse samples  T≥1.0       diversity is the whole point
──────────────────────────────────────────────────────────────
```

Two rules of hygiene: **tune one knob, not both** — temperature and top-p interact multiplicatively and combined-extreme settings are hard to reason about (many providers advise altering one or the other); and **don't cargo-cult T=0 for everything** — greedy output is measurably more repetitive and can loop ("the the the") on long generations; for anything human-facing, a little temperature is a feature.

## 3.5 Constrained Decoding: Guarantees Instead of Prayers

Bug report #2: prompting for JSON gets you JSON *usually*. Usually is not an engineering guarantee. The mechanical fix is beautiful once you see where it fits.

Recall: at every step, the sampler holds 128,000 logits. **Constrained decoding adds a mask**: before sampling, set the logits of every token that would violate your format to −∞ (probability exactly 0 after softmax), then sample normally among survivors.

What computes the mask? A compiled representation of your format — for JSON schemas, typically the schema compiled to a grammar, compiled to a **finite-state machine over tokens**. At each step the FSM knows its state ("inside a string value for key `age`, which the schema says is an integer") and therefore exactly which tokens may come next (digits, or a closing quote — never `{`, never `Sure! Here's`):

```
Generated so far:  {"name": "Dana", "age":
FSM state:         expecting integer value
Allowed:           " 4", "2", " 17", ...          (number-forming tokens)
Masked to -inf:    everything else — 127,000+ tokens
```

The model cannot emit invalid output because invalid tokens are unpickable. Not "unlikely." **Unpickable.** This is what "JSON mode" / "structured outputs" / "response schemas" do on the big APIs, and what Outlines, llguidance, XGrammar, and llama.cpp's GBNF grammars do in open stacks. Precomputed cleverly (the Outlines paper's contribution), the per-step masking cost is negligible.

The same mechanism handles more than JSON: regexes, context-free grammars (SQL dialects, DSLs), and — the degenerate case — **forced tool-call formats**. When an API guarantees a tool call matches its schema, this machinery is why.

### The fine print (where quality goes to die)

1. **Constraints guarantee syntax, not sense.** `{"age": -7}` is schema-valid if your schema said "integer." Garbage in perfect JSON is still garbage. Constraint ≠ correctness.
2. **A fighting model produces valid-but-worse output.** If the prompt never *told* the model to produce JSON and the mask forces it anyway, you get technically-valid contortions. Best practice is always: prompt for the format *and* constrain it. The constraint is the safety net, not the instruction.
3. **Token/character mismatch is the hard engineering problem.** Grammars are over characters; models emit tokens that can span grammar boundaries (a single token `"}\n{"` crosses three JSON states). Real implementations handle this; naive homemade ones have subtle bugs. Don't hand-roll.
4. **Overly rigid schemas hurt reasoning.** Forcing an answer-first field order (`{"answer": ..., "reasoning": ...}`) makes the model commit to the answer before generating its reasoning. Order fields reasoning-first. (Field order in the schema is a prompt-engineering decision!)
5. **Masking interacts with sampling normally** — surviving tokens are renormalized and temperature still applies among them.

## 3.6 Stop Sequences

A generation ends in exactly one of three ways, and your code must handle all three:

1. The model emits its special **end-of-sequence token** (it decided it was done) → finish reason `stop` / `end_turn`.
2. The output matches one of your **stop sequences** → also `stop` (some APIs tell you which sequence fired).
3. The token count hits **max_tokens** → finish reason `length` / `max_tokens`. This one is a *truncation*, and it's the subject of 3.7.

A **stop sequence** is a string you supply (typically up to 4 of them): the server watches the decoded output stream, and the moment the accumulated text contains that string, generation halts. Universal convention: **the stop sequence itself is not included** in the returned text.

Uses: ending a list after one item (`stop=["\n\n"]`), fencing a code block (`stop=["```"]`), keeping a simulated dialogue from writing the other speaker's lines (`stop=["User:"]`), terminating at a sentinel your prompt defined (`stop=["<END>"]`).

Mechanics worth knowing:

- **Matching is on decoded text, not token IDs** — necessarily, since a stop string can straddle token boundaries (`"User:"` might arrive as `"User"` + `":"`). Consequence for streaming: the server must *withhold* any streamed suffix that could be a prefix of a stop sequence until it's disambiguated, which can add a tiny stutter at exactly those characters.
- **Stop sequences are a blunt instrument.** `stop=["\n"]` ends at the first newline — including one inside a code block you wanted. Prefer structural sentinels the prompt establishes over ambient punctuation.
- **They also cap cost:** generation that stops at token 40 bills 40 output tokens, not max_tokens.

## 3.7 max_tokens and Truncation

`max_tokens` caps how many **output** tokens may be generated. Three things people chronically get wrong:

**It's a limit, not a target.** The model doesn't know your max_tokens and doesn't pace itself toward it. Setting 4,000 doesn't make answers longer; setting 50 doesn't make the model concise — it makes the model write a normal opening and get **cut off mid-sentence** (bug report #3). If you want short answers, *ask for short answers in the prompt*; use max_tokens as a cost/safety ceiling above the length you actually expect. A sensible pattern: prompt for ~100 words, set max_tokens ≈ 300.

**Truncation is silent unless you check.** A `length` finish reason means the tail of the response *does not exist*. For prose that's ugly; for structured output it's fatal — truncated JSON is invalid JSON, and *no amount of constrained decoding can save you*, because the constraint machinery masks each next token; it cannot force closing braces to arrive before an external guillotine falls. The #1 cause of "JSON mode returned invalid JSON" in the wild is max_tokens truncation, not the sampler. **Always check the finish reason. Treat `length` on structured output as a hard error: retry with a bigger budget or a tighter schema.**

**Budget accounting:** input + output must fit the context window (and some APIs also require input + max_tokens to fit *up front*, since the KV cache must have room — Lesson 1 again: max_tokens is a memory *reservation*). With a 200K window and a 195K prompt, no setting gets you more than ~5K of output. Long-context reading and long-form writing compete for the same budget. For thinking models, hidden reasoning tokens draw from the same output budget — a max_tokens that comfortably fits your visible answer can still truncate because thinking spent it first.

## 3.8 Putting It Together: A Production Request Checklist

Every parameter on the request, and the mechanical question it answers:

```
temperature      How sharp is the distribution?      0–0.3 tasks with one right answer;
                                                     ~0.7 chat; ~1.0 creative.
top_p            How much tail is deleted?           0.9–0.95; tune this OR temperature.
response format  Is syntax guaranteed or hoped for?  Constrain any output a machine parses.
                                                     Prompt for the format too.
stop             What ends the answer early?         Structural sentinels > punctuation.
max_tokens       Cost/safety ceiling.                ~2–3x expected length; NEVER exact.
finish reason    Did I get everything?               Check it. 'length' + JSON = error path.
stream           Perceived latency.                  On, for anything human-facing.
```

## 3.9 Summary: The Rules

1. **The model outputs a distribution; the sampler picks.** Everything here is post-processing on logits — the weights never change.
2. **Temperature divides logits pre-softmax.** Low sharpens, high flattens, 0 means argmax (greedy).
3. **T=0 removes sampler randomness, not system randomness.** Batch-dependent floating-point order can still flip near-ties. Don't promise byte-identical outputs.
4. **Top-p adaptively truncates the tail** and is preferred over top-k. Tune temperature or top-p, not both.
5. **Match sampling to task:** one-right-answer → cold; human-facing → warm; diversity-seeking → hot.
6. **Constrained decoding masks invalid tokens to −∞ per step** via a schema→grammar→FSM pipeline. It guarantees syntax, never correctness — and prompt for the format anyway.
7. **Schema field order is prompt engineering.** Reasoning fields before answer fields.
8. **Stop sequences match decoded text, end generation, and are not returned.**
9. **max_tokens is a guillotine, not a request.** Check finish reasons; `length` on structured output is an error, and no constraint can out-mask a truncation.
10. **max_tokens is also a KV memory reservation** — it competes with your prompt for the context window.

## 3.10 Drill 3

Rules: mechanisms and numbers. Any answer that could have been written without reading this lesson scores zero.

**Q1. Hand-run the sampler.**
Next-token logits for a 5-token toy vocabulary: [4.0, 3.0, 2.0, 1.0, −1.0].
(a) Compute softmax probabilities at T = 1.0, T = 0.5, T = 2.0 (show the arithmetic; 3 decimal places).
(b) At T = 1.0, which tokens survive top-p = 0.90? After renormalization, what are their probabilities?
(c) Which token does T = 0 pick, and what specific failure mode of greedy decoding (name it, from 3.4) would repeated greedy steps risk on long generations?

**Q2. The determinism postmortem.**
A teammate writes: "We set temperature=0, but the same prompt returned two different answers in prod, so the provider must be ignoring our parameter." Write the correction (≥150 words). Must correctly invoke: argmax, floating-point non-associativity, batch composition, divergence compounding. Then state what T=0 IS good for.

**Q3. The 2% JSON bug.**
The team from bug report #2 (prompt-only JSON, 2% parse failures at 1M req/day) asks you to fix it. (a) Explain mechanically why prompting can never get that 2% to 0%. (b) Explain what enabling schema-constrained output does at the logit level, step by step, for the concrete generation state `{"user_id": ` with schema `{"user_id": integer, "active": boolean}` — list what's allowed and what's masked. (c) After enabling it, failures drop to 0.3% — all truncations. Diagnose, and specify the exact two-line fix in their request/response handling.

**Q4. Design the request.**
For each scenario, give: temperature, top_p (or "leave default"), constrained-output yes/no (and schema sketch if yes, with field order justified), stop sequences if any, max_tokens with justification, and how you'd handle each finish reason. One short paragraph each:
(a) Extracting {name, date, amount} from OCR'd invoices, feeding an accounting system.
(b) A brainstorming endpoint returning 8 *diverse* campaign slogans as a JSON array.
(c) An inner-monologue agent step that must end with exactly one tool call.

**Q5. Break it on purpose.**
Predict the observable failure, and the mechanism, for each sabotage:
(a) T = 5.0, no top-p.
(b) top_p = 0.05 on a creative-writing prompt.
(c) stop = ["\n"] on a "write a Python function" prompt.
(d) max_tokens = 60 with a JSON schema of 12 required fields.
(e) Schema `{"final_answer": string, "step_by_step_reasoning": string}` — in that field order — on a hard math task. (This one degrades *accuracy*, not syntax. Why?)

**Q6. Reading.**
Read Holtzman et al., "The Curious Case of Neural Text Degeneration" (2019) and Willard & Louf, "Efficient Guided Generation for Large Language Models" (2023, the Outlines paper). Answer: (a) what pathology of *pure sampling* and what opposite pathology of *greedy/beam search* motivate nucleus sampling — one sentence each; (b) how does the Outlines paper reduce per-step mask computation to O(1)-ish — what is precomputed, over what structure; (c) name the token/character alignment problem both the paper and section 3.5 flag, with your own two-token example.

---

## Module 1 Master Rules

### Statelessness & caching
- The API remembers nothing; the client resends everything. Memory is a client-side illusion.
- KV cache = stored K/V per token per layer; it's why decode is O(1)-ish per token instead of O(n).
- KV size: `2 × layers × kv_heads × head_dim × 2 bytes` per token. HBM occupancy limits concurrency.
- Per-request caches die at request end. Prefix caching resurrects exact-token prefixes for minutes.
- Prompts: stable content first, append-only history, volatile data last or never. One early changed byte invalidates all downstream cache.

### Cost & latency
- ~2 FLOPs/param/token both directions; the input/output price gap is memory bandwidth, not math.
- Prefill: parallel, compute-bound → TTFT. Decode: sequential, bandwidth-bound → TPOT.
- Batching amortizes weight reads; KV reads are per-sequence and dominate at long context.
- Total latency ≈ TTFT + output_tokens × TPOT. Shorten the output before optimizing anything else.
- Conversation input cost is quadratic in turns. Cache, window, route, cap.

### Output control
- Sampler pipeline: logits ÷ T → top-k/top-p truncation → (constraint mask) → sample.
- T=0 = argmax; deterministic sampler ≠ deterministic system.
- Constrained decoding = per-step −∞ masking driven by a schema-compiled FSM. Syntax guaranteed; sense not.
- Stop sequences match decoded text and aren't returned. max_tokens is a guillotine and a memory reservation.
- Always check the finish reason. `length` + structured output = hard error.

### Success criteria

You've passed Module 1 when you can, unassisted:
- Compute a request's KV cache footprint and a GPU's rough concurrency ceiling from an architecture spec.
- Predict cold vs warm TTFT for a given prompt structure, and restructure a prompt for >90% cache hits.
- Reconstruct a monthly bill from turn counts and token sizes, and cut it ≥3x on paper with named mechanisms.
- Choose sampling/constraint/stop/max_tokens settings for a new endpoint in under five minutes, with reasons.
- Diagnose "different answers at T=0," "invalid JSON in JSON mode," and "response cut off" *without guessing*.

---

## Extra Reading

Ordered roughly easiest → deepest within each group. The starred (★) six are the core; the rest are depth.

### Foundations (how the machine works at all)
1. ★ **Jay Alammar — "The Illustrated Transformer"** (blog). The canonical visual walkthrough of attention, Q/K/V, and layers. Read before anything below.
2. ★ **Andrej Karpathy — "Let's build GPT: from scratch, in code"** (YouTube, ~2h) + the nanoGPT repo. You watch the autoregressive loop, tokenization, and sampling get typed out. Nothing makes statelessness clearer than seeing the loop.
3. **Vaswani et al. — "Attention Is All You Need"** (2017). The original paper. Skim for architecture; the serving-relevant intuition is all in #1 and #2.
4. **Karpathy — "Let's build the GPT Tokenizer"** (YouTube). BPE from scratch; explains every weird token-count behavior you'll ever see.

### Inference systems & the KV cache (Lesson 1 depth)
5. ★ **Kwon et al. — "Efficient Memory Management for LLM Serving with PagedAttention"** (SOSP 2023, the vLLM paper). Required for Drill 1 Q6. The clearest published account of KV memory pressure, paging, and block sharing.
6. ★ **Lilian Weng — "Large Transformer Model Inference Optimization"** (lil'log blog). A structured survey: KV caching, quantization, batching, distillation — the map of the whole territory.
7. **Anthropic docs — "Prompt caching"** and **OpenAI docs — "Prompt caching"**. The authoritative, current invalidation rules, TTLs, and prices for the APIs you'll actually call. Reread whenever pricing changes.
8. **Ainslie et al. — "GQA: Training Generalized Multi-Query Transformer Models"** (2023). Why modern KV caches are 8x smaller; short and readable.
9. **Zheng et al. — "SGLang / RadixAttention"** (2024). Prefix caching generalized to a radix tree over sessions; the natural next step after #5.

### Cost & performance arithmetic (Lesson 2 depth)
10. ★ **kipply — "Transformer Inference Arithmetic"** (blog). Required for Drill 2 Q6. The 2N FLOPs rule, bandwidth-vs-compute boundaries, and worked latency floors. The single highest-value read in this list for Lesson 2.
11. **Pope et al. — "Efficiently Scaling Transformer Inference"** (2022). Google's treatment of batching, sharding, and the latency/throughput Pareto frontier. Skim the graphs even if the math is heavy.
12. **Yu et al. — "Orca: A Distributed Serving System for Transformer-Based Generative Models"** (OSDI 2022). Where continuous batching comes from.
13. **NVIDIA — "Mastering LLM Techniques: Inference Optimization"** (developer blog). Practitioner-level restatement of Lessons 1–2 with current hardware numbers.

### Sampling & constrained decoding (Lesson 3 depth)
14. ★ **Holtzman et al. — "The Curious Case of Neural Text Degeneration"** (2019). Required for Drill 3 Q6. Why greedy loops and pure sampling rambles; the nucleus-sampling paper, and the most fun read here.
15. **Willard & Louf — "Efficient Guided Generation for Large Language Models"** (2023, the Outlines paper). Required for Drill 3 Q6. Regex/grammar → FSM over tokens → O(1) masking.
16. **The llguidance / XGrammar write-ups** (GitHub docs) and **llama.cpp GBNF grammar docs**. How production constraint engines handle the token/character alignment problem; good after #15.
17. **Chip Huyen — *AI Engineering*** (book, 2025), the inference-optimization and structured-output chapters. The best single consolidation of this whole module in print.

### How to use this list
After Lesson 1: read #1, #2, #5, #7. After Lesson 2: #10, then #6. After Lesson 3: #14, #15. Everything else is for when a drill answer of yours gets torn apart and you need to know *exactly* why.

---

*Module 1 complete. Module 2 (suggested): serving your own model — vLLM deployment, quantization in practice, speculative decoding, and benchmarking TTFT/TPOT yourself.*
