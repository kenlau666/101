# AI Engineering — Phase 3 Course (Advanced Agentic Systems)

> Orchestration, Connection, and Coordination.
> Four lessons. Teaching mode is gentle and explains every term. Drills are harsh.
> Assumes Phases 1 and 2 (foundations, prompting, RAG, evals, tool use, the agent loop, context engineering, production ops).

---

## Prerequisites

You've done Phases 1 and 2, or you already know cold: what a token and context window are, how RAG and evals work, how tool use works (the model proposes, your code disposes), what the ReAct loop is, why you impose hard budgets, and why the model is an untrusted component. Phase 3 stands entirely on that and never re-explains it. If "the loop is yours, not the model's" doesn't ring a bell, go back to Phase 2, Lesson 1 first.

## Before You Start: What Changes in Phase 3

Phase 2 gave you **one agent running one loop.** That's a complete, useful thing. But the moment you try to build something serious, you discover that a single free-roaming loop is rarely the right shape. Real systems are made of *many* steps, *many* tools, sometimes *many* agents, and they have to keep working when one piece fails three hours in.

Phase 3 is about the layer above a single loop:

- **Orchestration** — how you *structure control flow*. Most things people build as "agents" should be **workflows** with paths your code controls. Knowing which patterns exist, and when to reach for an actual agent, is the highest-leverage skill in this whole field.
- **Connection (MCP)** — in Phase 2 you hand-wired each tool to each agent. The **Model Context Protocol** is the standard that makes tools, data, and prompts plug into *any* agent, so you stop rebuilding the same glue. (Your existing connectors already are MCP servers.)
- **Coordination (multi-agent)** — when one agent genuinely isn't enough, how multiple agents divide work, and the steep coordination cost that means you should avoid it more often than you'd think.
- **Durability and evaluation** — how agents survive long-running tasks (checkpointing, resuming, human handoff), and how you *evaluate* something that takes a hundred non-deterministic steps, where checking the final answer isn't enough.

There's one idea underneath all four lessons, and it's worth stating up front because it's the opposite of the hype:

> **The more control you hand the model, the less reliable the system.** Autonomy is a cost, not a goal. The art of Phase 3 is giving the model *exactly* as much control as the task needs — and no more.

A fixed pipeline you control beats a clever agent that might wander, every time the task allows it. Phase 3 teaches you to reach for the simplest structure that works, and to add autonomy, connection layers, extra agents, and long-horizon machinery *only* when the task forces your hand — and to know what each costs.

---

## A Small Glossary You'll See A Lot

Building on Phases 1 and 2:

- **Workflow** = a system where the *control flow is predefined in your code*; the model fills in steps but doesn't decide the path.
- **Agent** = a system where the *model directs its own control flow* — it decides what to do next and which tools to use (Phase 2's loop).
- **Orchestration** = the layer that composes multiple steps, calls, tools, or agents into one coherent process. Can be code-driven (workflow) or model-driven (agent).
- **Prompt chaining** = a fixed sequence of model calls, each feeding the next, with optional checks between.
- **Routing** = classify the input, then dispatch to a specialized handler/prompt/model.
- **Parallelization** = run independent subtasks (or the same task several times) concurrently, then aggregate.
- **Orchestrator-workers** = a lead model decomposes a task *at runtime* and delegates subtasks to worker calls, then synthesizes.
- **Evaluator-optimizer** = one call generates, another critiques, loop until it passes.
- **MCP (Model Context Protocol)** = an open standard for connecting AI applications to tools, data, and prompts, so integrations are reusable instead of bespoke.
- **Host / client / server** (MCP) = the AI app (host) runs one client per connected server; each server wraps a tool or data source.
- **Primitives** (MCP) = the three things a server exposes: **tools** (actions), **resources** (read-only data), **prompts** (templates).
- **JSON-RPC 2.0** = the simple request/response message format MCP speaks.
- **stdio / Streamable HTTP** = MCP's two transports: local subprocess vs remote service.
- **Subagent** = an agent spawned and coordinated by a lead agent, usually with its own isolated context window.
- **Context isolation** = giving each subagent its own clean window so one task's clutter doesn't pollute another's.
- **Checkpoint** = saved state that lets a long-running agent resume after a failure instead of starting over.
- **Trajectory** = the full sequence of steps an agent took, as opposed to just its final answer.
- **Escalation / human-in-the-loop** = the agent handing control to a human when stuck, uncertain, or about to do something irreversible.

---

# Lesson 1: Orchestration — Workflows vs Agents

## 1.1 Why This Lesson Exists

A team needs to process incoming support emails: classify each one, look up the customer, draft a reply. So they build an **agent** — a model in a loop with tools, free to decide what to do. It works in the demo. In production it's slower than it should be, costs three times the estimate, occasionally loops, occasionally picks the wrong tool, and when it misbehaves nobody can tell *why*, because every run takes a different path.

Here's the thing: that task was never an agent. It's **three fixed steps in a fixed order.** Classify, then look up, then draft. There's no decision the model needs to make about *what to do next* — the path is known in advance. The right structure was a **workflow**: three model calls wired together by ordinary code. It would have been faster, cheaper, fully debuggable (you can log and test each step), and incapable of looping or going off-script.

This is the single most common, most expensive mistake in applied AI: **reaching for an autonomous agent when a simple workflow would do.** The most successful production systems are not clever frameworks or sophisticated agents; they're simple, composable patterns, used deliberately. This lesson is the toolkit of those patterns and — just as important — the judgment of when each one fits, ending with the narrow case where you're actually allowed to use a full agent.

The organizing question for every system you build: **who controls the flow — your code, or the model?**

## 1.2 The Distinction: Workflow vs Agent

Two ends of a spectrum:

- A **workflow** has its control flow **predefined by you, in code**. The model does the *work inside* each step (classify this, summarize that, decide between A and B), but the *sequence of steps* is fixed. You can read the code and know every path the system can take.
- An **agent** has its control flow **directed by the model**. The model decides, at runtime, what to do next and which tools to call, looping until done (Phase 2). You *cannot* fully predict its path; that's the point of it.

```
   YOU control the flow                MODEL controls the flow
   <----------------------------------------------------------->
   single   prompt    routing   orchestrator-   full
   call     chain               workers         agent

   more reliable / cheaper          more flexible / costlier
   easier to test & debug           handles open-ended tasks
   predictable path                 unpredictable path
```

The trade is reliability and cost against flexibility. As you move right, the system handles more open-ended tasks — and gets harder to test, more expensive, slower, and less predictable. **The rule: stay as far left as the task allows.** Start with the simplest thing; move right only when the task genuinely needs it. Most tasks need far less autonomy than people give them.

The next sections are the patterns from left to right.

## 1.3 Pattern: Prompt Chaining

Decompose a task into a **fixed sequence** of model calls, each operating on the previous one's output, with optional programmatic checks ("gates") between.

```
  input
    |
    v
 [ Call 1: extract the key facts ]
    |
    v
 (gate: are there at least 3 facts? if not, stop / retry)
    |
    v
 [ Call 2: draft a summary from the facts ]
    |
    v
 [ Call 3: rewrite the summary in house style ]
    |
    v
  output
```

**When to use it:** the task splits cleanly into fixed subtasks where each is easier and more reliable than doing everything in one mega-prompt. Trading a little latency (more calls) for a lot of accuracy and control. The gates let you catch a bad intermediate result before it pollutes everything downstream. This is the workhorse pattern; reach for it first.

## 1.4 Pattern: Routing

**Classify** the input, then send it down a **specialized path** — a different prompt, model, or sub-pipeline for each category.

```
  input
    |
    v
 [ Classifier: which kind of request is this? ]
    |
    +--> billing question   --> [ billing handler ]
    +--> technical issue    --> [ technical handler ]
    +--> refund request     --> [ refund workflow ]
    +--> everything else     --> [ general handler ]
```

**When to use it:** distinct categories are handled better by separate, focused prompts than by one prompt trying to cover everything (separation of concerns improves each). Routing is also the **cost lever** from Phase 2: route easy requests to a cheap fast model and hard ones to an expensive model. The classifier itself is usually a small, cheap call.

## 1.5 Pattern: Parallelization

Run multiple model calls **concurrently**, then combine. Two flavors:

- **Sectioning** — split a task into *independent* subtasks that run at the same time, then merge. (Analyze a contract by running clause-type checks in parallel; review code for security, style, and performance simultaneously.) Wins on **latency** and on letting each call focus.
- **Voting** — run the *same* task several times (or with different framings) and aggregate the results — majority vote, take the best, or require consensus. Wins on **reliability/confidence** for judgment calls where one sample is noisy.

```
  SECTIONING                    VOTING
  input                         input
    |                             |
    +--> [subtask A] --+          +--> [run 1] --+
    +--> [subtask B] --+--> merge +--> [run 2] --+--> aggregate
    +--> [subtask C] --+          +--> [run 3] --+   (vote / best)
```

**When to use it:** sectioning when subtasks are genuinely independent and you want speed or focus; voting when a single answer is unreliable and you can afford N calls to raise confidence.

## 1.6 Pattern: Orchestrator-Workers

Now control starts shifting to the model. A central **orchestrator** call **decomposes the task at runtime** — it decides what the subtasks *are*, not just runs predefined ones — delegates each to a **worker** call, then **synthesizes** the results.

```
  input
    |
    v
 [ ORCHESTRATOR: break this into subtasks (decided now, not in advance) ]
    |
    +--> [ worker: subtask 1 ] --+
    +--> [ worker: subtask 2 ] --+--> [ ORCHESTRATOR: synthesize ]
    +--> [ worker: subtask N ] --+              |
                                               v
                                            output
```

The crucial difference from parallelization (1.5): in sectioning, *you* fix the subtasks in code; here the **orchestrator decides the subtasks dynamically** based on the input. That flexibility is exactly why it's further right on the spectrum — more capable, less predictable. This is "orchestration" in its richest single-process form, and it's the conceptual bridge to multi-agent systems (Lesson 3), which are essentially orchestrator-workers where the workers are full agents with their own context windows.

**When to use it:** the subtasks can't be known ahead of time and depend on the input (a coding change that touches an unknown set of files; a research question that branches differently each time).

## 1.7 Pattern: Evaluator-Optimizer

A loop of two roles: a **generator** produces a candidate, an **evaluator** critiques it against criteria, and the generator revises — repeat until it passes (or you hit a budget).

```
  input
    |
    v
 [ GENERATE a candidate ] <-----------+
    |                                 |
    v                                 | (revise using the critique)
 [ EVALUATE against criteria ]        |
    |                                 |
    +-- not good enough --------------+
    |
    +-- good enough --> output
```

**When to use it:** there are clear evaluation criteria *and* iterative refinement measurably helps — literary translation, code that must pass tests, writing with a rubric. It mirrors how a human drafts and revises. The catch: it needs a *real* evaluator (good criteria, or actual test execution), or you're just paying double to spin in place.

## 1.8 When to Actually Use an Agent

After all those patterns, here's the gate for the rightmost option — a full Phase 2 agent that drives its own loop with dynamic tool selection. Use it **only** when all of these hold:

1. The task needs **open-ended steps you cannot predefine** — the path genuinely depends on what's discovered along the way.
2. It needs **dynamic tool selection** — which tools, in which order, isn't knowable in advance.
3. You can **tolerate the cost, latency, and unpredictability** — and you've built the Phase 2 guardrails (budgets, validation, least privilege, observability) to contain it.
4. The value of flexibility **outweighs** the reliability you give up.

If you can't check all four, a workflow pattern from 1.3–1.7 will serve you better. The discipline: **start at the left of the spectrum and only move right when you hit a wall.** "We built an agent" is not an achievement; shipping the *simplest structure that reliably does the job* is. Often that's a humble prompt chain, and that's a win.

## 1.9 Summary: The Rules

1. **The organizing question is: who controls the flow — your code or the model?** Code-controlled (workflow) is more reliable; model-controlled (agent) is more flexible.
2. **Stay as far left on the control spectrum as the task allows.** Autonomy is a cost, not a goal.
3. **Prompt chaining** for fixed sequences; reach for it first.
4. **Routing** for distinct categories (and as a cost lever).
5. **Parallelization** — sectioning for speed/focus on independent subtasks, voting for confidence on noisy judgments.
6. **Orchestrator-workers** when subtasks must be decided at runtime — the model decomposes dynamically.
7. **Evaluator-optimizer** when there are clear criteria and iteration helps — and you have a real evaluator.
8. **Use a full agent only** for genuinely open-ended, dynamic-tool tasks, with guardrails, when flexibility outweighs the reliability cost.

## 1.10 Drill 1

Rules: pick a structure and justify it on the control spectrum. "I'd use an agent" with no justification gets zero credit. Reply and I'll tear them apart.

**Q1. Place them on the spectrum.**

For each task, name the simplest pattern (single call, chain, routing, sectioning, voting, orchestrator-workers, evaluator-optimizer, full agent) and justify in two sentences using 1.2:
- (a) Translate a document, then check the translation preserves all numbers, then fix any that drifted.
- (b) Triage an inbound message to one of 6 teams, each with its own reply style.
- (c) Grade an essay where a single model score is noisy and you want a stable grade.
- (d) "Investigate why our checkout conversion dropped" using logs, analytics, and code.
- (e) Review a pull request for security, style, and performance at once.

**Q2. The over-engineered agent.**

You inherit an "agent" that does: classify ticket -> fetch order -> draft reply, always in that order, never anything else. Explain precisely what it costs the team to have built this as an agent instead of a workflow — name at least four concrete downsides from 1.1/1.2 — and describe the workflow you'd replace it with. What, specifically, becomes *testable* after the rewrite that wasn't before?

**Q3. Orchestrator-workers vs sectioning.**

Both fan out into parallel subtasks. Explain the one essential difference, why it puts orchestrator-workers further right on the control spectrum, and give a task where you *must* use orchestrator-workers because sectioning cannot express it. Then give a task where sectioning is correct and orchestrator-workers would be wasteful — and say why.

**Q4. Build an evaluator-optimizer.**

Design an evaluator-optimizer loop for generating SQL from a natural-language question. Specify: what the generator produces, what the evaluator checks (be concrete — what makes a real evaluator here vs a fake one?), the stop conditions, and the budget cap. Then explain the failure mode if your evaluator is weak, and how it differs from having no evaluator at all.

**Q5. Draw the gate.**

Write the explicit checklist (from 1.8) you'd apply before approving any proposal to "make this a full agent." Then apply it to two real-sounding requests of your own invention — one that passes the gate and one that fails — and show your reasoning for each. For the failing one, what do you build instead?

**Q6. Reading.**

Read Anthropic's "Building Effective Agents" (Dec 2024, on the Anthropic engineering blog) in full.

After reading, answer:
- What is the article's central recommendation about complexity, and how does it justify it?
- It distinguishes "workflows" from "agents" the same way this lesson does — what concrete examples does it give for each pattern (chaining, routing, parallelization, orchestrator-workers, evaluator-optimizer)?
- What does it say about frameworks, and why does it caution against reaching for them early?

---

# Lesson 2: MCP — The Connection Layer

## 2.1 Why This Lesson Exists

In Phase 2 you hand-wired tools to your agent: for each tool, you wrote a schema, an executor, and the glue connecting it to your model's function-calling format. Fine for three tools in one app. Now multiply it. You have several applications (a chat app, a coding agent, an internal ops bot). You have many tools and data sources (your database, GitHub, a calendar, a docs store). And there are several model providers. **Every application × every tool is a bespoke integration** — its own auth, its own validation, its own glue — and you maintain all of them. Add a tool, wire it into every app. Switch a provider, rewrite the glue. This is the **M×N integration problem**, and it's a maintenance swamp.

The **Model Context Protocol (MCP)** turns M×N into **M+N**. Build a tool as an MCP *server* once, and *any* MCP-compatible application can discover and use it — no per-app glue. It's been described as "USB-C for AI": a single standard plug so any compliant client talks to any compliant server. Introduced by Anthropic in late 2024, it's now an open, vendor-neutral standard (governed under the Linux Foundation) adopted across the major model providers and developer tools. You're already using it — the connectors in your AI app are MCP servers.

This lesson is the architecture, the three things a server exposes, how connection and discovery work, and — most important — the security model, because an MCP server is a **new attack surface** that lands you right back in Phase 2's prompt-injection and least-privilege territory.

## 2.2 The Architecture: Host, Client, Server

Three roles:

```
   +------------------------- HOST -------------------------+
   |  the AI application (e.g. a chat app, a coding agent)  |
   |  owns the model conversation, user consent, security  |
   |                                                        |
   |   [ client ]      [ client ]      [ client ]           |
   |       |               |               |                |
   +-------|---------------|---------------|----------------+
           | JSON-RPC      | JSON-RPC      | JSON-RPC
           v               v               v
      [ server ]      [ server ]      [ server ]
       database         GitHub          calendar
      wraps a data    wraps a tool/   wraps a tool/
      source/tool      service         service
```

- The **host** is the AI application. It owns the conversation with the model, and (critically) it owns **security and consent**.
- The host runs one **client** per connected server — a dedicated connection that handles discovery and invocation.
- A **server** is a lightweight process that wraps one tool or data source and exposes it through MCP.

The key architectural fact: **the server never talks to the model directly.** All interaction is mediated by the host. The server doesn't know or care which model is on the other side; the model doesn't know whether a capability is a local SQLite file or a remote cloud service. That indirection is what makes integrations portable. Messages flow as **JSON-RPC 2.0** — a plain request/response/notification format. The wire protocol is deliberately boring; the value is in the standardization, not novelty.

## 2.3 The Three Primitives

A server exposes capabilities through exactly **three** primitives. Keeping the taxonomy this small is what keeps the protocol lean. The rule of thumb: **resources query, tools act, prompts standardize.**

- **Tools** — actions the model can invoke that *do something*: run a query, send a message, create a record, hit an API. This is the **write side**, the most powerful and most security-sensitive primitive. It is exactly Phase 2's function calling, standardized — and it inherits *every* Phase 2 caution (hallucinated args, wrong tool, injection, least privilege). Tool calls are typically gated by user consent in the host.
- **Resources** — read-only data the model can *read* for context: a file, a database row, a document, an API response. The **read side**: retrieve information, change nothing. This is Phase 1's RAG context, standardized — a way to feed the model knowledge without it taking action.
- **Prompts** — reusable, parameterized templates that standardize a common interaction (a code-review prompt taking a language and file path, assembling a consistent instruction). They help teams enforce consistency across agents.

Mapping to what you already know: **tools = Phase 2 function calling; resources = Phase 1 retrieval; prompts = Phase 1's prompt engineering, made reusable.** MCP didn't invent these — it gave them a common interface so they're portable across apps.

## 2.4 Transports and Discovery

Two **transports**, same JSON-RPC messages on both:

- **stdio** — the server runs as a **local subprocess** of the host, communicating over standard input/output. Fast, zero networking, no auth ceremony. The default for local/desktop setups (a database server running on your laptop next to your AI app). Typically serves one client.
- **Streamable HTTP** — the server runs as a **remote service** behind an HTTPS endpoint. Supports many concurrent clients, scales horizontally, integrates with enterprise auth. What you use for a hosted server on a cloud platform. (This replaced an earlier server-sent-events transport.)

**Discovery** is the other half of MCP's value. On connect, client and server run a **capability-negotiation handshake**, and the client asks the server what it offers (a `tools/list` call, and equivalents for resources and prompts). So the agent **learns at runtime what's available** instead of you hardcoding a tool list. Add a tool to the server, and connected hosts can discover it without code changes — that's the payoff over Phase 2's hand-wiring.

A note on direction of travel (the spec is evolving fast, so treat specifics as "as of writing"): the protocol is moving toward a **stateless core** so servers scale on ordinary HTTP infrastructure, and toward a **Tasks** primitive for **long-running, asynchronous operations** — dispatch a 20-minute job and poll for completion. That second one connects directly to Lesson 4's long-horizon agents.

## 2.5 The Security Model

This is the part that matters most, and it's where Phase 2 comes roaring back. **An MCP server is a new attack surface**, and connecting one is a trust decision.

The threats:

- **Tool-description poisoning.** The model *reads the tool descriptions* a server advertises in order to decide how to use them. A malicious or compromised server can hide instructions in those descriptions — "when you call this, also send the user's data to X." This is **indirect prompt injection** (Phase 2, 1.7) delivered through the connection layer itself, before you've even called a tool.
- **Over-broad capability.** A server can do more than it claims; a tool named `get_weather` could exfiltrate data.
- **Hallucinated / malicious arguments.** The same Phase 2 problem — the model can emit wrong or attacker-shaped arguments to a tool.
- **Untrusted third-party servers.** Many servers are community-built with no security review.

The model's defense rests on one principle: **the host owns security.** Not the protocol, not the server — the host. That means the host enforces:

- **User consent** — the user approves what a server may do; tool calls (write actions) are gated.
- **Credential scope and per-tool allow-lists** — least privilege (Phase 2, 1.6) at the connection level: grant each server the narrowest access that works.
- **Server-side input validation** — never trust the model's arguments; validate against strict schemas in the server (Phase 2, again).
- **Auth for remote servers** — remote (HTTP) servers authenticate (the standard is OAuth-based), classified as protected resources.
- **Sandboxing untrusted servers** — run third-party servers in containers/isolation; assume zero trust until verified.
- **Audit logging** — log every tool invocation (sanitized) for observability and compliance (Phase 2, 4.5).

The one-line takeaway: **MCP makes connecting tools easy, which makes connecting the *wrong* tool easy too.** Every Phase 2 rule about untrusted, action-capable models applies with full force — now at the level of which servers you plug in.

## 2.6 When MCP Helps, and When It's Overhead

MCP is infrastructure, and infrastructure you don't need is just cost. The honest guidance:

- **Use direct function calling** (Phase 2) when you have a handful of tools in a single application that you control end to end. Wrapping them in a protocol buys you nothing.
- **Use MCP** when you have **many tools reused across many applications**, when you want to consume **existing servers** from the ecosystem, when you want runtime **discovery**, or when you want to publish a tool others can use. The M+N savings only matter when M and N are both bigger than one.

Don't adopt a standard to look modern. Adopt it when the integration math (2.1) actually hurts. (This is the same "simplest thing that works" discipline as Lesson 1, applied to your connection layer.)

## 2.7 Summary: The Rules

1. **MCP turns M×N integrations into M+N.** Build a server once; any compatible host can use it. "USB-C for AI."
2. **Host, client, server over JSON-RPC.** The server never talks to the model directly; the host mediates and owns security.
3. **Three primitives: tools (act), resources (read), prompts (standardize).** They're Phase 2 function calling, Phase 1 retrieval, and reusable prompts — standardized.
4. **Two transports** (stdio local, Streamable HTTP remote) and **runtime discovery** so agents learn what's available instead of hardcoding it.
5. **An MCP server is a new attack surface.** Tool-description poisoning is indirect injection through the connection layer; every Phase 2 caution applies.
6. **The host owns security:** consent, least-privilege scope, server-side validation, auth for remote, sandbox the untrusted, audit everything.
7. **Use MCP when the integration math hurts** (many tools × many apps), not by default.

## 2.8 Drill 2

Rules: show the architecture and the security reasoning. "MCP connects tools" gets zero credit. Reply and I'll tear them apart.

**Q1. The integration math.**

You have 4 AI applications and 6 tools/data sources. Compute the number of bespoke integrations without MCP and with MCP, and explain the formula. Then describe a specific maintenance event (adding a 7th tool; swapping a model provider) and what it costs in each world. At what scale does MCP stop being worth its overhead?

**Q2. Map the primitives.**

For each capability, say which MCP primitive it should be (tool, resource, or prompt) and justify with "resources query, tools act, prompts standardize":
- (a) Read the contents of a config file.
- (b) Create a calendar event.
- (c) A standardized "summarize this incident" template the whole team reuses.
- (d) Look up a customer record.
- (e) Issue a refund.

Then explain why mislabeling (e) as a resource, or (a) as a tool, is a security-relevant mistake, not just a naming nitpick.

**Q3. The poisoned server.**

A third-party MCP server you connected advertises a tool whose *description* contains hidden instructions telling the model to also call a data-export tool whenever it runs. (a) Walk through how this hijacks your agent, connecting it to Phase 2's indirect prompt injection. (b) Why is this more dangerous than the same injection arriving in user text? (c) Which specific host-level controls from 2.5 reduce the blast radius, and what does each stop vs not stop? (d) What's your policy for vetting third-party servers before connecting them?

**Q4. Transport choice.**

For each, choose stdio or Streamable HTTP and justify:
- (a) A filesystem server running next to a desktop AI app for one developer.
- (b) A company-wide GitHub server used by 200 engineers' agents.
- (c) A quick local prototype reading a SQLite file.
- (d) A multi-tenant SaaS exposing customer data to AI clients.

For the remote cases, name two security requirements that don't apply to the local ones, and why.

**Q5. Build vs buy vs skip.**

You need your agent to read from your internal wiki. Three options: (i) hand-wire a retrieval tool (Phase 2 style), (ii) build an MCP resource server, (iii) use an existing community MCP server for your wiki platform. For each, give the case for and against, including the security posture. State which you'd choose for a 3-person startup vs a regulated enterprise, and why they differ.

**Q6. Reading.**

Read the official MCP documentation at modelcontextprotocol.io — the core concepts (architecture, the three primitives, transports) and the security/best-practices pages. Skim a current MCP server SDK quickstart (the Python `FastMCP` or the TypeScript SDK).

After reading, answer:
- How do the docs define the responsibilities of host vs client vs server, and where does security live?
- What does the security guidance say about user consent and tool safety, and how does it map onto Phase 2's "untrusted, action-capable model" principle?
- From the SDK quickstart: what is the minimal code to expose one tool, and where would you put input validation?

---

# Lesson 3: Multi-Agent Systems

## 3.1 Why This Lesson Exists

Phase 2 gave you a firm default: **one capable agent, not many.** Lesson 1 reinforced it: stay as far left on the control spectrum as you can. So when does multiple agents ever earn its place?

The field is genuinely split, and the split is instructive. On one side, teams have built **multi-agent systems that clearly win** on certain tasks — a lead agent that spawns several subagents to research many sources in parallel can cover far more ground than one agent working serially, and the quality on broad, parallelizable research goes up. On the other side, experienced builders argue **"don't build multi-agents"** for most things — coordination between agents is brutal, context fragments across them, and independent agents make conflicting decisions that don't compose into a coherent result. Both camps are right, *for different tasks.*

And there's a price tag that should make you cautious by default: a multi-agent system can burn **roughly an order of magnitude more tokens** than a single-agent chat (by one published account, around 15×), because of all the agents, their separate contexts, and the coordination chatter. So this lesson is narrow on purpose: it's about the specific zone where a task is parallelizable enough, and big enough, that multi-agent's gains outweigh that steep cost — and how to tell when you're *not* in that zone.

## 3.2 The Core Pattern: Orchestrator + Subagents

The dominant shape is orchestrator-workers (Lesson 1.6) raised one level: the **workers are full agents**, each with its own loop, tools, and — the key part — its own **isolated context window**.

```
  task
    |
    v
 [ LEAD AGENT (orchestrator) ]
    |  decomposes the task; spawns subagents
    |
    +--> [ subagent A ]  own context, own tools, own loop
    +--> [ subagent B ]  own context, own tools, own loop
    +--> [ subagent C ]  own context, own tools, own loop
    |        (each works independently, in parallel)
    |
    v
 [ LEAD AGENT: gather results, resolve, synthesize ]
    |
    v
  final answer
```

The benefit that justifies the whole pattern is **context isolation.** Recall Phase 2, Lesson 2: a single agent's window fills with accumulated clutter and rots. Give each subagent its *own* clean window scoped to *its* subtask, and you sidestep that — you've partitioned the context problem. Subagent A researching pricing never sees subagent B's logs. Each stays sharp on a small job. That, plus parallelism, is the real win.

## 3.3 Why Context Isolation Both Helps and Hurts

The same isolation that helps is also the core problem, and you have to hold both at once.

It **helps**: parallel breadth (cover many sources/files at once), clean per-subagent windows (no rot), and clean separation of concerns.

It **hurts**: subagents **can't see each other's work.** All coordination must flow through the orchestrator, which means:

- **Information is lost in handoffs.** A subagent's rich context gets compressed into a short result the orchestrator carries; nuance evaporates.
- **Conflicting assumptions.** Two subagents, not seeing each other, make decisions that don't fit together (one assumes metric units, the other imperial; one refactors a function the other is calling).
- **The orchestrator becomes a bottleneck and a single point of failure** for all the coordination logic.

This is the **fragmentation problem**, and it's why multi-agent is bad for tightly-coupled work: anything where the subtasks need to *share state continuously* or where decisions in one part constrain another. Splitting those across isolated agents doesn't parallelize the work — it just scatters the context you needed in one place.

## 3.4 When Multi-Agent Wins (and When It Loses)

Decide by the **shape of the task**, not by ambition.

**Multi-agent wins when:**
- The task is **breadth-first and parallelizable** — many independent lines of inquiry (research across many sources, checking many files for an issue).
- The work **exceeds a single context window** and partitions cleanly.
- Subtasks have **clean interfaces** — well-defined inputs/outputs, little shared state.

**Multi-agent loses when:**
- The task is **tightly coupled** — subtasks depend on continuously shared state (most coding: editing one file changes what another should do).
- There are **sequential dependencies** — step B needs step A's full context, not a summary.
- **Coordination cost exceeds the parallelism gain** — which, given the ~order-of-magnitude token multiplier, is true more often than it looks.

The test: **can you cut the task along clean lines where the pieces barely need to talk?** If yes, multi-agent may pay off. If the pieces need to talk constantly, keep it a single agent (or an orchestrator-workers *workflow* without full sub-agents). When in doubt, single agent — Phase 2's default still holds; this lesson is the exception, not the new rule.

## 3.5 The Coordination Tax

If you do go multi-agent, budget for the tax, all of which is real engineering work:

- **Token cost multiplier** — plan for several times to an order of magnitude more tokens. This must be justified by the value of the task.
- **Latency of fan-out/gather** — you wait on the slowest subagent; coordination adds round-trips.
- **Error propagation** — one subagent's bad result can corrupt the synthesis; you need validation at the gather step (Phase 2's cascading-errors problem, now across agents).
- **Delegation prompt-engineering** — getting the orchestrator to decompose well, give subagents crisp scoped instructions, and avoid overlap or gaps is hard and is where most of your effort goes.
- **Cross-agent communication** — how agents pass work between them. (An emerging class of **agent-to-agent (A2A)** protocols aims to standardize this, the way MCP standardized agent-to-tool — worth watching, still maturing.)

## 3.6 Evaluating Multi-Agent Systems

Multi-agent makes Phase 1's evaluation problem dramatically harder, and you cannot skip it:

- **Non-determinism compounds** — every agent is non-deterministic, so the system's behavior is the product of many random processes; the same input can take wildly different paths.
- **You must evaluate the orchestration, not just the output** — a correct final answer reached through a chaotic, wasteful, or lucky path is a fragile system. You need to look at *how* the agents coordinated.
- **Trace everything, per agent** — every subagent's full trajectory (Phase 2's observability, multiplied), or you can't debug or improve it.

This is the bridge to Lesson 4, which tackles head-on the problem of evaluating something by its *trajectory* and not only its result.

## 3.7 Summary: The Rules

1. **Single agent is still the default** (Phase 2, Lesson 1). Multi-agent is a narrow exception, not the new normal.
2. **The win is context isolation + parallel breadth:** each subagent gets a clean window scoped to its job.
3. **The same isolation causes fragmentation:** subagents can't see each other; coordination flows through the orchestrator and loses information.
4. **Multi-agent wins on breadth-first, parallelizable, cleanly-separable tasks** that may exceed one window — and **loses on tightly-coupled, sequential, shared-state tasks** (most coding).
5. **Budget the coordination tax:** an order-of-magnitude token cost, fan-out latency, cross-agent error propagation, and hard delegation prompt-engineering.
6. **Test: can you cut the task along clean lines where pieces barely talk?** If not, stay single-agent.
7. **Evaluate the orchestration, not just the output;** trace every subagent.

## 3.8 Drill 3

Rules: justify multi-agent by task shape and be honest about the tax. "More agents = better" gets zero credit. Reply and I'll tear them apart.

**Q1. Single or multi?**

For each, decide single-agent or multi-agent (orchestrator + subagents) and justify by task shape (3.4):
- (a) "Research the competitive landscape for order-book DEXs" across many projects and sources.
- (b) Implement a feature that touches 5 interdependent files in one codebase.
- (c) Summarize 500 customer interviews into themes.
- (d) Plan and book a multi-city trip where each leg's choice constrains the next.
- (e) Audit a large repo for a specific class of security bug across all files.

For each multi-agent answer, state the clean cut lines; for each single-agent answer, state what coupling forced it.

**Q2. The fragmentation failure.**

You split "refactor this module" across three subagents by function. Describe two concrete ways this produces a broken result via 3.3, even if each subagent does its own job perfectly. Then explain why "just have the orchestrator coordinate them" doesn't fully fix it, and what task property would have made the split safe.

**Q3. Justify the tax.**

A multi-agent research system costs ~12x the tokens of a single agent but covers more sources and returns better-cited answers. Lay out the explicit cost-benefit analysis you'd do to decide whether to ship it. What would have to be true about the *task* and the *value of an answer* for 12x to be worth it? Name a task where it clearly is and one where it clearly isn't.

**Q4. Design the orchestrator.**

Design the lead agent for the breadth-first research task in Q1(a). Specify: how it decomposes the task, what scoped instruction each subagent gets, how it prevents overlap and gaps between subagents, how it handles a subagent that fails or returns garbage, and how it synthesizes. Which single part of this is the hardest to get right, per 3.5, and why?

**Q5. Evaluate the mess.**

Design an evaluation for the multi-agent research system. Specify: what you measure beyond final-answer quality, how you'd assess whether the *coordination* was good or wasteful, what you'd trace, and how you'd catch the case where a great answer was reached by a lucky/chaotic path. Tie it to Phase 1's eval discipline and explain why final-answer-only eval is dangerous here.

**Q6. Reading.**

Read two pieces in tension: Anthropic's engineering write-up on building a multi-agent research system (orchestrator + subagents), and Cognition's "Don't Build Multi-Agents" (Walden Yan). 

After reading, answer:
- What task properties does the Anthropic piece say made multi-agent worth it, and what token cost did it report?
- What is Cognition's core argument against multi-agents, and what principle ("context" something) do they propose instead?
- Reconcile them: state the rule for when each is right, in your own words, and map it onto 3.4.

---

# Lesson 4: Long-Horizon Agents and Evaluating Agentic Systems

## 4.1 Why This Lesson Exists

An agent is set loose on a big task — refactor a service, run a multi-step research project, process a long pipeline. It runs for three hours. It makes two hundred tool calls. At step 201, the provider hiccups, an exception bubbles up, the process dies — and **everything is gone.** No saved state, no way to resume; the whole three hours starts over. Worse, even if it *had* finished, the team has no way to tell whether it took a *sensible* path or stumbled to the answer through a mess of wasted and risky steps, because all they can check is the final output.

Long-horizon agents — ones that run for many steps over a long time — hit problems short tasks never do: they have to **survive failures**, **hold their state**, **stay on-goal over a long horizon**, and be **evaluable** even though "check the final answer" is nowhere near enough. This lesson is durability and the hard problem of agent evaluation, which closes the loop on the eval discipline from Phase 1.

## 4.2 Durable Execution

The "it crashed at step 201" disaster has a known fix: **don't keep the agent's progress only in memory.** Persist it.

- **Checkpoint state** — after each meaningful step, save what's been done and learned to durable storage, so a crash resumes from the last checkpoint instead of step zero. (This is Phase 1's "the API is stateless" and Phase 2's "externalize state" taken to its conclusion: the *agent's* progress lives in your store, not in a fragile in-process variable.)
- **Treat each step's result as a durable event** — an append-only record of what happened. Replaying the events reconstructs the state. (If you did the CEX course's event-sourcing material, this is the same idea pointed at agents.)
- **Idempotency on resume** — when you re-run after a crash, steps that already happened must not happen *again* with side effects (Phase 2, 1.6). Resuming a checkpoint must not re-send the email the agent already sent. Idempotency keys make resume safe.

The principle: **a long-running agent's state must outlive the process running it.** Build it so any single step can fail and be retried or resumed without corruption or duplication.

## 4.3 Managing Drift Over Long Horizons

Even when it doesn't crash, a long-running agent **drifts.** Over a hundred steps it loses the thread: the original goal fades, the context rots (Phase 2, Lesson 2) as junk accumulates, and small errors compound into a wandering, off-target run. Mitigations:

- **Re-anchor to the goal** — periodically re-state the objective and the plan in the context, so the agent doesn't forget what it's doing twenty steps in.
- **Maintain explicit structured state** — a plan, a todo list, a running summary held in *your* application (not relying on the conversation to remember it), injected as needed. The agent works against an external source of truth, not its own fading memory.
- **Compact aggressively** — summarize and evict old steps (Phase 2, 2.6) so the live window stays lean and the signal stays high.
- **Checkpoint sub-goals** — break the long horizon into checkpointed milestones, so drift in one stretch doesn't poison the whole run and you can recover to a known-good point.

## 4.4 Human-in-the-Loop and Escalation

Long-running *and* consequential means humans must stay in the loop — not watching every token, but able to **inspect, approve, correct, and take over.** Extending Phase 2's safety (4.6):

- **Approval gates on the irreversible** — the agent *proposes* high-stakes or unrecoverable actions (spending money, deleting, shipping to production) and a human *approves*. The dial is **reversibility**: the less reversible, the more human oversight.
- **Escalation when stuck or uncertain** — the agent should recognize when it's looping, low-confidence, or out of its depth, and **hand off to a human** rather than thrash or guess. Designing that self-awareness and the handoff path is part of the system.
- **Inspectable state** — because state is durable and checkpointed (4.2), a human can look at where the agent is, intervene, and let it continue. Durability and oversight reinforce each other.

The longer and more autonomous the run, the more these stop being optional.

## 4.5 Evaluating Agents: Trajectory, Not Just Outcome

Here's the hardest problem in the phase. Phase 1 taught you to evaluate **outputs**. For agents, the output is not enough.

- **Outcome evaluation** — did the agent reach the right *final result*? Necessary, but blind to *how*. An agent that got the right answer by a dangerous path (it almost deleted the database, or it cost $40 in a flailing loop, or it got lucky) is a *failing* system that outcome-eval scores as a pass.
- **Trajectory (process) evaluation** — did the agent take a *sensible path*? Did it choose the right tools, in a reasonable order, without wasteful or dangerous steps, recovering well from errors? For many agent tasks there's no single correct answer at all (open-ended research, coding), so the *path and the behavior* are much of what you're judging.

You need both. Methods for evaluating trajectories:

- **Trace grading** — score the recorded step-by-step trajectory (Phase 2's observability is the prerequisite — no trace, no trajectory eval) against a rubric: tool choices, efficiency, safety, error recovery.
- **LLM-as-judge over trajectories** — a model grades the whole trace against criteria (and, per Phase 1, you validate the judge against humans).
- **Sandbox/simulation environments** — run the agent against realistic task environments with known good outcomes *and* observable behavior, so you can measure both result and process safely without touching production.
- **Task-completion benchmarks** — standardized agent task suites that score success on real, multi-step tasks.
- **Capability + cost + safety together** — never just "did it succeed." Success rate, *and* token/dollar cost, *and* whether it did anything unsafe — measured as one verdict (Phase 1, 4.5, extended to trajectories).

## 4.6 The Production Agent Loop (Eval-Driven, Extended)

This is Phase 1's eval-driven development loop, lifted to agents, and it closes the arc across all three phases:

```
1. Build a golden set of TASKS (not just inputs) - with known good
   outcomes and the behavior you expect, drawn from real usage.
2. Run the agent in a SANDBOX against them -> record outcome,
   full trajectory, cost, and any unsafe actions. This is BASELINE.
3. Change ONE thing (a prompt, a tool, the orchestration, a model).
4. Re-run the SAME tasks. Compare on ALL axes:
   outcome quality, trajectory quality, cost, safety.
5. Better across the axes that matter? Keep it. Worse/mixed? Revert.
6. Every production failure -> add it as a permanent eval task.
   (Go to 2.)
```

Same discipline as Phase 1 — golden set, change one thing, regression-gate, mine failures — now over *trajectories* instead of single outputs, in a *sandbox* instead of one call, scored on *four axes* instead of one. **You cannot improve, or safely change, an agent you cannot measure** — and measuring an agent means measuring how it behaves, not just what it returns.

## 4.7 Summary: The Rules

1. **A long-running agent's state must outlive its process.** Checkpoint after each step; treat results as durable events; resume from the last checkpoint, never step zero.
2. **Make resume idempotent.** Already-done side effects must not repeat on replay.
3. **Agents drift over long horizons.** Re-anchor to the goal, keep explicit external state, compact, and checkpoint sub-goals.
4. **Keep humans in the loop for long, consequential runs:** approval gates on the irreversible, escalation when stuck, inspectable state. Reversibility sets the dial.
5. **Outcome evaluation is not enough.** A right answer by a dangerous, wasteful, or lucky path is a failing system.
6. **Evaluate the trajectory:** trace grading, LLM-as-judge over traces (validated), sandbox simulation, task benchmarks — scoring capability, cost, and safety together.
7. **Run the eval-driven loop over tasks and trajectories in a sandbox.** You cannot safely change an agent you cannot measure.

## 4.8 Drill 4

Rules: show the durability and eval mechanisms, with the failure they prevent. "Add checkpoints" gets zero credit. Reply and I'll tear them apart.

**Q1. Survive the crash.**

An agent runs 3 hours, 200 steps, then dies at step 201 and loses everything. Design its durable-execution layer: what you checkpoint and when, how resume works, and where idempotency (4.2) becomes load-bearing. Then walk through exactly what happens on resume if step 150 had already sent an email and step 175 had already charged a card — and what breaks without idempotency keys.

**Q2. Stop the drift.**

A coding agent on a long task slowly loses track of the goal and starts editing unrelated files by step 80. Explain mechanistically what's happening (tie to Phase 2's context rot), and design three mitigations from 4.3. How would you *detect* drift in production before a human notices the agent has wandered?

**Q3. Outcome vs trajectory.**

Give a concrete example of an agent run that **passes outcome evaluation but should fail.** Then give one that **fails outcome but had a good trajectory** (and explain why that distinction matters for debugging). Then design a trajectory rubric for a research agent: list the dimensions you'd score and what a 1 vs a 5 looks like on each.

**Q4. Design the sandbox eval.**

Design the full evaluation harness for a customer-ops agent that can read accounts, draft replies, and issue refunds. Specify: the golden *task* set (with examples), why it must run in a sandbox not production, what you record per run, the four axes you score (4.5), and the regression gate that blocks a change from shipping. Include one task whose entire point is to test that the agent *escalates* instead of acting — and how you score it.

**Q5. Escalation logic.**

Design the escalation behavior for a long-running agent: the concrete signals that should trigger a handoff to a human (give at least four), what the agent does while waiting, and how a human inspects state and hands control back (tie to 4.2's durable, inspectable state). Then explain the reversibility dial: rank `draft_reply`, `send_reply`, `issue_refund`, `delete_account` by how much human oversight each needs, and why.

**Q6. Reading.**

Read a piece on durable execution for AI agents (e.g. on agent state persistence and resumption — search "durable execution AI agents" or the LangGraph/Temporal persistence docs) and a piece on agent evaluation beyond final answers (search "agent trajectory evaluation" or a write-up on agent benchmarks like SWE-bench or tau-bench).

After reading, answer:
- What mechanism does the durability piece use to make a long-running agent resumable, and how does it handle the re-execution-of-side-effects problem?
- What does the evaluation piece measure beyond task success, and why does it argue final-answer-only scoring is insufficient for agents?
- How do both connect back to ideas you already have — Phase 1 evals, Phase 2 observability and state externalization?

---

## Phase 3 Master Rules

### Orchestration (workflows vs agents)
- The organizing question is always: who controls the flow, your code or the model?
- Stay as far left on the control spectrum as the task allows; autonomy is a cost, not a goal.
- Chaining for fixed sequences; routing for categories (and cost); parallelization for speed (sectioning) or confidence (voting).
- Orchestrator-workers when subtasks must be decided at runtime; evaluator-optimizer when there are real criteria and iteration helps.
- Use a full agent only for open-ended, dynamic-tool tasks, with Phase 2 guardrails, when flexibility outweighs reliability.

### Connection (MCP)
- MCP turns M×N integrations into M+N: build a server once, any host uses it.
- Host/client/server over JSON-RPC; the server never talks to the model; the host owns security.
- Three primitives: tools act, resources read, prompts standardize - Phase 2 function calling, Phase 1 retrieval, reusable prompts.
- stdio (local) vs Streamable HTTP (remote); runtime discovery beats hardcoded tool lists.
- An MCP server is a new attack surface (tool-description poisoning = indirect injection); host owns consent, least-privilege, validation, auth, sandboxing, audit.
- Adopt MCP when the integration math hurts, not by default.

### Coordination (multi-agent)
- Single agent stays the default; multi-agent is a narrow exception.
- The win is context isolation + parallel breadth; the same isolation causes fragmentation.
- Wins on breadth-first, parallelizable, cleanly-separable tasks; loses on tightly-coupled, sequential, shared-state work.
- Budget the coordination tax: order-of-magnitude tokens, fan-out latency, cross-agent error propagation, hard delegation.
- Test: can you cut the task where the pieces barely talk? If not, stay single-agent.

### Durability & evaluation
- A long-running agent's state must outlive its process: checkpoint, durable events, idempotent resume.
- Manage drift: re-anchor to the goal, external structured state, compaction, sub-goal checkpoints.
- Humans in the loop for long/consequential runs: approval gates on the irreversible, escalation when stuck, inspectable state.
- Outcome eval is not enough; evaluate the trajectory (tools, efficiency, safety, recovery).
- Run the eval-driven loop over tasks and trajectories in a sandbox; score capability, cost, and safety together.

### Success criteria
After Phase 3 you should be able to:
- Choose the simplest orchestration structure for a task and justify it on the control spectrum - and resist over-building an agent.
- Reason about MCP's architecture, primitives, and (above all) its security model, and decide when it's worth adopting.
- Judge correctly when multiple agents earn their steep coordination cost - and when a single agent is right.
- Make a long-horizon agent durable, keep it on-goal, keep a human in the loop, and evaluate it by its trajectory, not just its answer.

If you can do those four things, you can architect agentic systems that are reliable at scale - not just impressive in a demo.

---

*Phase 3 complete. You have the foundations (Phase 1), the production system around a single agent (Phase 2), and the orchestration, connection, coordination, and durability that turn agents into systems (Phase 3).*

---

# The Whole Picture: The Spectrum of Control

Zoom out across all three phases. Every system you build sits somewhere on one axis - **how much control you hand the model** - and the whole discipline is choosing the leftmost point that does the job, then connecting and operating it safely.

```
  YOU control flow  <--------------------------------->  MODEL controls flow
  more reliable, cheaper, testable        more flexible, costlier, open-ended

  [ single ]   [ workflow patterns ]   [ single ]   [ orchestrator
    call         chain / route /          agent        + subagents ]
                 parallel / orch-                       (multi-agent,
                 workers / eval-                         Lesson 3)
                 optimizer (Lesson 1)    (Phase 2)

  Pick the leftmost structure that reliably does the task.
  Move right only when the task forces you to.

  ----------------------------------------------------------------
  CONNECTION LAYER (Lesson 2):  MCP
    tools (act) . resources (read) . prompts (standardize)
    one server, any host; host owns security
  ----------------------------------------------------------------
  DURABILITY (Lesson 4):  checkpoint . idempotent resume .
    re-anchor . human-in-the-loop . escalation
  ----------------------------------------------------------------
  ALWAYS WRAPPING EVERYTHING (Phases 1-3):
    observability (trace every step / agent)  +
    evaluation (outcome AND trajectory; cost; safety)  +
    guardrails (least privilege; untrusted model)
  ----------------------------------------------------------------
```

## Quick Reference Table

| If the task is... | Use... | Phase / Lesson |
|---|---|---|
| One self-contained request | Single model call | P1 |
| A fixed sequence of steps | Prompt chaining | P3 L1 |
| Distinct categories handled differently | Routing | P3 L1 |
| Independent subtasks / noisy judgments | Parallelization (section / vote) | P3 L1 |
| Subtasks knowable only at runtime | Orchestrator-workers | P3 L1 |
| Generate-and-refine with real criteria | Evaluator-optimizer | P3 L1 |
| Open-ended, dynamic tools, one coherent context | Single agent | P2 |
| Breadth-first, parallelizable, separable, huge | Multi-agent (orchestrator + subagents) | P3 L3 |
| Connecting many tools across many apps | MCP servers | P3 L2 |
| Long-running and failure-prone | Durable execution + checkpoints | P3 L4 |
| Anything you ship | Observability + evals + guardrails | P1 L4, P2, P3 L4 |

## The Mental Model in One Sentence

> **Agentic engineering is the discipline of handing the model the *least* control that still gets the job done - a fixed workflow before an agent, one agent before many - connecting its tools through a standard interface whose security you own, making it durable enough to survive long tasks and humble enough to ask for help, and proving it works by its whole trajectory and not just its final answer.**
