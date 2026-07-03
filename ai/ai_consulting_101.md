# AI Consulting — Phase 1 Course (Beginner Edition)

> From Builder to Advisor.
> Four lessons. Teaching mode is gentle and explains every term. Drills are harsh.
> Assumes AI Engineering 101–103 in full: you know what a model, a token, RAG, an eval, an agent, and MCP are, cold. This course never re-explains them. What it teaches is the other half of the job: finding the client, qualifying the problem, pricing the work, and delivering it without destroying yourself or them.

---

## Prerequisites

You can build the thing. You know why hallucination is structural, why evals are the product, why the demo-to-production gap exists, and why "just add an agent" is usually the wrong answer. If any of that is fuzzy, go back to 101–103 — every lesson here weaponizes it commercially.

You do **not** need consulting experience, sales experience, or an MBA. Every business term is defined on first use.

## Before You Start: What AI Consulting Actually Is

There are two jobs people call "AI consulting." They are almost entirely different.

The first is **strategy theater**: slide decks about "AI transformation," maturity models, workshops that end with a roadmap nobody executes. This work is real and lucrative for large firms, but it is not this course, and it is dying — because clients have noticed that the deck does not ship.

The second is **applied AI consulting**, which is what this course is about: a client has a business, the business has workflows, some of those workflows contain expensive, repetitive language-and-judgment work, and you — someone who can *actually build* — are hired to find those workflows, decide which ones a model can genuinely absorb, build the system, prove it works with evals, and leave the client better off than your fee.

That sounds like the engineering job with a sales call bolted on. It is not. The engineering is maybe 40% of it. The other 60% is a different discipline:

- **Diagnosis** — the client will tell you what they want ("we need a chatbot"). What they want is almost never what they need. Your first job is to find the real problem, which is usually buried two questions deeper.
- **Qualification** — most AI project ideas should not be built. The single highest-leverage thing a consultant does is kill bad projects *before* money is spent. This is also the thing that builds trust fastest, because every other vendor is saying yes to everything.
- **Economics** — the project must make the client money (or save it) by more than it costs, including the costs the client cannot see yet: evals, monitoring, prompt maintenance, model migrations. If you cannot do this arithmetic, you are selling hope.
- **Risk transfer** — when the system fails in public, the client eats the lawsuit and the headline, not the model provider and not you (if your contract is written correctly). Managing that risk is billable value, not overhead.

Here is the uncomfortable market fact that frames everything: in 2025 MIT's Project NANDA reported that roughly **95% of enterprise GenAI pilots produced no measurable P&L impact**, and S&P Global found over 40% of companies had abandoned most of their AI initiatives. The market is not short of AI builders. It is drowning in failed pilots. What it is short of is people who can tell, *before the money is spent*, which projects will survive contact with production — and that judgment is exactly what 101–103 gave you. This course teaches you to sell it.

```
  WHAT CLIENTS THINK THEY ARE BUYING      WHAT THEY ARE ACTUALLY BUYING
  +------------------------------+        +------------------------------+
  |  "an AI"                     |        |  judgment about where AI     |
  |  a chatbot                   |        |  fits (and where it doesn't) |
  |  a demo like the one on      |  --->  |  a system with evals that    |
  |  LinkedIn                    |        |  proves it works             |
  |  magic                       |        |  someone accountable when    |
  |                              |        |  it breaks                   |
  +------------------------------+        +------------------------------+
```

## Glossary (business terms used throughout)

| Term | Meaning |
|---|---|
| **Engagement** | The whole paid relationship: one project, one contract, defined start and end (or a retainer). |
| **Scope** | The written list of what you will and will not deliver. The most important document you will ever write. |
| **Statement of Work (SOW)** | The contract appendix that defines scope, deliverables, acceptance criteria, timeline, and price. |
| **Acceptance criteria** | The objective, pre-agreed test that decides "done and payable" vs "not done." For AI work: an eval threshold. |
| **Discovery** | The paid or unpaid phase where you learn the client's business before committing to build anything. |
| **Pilot** | A small, timeboxed, production-adjacent project designed to prove or kill an idea cheaply. |
| **Retainer** | Recurring monthly fee for ongoing work (maintenance, monitoring, advice). |
| **T&M (time and materials)** | Billing by the hour/day. Risk sits with the client. |
| **Fixed price** | One price for a defined scope. Risk sits with you. |
| **Value-based pricing** | Price anchored to the value created, not the hours spent. |
| **Churn** | Losing a client. |
| **Scope creep** | The scope silently growing without the price growing. The default failure mode of all consulting. |
| **Stakeholder** | Anyone who can help, fund, block, or sabotage the project. Rarely just the person who hired you. |
| **Champion** | Your internal advocate at the client — the person whose career is attached to the project succeeding. |

---

# Lesson 1 — The Product Is Judgment, Not Software

## Why This Lesson Exists

Between 2013 and 2017, the University of Texas MD Anderson Cancer Center — one of the most respected cancer hospitals on Earth — paid IBM and its consulting partners over **$62 million** to deploy Watson for Oncology, an AI that would recommend cancer treatments. A university audit later found the project was benched without ever being used on patients. Internal documents reported by Stat News showed the system sometimes recommended treatments that were, in the words of one physician, unsafe — partly because it had been trained on a small number of synthetic, hypothetical cases rather than real patient data. The technology partner had every incentive to keep the engagement alive. Nobody in the room was paid to say "this should not be built this way, and possibly not at all."

Sixty-two million dollars, four years, world-class client, famous vendor — and the deliverable was a write-off. Not because the engineers were bad. Because **no one in the engagement was selling judgment; everyone was selling effort.**

This is the founding lesson of AI consulting: your product is not the system. Your product is the decision about which system to build, whether to build one at all, and the proof that it works. The build is how you deliver the judgment. Get this backwards and you become one of the 95%.

## 1.1 Why "We Need AI" Is Never the Real Problem

No business has a problem called "lack of AI." Businesses have problems like:

- "Our two customer-service staff spend four hours a day answering the same 30 questions on WhatsApp."
- "Quotes take three days to produce, and we lose deals to competitors who quote in one."
- "Our founder is the only person who can write proposals, and she is the bottleneck on all revenue."

When a client says "we need AI" or "we want a chatbot," they are handing you a **proposed solution**, not a problem. Accepting a proposed solution as if it were a problem is the root cause of most failed engagements, because you end up delivering exactly what they asked for, which does not fix what hurts, which means no measurable impact, which means you were expensive and useless — even though you did precisely what you were told.

The consultant's move is always the same: **trade the solution back for the problem.** Ask what would be different in the business if the chatbot existed. Ask what it costs them today. Ask who suffers. Keep pulling until you hit something measurable in hours or dollars. That measurable thing is what you are actually being hired to change, and it — not "a chatbot" — is what goes in the scope document.

```
  client's words        first pull              second pull            the real problem
  +--------------+      +------------------+    +------------------+   +---------------------+
  | "we want a   | ---> | "what would it   |--> | "what does that  |-->| CS answers 30 FAQs  |
  |  chatbot"    |      |  do for you?"    |    |  cost you now?"  |   | manually; ~20 staff-|
  +--------------+      +------------------+    +------------------+   | hours/week; slow    |
                                                                       | replies lose sales  |
                                                                       +---------------------+
                                                                            THIS is the scope
```

## 1.2 The Kill Decision Is the Premium Service

You know from 101 that hallucination is structural and from 102 that the demo-to-production gap is where projects die. The commercial translation: **a large fraction of the AI projects clients propose should never be started**, and you can often tell in the first meeting. The signals:

1. **Deterministic software solves it.** If the workflow is "look up X in a database and return it," a model adds cost, latency, and a failure mode to a solved problem. (From 101: don't use a probabilistic component where a lookup table works.)
2. **Error tolerance is near zero and the failure surface is public.** Medical dosing, legal filings, prices quoted to customers. Possible, but the eval and guardrail budget will dwarf the naive estimate — the client must want it enough to fund that.
3. **The data doesn't exist.** RAG over documents nobody has written, fine-tuning on examples nobody has collected. The project is secretly a data-creation project, which is fine, but it must be scoped and priced as one.
4. **No one owns the outcome.** If no individual at the client is measured on the metric you're improving, the project has no champion, and unowned projects die at the adoption stage no matter how good the system is.
5. **The value is a rounding error.** Automating a task that costs 2 staff-hours a month cannot repay a build. The arithmetic (Lesson 3) fails immediately.

Saying "I would not build this, and here is the arithmetic" in the first meeting feels like walking away from revenue. It is the opposite. Every other vendor in the room is nodding along. Being the one who kills a bad idea with a clear reason is the fastest trust-building move that exists in this market, and trust is the only durable asset a consultant has. The client who hears you kill their bad idea brings you their next three ideas.

## 1.3 What You Are Actually Selling (The Stack)

From the client's side, an engagement with you should deliver four layers. Notice how little of it is "writing the prompt":

```
  +---------------------------------------------------------------+
  |  L4  ACCOUNTABILITY   someone to call when it misbehaves;     |
  |                       monitoring, maintenance, model swaps    |
  +---------------------------------------------------------------+
  |  L3  PROOF            evals on a golden dataset the CLIENT    |
  |                       helped define; acceptance = numbers     |
  +---------------------------------------------------------------+
  |  L2  THE SYSTEM       the actual build: RAG/agent/workflow,   |
  |                       guardrails, integration into their ops  |
  +---------------------------------------------------------------+
  |  L1  JUDGMENT         which workflow, which architecture,     |
  |                       and crucially which projects NOT to do  |
  +---------------------------------------------------------------+
```

L1 and L3 are where your 101–103 knowledge becomes money. Most competitors sell only L2, which is why their pilots join the 95%. L4 is where recurring revenue lives (Lesson 4). A useful self-check for every proposal you ever write: if the document only describes L2, it is a body-shop quote, not a consulting proposal.

## 1.4 The Demo Trap, Commercially

From 102 you know the demo-to-production gap technically: the demo works on five friendly inputs; production is ten thousand adversarial ones. The commercial version of this trap is more dangerous because it is *profitable in the short term*: demos close deals. A slick demo built in two days makes the client believe the project is 80% done, so they anchor on a small budget and a short timeline, and now you have sold a production system for the price of a demo.

The discipline: **use demos to prove possibility, never to imply completeness — and say the second half out loud, in writing.** The sentence that saves you is some version of: "What you just saw handles the happy path. The engagement is making it survive the unhappy paths, and that is where the budget goes — roughly 20% demo, 80% evals, edge cases, guardrails, and integration." Clients respect this, because every client has already been burned by a demo that never shipped. You are describing the burn they already felt.

## Summary: The Rules

1. **A proposed solution is not a problem.** Trade "we want a chatbot" back for a measurable pain in hours or dollars before scoping anything.
2. **Killing bad projects is the premium service.** The market's problem is failed pilots, not missing builders. Your kill criteria from 101–103 are the product.
3. **Sell all four layers** — judgment, system, proof, accountability. A proposal that only describes the build is a commodity quote.
4. **Never let a demo price a production system.** Say "20% demo, 80% everything else" out loud, in writing, early.
5. **Trust compounds; fees don't.** The engagement that ends with "you saved us from ourselves" is worth more than the one that ends with an invoice.

## Drill 1 (harsh)

Answer in writing. No hedging. Where arithmetic is possible, do it.

**Q1.** A Hong Kong logistics SME says: "We want an AI agent that reads incoming customer emails and automatically books shipments in our system." Using the five kill signals from 1.2, list the questions you ask in the first meeting, in order, and state for each what answer would kill the project on the spot.

**Q2.** Explain the MD Anderson failure using the L1–L4 stack from 1.3: which layers were purchased, which were delivered, and which missing layer was most causally responsible for the $62M write-off? Defend your choice against the obvious alternative.

**Q3.** A client saw your demo and says: "This looks basically done — can we go live next week and just fix issues as they come up?" Write your exact reply, in four sentences or fewer, that (a) doesn't insult the demo, (b) re-anchors the budget, and (c) uses one concept from AI Engineering 102 translated into client language.

**Q4.** Your prospective client's proposed use case: an LLM that answers staff questions about the company's 40-page HR policy PDF. A competitor quoted them a four-week agent build. What do you quote, and why? (Hint: what is the simplest architecture from 101 that solves this, and what does intellectual honesty about that do to your price and to your positioning?)

**Q5.** True or false, with reasoning: "The 95% pilot failure rate means the market is bad for AI consultants." Your answer must reference who fails, why, and what that implies about positioning.

**Q6.** Rewrite this sentence from a real (bad) proposal so it sells judgment instead of effort: "We will spend 6 weeks developing an AI chatbot for your customer service using the latest LLM technology."

**Reading assignment.** MIT Project NANDA, *The GenAI Divide: State of AI in Business 2025* (the "95%" report) — read for *why* pilots fail, not the headline number; note the finding about the "learning gap" and workflow integration. Then read the Stat News investigation into Watson for Oncology ("IBM's Watson supercomputer recommended 'unsafe and incorrect' cancer treatments," 2018). Question to answer: in both, was the primary failure technical or organizational?

---

# Lesson 2 — Discovery: Mapping the Client Before Touching a Model

## Why This Lesson Exists

In 2021, McDonald's and IBM launched a high-profile partnership to automate drive-thru order taking with voice AI, tested across more than 100 US locations. In June 2024, McDonald's ended it. In between, the project became a genre of viral TikTok video: the AI adding bacon to ice cream, a customer begging it to stop as it added hundreds of chicken nuggets to an order. The models were not garbage — voice recognition in 2023 was the best it had ever been. The *workflow* was mischosen: a noisy, open-mic, real-time environment; infinite menu-adjacent phrasing; customers who deviate, joke, and talk over each other; near-zero user patience; and every failure filmed and published. Error tolerance: effectively zero. Failure surface: maximally public. Fallback path: awkward (a human takes over mid-order, slower than if they'd just taken it).

The lesson: **project selection is done during discovery, not during engineering.** By the time you are tuning prompts, the outcome was mostly decided by which workflow you picked and how the humans around it were arranged. This lesson is the method for picking.

## 2.1 Workflow Mapping: The Unit of Analysis Is the Task, Not the Job

Clients describe their operations in terms of roles ("our admin girl," "the sales team"). Roles are not automatable; **tasks** are. Discovery starts by decomposing the painful workflow into tasks and scoring each one. From 101–103 you already have the technical scoring instincts; here is the full grid, with the business columns added:

For each task, record:

1. **I/O shape** — is it language in, language/structured-data out? (Models eat these. Physical tasks, judgment-with-liability tasks, relationship tasks — no.)
2. **Volume** — how many times per week? (Value scales with volume; a task done twice a month rarely repays a build.)
3. **Current cost** — minutes per instance x instances x loaded hourly cost of whoever does it.
4. **Error tolerance** — what happens when the output is wrong 2% of the time? Annoyance, rework, refund, lawsuit, or headline?
5. **Verifiability** — can a human check the output in far less time than doing the task? (High verifiability is gold: it enables the draft-and-review pattern, which ships 10x more often than full autonomy.)
6. **Data availability** — do the documents/examples/ground truth needed actually exist today, in accessible form?
7. **Failure surface** — internal (a colleague sees the mistake) or external (a customer, a regulator, TikTok)?

```
  score every task, then place it here:

                         VALUE (volume x cost)
                      low                  high
                 +----------------+----------------------+
     FEASIBILITY |   ignore       |  roadmap: revisit    |
     (I/O shape, |                |  when feasibility    |
      error tol, |                |  or tooling improves |
      data,      +----------------+----------------------+
      verifiab.) |   ignore       |  BUILD HERE FIRST    |
           high  |  (hobby tier)  |  esp. if verifiable  |
                 |                |  + internal failure  |
                 +----------------+----------------------+
```

The discipline most builders lack: **the first project is chosen for trust-building, not for maximum value.** Pick something high-feasibility, medium-value, internal failure surface, humanly verifiable, shippable in weeks. The McDonald's drive-thru is the exact inverse on every axis — which a one-hour discovery session scoring the grid would have shown. (IBM had years and did not kill it; that's Lesson 1's incentive problem again.)

## 2.2 Autonomy Is a Dial, Not a Switch

From 103 you know the workflow-vs-agent control spectrum. Discovery is where you set the dial *with the client*, because the dial position determines cost, risk, and adoption. Four stable positions, in ascending autonomy:

```
  1. ASSIST      human does task, AI drafts/suggests      (review: 100%)
  2. DRAFT       AI does task, human approves each one    (review: 100%, cheap)
  3. EXCEPTION   AI does task, flags low-confidence       (review: ~10-20%)
  4. AUTONOMOUS  AI does task, humans audit samples       (review: ~1%)
```

Almost every successful SME engagement **enters at 2 and earns its way rightward** as eval scores and trust accumulate. Selling position 4 on day one is selling the McDonald's drive-thru. The commercial beauty of position 2 is that it is *immediately* valuable (drafting is most of the labor), *safe* (a human gates every output), and it *generates the eval dataset for free* — every human approval/correction is a labeled example. You are being paid while the client manufactures your golden dataset. Design engagements so this happens on purpose.

## 2.3 Stakeholders: The Org Chart of Sabotage

Technical discovery tells you what to build. **Political discovery tells you whether it will survive.** Minimum map, even at a 20-person SME:

- **Economic buyer** — signs the invoice. Cares about the arithmetic in Lesson 3.
- **Champion** — wants this to succeed for their own career/sanity. If you cannot identify one, you do not have a project; you have an invoice waiting to be disputed.
- **Operators** — the people whose task you are touching. They believe, correctly or not, that you were hired to eliminate them. If they quietly refuse to use the system or feed it garbage, the project dies at adoption, and the post-mortem will blame "the AI." Discovery must include them: they know where the bodies are buried (the edge cases that never made it into the SOP document), and involving them converts saboteurs into co-designers.
- **Blockers** — IT ("no external APIs touch our data"), finance, sometimes legal/compliance. Find them in week one, not week six.

The MIT NANDA finding from Lesson 1's reading belongs here: the dominant reason pilots fail is not model quality — it's that systems don't fit workflows and organizations don't adapt. Adoption is not a phase after delivery. It is a design input during discovery.

## 2.4 The Discovery Deliverable (and Why Discovery Should Usually Be Paid)

Discovery is real work and produces a real artifact. Structure a paid discovery (for SMEs: days, not months) that ends in a written document containing:

1. The workflow map with the scoring grid, all candidate tasks ranked.
2. The recommended first project, its autonomy dial position, and the *reason* the others lost.
3. The arithmetic: current cost of the task, projected cost after, payback estimate (Lesson 3 math).
4. Data audit findings: what exists, what's missing, what must be created.
5. Draft acceptance criteria: the eval metric and threshold that will define "done."
6. Explicit kill criteria: what discovery-stage facts would make you recommend *not* proceeding.
7. A fixed-price quote for the pilot.

Two commercial effects. First, paid discovery filters unserious clients cheaply — someone unwilling to pay for a few days of diagnosis will never pay for a build. Second, the document is valuable *even if you never build*: the client can take it and run. Paradoxically this makes them far more likely to hire you for the build, because you have just demonstrated L1 (judgment) instead of claiming it. Free discovery, by contrast, pressures you to recommend building (you need to recoup the free work) — which is exactly the IBM incentive rot from Lesson 1, at small scale, inside you.

## Summary: The Rules

1. **Decompose to tasks and score them.** Roles aren't automatable; tasks are. Seven columns: I/O shape, volume, cost, error tolerance, verifiability, data, failure surface.
2. **First project = trust-builder**: high feasibility, medium value, internal failure surface, human-verifiable, weeks not months. Never the moonshot first.
3. **Enter at Draft (dial position 2)** and earn autonomy with eval evidence. Every human review is a free labeled example — design for it.
4. **Map the politics.** No champion, no project. Operators co-design or they sabotage. Find blockers in week one.
5. **Charge for discovery, deliver a document with kill criteria in it.** Free discovery corrupts your own judgment.

## Drill 2 (harsh)

**Q1.** Score the McDonald's drive-thru use case against all seven columns of the 2.1 grid, one line each. Then name a *different* task inside a fast-food operation that scores well, and state its dial position.

**Q2.** A client (a HK event-management company) wants "an AI that fully handles vendor negotiations on WhatsApp." Which dial position are they asking for, which do you counter-offer, and — in client language, no jargon — what is your two-sentence justification? Include the free-golden-dataset argument without using the words "golden dataset."

**Q3.** During discovery, the operator who currently does the task answers your questions in monosyllables and "forgets" to send you the example files twice. Diagnose using 2.3, and give the concrete move you make *this week*. "Escalate to the boss" is an automatic fail — explain why.

**Q4.** Compute: a task takes 12 minutes, occurs 300x/month, done by staff at HK$180/hour loaded cost. (a) Monthly cost of the task today. (b) At dial position 2, assume drafting cuts human time to 3 minutes per instance — new monthly cost and monthly saving. (c) If your pilot costs HK$120,000 fixed, what is the payback period, and do you take this to the economic buyer as-is or keep hunting? Justify.

**Q5.** Write the "kill criteria" section (item 6 of the discovery deliverable) for a RAG-over-company-docs project, as you would put it in the client document: at least three conditions, each objectively checkable, each tied to a concept from AI Engineering 101–102.

**Q6.** Your paid discovery concludes the client's cherished idea lands in the top-left of the 2x2 (high value, low feasibility) but a boring idea they never mentioned lands bottom-right (build here). The CEO is emotionally attached to the cherished idea. Write the paragraph of the discovery document that delivers this, preserving the relationship without softening the recommendation.

**Reading assignment.** Read coverage of the McDonald's–IBM drive-thru wind-down (June 2024, e.g. CNBC/Restaurant Business) and, separately, Anthropic's *Building Effective Agents* one more time — but this time read it as a *sales document*: note how it argues for the simplest architecture that works. Question to answer: how does "use the simplest pattern that works" translate into a pricing and trust advantage for a consultant, not just an engineering one?

---

# Lesson 3 — The Arithmetic: ROI, Pricing, and the Contract

## Why This Lesson Exists

In February 2024, Klarna announced its AI assistant was doing the work of **700 full-time customer-service agents**, handling two-thirds of all chats. The press cycle was enormous; the CEO said the company would shrink headcount and let AI absorb the work. By mid-2025, the same CEO publicly reversed course: cost had dominated the evaluation criteria, quality had suffered, and Klarna began recruiting humans back into customer service, landing on a hybrid model where customers can always reach a person.

Read commercially, this is not an "AI doesn't work" story — the assistant kept operating. It is a story about **arithmetic done with the wrong terms in the equation.** The visible saving (agent salaries) was counted; the invisible costs (quality degradation, brand damage, customer trust, the price of winning customers back) were not. Klarna could afford the correction. Your SME client cannot. When you present ROI, you are professionally responsible for the whole equation — including the terms the client cannot see and the ones that make your project look worse. This lesson is that equation, and then the contract that wraps it.

## 3.1 The Full ROI Equation

The naive pitch every competitor makes:

```
  saving = (hours automated) x (hourly cost)          <- this is the LIE OF OMISSION
```

The honest equation you present:

```
  monthly value  = labor saving
                 + speed value        (deals won by responding in 5 min vs 3 days)
                 + capacity value     (growth absorbed without hiring)
                 - review cost        (dial position 2-3: humans still check)
                 - error cost         (residual error rate x cost per error)

  monthly cost   = inference (tokens)                 <- usually the SMALLEST line
                 + integration upkeep (APIs change)
                 + prompt/eval maintenance            <- from 102: models drift, docs change
                 + monitoring
                 + model migration reserve            <- providers deprecate; budget it

  payback months = build fee / (monthly value - monthly cost)
```

Three professional habits around this equation:

1. **Inference cost is almost never the story.** SME workloads measured in thousands of requests/month cost tens to hundreds of USD in tokens. Clients fixate on it because it's the visible meter; redirect them to review cost and maintenance, which are 10–50x larger and determine whether the project survives year two.
2. **Present the error term explicitly.** "At 97% eval accuracy on your dataset and 2,000 requests/month, expect ~60 wrong outputs monthly; at dial position 2 a human catches nearly all of them at a review cost of X; here's what dial 3 does to that." A client who accepts a number they chose cannot later claim betrayal. This is the anti-Klarna move: putting the quality term in the equation *before* go-live instead of discovering it in a headline.
3. **Refuse to present ROI you don't believe.** The champion will sometimes push you to inflate the saving to get budget approved. The inflated number becomes your acceptance criterion in the buyer's head, whatever the SOW says. Consultants are fired over the gap between the pitch deck and month three.

## 3.2 Pricing Models and Where the Risk Sits

Every pricing model is a decision about who holds which risk. There is no best one; there is a right one per phase:

```
  model          you hold              client holds           right phase
  -------------  --------------------  ---------------------  -------------------------
  T&M            reputation only       cost overrun risk      genuinely unknown scope
  fixed price    overrun risk          almost none            AFTER paid discovery only
  retainer       delivery discipline   paying for idle time   maintenance, L4 (Lesson 4)
  value-based    proving attribution   overpaying             mature trust, clean metric
  usage-based    margin volatility     bill surprises         high-volume, post-pilot
```

The rules that keep you solvent:

- **Never fixed-price undiscovered scope.** Fixed price is fine — *after* paid discovery has bounded the unknowns. Fixed-pricing a project whose data you haven't audited is gambling your margin on the client's filing habits.
- **The pilot is a product: timeboxed, fixed-price, kill-criteria'd.** E.g., "4 weeks, HK$X, ends in a system at dial position 2 plus an eval report; if the eval can't reach the agreed threshold, we stop and you keep the report and the dataset." The client is buying certainty of *learning*, not certainty of success — say exactly that.
- **Anchor on value, charge above cost, land between.** If the task costs the client HK$40k/month, a HK$120k pilot with 3-month payback is cheap at any hourly rate implied. Present price next to the arithmetic, never next to your day count — day counts invite hourly-rate haggling, arithmetic invites payback haggling, and you win the second argument.
- **Discounting the fee is worse than shrinking the scope.** A discount reprices your judgment forever. Removing a deliverable preserves the rate and gives the client a real choice.

## 3.3 Acceptance Criteria: Evals as Contract Law

From 101: you can't ship on vibes. The consulting corollary: **you can't invoice on vibes.** "The client is satisfied" is not an acceptance criterion; it is a hostage situation — satisfaction is non-deterministic, revocable, and owned by whoever is grumpiest at the client on invoice day.

The professional pattern, and the single biggest upgrade this course makes to your contracts:

1. During discovery, you and the client **jointly build the golden dataset** — real examples of the task with agreed-correct outputs, including the nasty edge cases the operators know about (2.3: this is why operators must be in the room).
2. The SOW states: *"Acceptance: the system achieves >= [threshold]% on the evaluation dataset defined in Appendix A, measured by [method]. Payment milestone 3 is due on demonstration of this result."*
3. Anything not represented in the dataset is, by construction, out of scope. New edge case discovered in week 3? It goes in the dataset *and* triggers the change-control clause (a written, priced scope amendment) — not a silent weekend of free work.

This converts the three chronic diseases of consulting — subjective acceptance, scope creep, and payment disputes — into one objective number that both sides watched being born. It also quietly forces you to do the engineering right, because your invoice now depends on your eval pipeline. Incentive alignment you built yourself.

One warning from 102, translated: **do not let the golden dataset be built solely from the client's happy examples.** A dataset of easy cases produces a threshold you'll hit in week one and a system that fails in production, which triggers exactly the dispute the mechanism exists to prevent. Adversarial examples go in on purpose, and you say why: "this dataset is hard on purpose, because production will be."

## 3.4 Build vs Buy vs Configure

The client can (a) buy a SaaS product, (b) hire you to build custom, or (c) hire you to configure/glue existing tools. Your incentive screams (b). Your judgment must actually run the comparison, out loud, in the discovery document — because the client will eventually discover the SaaS that does 80% of the job for US$99/month, and what they conclude about you at that moment depends entirely on whether *you* told them about it first.

The honest heuristics: buy when the workflow is generic (meeting notes, generic support deflection) — your value there is selection and rollout, a smaller but real engagement. Build when the workflow is the client's *differentiator*, touches proprietary data, or needs deep integration into their systems. Configure/glue when the pieces exist but the connective tissue doesn't — increasingly the dominant SME pattern, and (as you know from 103) exactly the gap MCP-style integration standards are shrinking: the consultant's job shifts from writing glue code to choosing and securing the connections. Pricing note: configure-engagements bill less than builds but churn less, renew more, and generate the retainers that Lesson 4 lives on.

## Summary: The Rules

1. **Present the whole equation** — review cost, error cost, maintenance, migration reserve. The Klarna failure was arithmetic with terms deleted.
2. **Inference cost is a distraction**; maintenance and review are the real recurring lines. Redirect the client's attention there.
3. **Match pricing to risk**: T&M for the unknown, fixed only after paid discovery, retainer for the long tail. Never fixed-price an unaudited scope.
4. **Acceptance = eval threshold on a jointly-built, deliberately hard dataset, written into the SOW.** New edge cases amend the dataset via change control, in writing, priced.
5. **Run build-vs-buy against yourself in writing.** The client finding the $99 SaaS before you tell them about it costs you every future engagement.
6. **Shrink scope, never rate.**

## Drill 3 (harsh)

**Q1.** Reconstruct Klarna's mistake as the 3.1 equation: name at least three terms that were plausibly omitted or mis-estimated in the 2024 announcement, and state which single term the 2025 reversal proves was largest. One line each.

**Q2.** Full arithmetic. Client task: drafting quotation emails. 8 min/instance, 500 instances/month, staff at HK$200/hr loaded. Dial 2 cuts human time to 2 min. Eval accuracy 96%; a bad draft that slips through costs ~HK$500 on average (rework + client annoyance); at dial 2 assume humans catch 95% of the 4% errors. Inference ~HK$400/mo, maintenance retainer HK$4,000/mo. Pilot fee HK$150,000. Compute monthly value, monthly cost, net monthly benefit, payback in months — then write the one-sentence version you'd say to the economic buyer.

**Q3.** The champion says: "Can you put 12 months' payback as 4 months in the deck? Finance won't approve otherwise, and we both know the soft benefits are real." Write your exact reply (max 5 sentences) that keeps the champion, keeps your number, and offers a legitimate path to a stronger business case.

**Q4.** Draft the acceptance-criteria clause for the Q2 engagement as it would appear in the SOW: dataset definition, threshold, measurement method, what happens below threshold, and the change-control trigger. Contract register, not chat register.

**Q5.** A client asks for a fixed price on "an agent that handles our whole procurement inbox" with no discovery done. Give the three distinct risks (one technical from 102/103, one commercial, one relational) that make this a refusal, and then the counter-offer you make instead — with a structure and rough price logic, not just "let's do discovery."

**Q6.** Your discovery finds that an off-the-shelf tool at US$149/month covers ~75% of the client's need; a custom build covering ~95% costs HK$200k plus retainer. Write the recommendation paragraph for the discovery document. It must contain a defensible recommendation, the condition under which the recommendation flips, and no hedging.

**Reading assignment.** Read the 2025 coverage of Klarna's reversal (e.g. Bloomberg interview with Siemiatkowski, "AI customer service U-turn") against the original Feb 2024 announcement — list every quantitative claim in the 2024 piece and mark which survived. Then read a16z's *The New Business of AI* (Casado & Bornstein) on why AI-product gross margins differ from software margins. Question to answer: which of that essay's margin problems transfer to a consulting/services business, and which conveniently don't?

---

# Lesson 4 — Delivery, Risk, and the Long Tail

## Why This Lesson Exists

In February 2024, a Canadian tribunal ruled in *Moffatt v. Air Canada* — you know the technical anatomy from 101. Read it again as a consultant and a different clause jumps out: Air Canada argued the chatbot was "a separate legal entity responsible for its own actions." The tribunal called that submission remarkable and rejected it flatly: the airline is responsible for all information on its website, chatbot included. The same year, delivery firm DPD had to disable parts of its chatbot after a customer induced it to swear and compose a poem about how useless DPD was — screenshots, millions of views, a brand-damage incident from a system that had run quietly for years until one model update.

Now perform the substitution that matters for this course: in both incidents, imagine the system had been built by an outside consultant. Whose name is in the client's post-mortem meeting? Whose contract is being re-read by their lawyer that afternoon? **Liability lands on the client, and the client's fury lands on you.** The last lesson of this course is that delivery is not the end of risk — it is the transfer of risk into production, where it compounds monthly. Managed well, that fact is not a threat; it is the recurring-revenue engine of the entire practice.

## 4.1 Guardrails Are Scope Items, Not Engineering Hygiene

From 102 you know defense-in-depth: input filtering, grounding, output validation, topic fences, rate limits, escalation to humans. The consulting move is to surface these as **named, priced, client-visible deliverables** rather than silent engineering:

- It converts invisible work into visible value ("Deliverable 4: containment layer — the system cannot discuss competitors, legal topics, or make commitments about price/refunds; out-of-bounds queries route to a human with full context").
- It forces the *client* to decide policy questions that are genuinely theirs, not yours: what may the bot never say? what must always escalate? whose name signs off? This meeting — call it the red-lines workshop — is an hour long, produces the fence list, and is the cheapest liability insurance either of you will ever buy.
- It creates the paper trail. When something eventually goes wrong (it will; residual error rate is in the SOW from Lesson 3), the difference between "vendor was negligent" and "a known residual risk we jointly accepted occurred, and the escalation path worked" is documentation you wrote in week two.

The DPD incident adds the time dimension: the system ran fine for years; an update changed behavior. Which is why guardrails are not a phase — they are a subscription. Hold that thought for 4.3.

## 4.2 Adoption: The Last Mile Is Human

Recall the MIT NANDA finding (Lesson 1 reading): the dominant failure driver isn't model quality, it's workflow fit and organizational adaptation. You designed for adoption in discovery (operators as co-designers, dial position 2). Delivery is where it pays off or doesn't. The mechanics that separate used systems from expensive shelfware:

1. **Ship into the tool they already live in.** For your SME clients that is WhatsApp, email, the spreadsheet — not a new dashboard with a new login. Every additional login halves usage; this is close to a law of nature.
2. **The workflow must be faster on day one for the operator personally,** not just cheaper for the company in aggregate. If reviewing drafts takes longer than writing them, dial position 2 dies quietly and the operator was right to kill it.
3. **Name an internal owner before go-live.** Not the champion (who sponsors) — an operator-level owner who triages weird outputs, feeds new edge cases into the eval set, and is the human in human-in-the-loop. Systems without an internal owner decay in roughly one quarter.
4. **Instrument usage from day one and put it in front of the client monthly.** Usage collapsing is churn telling you in advance. It is also, honestly reported, the thing that most builds trust — you are auditing your own deliverable in front of them.

## 4.3 The Retainer: Where the Business Model Actually Lives

Everything in AI drifts: the client's documents go stale (RAG rot, from 102), their business rules change, providers deprecate models on quarterly cycles, prompt behavior shifts across model versions, new edge cases arrive weekly, and adversarial users keep probing (DPD). A delivered-and-abandoned AI system degrades the way an untended garden does — silently, then suddenly.

This is not a bug in your business model. It **is** the business model:

```
  one-off build economics            build + retainer economics
  +--------------------------+      +----------------------------------+
  | fee: one lump            |      | fee: lump + monthly              |
  | revenue: spiky, resold   |      | revenue: compounding base        |
  | relationship: ends       |      | relationship: standing access    |
  | system: decays, and the  |      | system: maintained; decay        |
  |   decay is blamed on YOU |      |   becomes billable work          |
  | next project: re-sell    |      | next project: spotted from       |
  |   from zero              |      |   inside, no sales cycle         |
  +--------------------------+      +----------------------------------+
```

A well-formed AI maintenance retainer, concretely: monthly eval re-runs against the (growing) golden dataset with a scorecard the client actually reads; drift and usage monitoring; an edge-case intake channel; N hours of prompt/knowledge-base updates; model-migration handling when providers deprecate; and a quarterly review where the 2.1 task grid gets re-scored — which is where the *next* engagement is discovered, sold with zero acquisition cost, because you're inside. Price it as a meaningful fraction of the build (order of 10–20% of build fee, monthly, scaled to criticality), and defend it with the DPD story: the incident cost of one bad public output versus the retainer is not a close comparison.

The compounding effect is the whole game: five clients on retainers is a stable base plus five warm pipelines. The consultant who sells only builds re-enters the cold market after every project. The one who sells builds-plus-retainers is, eighteen months in, running a small recurring-revenue firm.

## 4.4 Saying No, and the Reputational Asymmetry

Last rule, holding everything above in place. Your reputation as an AI consultant obeys the same asymmetry as the systems you build: a thousand good outputs are invisible; one bad one is the story. One MD-Anderson-shaped engagement — overpromised, under-evaled, publicly dead — follows a small consultancy around forever, because your market (SMEs in one city, one ecosystem) all know each other.

Which means the highest-ROI action available to you, on a multi-year horizon, remains the one from Lesson 1: **decline the engagement that will fail.** The client pushing for dial position 4 on a public failure surface with no data and no champion is not revenue; they are a future case study with your name in it. Decline in writing, kindly, with the arithmetic and the grid attached — the document itself is a marketing artifact, and roughly half of the declined come back a year later with a better-shaped problem and no interest in talking to anyone else. They have already seen your judgment; it told them a hard truth for free. That is the product. Everything else in this course was packaging.

## Summary: The Rules

1. **Liability lands on the client; their fury lands on you.** Air Canada settled the legal question — the "AI did it" defense does not exist.
2. **Sell guardrails as named deliverables** and make the client set the red lines in a workshop. The paper trail is the insurance.
3. **Adoption is designed, not hoped for**: ship into existing tools, make the operator personally faster on day one, name an internal owner, report usage monthly.
4. **The retainer is the business model**: drift is permanent, so maintenance is permanent, so revenue is recurring — and the next project is found from inside.
5. **Protect the asymmetry.** One public failure outweighs a thousand quiet successes; declining doomed work is a compounding investment.

## Drill 4 (harsh)

**Q1.** Write the agenda (5–7 items, one line each) for the red-lines workshop with a client deploying a customer-facing WhatsApp assistant for a food business — including the two agenda items most consultants forget, and mark them.

**Q2.** DPD's chatbot ran for years before the swearing incident, which followed a system update. Name the specific 102-level failure mode this implicates, the retainer line item that exists to catch it, and the sentence you'd use to sell that line item to a CFO who thinks maintenance is "paying for something that already works."

**Q3.** Three weeks post-launch, usage by the operators has fallen 60%. List the diagnostic questions you ask *in order*, mapped to the four mechanics of 4.2, and state the most likely root cause given dial position 2. What do you do if the root cause is "reviewing drafts is slower than writing"?

**Q4.** Price and structure a retainer for the Lesson 3 Q2 engagement (HK$150k build). Line items, monthly price with one-line justification, and the quarterly-review mechanism that generates the next engagement. Then: the client counters at half your price — what do you remove first and why?

**Q5.** A prospect insists on a fully autonomous (dial 4) public-facing agent that quotes prices and accepts orders, no human review, live in 3 weeks, "our competitor already has one." Write the decline: short, warm, arithmetic or grid reasoning included, door left open. Then state — honestly — the condition under which you would take a modified version of this engagement.

**Q6.** Synthesis. Trace one thread through all four lessons: start from "we want a chatbot," and in 8–12 lines of a client-facing narrative (not bullet-point telegraphese), show the engagement passing through problem-extraction (L1), the scoring grid and dial (L2), the equation and eval-gated SOW (L3), and the red-lines workshop and retainer (L4). This is the elevator version of your entire practice; you should be able to say it from memory.

**Reading assignment.** Read the *Moffatt v. Air Canada* decision itself (BC Civil Resolution Tribunal, 2024 BCCRT 149) — it is short and readable — noting exactly how the tribunal disposed of the "separate entity" argument. Then read David Maister's *The Trusted Advisor*, chapters 1–3 (or the summary essay "It's Not Enough to Be Right"). Question to answer: Maister argues expertise is table stakes and trust is the differentiator — what, concretely, in the four lessons of this course is trust-manufacturing machinery rather than expertise display?

---

# Master Rules (Capstone)

1. **Trade the solution back for the problem.** No scope until the pain is measured in hours or dollars.
2. **Kill bad projects early and out loud** — the kill is the premium service in a market defined by a 95% pilot failure rate.
3. **Sell the stack: judgment, system, proof, accountability.** A build-only proposal is a commodity quote.
4. **Choose the first project for trust**: high feasibility, medium value, internal failure surface, verifiable, weeks.
5. **Autonomy is a dial entered at Draft** and advanced only on eval evidence; every human review is a free labeled example.
6. **No champion, no project.** Operators co-design or they sabotage.
7. **Charge for discovery; deliver a document with kill criteria inside it.**
8. **Present the whole ROI equation**, including the terms that make you look worse — review cost, error cost, maintenance, migration.
9. **Acceptance is an eval threshold on a jointly-built, deliberately hard dataset, written into the SOW**; new edge cases go through change control.
10. **Never fixed-price undiscovered scope; shrink scope, never rate.**
11. **Run build-vs-buy against your own interest, in writing.**
12. **Guardrails are named deliverables; red lines are the client's decision, documented.**
13. **Design adoption**: existing tools, operator-faster-on-day-one, internal owner, monthly usage reporting.
14. **The retainer is the business** — drift is permanent, maintenance is recurring, and the next project is found from inside.
15. **Protect the asymmetry**: one public failure outlives a thousand successes; the declined engagement is a marketing artifact.

## Quick Reference Table

| Concept | What it is | Who controls it | Main lever |
|---|---|---|---|
| **Problem extraction** | Converting "we want AI" into measured pain | You (questioning) | Hours/dollars before scope |
| **Kill criteria** | Pre-agreed conditions to stop | Jointly, in writing | Credibility + loss control |
| **Task grid** | 7-column scoring of decomposed tasks | You | First-project selection |
| **Autonomy dial** | Assist / Draft / Exception / Autonomous | Jointly | Enter at 2, earn rightward |
| **Champion** | Internal owner of success | The client org | No champion = no project |
| **Paid discovery** | Diagnosis phase with document deliverable | You | Filters clients, protects judgment |
| **ROI equation** | Full value minus full cost | You (honesty) | Include review/error/maintenance |
| **Pricing model** | Who holds which risk | Negotiated | Match model to phase |
| **Eval-gated SOW** | Acceptance = threshold on joint dataset | Jointly | Objective payment, creep control |
| **Red-lines workshop** | Client decides forbidden zones | The client | Liability paper trail |
| **Retainer** | Monthly maintenance + monitoring | You | Recurring revenue + warm pipeline |
| **Reputational asymmetry** | One failure outweighs many successes | You (project selection) | Decline doomed work |

## The Mental Model in One Sentence

> **AI consulting is the discipline of selling judgment under uncertainty: extract the real problem, kill what shouldn't be built, choose the winnable first project, price the whole equation honestly, let evals — not vibes — define "done" in the contract, and stay for the drift, because in a market where 95% of pilots fail, the consultant who can prove what works and refuse what won't is not competing with other builders at all.**
