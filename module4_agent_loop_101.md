# The Agent Loop — Module 4 Course (Beginner Edition)

> The Loop, Stopping, Thinking, Planning, Delegation, and the Bill.
> Four lessons. Teaching mode is gentle and explains every term. Drills are harsh.
> Assumes Module 1 (statelessness, prefix caching, TTFT/TPOT, finish reasons) and that you've seen a tool/function call on the wire. Section 1.2 recaps the tool-call mechanics you need.

---

## Before You Start: What You're Building Toward

A chatbot answers. An **agent** *does things*: it searches, reads files, runs code, calls APIs, looks at what happened, and decides what to do next — over and over, until the job is done. Every coding assistant, research assistant, browser agent, and "autonomous" workflow you've seen is the same small machine underneath: **a loop wrapped around an LLM call.**

That machine is about fifteen lines of code. Building one that works in production is not. The loop takes every mechanic from Module 1 — statelessness, prefill, caching, output tokens, max_tokens — and runs it thirty, fifty, two hundred times in a row, where every mistake compounds and every token gets re-read on every later step.

The questions this module answers:

1. **Why did my agent run 80 steps on a task that needs 5 — or run all night?** (Termination and iteration caps.)
2. **Why did my agent announce "Done!" when nothing was done?** (Premature termination and verification.)
3. **Why does making the model "think" before each action help, and is that different from a reasoning model's hidden thinking?** (ReAct; reasoning tokens vs explicit chain-of-thought.)
4. **Should my agent write a plan first, or figure it out as it goes?** (Upfront vs interleaved planning.)
5. **When do subagents make an agent better, and when do they just multiply the bill?** (Delegation.)
6. **Why does a 30-step task cost 50x a single call instead of 30x, and why does it take four minutes?** (Cost and latency compounding.)
7. **Why does an agent that's "98% accurate per step" fail almost half its tasks?** (Reliability compounding.)

People who don't understand the loop build agents that wander, loop, lie about finishing, and cost ten times what they should — then try to fix it by adding more instructions to the system prompt. People who do understand it can read a trajectory log and point at the exact iteration where things went wrong, and can price a task on paper before running it.

By the end of this module you'll be able to write a production-grade loop with correct termination, choose a reasoning and planning strategy on purpose, decide when delegation pays, and compute a task's token bill and wall-clock time from first principles.

---

## A Small Glossary You'll See A Lot

I'll explain these properly as they come up, but bookmark this for quick reference:

- **Agent** = an LLM that directs its own control flow: it decides which action to take next, based on what it has observed so far, in a loop.
- **Workflow** = LLM calls orchestrated along a path *your code* fixes in advance. Not an agent, even if it has several LLM steps.
- **Harness / scaffold** = the ordinary code around the model: runs the loop, executes tools, enforces limits. The model proposes; the harness disposes.
- **Tool** = a function the model can ask the harness to run (search, read_file, run_tests, send_email), described to the model by name, description, and a JSON schema.
- **Tool call** = the model's structured request to run a tool with specific arguments.
- **Observation** = whatever comes back into the context after an action: a tool result, an error message, a user reply.
- **Iteration / step / turn** = one pass around the loop: one model call, plus the tool executions it requested.
- **Trajectory / transcript** = the full accumulated history of a run: task, every model output, every observation.
- **Stop reason / finish reason** = why a single model call ended (`tool_use`, `end_turn`, `max_tokens`...). Not the same as why the *loop* ended.
- **Termination condition** = any rule that ends the loop: task complete, cap reached, verification passed, human interrupt.
- **Iteration cap** = a hard maximum on loop iterations (`max_steps`). The agent's circuit breaker.
- **ReAct** = "Reason + Act": the pattern of writing a short thought before each action and reading the observation after it, interleaved.
- **Chain-of-thought (CoT)** = reasoning written out as visible text before an answer, usually because the prompt asked for it.
- **Reasoning tokens / thinking tokens** = output tokens a model was *trained* to generate in a separate thinking phase before its visible answer. Often hidden or summarized; always billed.
- **Interleaved thinking** = the model produces a thinking phase after each tool result, not only once at the start of its turn.
- **Plan** = an explicit list of intended steps, written down (in context or in a file) before or during execution.
- **Replanning** = revising the plan after an observation invalidates it.
- **Orchestrator / lead agent** = an agent that breaks work into pieces and hands them to other agents.
- **Subagent / worker** = an agent run on a sub-task, in its own fresh context, returning a condensed result.
- **Context isolation** = the subagent's working tokens never enter the orchestrator's context; only its result does.
- **Fan-out** = launching several subagents or tool calls in parallel.
- **Compaction** = replacing a long stretch of old trajectory with a shorter summary to keep the context window in check.
- **Cost per successful task** = total spend ÷ tasks that actually succeeded. The only agent cost metric that matters.

---

# Lesson 1: The Loop and How It Ends

## 1.1 Why This Lesson Exists

A team builds their first agent on a Friday. It has one tool, `web_search`, and the instruction "research the question and answer it." In the demo it's magical: three searches, a crisp answer, done in twenty seconds.

Monday morning there's a $412 charge on the API account. The logs show one run that went 1,900 iterations overnight. Around iteration 12 the search tool started returning an error (rate limit), the model politely said "Let me try that search again," the harness dutifully re-ran it, the error came back, and the loop had no reason to ever stop. Every iteration resent the whole growing transcript — and you already know from Module 1 what that does to the bill.

Nothing in that story is a model problem. The model did exactly what it was asked, one step at a time. The failure was in the **loop**: what counts as an observation, who decides the task is over, and what happens when nobody decides. This lesson is about the loop itself — how it's built, what each phase really does, and the four ways it's allowed to end.

## 1.2 What an Agent Actually Is (Just Enough)

Strip away the frameworks and the marketing, and an agent is this:

```python
def run_agent(task, tools, max_steps=25):
    messages = [{"role": "user", "content": task}]

    for step in range(max_steps):
        response = llm.call(system=SYSTEM, tools=tools,           # DECIDE
                            messages=messages)
        messages.append({"role": "assistant", "content": response.content})

        if response.stop_reason == "end_turn":                    # model says: done
            return Result("complete", response.text)
        if response.stop_reason == "max_tokens":                  # truncated call!
            messages.append(user_msg("Your last output was cut off. Be briefer."))
            continue

        results = []
        for call in response.tool_calls:                          # ACT
            output = execute(call.name, call.args)
            results.append(tool_result(call.id, output))          # OBSERVE
        messages.append({"role": "user", "content": results})

    return wrap_up(messages)                                      # cap hit
```

That's it. **An agent is a `for` loop around an LLM call.** Everything else in this module is about making those fifteen lines behave.

### Tool calls on the wire (the 60-second recap)

You send the model, alongside the conversation, a list of tool definitions:

```json
{"name": "read_file",
 "description": "Read a UTF-8 text file from the repo. Returns at most 400 lines.",
 "input_schema": {"type": "object",
                  "properties": {"path": {"type": "string"}},
                  "required": ["path"]}}
```

When the model wants a tool, it doesn't run anything. It emits a structured block — tool name, arguments as JSON, an ID — and the call ends with stop reason `tool_use`. (Under the hood, this is constrained decoding from Module 1 §3.5 doing its job: the arguments are schema-shaped.) Your harness runs the function, then sends a new request containing *everything so far* plus a `tool_result` block carrying the output and the matching ID. The model reads the result and either calls another tool or writes a final answer and stops with `end_turn`.

Two Module 1 facts now become the most important facts in this module:

1. **The API is stateless.** Iteration 20 resends the task, all 19 previous model outputs, and all 19 previous observations. The trajectory *is* the agent's memory, and you pay to re-read it every iteration.
2. **The trajectory is append-only by nature.** Each iteration extends the previous request. That makes agent loops the best-case scenario for prefix caching — *if* you don't sabotage it (Lesson 4).

### Agents vs workflows

Not every multi-step LLM system is an agent, and it's worth being precise, because the distinction decides how you test, price, and debug it:

```
Single call      one prompt → one answer
Workflow         your code fixes the path: extract → classify → draft → check
                 (LLM calls at each box, but the arrows are code)
Router           an LLM picks WHICH fixed path to run
Agent loop       the LLM picks the NEXT STEP, every step, based on observations
                 (the arrows themselves are decided at runtime)
```

Workflows are predictable, cheap, and testable. Agents are flexible and handle tasks whose steps can't be known in advance — at the price of variable cost, variable latency, and new failure modes. **Reach for an agent only when the path genuinely can't be written down ahead of time.** If you can draw the flowchart, write the flowchart.

## 1.3 Observe → Decide → Act → Repeat

Every iteration has three phases. Knowing exactly where each one lives in the code is how you know where to put every safety check.

**Observe.** Something enters the context window: the original task, a tool result, an error, a user interruption. The crucial, easily-forgotten point: **the model knows only what is in its context.** It cannot see the file you didn't show it, the error you swallowed, or the side effect that happened silently. If an observation is missing, misleading, or 40,000 tokens of noise, every downstream decision is made blind.

**Decide.** One inference call. The model reads the full trajectory (prefill — mostly cache hits if you've structured it well) and generates either a tool call or a final answer (decode). This is the *only* phase where the LLM runs. It is also the only phase that is non-deterministic.

**Act.** The harness executes the requested tool. The model never touches the world; it *proposes*, your code *performs*. That's a gift: every act passes through ordinary code you control, which is exactly where you put validation ("is that path inside the repo?"), permissions ("`delete_branch` requires human approval"), rate limits, and timeouts.

**Repeat.** The observation from this act becomes the input to the next decide.

```
           ┌──────────────────────────────────────────────┐
           ▼                                              │
  [context window] ──► DECIDE (LLM call) ──► tool call? ──yes──► ACT (harness)
     ▲                         │                                  │
     │                         no                                 │
     │                         ▼                                  │
     │                   final answer                             │
     │                   (termination candidate)                  │
     └──────────────── OBSERVE (append result) ◄──────────────────┘
```

### Observations are the agent's eyes: four rules

1. **Errors are observations, not exceptions.** If a tool throws, catch it and return the error text as the tool result: `"Error: file not found: src/utlis.py. Did you mean src/utils.py?"` A model that sees a clear error recovers; a harness that crashes on the error ends the run; a harness that silently returns `""` makes the model hallucinate.
2. **Bound every observation.** A `cat` of a log file can return 2 million tokens. Truncate, paginate, or summarize tool output at the harness level — and *say so* in the output (`"[truncated: showing lines 1–400 of 18,212]"`) so the model knows more exists.
3. **Make observations decision-ready.** Return what the model needs to choose the next step, not raw dumps. "Tests: 47 passed, 2 failed: test_auth_expiry (AssertionError line 88), test_refresh (Timeout)" beats 900 lines of pytest output.
4. **Keep formats stable.** Same structure every time for the same tool. Stable formats are easier for the model to read and, as a bonus, cache-friendly.

## 1.4 Termination: The Four Ways a Loop Ends

A loop must end. There are exactly four families of reasons it can, and a production harness uses all four.

**1. Natural completion — the model decides it's done.** In native tool-calling APIs, the model returns a response with no tool calls and stop reason `end_turn`. Many harnesses make this sharper with an explicit **`finish` / `submit` tool**: the only way to end the run is to call `finish(answer=..., status=...)`. Advantages: the final answer is schema-validated (Module 1's constrained decoding again), you can require structured fields like `status` and `evidence`, and "done" is an unambiguous event rather than the absence of a tool call.

**2. Hard caps — the harness decides it's over.** Budgets the model cannot argue with:

```
max_steps          iterations                 e.g. 25
max_total_tokens   input + output, summed     e.g. 2,000,000
max_cost           dollars                    e.g. $2.00
max_wall_clock     seconds                    e.g. 600
max_context        tokens in the window       must stay under the model's limit
```

**3. External verification — the world decides.** A goal predicate your code can check: tests pass, the file exists and parses, the API returned 200, the SQL result matches the expected row count. The strongest termination signal there is, because it doesn't depend on the model's opinion of its own work.

**4. Interrupts — a human decides.** User cancels, an action requires approval that's denied, an upstream deadline fires.

### The failure modes each one guards against

**Runaway loops.** The model keeps acting forever: retrying a failing tool (the $412 story), paging through infinite results, "improving" code that already works. Guard: hard caps + loop detection.

**Premature termination.** The model says "I've fixed the bug and all tests pass!" having run no tests. This is the most dangerous failure because it *looks like success*. Language models are trained to produce plausible completions, and "Done! Everything works." is a very plausible completion. Guard: never accept the model's claim of success when a check is available — run the verification yourself, and if it fails, feed the failure back as an observation:

```
Observation (from harness, not a tool):
"You called finish(), but verification failed: 2 tests still failing
 (test_auth_expiry, test_refresh). Continue working."
```

That costs another iteration. It's supposed to.

**Oscillation.** A→B→A→B: the agent edits a line, a test fails, it reverts the edit, a different test fails, it re-applies the edit. Each step is locally reasonable; the loop makes no progress. Guard: loop detection + no-progress detection.

**Context overflow.** The trajectory outgrows the context window, and the next call fails with a hard error (or, worse, a harness silently drops the oldest messages — including the task statement). Guard: track context size every iteration; compact or stop well before the limit.

### Loop detection, concretely

```python
fingerprint = hash((call.name, canonical_json(call.args)))
recent.append(fingerprint)
if recent[-10:].count(fingerprint) >= 3:
    # Same exact action 3 times in the last 10 steps.
    inject_observation("You have tried this exact action 3 times with the same "
                       "result. Try a different approach or call finish() with "
                       "status='blocked' and explain why.")
    strikes += 1
    if strikes >= 2: return wrap_up(messages, reason="loop_detected")
```

No-progress detection is the same idea over a *metric* instead of an action: if the number of passing tests (or rows extracted, or pages processed) hasn't improved in K iterations, nudge, then stop.

## 1.5 Iteration Caps: Setting Them, and What Happens When They Hit

### Setting the cap

Remember Module 1's rule about `max_tokens`: **a limit, not a target.** Iteration caps are the same. The model doesn't pace itself to your cap (unless you tell it the cap — more on that below). The cap exists to bound the damage of runs that have already gone wrong.

So set it from data, not vibes. Run your agent on a representative task set and record steps-to-success:

```
Steps used by SUCCESSFUL runs (500 tasks):
  p50: 7    p90: 14    p95: 19    p99: 31    max: 44

Steps used by FAILED runs: bimodal — many die early (errors),
and a long tail wanders until whatever cap you set.
```

A reasonable starting cap is a bit above the p99 of successful runs — say 35–40 here. Below that, you're killing runs that would have succeeded. Far above it, you're mostly paying for runs that were already lost: past p99, the probability that *this* run is going to succeed drops sharply, while each extra iteration costs *more* than the last (Lesson 4 — every iteration re-reads a longer transcript). The late iterations are the most expensive and the least likely to help.

### What to do when the cap hits

The worst response to a cap is `return None`. The run did work; some of it is probably useful; the caller needs to know what happened. Graceful degradation:

```python
def wrap_up(messages, reason):
    # One final call, tools disabled: the model can only write text.
    summary = llm.call(system=SYSTEM, tools=[], messages=messages + [user_msg(
        f"Stop now ({reason}). Report: what you completed, what remains, "
        f"and any partial results. Do not claim completion of unfinished work.")])
    return Result(status="incomplete", reason=reason, report=summary.text)
```

The caller now gets `status="incomplete"` — machine-readable — plus a human-readable handoff. Treat `incomplete` like Module 1 taught you to treat finish reason `length` on structured output: **an error path, not a result.**

### Should the model know its budget?

You can tell the model "You have 20 tool calls for this task" or inject a running counter ("Step 14 of 20"). Trade-offs:

- **Pro:** the model can pace itself — skip nice-to-have exploration, prioritize, wrap up gracefully before the guillotine.
- **Con:** models may wrap up *too early*, cut corners near the limit, or burn tokens discussing the budget.
- **Practical middle ground:** state the budget up front, and inject a single warning near the end ("3 steps remaining — prioritize finishing and reporting"). Measure success rate with and without; it's an empirical question for your task.

## 1.6 Summary: The Rules

1. **An agent is a loop around an LLM call.** Observe → decide → act → repeat. Only *decide* runs the model.
2. **The model proposes; the harness acts.** Every validation, permission, and limit lives in the act phase, in ordinary code.
3. **The model knows only its context.** Errors must be returned as observations; outputs must be bounded and decision-ready.
4. **Loops end four ways:** natural completion, hard caps, external verification, interrupts. Use all four.
5. **Never trust "done" when you can check.** Verification failures go back in as observations.
6. **Detect loops and non-progress** by fingerprinting actions and tracking a progress metric.
7. **Set caps from the distribution of successful runs** (a bit above p99). A cap is a limit, not a target.
8. **Cap hits return a structured `incomplete` result**, via a final tools-disabled wrap-up call. Never `None`, never silent.
9. **Stop reason ≠ termination reason.** `end_turn` ends a call; your harness decides whether it ends the task.

## 1.7 Drill 1

Rules: show mechanism, not vibes. "Add a limit" with no number and no justification gets zero credit. Reply with your answers and I'll tear them apart.

**Q1. Explain the mechanism.** In your own words (≥200 words), trace one full iteration of an agent loop from the moment a tool result arrives to the moment the next tool executes. Your answer must correctly use: observation, stateless, prefill, decode, stop reason, harness, prefix cache. Then explain why "the model ran the command" is technically false, and why that distinction is a *security* property, not a pedantic one.

**Q2. The loop audit.** Here is a (bad) agent loop:

```python
def agent(task):
    history = f"Task: {task}\n"
    while True:
        out = llm(SYSTEM + f"\nCurrent time: {datetime.now()}\n" + history,
                  max_tokens=200)
        if "DONE" in out:
            return out
        tool, args = parse(out)
        result = TOOLS[tool](**args)
        history = summarize(history) + out + str(result)
```

Find **every** defect. For each: name the line, the concrete failure it causes (describe the transcript or the bill you'd see), and the fix. There are at least eight. At least two of them are Module 1 defects wearing Module 4 clothing — name which. Then rewrite the loop correctly.

**Q3. Set the cap.** Successful runs: p50 = 9, p90 = 18, p95 = 24, p99 = 41, max = 63 steps. 22% of all runs fail; of those, 60% currently hit your cap of 100. Assume an iteration's cost grows linearly: iteration k costs $0.01 + $0.002·k.
(a) Total cost of one run that goes all the way to a cap of 100? To a cap of 45?
(b) Choose a cap and justify it with the numbers. What fraction of eventual successes do you estimate you'd lose, and what do you save per 1,000 runs?
(c) Your PM says "just set it to 15, most tasks finish by then." Quantify what breaks.

**Q4. Diagnose from the transcript.** For each trajectory excerpt, name the failure mode from §1.4, the missing harness mechanism, and the fix:
(a) `read_file("config.yaml")` → `""` ... `read_file("config.yaml")` → `""` ... (×40)
(b) Step 3: `edit(line 42, "x+1")` → tests 11/12 ... Step 4: `edit(line 42, "x")` → tests 10/12 ... Step 5: `edit(line 42, "x+1")` → tests 11/12 ...
(c) Step 2: `finish(status="success", summary="Migrated all 14 tables.")` — the migration tool was never called.
(d) Step 37: API error `prompt is too long`.

**Q5. Design termination.** You're building an agent that fixes failing CI builds by editing code and pushing a branch. Specify all four termination families concretely: the `finish` tool schema (fields and why), every hard cap with a number and justification, the verification predicate, the interrupt points (which actions need human approval, and why those), and exactly what the caller receives in each ending.

**Q6. Reading.** Read Anthropic's "Building Effective Agents" (Dec 2024) and the "Agent System Overview" and "Component One: Planning" parts of Lilian Weng's "LLM Powered Autonomous Agents." Answer: (a) how does Anthropic distinguish workflows from agents, and which five workflow patterns does it name? (b) When does it recommend *not* building an agent? (c) Which of Weng's three components (planning, memory, tool use) maps to which phase of observe → decide → act?

---

# Lesson 2: Thinking Inside the Loop — ReAct and Reasoning Tokens

## 2.1 Why This Lesson Exists

Take one model, one set of tools, one task: *"Why did last night's nightly build fail?"* Run it two ways.

**Version A** is told: "Use the tools to answer." It immediately calls `read_log(stage="unit-tests")` — the wrong stage — gets a clean log, calls `read_log(stage="lint")`, clean again, then answers "The build failed due to a flaky test," which is invented.

**Version B** is told: "Before each tool call, write one or two sentences on what you know and what you need next." It writes *"I don't know which stage failed; I should get the build summary first,"* calls `get_build("nightly-latest")`, learns the integration stage failed, writes *"Integration tests failed; I need the actual error,"* and so on to the correct root cause in four steps.

Same weights, same tools. The only difference is a few dozen tokens of text written *before* each action. This lesson explains mechanically why those tokens matter, names the pattern (**ReAct**), and then deals with the question that 2024–2026 made unavoidable: now that many models have built-in, trained **reasoning tokens**, when do you still prompt for visible reasoning, and when is that just paying twice?

## 2.2 The ReAct Pattern

**ReAct** (Yao et al., 2022 — "Reasoning + Acting") is the observation that two earlier approaches each fail in a complementary way:

- **Reason-only (chain-of-thought):** the model thinks step by step, but entirely from its own memory. It can't check facts, so its reasoning drifts into confident hallucination. Great logic, no grounding.
- **Act-only:** the model fires tool calls without articulating why. It gets real observations but has no working plan, loses track of what it's looking for, and can't recover gracefully when something unexpected comes back. Great grounding, no logic.

ReAct interleaves them: **Thought → Action → Observation → Thought → Action → Observation → ... → Answer.**

```
Question: Why did last night's nightly build fail?

Thought 1:  I don't know which stage failed. Get the build summary first.
Action 1:   get_build(id="nightly-latest")
Observation 1: status=failed, failed_stage="integration-tests", duration=14m

Thought 2:  Integration tests failed. I need the actual error, not the whole log.
Action 2:   read_log(stage="integration-tests", tail=50)
Observation 2: ... ConnectionRefusedError: could not connect to postgres:5432 ...

Thought 3:  The DB wasn't reachable. Is that a code change or the DB service itself?
            Check the service startup logs before blaming code.
Action 3:   read_log(stage="services", grep="postgres")
Observation 3: postgres: FATAL: could not write to file: No space left on device

Thought 4:  Root cause is infrastructure — the CI runner's disk is full, so Postgres
            never started. No code change is implicated.
Answer:     The build failed because the CI runner ran out of disk space ...
```

### Why it works (the mechanical version)

Recall Module 1's one-sentence summary of generation: **every token is conditioned on every token before it.** So:

1. **Thoughts condition actions.** When the model writes "I need the actual error, not the whole log" and *then* generates the tool call, the arguments (`tail=50`) are sampled from a distribution shaped by that sentence. Same reason Module 1 said to put reasoning fields *before* answer fields in a schema.
2. **Thoughts are extra compute.** Each thought token is another forward pass over the whole context. The model gets more computation spent on the decision before committing to it.
3. **Thoughts persist.** Thought 2 stays in the transcript. At iteration 6, the model can still see *why* it did what it did at iteration 2. It's a scratchpad that doubles as working memory — which, remember, is the only memory a stateless model has.
4. **Thoughts are inspectable.** When a run goes wrong, the thought before the bad action usually tells you *which belief* was wrong. That turns debugging from superstition into diagnosis.

### Original ReAct was plain text — and needed a Module 1 trick

The 2022 paper predates native tool calling. The model wrote literal text like `Action: search[nightly build]`, and the harness regex-parsed it. There was an obvious problem: having written `Action: ...`, the most plausible next text is `Observation: ...` — so the model would happily **invent the observation** and keep going, reasoning over results it hallucinated.

The fix was a **stop sequence** (Module 1 §3.6): `stop=["\nObservation:"]`. Generation halts the instant the model tries to write an observation; the harness runs the real tool and inserts the real text. If you ever build a text-protocol agent (small local models, unusual APIs), this is the single line that separates an agent from a fiction generator.

Modern native tool calling does this structurally: the model's turn *ends* at the tool call (stop reason `tool_use`), and observations come back as `tool_result` blocks the model can't author. The ReAct *idea* — reason before acting, reason after observing — carries over intact. Where the "Thought" physically lives now depends on the model: either as visible text before the tool call, or as **reasoning tokens**.

## 2.3 Reasoning Tokens vs Explicit Chain-of-Thought

These two things look similar — both are "the model thinking before it acts" — but they're different mechanisms with different costs, controls, and failure modes. Getting this distinction right is worth real money.

### Explicit chain-of-thought (prompted)

Visible text you ask for: "Think step by step," "Before each tool call, briefly explain your reasoning," or "Reason inside `<thinking>` tags, then act." Mechanically it's **ordinary output tokens**:

- You see all of it, can log it, evaluate it, and parse it.
- It's billed as output.
- **It stays in the transcript**, so every later iteration re-reads it as input (this matters — see §2.4).
- Its quality is whatever the model's habits produce when asked; the model wasn't specifically trained to make this text maximally useful for its own decisions.

### Reasoning tokens (trained)

Reasoning models are trained — typically with reinforcement learning on problems with checkable answers — to generate a thinking phase *before* the visible response. The API exposes this as a separate block (or hides it) and gives you a knob for how much to think (a token budget, or an "effort" level). Mechanically:

- They are still **output tokens, generated by the same autoregressive loop** — same TPOT, same billing as output, same draw on `max_tokens` (Module 1 §3.7: a `max_tokens` that fits your answer can still truncate if thinking spends the budget first).
- **Visibility is provider-dependent:** full raw thinking, a summary of it, or only an opaque/encrypted blob plus a token count.
- **Persistence across iterations is provider-dependent**, and you must know your provider's rule. Common variants: prior-turn reasoning is automatically dropped from context; reasoning must be passed back verbatim along with tool results during a tool-use sequence (Anthropic's API requires this for thinking blocks); reasoning can be carried forward as opaque items (OpenAI's Responses API supports this). Check current docs — this changes.
- You can't edit reasoning blocks, and some APIs restrict other knobs while thinking is on (e.g., forcing a specific tool or changing temperature).
- On hard problems, trained reasoning is usually substantially stronger than prompted CoT from the same model family. That's the whole point of the training.

### Interleaved thinking: ReAct, built in

Early reasoning models thought once, at the start of a turn, then could chain several tool calls with no reasoning between them — effectively act-only after the first step. **Interleaved thinking** means the model gets a thinking phase after *each* tool result, before choosing the next action. That is the ReAct loop implemented in the model's trained behavior instead of your prompt. For agents, this is the mode you want when the model supports it.

### The comparison table

```
                        Explicit CoT (prompted)       Reasoning tokens (trained)
──────────────────────────────────────────────────────────────────────────────────
Origin                  Your prompt asks for it       Model trained to think first
Visible to you?         Yes, plain text               Raw / summarized / hidden (provider)
Billed as               Output                        Output — even when hidden
Draws on max_tokens?    Yes                           Yes (often + a separate budget knob)
Stays in transcript?    Yes, every iteration          Provider-dependent; often dropped
Re-read as input later? Yes — compounds (§2.4)        Only if carried forward
Editable / parseable?   Yes                           No; pass back verbatim if required
Latency                 Delays the tool call          Delays the tool call, invisibly
Strength on hard steps  Model's untrained habit       Usually much stronger
Faithful to the real    Not guaranteed                Not guaranteed
  computation?
──────────────────────────────────────────────────────────────────────────────────
```

### The faithfulness warning

Neither form of reasoning is a guaranteed window into *why* the model chose what it chose. Research (e.g., Turpin et al., 2023, "Language Models Don't Always Say What They Think") shows models can produce plausible reasoning that omits the factor that actually drove the answer. Use reasoning text as a debugging aid and a quality lever. **Never use it as a security or compliance control** ("we check the reasoning for bad intent before executing") — enforce safety in the harness's act phase, where it's mechanical.

### What to actually do

```
Situation                                       Configuration that makes sense
──────────────────────────────────────────────────────────────────────────────────
Reasoning model, hard multi-step task           Thinking ON (interleaved if available).
                                                Don't ALSO demand long visible CoT —
                                                you're paying twice. Ask for a one-line
                                                visible rationale only if you need an audit log.
Reasoning model, mechanical steps               Low thinking budget/effort. Most steps
  (fetch page 2, rename file)                   of a long run are mechanical.
Non-reasoning model                             Prompt ReAct-style: 1–3 sentence thought
                                                before each tool call. Keep it short.
Compliance needs a readable decision trail      Explicit, brief rationale field — as
                                                documentation, not as a safety control.
Latency-critical agent                          Smallest thinking budget that holds
                                                success rate; measure, don't guess.
──────────────────────────────────────────────────────────────────────────────────
```

The general principle: **thinking is a per-step spend.** The best agents spend a lot on the few steps where a decision is hard (choosing an approach, interpreting a surprising error) and very little on the many steps that are mechanical.

## 2.4 The Hidden Cost: Thoughts Compound

Here is the part people miss. Visible thoughts don't cost their output price once — they cost it once, **plus an input re-read on every later iteration**, because they live in the transcript.

```
A 20-iteration run. Each iteration writes a 300-token visible thought.
Prices: $3/M input, $15/M output, $0.30/M cached read.

Output cost of the thoughts:     20 × 300 = 6,000 tokens × $15/M     = $0.090

Re-read cost: the thought from iteration k is re-read in iterations k+1..20.
  Total re-reads = 300 × Σ(k=1..20) (20 − k) = 300 × 190 = 57,000 tokens
  Uncached:  57,000 × $3/M     = $0.171
  Cached:    57,000 × $0.30/M  = $0.017

Uncached, the thoughts' re-reads cost ~2x their generation.
```

The re-read term grows with the **square** of the iteration count (Σ(N−k) = N(N−1)/2), so at 60 iterations it's 300 × 1,770 = 531,000 tokens of re-reading — 30x the 18,000 output tokens that generated them. Caching tames the constant; it doesn't change the shape.

Now compare reasoning tokens under a provider that **drops** prior reasoning from context: the thinking is paid once, as output, and never re-read. On long loops that can make hidden reasoning *cheaper* per unit of thought than visible CoT — the opposite of most people's intuition. Under a provider that **carries reasoning forward**, it compounds exactly like visible text. Know which you're on, and do the arithmetic.

## 2.5 Summary: The Rules

1. **ReAct = interleave thought, action, observation.** Reason-only hallucinates; act-only flails; interleaved gets both grounding and direction.
2. **Thoughts work because generation is autoregressive:** text before the tool call conditions the call, adds compute, and persists as working memory.
3. **Text-protocol ReAct needs a stop sequence on "Observation:"**, or the model invents its own tool results. Native tool calling enforces this structurally.
4. **Explicit CoT is ordinary output text:** visible, billed, persistent, re-read every later iteration.
5. **Reasoning tokens are trained, billed as output, draw on max_tokens, and have provider-specific visibility and persistence rules.** Learn your provider's rules.
6. **Interleaved thinking is ReAct built into the model.** Prefer it for agents.
7. **Don't pay twice:** with thinking on, don't also demand long visible reasoning.
8. **Spend thinking where decisions are hard,** not uniformly on every step.
9. **Visible thoughts compound quadratically** through re-reads. Stripped reasoning doesn't.
10. **Reasoning text is not a faithful record** and never a safety control. Safety lives in the harness.

## 2.6 Drill 2

Rules: mechanisms and arithmetic. Any answer that could be written without reading this lesson scores zero.

**Q1. Explain the mechanism.** In ≥200 words, explain why writing a thought *before* a tool call changes which tool call gets generated. Must correctly use: autoregressive, conditioning, forward pass, transcript, working memory. Then explain why putting the thought *after* the tool call (in the same model output) gives none of the decision benefit — and which Module 1 lesson made the identical argument about JSON field order.

**Q2. The hallucinated observation.** A team runs a small local model with a text ReAct prompt and native tool calling unavailable. Their traces look like this:

```
Thought: I should check the user's order status.
Action: lookup_order[#4471]
Observation: Order #4471 shipped on March 3 via UPS, tracking 1Z999...
Thought: Great, it shipped. I'll tell the user.
```

The `lookup_order` tool was never executed — the logs prove it. (a) Explain exactly what happened, token by token. (b) Give the one-line fix and explain *where in the pipeline* it acts (Module 1 §3.6 — be specific about decoded text vs token IDs). (c) After the fix, the model sometimes stops mid-word at "Observ". Why could that happen, and why is it harmless?

**Q3. Compounding arithmetic.** An agent runs N = 40 iterations. Each writes a visible thought of t = 250 tokens. Prices: $3/M input, $15/M output, $0.30/M cached read.
(a) Output cost of all thoughts.
(b) Total re-read tokens of those thoughts, and their cost uncached and cached.
(c) You switch to a reasoning model that uses 800 hidden thinking tokens per iteration, dropped from context after each turn, plus a 40-token visible rationale. Compute the thinking-related cost (output + re-reads) for 40 iterations and compare to (a)+(b) uncached. At what N does the reasoning configuration become cheaper, if ever?
(d) Name the one provider behavior that would flip your conclusion in (c).

**Q4. Configure three agents.** For each, choose: reasoning model or not; thinking on/off and budget level (low/medium/high, with reason); whether to request visible rationale and how long; interleaved or not. One short paragraph each, with at least one number.
(a) A bulk agent that renames and re-tags 10,000 product listings via an API, ~6 steps each.
(b) A coding agent debugging race conditions in a large repo, ~40 steps.
(c) A customer-support agent that must keep an audit trail of *why* it issued each refund, ~5 steps, p95 latency budget 8 seconds.

**Q5. The faithfulness trap.** A fintech proposes: "Before any `transfer_funds` call executes, a second model reads the agent's reasoning, and blocks the transfer if the reasoning looks suspicious." (a) Explain why this is a weak control, citing the faithfulness research. (b) Design the control that should exist instead, and state which phase of observe → decide → act it lives in. (c) Is there *any* legitimate value in the reasoning-reviewer? Where would you put it?

**Q6. Reading.** Read Yao et al., "ReAct: Synergizing Reasoning and Acting in Language Models" (2022), sections 1–3, and skim Wei et al., "Chain-of-Thought Prompting Elicits Reasoning in Large Language Models" (2022). Then read your provider's current docs on extended thinking / reasoning models, specifically the sections on tool use. Answer: (a) on which kind of task did ReAct beat CoT, and on which did CoT hold its own — and what explains the difference? (b) What failure mode did the ReAct authors see when reasoning and acting were *not* interleaved? (c) Per your provider's docs: what happens to reasoning blocks between tool calls and between user turns, and what must your harness send back?

---

# Lesson 3: Structuring the Work — Planning and Delegation

## 3.1 Why This Lesson Exists

Pure ReAct is a greedy algorithm: at every step, pick the best-looking next action given what you've seen. On a five-step task, that's fine. On a fifty-step task, three things go wrong:

1. **Myopia.** The agent takes the locally obvious step, then the next, and ten steps later discovers the whole approach was wrong. Nobody ever looked at the task as a whole.
2. **Goal drift.** By iteration 40, the original task statement is 60,000 tokens back in the context, buried under file contents and logs. The agent is now very focused on fixing a lint warning it created at step 31, and has forgotten the user asked for a migration.
3. **Context pollution.** Every search result, every file read, every failed attempt stays in the window forever. The context fills with junk that is irrelevant to the current decision but still costs money to re-read and still competes for the model's attention.

Two structural tools fix these: **planning** (deciding the shape of the work before or during doing it) and **delegation** (handing pieces of the work to subagents with their own clean contexts). Both are powerful, both have real costs, and both are frequently used in exactly the wrong situations.

## 3.2 Upfront Planning (Plan-then-Execute)

In **upfront planning**, one model call produces the whole plan before any action is taken:

```
Task: "Add rate limiting to all public API endpoints."

PLAN (one call, before any tools run):
  1. List all route files under src/api/.
  2. Identify which endpoints are public (no auth decorator).
  3. Check whether a rate-limit middleware already exists.
  4. Implement or configure middleware; apply to public endpoints.
  5. Add tests for 429 responses.
  6. Run full test suite.

EXECUTE: steps 1–6, in order.
```

Research variants push this further. **Plan-and-Solve** prompting (Wang et al., 2023) simply asks the model to devise a plan before solving. **ReWOO** (Xu et al., 2023) has the planner write the entire tool chain up front with placeholders (`#E1 = search(...)`, `#E2 = read(#E1)`) so the executor can run it with *no LLM call between tools*, then a solver call reads all the evidence at the end — far fewer model calls and tokens. **LLMCompiler** (Kim et al., 2023) plans a dependency graph (DAG) and runs independent tool calls in parallel.

**Why upfront planning is attractive:**
- **Fewer decide calls.** If the plan is good, execution needs little or no LLM involvement per step. Fewer calls = fewer transcript re-reads (Lesson 4).
- **Parallelism.** A plan exposes which steps are independent; independent steps can run at once.
- **Auditability and approval.** A human can review the plan *before* anything irreversible happens. For high-stakes tasks, this is often the whole reason to plan.
- **Cheaper execution.** A strong model plans; a cheaper model (or plain code) executes.

**Why it's brittle:** the plan is written **before any observations exist.** Every step encodes assumptions about a world the planner hasn't seen yet. Step 3 above assumes "a rate-limit middleware" is a thing the codebase might have; if the repo turns out to use an API gateway that handles rate limiting outside the application, steps 4–6 are wrong, and a rigid executor will do them anyway. It's the same failure as Module 1's answer-first JSON schema: **committing before the evidence is in.**

## 3.3 Interleaved Planning (Plan-as-You-Go)

**Interleaved planning** decides the next step (or next few) after each observation. Pure ReAct is the extreme case: no written plan, one step at a time, full adaptivity.

**Strengths:** adapts instantly to surprises; never executes a step whose premise has already been falsified; works in environments you genuinely can't predict (web browsing, debugging, research).

**Weaknesses:** everything in §3.1 — myopia, drift, no global view. Also: **no cost forecast.** An upfront plan with 6 steps gives you a rough bill in advance; a ReAct agent tells you what it cost afterwards.

## 3.4 The Hybrid Everyone Actually Ships

In practice, strong agents do both: **plan upfront, execute interleaved, replan on surprise.** The plan is a *living artifact*, not a contract.

```
1. PLAN      Write a short plan (a todo list) at the start.
2. EXECUTE   Work ReAct-style on the current item; tick items off as they finish.
3. REPLAN    Revise the list when:
               - a step fails in a way that isn't a simple retry,
               - an observation contradicts an assumption the plan relied on,
               - (optionally) every K steps, as a checkpoint.
4. FINISH    When the list is empty and verification passes (Lesson 1).
```

Two implementation details carry most of the value:

**Recitation fights drift.** Periodically restating the current plan — "Remaining: 4. Apply middleware to /orders, /search. 5. Tests. 6. Full suite." — puts the goal back at the *end* of the context, where it's most recent and most salient for the next decision. The team behind the Manus agent described exactly this: having the agent keep rewriting a todo file, pushing the global plan into the model's recent attention. A todo list isn't just bookkeeping; it's an attention-steering device.

**Where the plan lives is a caching decision.** Module 1 Rule 3: dynamic content at the top of the prompt is cache poison. If you store the plan in the system prompt and *edit it in place* every iteration, you invalidate the cache for the entire transcript every iteration — maximum cost. Instead, append plan updates at the end (as tool results from a `update_todos` tool, or as short user-role notes), or store the plan in a file the agent re-reads. The prefix stays stable; the recitation lands at the tail where it's most useful. One design, two wins.

### Upfront vs interleaved: when to lean which way

```
Lean UPFRONT (plan heavily, execute lightly)     Lean INTERLEAVED (plan lightly, adapt)
──────────────────────────────────────────────────────────────────────────────────────
Environment is predictable & well-specified       Environment is unknown or surprising
Actions are irreversible / costly (approve plan)  Actions are cheap and reversible
Many steps are independent (parallelize)          Each step depends on the last result
Task is long; drift risk is high                  Task is short; overhead not worth it
You need a cost estimate before starting          Exploration IS the task (research, debug)
──────────────────────────────────────────────────────────────────────────────────────
```

## 3.5 Subagents and Delegation

### What a subagent actually is

Mechanically, a subagent is boring: **a tool whose implementation is another agent loop.**

```python
def delegate(brief: str, tools: list, model: str, max_steps: int) -> str:
    # A brand-new loop with a brand-new, EMPTY context.
    result = run_agent(task=brief, tools=tools, model=model, max_steps=max_steps)
    return result.report          # only this comes back to the orchestrator
```

The orchestrator calls `delegate(...)` like any tool. The subagent starts with a fresh context containing only its system prompt and the brief, runs its own observe → decide → act loop, and returns a condensed report. The orchestrator never sees the subagent's trajectory — only the result.

### Why delegate: four real benefits

1. **Context isolation.** The subagent reads 50,000 tokens of search results and source files; the orchestrator receives an 800-token summary. The orchestrator's context stays small and relevant, which improves its decisions *and* breaks the quadratic re-read cost (worked example below).
2. **Parallelism.** Independent subtasks run simultaneously. Wall-clock time becomes the *slowest* subagent, not the *sum* of all of them.
3. **Specialization.** Each subagent can have its own system prompt, its own narrow toolset, and its own model — a cheap fast model for grunt work, the expensive one for orchestration.
4. **Blast radius.** A read-only research subagent can't delete anything, no matter what it gets confused about. Permissions become per-role.

Anthropic's write-up of its multi-agent Research feature puts numbers on the upside: a lead agent on Claude Opus 4 directing Sonnet 4 subagents beat a single Opus 4 agent by 90.2% on their internal research eval, with the gains concentrated on breadth-first queries — the example being finding the board members of every IT company in the S&P 500, where parallel subagents succeeded and a single agent's sequential searches failed.

### Why not to delegate: the costs

1. **Tokens multiply.** Every subagent pays its own system prompt, its own brief, its own loop. The same Anthropic write-up reports agents using about 4x the tokens of chat, and multi-agent systems about 15x — and notes that token usage alone explained 80% of the performance variance on the BrowseComp browsing benchmark. Multi-agent systems largely *work because they spend more tokens*. That is only a good trade when the task is worth it.
2. **Lost context.** The subagent knows only what's in its brief. Everything the orchestrator learned in 20 iterations — the user's preferences, the dead ends already tried, the constraints discovered — is invisible to it unless written into the brief. **The brief is the subagent's entire world.**
3. **Conflicting decisions.** Actions carry implicit decisions. Two subagents building parts of one artifact will each make dozens of small unstated choices (naming, style, interfaces, assumptions) — and those choices won't match. Cognition's essay "Don't Build Multi-Agents" makes this its core argument: split a game-building task between two subagents and you can get a background from one art style and a character from another, each fine alone, broken together.
4. **Lossy compression, compounding errors.** A summary drops details. If the orchestrator later needs a detail the subagent discarded, it's gone (or must be re-fetched). A subagent's mistake arrives at the orchestrator as a confident-sounding fact.

### The rule of thumb

```
DELEGATE when subtasks are:                KEEP SINGLE-THREADED when work is:
─────────────────────────────────────────────────────────────────────────────
Independent of each other                  Tightly coupled (edits to one codebase,
                                             one document, one design)
Read-heavy (search, scan, investigate)     Write-heavy with shared decisions
Breadth-first (many directions at once)    Depth-first (each step builds on the last)
Summarizable (result << working tokens)    Detail-critical (summary would lose it)
Valuable enough to pay ~several x tokens   Cheap / high-volume
─────────────────────────────────────────────────────────────────────────────
```

A common, robust middle path: **delegate reading, keep writing.** Subagents investigate ("find every call site of `charge()` and report file, line, and the arguments passed"); one agent, with the full picture, makes all the edits.

### Writing the brief

A vague brief is the #1 cause of subagent failure: "Research the competitor landscape" produces duplicated, shallow, or off-target work. A good brief states:

```
Objective       What question, exactly, and why it matters to the parent task.
Scope           What's in, what's out. What other subagents are covering (avoid duplication).
Context         Relevant facts the parent already knows; dead ends already tried.
Tools & limits  Which tools, max steps, max tokens/cost.
Output format   Exact structure of the report (and length cap!) the parent will parse.
Done means      The stopping condition — when is "enough" enough?
```

And cap **recursion depth**: a subagent that can spawn subagents that can spawn subagents is a fork bomb with an API bill. Usually depth 1 (orchestrator → workers) is right.

### Worked example: what isolation does to the bill

Research task: 10 searches, then write a report. Search results are 5,000 tokens each. Prices irrelevant for now — count input tokens.

```
SINGLE AGENT
  Base context (system + tools + task): 6,500 tokens.
  Iterations 1–10: each does a search; adds 200 (call) + 5,000 (result) = 5,200.
    Input at iteration k = 6,500 + (k−1)×5,200
    Σ(k=1..10) = 10×6,500 + 5,200×45                       = 299,000
  Iterations 11–15: writing; each adds ~700.
    Input = 6,500 + 52,000 + (k−11)×700
    Σ = 5×58,500 + 700×10                                  = 299,500
  TOTAL input ≈ 598,500 tokens.   Final context ≈ 61,000 tokens (mostly raw search dumps).

ORCHESTRATOR + 10 SUBAGENTS (one search each)
  Orchestrator iteration 1: 6,500 → spawns 10 subagents (briefs: 1,500 tokens total).
  Each subagent: base 3,000 → 1 search (+5,200) → writes 800-token report.
    Subagent input = 3,000 + 8,200                         = 11,200  × 10 = 112,000
  Orchestrator iterations 2–6: base 6,500 + 1,500 + 8,000 reports = 16,000, +700/iter
    Σ = 5×16,000 + 700×10                                  = 87,000
  Orchestrator TOTAL = 6,500 + 87,000                      = 93,500
  SYSTEM TOTAL input ≈ 205,500 tokens.   Orchestrator final context ≈ 19,500.
```

Two lessons, and you need both:

1. **Isolation breaks the quadratic.** Doing the *same work*, the delegated version reads ~2.9x fewer input tokens, and the orchestrator decides with a 19.5K context instead of 61K of raw dumps.
2. **But nobody delegates to do the same work.** In practice you give each subagent room to do 3–5 searches, follow leads, and verify. Rerun the numbers with 4 searches per subagent and the system total climbs well past the single agent — that's how "15x chat" happens. **Delegation lowers the cost per unit of work, and then you buy much more work.** Whether that's a good deal depends entirely on what the extra thoroughness is worth.

Wall-clock, meanwhile, goes from 15 sequential iterations to roughly 1 + (slowest subagent's 2) + 5 = 8.

## 3.6 Summary: The Rules

1. **Pure ReAct is greedy.** Long tasks need structure against myopia, drift, and context pollution.
2. **Upfront plans are cheap, parallelizable, and approvable — and brittle,** because they're written before any observations.
3. **Interleaved planning adapts but can't forecast** and drifts on long runs.
4. **Ship the hybrid:** plan, execute interleaved, replan on surprise. The plan is a living todo list.
5. **Recite the plan at the tail of the context** to steer attention — and never edit it in place near the top (cache poison).
6. **A subagent is a tool implemented as another agent loop,** with a fresh context and a condensed return value.
7. **Delegate independent, read-heavy, breadth-first, summarizable work.** Keep coupled, write-heavy work single-threaded. Default: delegate reading, keep writing.
8. **The brief is the subagent's entire world.** Objective, scope, context, limits, output format, done-condition.
9. **Isolation breaks the quadratic per unit of work; multi-agent systems still cost more because they buy more work.** Delegate when the task's value justifies ~several-x tokens.
10. **Cap recursion depth.**

## 3.7 Drill 3

Rules: numbers and mechanisms. "It depends" without saying on what, quantitatively, is a zero.

**Q1. Explain the trade.** In ≥200 words, explain why an upfront plan and Module 1's answer-first JSON schema fail for the same underlying reason, and why interleaved planning and reasoning-first field order succeed for the same reason. Then describe one situation where you'd *deliberately* accept an upfront plan's brittleness, and name the benefit you're buying.

**Q2. The cache-hostile plan.** A team stores the agent's plan in the system prompt as `CURRENT PLAN: ...` and rewrites it after every iteration. Transcript at iteration k is ~8,000 + 2,500·k tokens; 30 iterations; $3/M input, $0.30/M cached read, $3.75/M cache write.
(a) What fraction of each request hits the prefix cache, and why?
(b) Estimate total input cost over 30 iterations under this design.
(c) Redesign so the plan is recited at the tail. Estimate the new total (assume everything except each iteration's new ~2,500 tokens hits the cache; new tokens are written).
(d) Which design gives the model *better* goal salience, independent of cost? Explain using where the plan sits in the context.

**Q3. Delegate or not?** For each task, decide: single agent, orchestrator + subagents, or hybrid (say exactly which part is delegated). Justify against the §3.5 table with at least two named criteria each.
(a) "Compare pricing pages of 25 competitors and produce a table."
(b) "Refactor our payment module from callbacks to async/await."
(c) "Find why p99 latency regressed last Tuesday" (logs, metrics dashboards, deploy history, code diff).
(d) "Answer this customer's billing question," 40,000 times a day.

**Q4. Rerun the isolation math.** Using §3.5's worked example, redo the ORCHESTRATOR + SUBAGENTS total with each subagent doing **4** searches (each +5,200 to its own context) before writing its 800-token report. (a) New system total input tokens? (b) Ratio vs the single agent's 598,500? (c) The single agent's quality on this task is 55%; multi-agent's is 85%. At $3/M input only, compute input cost per *successful* task for each. Which wins, and by how much?

**Q5. Write the brief.** The orchestrator is investigating a production incident and has learned: the error started at 14:02 UTC, only EU users are affected, a deploy went out at 13:55, and the database was already ruled out. Write the full brief for a subagent that will examine the 13:55 deploy's diff. Include all six elements from §3.5, with concrete limits and an exact output format. Then name two things a *bad* brief would omit, and the specific wrong behavior each omission would cause.

**Q6. Reading.** Read Anthropic's "How we built our multi-agent research system" (June 2025) and Cognition's "Don't Build Multi-Agents" (June 2025). They appear to disagree. Answer: (a) State each essay's core claim in one sentence. (b) Identify the property of the *task* that reconciles them — when is each right? (c) List three concrete practices from the Anthropic post for making subagents effective, and map each to an element of the brief in §3.5.

---

# Lesson 4: The Bill — Cost and Latency Compounding Per Iteration

## 4.1 Why This Lesson Exists

The prototype cost $0.03 a task in the demo. In production it averages $1.80, the p95 is $9, and one customer's task cost $64. Latency: the demo answered in 20 seconds; production p50 is three and a half minutes. Nobody changed the model or the prompt.

What changed is the **number of iterations** — real tasks are messier than demo tasks — and neither cost nor latency scales linearly with iterations. Module 1 §2.5 showed you the conversation cost spiral: cumulative input grows quadratically with turns. An agent loop is that same spiral with a turbocharger: observations are thousands of tokens instead of a human's fifty-word message, and iterations happen at machine speed instead of human typing speed. This lesson gives you the formulas, the worked numbers, and the levers, in the order you should pull them.

## 4.2 The Agent Cost Formula

Define, per run:

```
S  = stable prefix: system prompt + tool definitions             (e.g., 8,000)
U  = the task statement                                          (e.g., 500)
a  = model output per iteration: thought + tool call             (e.g., 300)
o  = observation per iteration: tool result                      (e.g., 2,000)
N  = number of iterations
```

At iteration k, the model reads everything so far:

```
input_k  = S + U + (k − 1)·(a + o)
output_k = a

Total input  = Σ(k=1..N) input_k = N·(S + U) + (a + o) · N(N−1)/2
Total output = N · a
```

There it is: the **N(N−1)/2** term. Every iteration's output and observation are re-read by every later iteration. Double the iterations and that term roughly quadruples.

### Worked example (do this once by hand)

```
S + U = 8,500   a = 300   o = 2,000   N = 30
Prices: $3/M input, $15/M output, $3.75/M cache write, $0.30/M cache read.

Total input  = 30 × 8,500 + 2,300 × (30 × 29 / 2)
             = 255,000   + 2,300 × 435
             = 255,000   + 1,000,500          = 1,255,500 tokens
Total output = 30 × 300                       =     9,000 tokens

UNCACHED
  Input:   1,255,500 × $3/M   = $3.77
  Output:      9,000 × $15/M  = $0.14
  Total ≈ $3.90.   Output is ~3.5% of the bill. Input re-reads ARE the bill.

WITH PREFIX CACHING (append-only transcript, iterations well within the TTL)
  Each iteration, roughly only the newest (a + o) is fresh; the rest is a cache hit.
  Fresh (written) ≈ 8,500 + 29 × 2,300 = 75,200    × $3.75/M = $0.28
  Cached reads    ≈ 1,255,500 − 75,200 = 1,180,300 × $0.30/M = $0.35
  Output                                                     = $0.14
  Total ≈ $0.77.   ~5x cheaper. Same agent, one structural property.

DOUBLE THE ITERATIONS (N = 60), uncached
  Input = 60 × 8,500 + 2,300 × 1,770 = 510,000 + 4,071,000 = 4,581,000 → $13.74
  2x the steps, ~3.5x the cost.
```

Three things to take from this:

1. **In agent loops, input dominates — overwhelmingly.** Module 1 told you output tokens cost 5x input. Irrelevant here: re-read input outnumbers output ~140:1 in this example. Optimize input first.
2. **Agents are the best case for prefix caching, and the worst case for cache sabotage.** The transcript is naturally append-only, and iterations fire seconds apart, so the cache TTL stays warm on its own. A single volatile token near the top — a timestamp, a step counter in the system prompt, an in-place-edited plan (§3.4), tool definitions serialized in random order — converts the $0.77 run back into the $3.90 run. Every iteration.
3. **The observation size `o` is multiplied by the quadratic term.** It is usually your biggest lever after caching. Truncating `o` from 2,000 to 500 in this example cuts uncached input from 1,255,500 to 255,000 + 800 × 435 = 603,000 tokens — more than 2x — before touching anything else.

### Compaction: resetting the quadratic

When the transcript gets long, **compaction** replaces old iterations with a summary: "Steps 1–25: explored the auth module, found the token-refresh bug in refresh.py:88, fixed it, 3 tests still failing in test_session.py (details: ...)." The context drops from, say, 70K to 10K, and the quadratic term restarts from the smaller base.

Costs to account for honestly: the summarization call itself; information loss (anything the summary omits is gone — the same lossy-compression risk as a subagent's report); and one cache rebuild, since everything after the compaction point is a new prefix. Do it in the append-only style Module 1 recommended: keep the system prompt and tool definitions untouched at the front, replace the *old middle*, continue appending. A lighter variant: **clear old tool results** — once a file's contents have been acted on, replace the 5,000-token result with "[read src/auth.py — 412 lines — contents cleared]" and let the agent re-read if it needs to.

## 4.3 Latency Compounds Too

Each iteration is a full round trip, and iterations are **sequential** — the decide of step k+1 cannot start until the act of step k returns. Per iteration:

```
t_k = TTFT_k                       (queue + prefill of the uncached tail;
                                    small when the cache is warm)
    + reasoning_tokens_k × TPOT    (hidden thinking, if any)
    + a_k × TPOT                   (visible thought + tool call)
    + tool_time_k                  (the actual action: API call, test run...)
    + harness overhead

total_latency = Σ t_k
```

Worked:

```
TTFT (warm cache) 0.6 s   a = 300 tokens   TPOT = 20 ms   tool = 1.2 s   N = 30
  per iteration = 0.6 + 300 × 0.020 + 1.2 = 0.6 + 6.0 + 1.2 = 7.8 s
  total         = 30 × 7.8 = 234 s ≈ 3.9 minutes

Add 1,000 reasoning tokens per iteration:  + 1,000 × 0.020 = +20 s per iteration
  per iteration = 27.8 s → total = 834 s ≈ 14 minutes
```

Notice where the time goes: **output tokens dominate latency** (the reverse of cost, where input dominates). Model output per step is the latency lever; context size is the cost lever. And look at what a uniform thinking budget did: +10 minutes. §2.3's advice — spend thinking on hard steps, not all steps — is a latency rule as much as a quality rule.

### The latency levers

1. **Parallel tool calls.** Most APIs let the model request several tool calls in one response. Five independent file reads as five iterations = five decide round trips and five transcript re-reads. As one iteration with five calls = one round trip. This cuts N, which cuts *both* the latency sum and the quadratic cost term. Encourage it explicitly in the system prompt when the tools are safe to run concurrently.
2. **Terser steps.** Fewer output tokens per iteration: short thoughts, no restating the observation back, compact tool arguments.
3. **Smaller thinking budgets on mechanical steps.**
4. **Faster models for easy steps** (routing), or for subagents doing grunt work.
5. **Subagent fan-out** for independent work: wall-clock becomes the slowest branch, not the sum (§3.5).
6. **Faster tools.** Tool time is often underestimated; a 20-second test suite run 15 times is 5 minutes of pure waiting. Run the targeted test file during iteration, the full suite once at the end.
7. **Stream progress.** As in Module 1, streaming fixes perceived latency, not physics — show the plan, the current step, and tick-offs so a 4-minute run feels like progress instead of a hang.

## 4.4 Reliability Compounds (the Multiplication Nobody Does)

If a task requires N steps and each step independently goes right with probability p, the chance every step goes right is **p^N**:

```
p = 0.99, N = 10  → 0.904
p = 0.99, N = 50  → 0.605
p = 0.98, N = 30  → 0.545
p = 0.95, N = 20  → 0.358
p = 0.95, N = 50  → 0.077
```

A "98% accurate" agent fails nearly half of 30-step tasks. This is why long-horizon agents are hard, and why demos (short tasks) mislead.

Real agents do better than p^N because **errors are observations** (Lesson 1): a failed step produces an error, the model reads it, and it tries something else. Recovery converts a fatal error into *extra iterations*. So reliability and cost are coupled: every recovered error adds iterations, and every added iteration costs more than the last (§4.2). The engineering levers that improve p per step — clear tool descriptions, decision-ready observations, verification, good plans — reduce both failures *and* cost.

That coupling is why the only honest agent cost metric is:

```
cost per successful task = total spend ÷ number of successful tasks
```

A configuration that costs $0.50 per run at 50% success costs $1.00 per success. One that costs $0.80 at 90% costs $0.89 per success. The "more expensive" agent is cheaper. Per-run cost alone will steer you wrong every time.

## 4.5 The Agent Budget Checklist

Every lever from this module, in roughly the order to reach for them:

```
Lever                        Cuts                         Watch out for
─────────────────────────────────────────────────────────────────────────────────────
Stable, append-only prefix   input cost ~5–10x            any volatile byte near the top
  (prefix caching)
Bound observations (o)       quadratic term, directly     truncating what the model needs;
                                                          always say "[truncated]"
Parallel tool calls          N → cost AND latency         only for independent, safe tools
Hard caps (steps/tokens/$)   tail-risk runs               set from data (§1.5), not vibes
Loop / no-progress detection wasted iterations           false positives on legit retries
Terse steps; right-sized     output tokens → latency      under-thinking hard decisions
  thinking per step
Compaction / clear old       context size, quadratic      information loss; one cache rebuild
  tool results                 restart
Model routing                per-token price              weak models failing → more iters
Subagents for isolation      orchestrator context,        they buy more work; total may rise
                               wall-clock
Workflow instead of agent    everything                   only when the path is knowable
─────────────────────────────────────────────────────────────────────────────────────
```

And instrument these per run, or you're debugging by superstition: iterations, input/output/cached tokens *per iteration*, cache hit rate, tool time, termination reason, success/failure — aggregated as distributions (p50/p95/p99), and rolled up into cost per successful task.

## 4.6 Summary: The Rules

1. **Agent input cost = N·(S+U) + (a+o)·N(N−1)/2.** It's quadratic in iterations. Double N ≈ quadruple the re-read term.
2. **Input dominates agent cost; output dominates agent latency.** Different levers for each.
3. **Agents are the best case for prefix caching** (append-only, warm TTL) and the most expensive place to break it. Nothing volatile at the top — including your plan.
4. **Observation size multiplies the quadratic term.** Bound every tool result.
5. **Compaction and clearing old results reset the quadratic,** at the price of information loss and a cache rebuild.
6. **Latency = Σ per-iteration round trips,** sequential by nature. Cut N with parallel tool calls; cut per-step output; right-size thinking.
7. **Reliability compounds as p^N.** Recovery converts failures into extra (increasingly expensive) iterations.
8. **Measure cost per successful task,** never cost per run.

## 4.7 Drill 4

Rules: every answer needs arithmetic, units, and stated assumptions. Round freely. A formula you can't evaluate is a zero.

**Q1. Derive it.** Starting from Module 1's statelessness, derive the total-input formula of §4.2 from scratch, including the N(N−1)/2 term. Then derive the version where each iteration makes m parallel tool calls instead of one, with the same total number of tool calls T (so N = T/m). Show that cost falls faster than linearly in m, and state which terms don't change.

**Q2. Price a run.** A coding agent: S = 12,000 (big tool definitions), U = 800, a = 400, o = 3,500, N = 45. Prices: $3/M input, $15/M output, $3.75/M cache write, $0.30/M cache read.
(a) Total input and output tokens.
(b) Cost uncached.
(c) Cost with prefix caching (same assumption as §4.2).
(d) The team caps observations at 1,200 tokens. New cached cost?
(e) The team also adds a timestamp to line 1 of the system prompt. New cost? Explain in one sentence why this single line undoes (c).

**Q3. Latency budget.** Product requirement: p50 task completion under 90 seconds. Measured: TTFT (warm) 0.5 s, TPOT 25 ms, a = 250 visible tokens, tool time 1.5 s, typical N = 18. Reasoning currently 600 tokens per step.
(a) Current p50 latency estimate. Does it meet 90 s?
(b) 8 of the 18 steps are independent file reads that could be batched into 2 iterations of 4 parallel calls. New N and latency?
(c) Thinking is only needed on ~4 of the remaining steps. Set thinking to 600 tokens on those and 0 elsewhere. New latency? Does it meet the bar now?
(d) What did (b) do to the *cost* quadratic term? Compute the ratio of N(N−1)/2 before and after.

**Q4. Cost per success.** Two configurations on the same 1,000-task benchmark:

```
Config A: small model, no thinking, cap 20 steps.
          $0.35/run average, 48% success.
Config B: large model, interleaved thinking, cap 40 steps, verification on finish.
          $1.10/run average, 91% success.
```

(a) Cost per successful task for each.
(b) Failed tasks in production are escalated to a human at $12 each. Total cost per 1,000 tasks for each config, including escalations.
(c) Name two mechanisms from this module that plausibly explain *why* B's per-step reliability p is higher, and use p^N to argue why a small per-step improvement produced a large success-rate gap.

**Q5. The $64 run.** A single run cost $64. Its logs show N = 180 iterations, S + U = 9,000, a ≈ 350, o ≈ 4,000, prefix cache hit rate 3%. (a) Verify the order of magnitude of the bill from the formula (state which prices you assume). (b) Name the two defects the 3% hit rate and the N = 180 each indicate, and the Lesson 1 and Lesson 4 mechanisms that would have prevented each. (c) With those fixes — cap at 40, hit rate 90% — what would the run have cost at most?

**Q6. Reading.** Read Anthropic's "Effective context engineering for AI agents" and "Context Engineering for AI Agents: Lessons from Building Manus" (Manus team blog, 2025). Answer: (a) What does Manus say about KV-cache hit rate as a production metric, and which three practices does it recommend to protect it? Map each to a Module 1 invalidation rule. (b) What do both sources recommend doing with old tool results, and what's the risk? (c) Explain Manus's argument for keeping failed actions *in* the context rather than cleaning them out — and reconcile it with this lesson's advice to bound context size.

---

## Module 4 Master Rules

### The loop

- An agent is a loop around an LLM call: observe → decide → act → repeat. Only *decide* runs the model; the harness does everything else.
- The model proposes, the harness acts. Validation, permissions, and limits live in the act phase, in plain code.
- The model knows only its context. Errors are observations; observations are bounded and decision-ready.
- If you can draw the flowchart, build a workflow, not an agent.

### Termination

- Four endings: natural completion (prefer an explicit `finish` tool), hard caps, external verification, interrupts.
- Never trust "done" when you can check. Failed verification goes back in as an observation.
- Fingerprint actions to detect loops; track a progress metric to detect stalls.
- Caps come from the distribution of successful runs, a bit above p99. Cap hits return a structured `incomplete`, via a tools-disabled wrap-up call.

### Reasoning

- ReAct: thought → action → observation, interleaved. Thoughts condition actions, add compute, and persist as memory.
- Text-protocol ReAct needs a stop sequence on "Observation:".
- Explicit CoT = visible output text that's re-read every later iteration. Reasoning tokens = trained, billed as output, provider-specific visibility and persistence.
- Prefer interleaved thinking for agents. Don't pay twice. Spend thinking on hard steps only.
- Reasoning text is not faithful and is never a safety control.

### Planning & delegation

- Upfront plans are cheap and approvable but written blind; interleaved planning adapts but drifts. Ship plan → execute → replan.
- Recite the plan at the tail; never edit it in place near the top.
- A subagent is a tool implemented as an agent loop with a fresh context. The brief is its entire world.
- Delegate independent, read-heavy, breadth-first work; keep coupled writes single-threaded. Isolation lowers cost per unit of work, but multi-agent systems buy more work.

### Cost & latency

- Input = N·(S+U) + (a+o)·N(N−1)/2. Quadratic in iterations; `o` multiplies the quadratic term.
- Input dominates cost; output dominates latency.
- Cache the append-only transcript; nothing volatile at the top.
- Parallel tool calls cut N — and therefore both latency and the quadratic.
- Reliability compounds as p^N. Measure cost per successful task.

### Success criteria

You've passed Module 4 when you can, unassisted:

- Write a production agent loop with all four termination families, loop detection, bounded observations, and a structured incomplete path — from memory.
- Read a failed trajectory and name the failure mode (runaway, premature termination, oscillation, overflow, drift) and the exact missing mechanism.
- Choose between prompted CoT and reasoning tokens (and a thinking budget) for a new agent, and price the difference over N iterations.
- Decide upfront vs interleaved planning and single vs multi-agent for a task, citing specific properties of the task.
- Compute a run's token bill, cached and uncached, and its wall-clock time from S, U, a, o, N — and cut both ≥3x on paper with named levers.

---

## Extra Reading

Ordered roughly easiest → deepest within each group. The starred (★) six are the core; the rest are depth.

### Foundations (what an agent is)

1. ★ **Anthropic — "Building Effective Agents"** (Dec 2024). Workflows vs agents, the five workflow patterns, and when *not* to build an agent. Required for Drill 1 Q6.
2. ★ **Lilian Weng — "LLM Powered Autonomous Agents"** (lil'log, 2023). The canonical map: planning, memory, tool use. Required for Drill 1 Q6.
3. **Chip Huyen — "Agents"** (blog, 2025), and the agents chapter of *AI Engineering*. A practitioner's consolidation of tools, planning, and failure modes.
4. **Anthropic — "Writing effective tools for agents"** (2025). Observation design from the tool-builder's side: concise, decision-ready, token-efficient tool outputs.
5. **Yang et al. — "SWE-agent: Agent-Computer Interfaces Enable Automated Software Engineering"** (2024). Evidence that the *interface* (tool and observation design) changes agent success as much as the model does.

### Reasoning in the loop (Lesson 2 depth)

6. ★ **Yao et al. — "ReAct: Synergizing Reasoning and Acting in Language Models"** (2022). The pattern's origin. Required for Drill 2 Q6.
7. **Wei et al. — "Chain-of-Thought Prompting Elicits Reasoning in Large Language Models"** (2022), and **Kojima et al. — "Large Language Models are Zero-Shot Reasoners"** (2022, "let's think step by step"). Where prompted CoT began.
8. **Shinn et al. — "Reflexion: Language Agents with Verbal Reinforcement Learning"** (2023). Agents that write lessons from failed attempts back into context — a natural next step after ReAct.
9. **Turpin et al. — "Language Models Don't Always Say What They Think"** (2023). The faithfulness problem; read before trusting any reasoning trace.
10. **Your provider's docs on extended thinking / reasoning models, especially the tool-use sections.** The authoritative, current rules for budgets, visibility, interleaving, and what must be passed back. Reread when they change.

### Planning (Lesson 3 depth)

11. **Wang et al. — "Plan-and-Solve Prompting"** (2023). Minimal upfront planning.
12. **Xu et al. — "ReWOO: Decoupling Reasoning from Observations for Efficient Augmented Language Models"** (2023). Plan the whole tool chain with placeholders; far fewer LLM calls.
13. **Kim et al. — "An LLM Compiler for Parallel Function Calling"** (2023). Plans as dependency graphs; parallel tool execution.
14. **Yao et al. — "Tree of Thoughts"** (2023). Search over plans instead of committing to one; useful for seeing the planning design space.

### Delegation & context (Lessons 3–4 depth)

15. ★ **Anthropic — "How we built our multi-agent research system"** (June 2025). Orchestrator-worker in production, with the token economics. Required for Drill 3 Q6.
16. ★ **Cognition (Walden Yan) — "Don't Build Multi-Agents"** (June 2025). The case for single-threaded agents and full context sharing. Required for Drill 3 Q6; read it right after #15.
17. ★ **Manus team — "Context Engineering for AI Agents: Lessons from Building Manus"** (2025). KV-cache hit rate as the key production metric, recitation via todo lists, keeping errors in context. Required for Drill 4 Q6.
18. **Anthropic — "Effective context engineering for AI agents"** (2025). Compaction, clearing tool results, note-taking, and subagents as context-management tools.

### How to use this list

After Lesson 1: #1, #2, #4. After Lesson 2: #6, then #10 for your provider. After Lesson 3: #15 and #16 back to back. After Lesson 4: #17, then #18. Everything else is for when a drill answer of yours gets torn apart and you need to know *exactly* why.

---

*Module 4 complete. Module 5 (suggested): evaluating and debugging agents — trajectory-level evals, graders for multi-step tasks, tracing and observability, and regression suites for non-deterministic loops.*
