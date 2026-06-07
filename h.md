# Local Excel Automation Hub — Build Spec

> **How to use this file:** Paste this whole document into your own Claude Code as the first
> message, then say *"Build this with me. Start at §2 (the interview)."*
> This spec was written by an agent that could **not** see your scripts, your files, or your
> bank's rules. You can. Wherever this spec says "ask the user" or "inspect the scripts,"
> do that for real instead of guessing.

---

## 0. Context for the building agent (read first)

You are helping a person who works on a **trading floor at a bank (S&T desk)**. Treat every
file, email, number, and client name as **sensitive / potentially confidential**. The
company uses **AWS Bedrock**, and the user already uses **Claude Code** day to day.

The user currently has roughly **4 Python scripts** that each take an Excel-related task and
produce a finished Excel. Today the workflow is **manual**:

- He receives emails with attachments.
- For each one, he opens Claude Code (or runs a script in a terminal), hands over the file
  plus *some words from the email*, and gets an Excel back.
- He has to mentally **route** each email to the right script, and he's tired of the terminal
  + manual file-shuffling.

**Goal:** a single **local web app** (think "iLovePDF, but on his own machine"): he drops a
file, pastes the relevant email text, the app figures out which tool to run, runs it, and
hands back the finished Excel. **Nothing gets deployed. Everything runs on localhost.**

### Hard constraints (non-negotiable)
1. **Local only.** Runs on his machine. Bind to `127.0.0.1` only — never `0.0.0.0`. No server,
   no cloud host, no public URL.
2. **No new data egress without it being understood.** See §4 — whether any data leaves the
   machine depends on one design decision. Make that decision *explicitly* with the user.
3. **Compliance-friendly and explainable.** Prefer deterministic, auditable behavior over
   clever-but-opaque behavior. He may have to explain this tool to IT/Compliance.
4. **Don't touch his inbox.** v1 does **not** read email programmatically. He keeps dragging
   attachments in by hand. (Mailbox access is a much bigger approval ask — out of scope.)

---

## 1. The exact per-task flow

For **each** task, the shape is:

```
INPUT                          PROCESS                         OUTPUT
─────                          ───────                         ──────
1. an input file        ──►    route to the correct      ──►   1. a finished Excel
   (xlsx / csv / pdf?)         tool, then apply it
2. some words from
   the email he got
```

The thing that is easy to miss: **the email words are part of the input, not just for
routing.** They may carry instructions or parameters that change what the script does
(a date, a client name, a cutoff, "exclude cancelled trades," etc.). So the UI must capture
**both** a file **and** a free-text box per task — not just a file.

---

## 2. Interview the user before building (ask, don't assume)

Ask these and write the answers down. Do not start coding until you have them.

1. **The 4 scripts:** Where are they? What does each one do, in one sentence? What file types
   go in, what comes out?
2. **The email words — what role do they play?** For each script, are the words:
   - (A) a few specific **values/options** (a date, a name, a threshold, which tab), or
   - (B) **free-form instructions** that genuinely need interpreting each time?
   *(This is the single most important question — it drives §4. Push for a concrete example
   of the email text for each script.)*
3. **Routing today:** How does he currently know which script a given email needs? Filename?
   Sender? A word in the subject? The sheet/column layout? Just "he knows"?
4. **Volume & file size:** How many files per day? How big? (Affects whether anything needs to
   be streamed or chunked — usually not, but ask.)
5. **Environment policy — confirm before installing anything:**
   - Is he allowed to `pip install` packages? (Some locked-down setups block this.)
   - Is he allowed to run a **local web server / open a localhost port?** (Endpoint security or
     DLP tooling sometimes flags even `127.0.0.1` servers.)
   - If either is blocked, stop and tell him to confirm with IT first — don't trigger an alert.
6. **Bedrock at runtime — allowed?** If any tool needs an LLM call while running (see §4 Type
   B), that sends file/email content to Bedrock. Bedrock keeps data inside their AWS
   environment and doesn't train on it, but it is still data leaving the laptop. Confirm with
   him whether that's acceptable, or whether everything must stay 100% on-machine.

---

## 3. Inspect the existing scripts (do this for real)

Before designing anything, **read all 4 scripts**. For each, produce a small table:

| Script | What it does | Input file type(s) | Uses email words? How? | Output | Hardcoded paths / inputs? |

You're looking for:
- **Hardcoded file paths** and terminal prompts/`input()`/`print`-driven interaction — these
  must be removed so the app can call the script as a function.
- **How email text currently enters the script** (typed manually? pasted into a variable?
  hardcoded?). This tells you what the refactored function signature needs.
- **Side effects** (writing to a fixed folder, opening Excel via COM, etc.) that won't behave
  well inside a web app.

---

## 4. THE key decision: what do the "email words" do?

Classify **each** of the 4 tasks as Type A or Type B. This decides the whole architecture.

### Type A — the words are *parameters*
The email words map to a small set of concrete values (a date, a client, a number, which
sheet). Example: *"Use Tuesday's close and exclude desk 7."*
→ **Stay deterministic.** The UI gives him structured inputs (text boxes / dropdowns / a
single text box the script parses with plain Python). **No LLM at runtime. Nothing leaves the
machine.** This is the ideal — fully local, fast, free, and trivially explainable to
compliance.

### Type B — the words are *instructions that need interpreting*
The words vary freely and require understanding to act on. Example: *"Reformat to match how we
sent it last quarter, drop anything that looks like a test trade, and flag the unusual ones."*
→ This genuinely needs an LLM. The honest options are:
   - **Best:** Sit with the user and try to **convert Type B into Type A** — pin down a fixed
     set of rules/inputs so a deterministic script can do it. Most "instructions" turn out to
     be 2–3 repeated parameters in disguise.
   - **If truly irreducible:** Make a **Bedrock** call at runtime to interpret the instructions.
     This sends content to Bedrock (see interview Q6). Keep these calls **narrow** — use Claude
     to *interpret the instruction into structured parameters*, then let deterministic Python do
     the actual Excel work. Don't hand the whole spreadsheet to the model if you can avoid it.

**Recommendation:** aim for everything to be Type A. Reach for Bedrock only where a task is
genuinely impossible to express as fixed rules. The more deterministic this tool is, the
easier his life is with compliance.

---

## 5. Architecture

Three pieces. He is currently doing the first two by hand.

```
┌─────────────┐      ┌──────────────┐      ┌────────────────────┐
│  UI         │ ──►  │  ROUTER      │ ──►  │  PROCESSORS         │
│ drop file + │      │ which tool?  │      │ his 4 scripts, now  │
│ paste email │      │ (see below)  │      │ callable functions  │
│ text        │ ◄──  │              │ ◄──  │ → finished .xlsx    │
└─────────────┘      └──────────────┘      └────────────────────┘
```

### UI — use **Streamlit** (recommended)
Fastest path to an iLovePDF-style local tool: drag-drop upload, file download, and a text box
are all built in; it's one Python file; it runs on localhost with one command; no HTML/JS.
*(Gradio is a fine alternative if he wants pure file-in/file-out with no dashboard. Pick
Streamlit if he wants to see all his tools on one page.)*

### Router — three options, ordered by how clean the compliance story is
1. **Manual pick (start here).** A dropdown or buttons: *"This is a Type-X file"* → runs script
   X. Zero intelligence, zero egress, fully auditable. For only 4 scripts this already removes
   the terminal and the file-shuffling — which is 90% of the pain. **Ship this first.**
2. **Rule-based auto-routing.** The app peeks at filename / sheet names / column headers /
   keywords in the pasted email text and matches to a script via a small config table. Still
   100% local and deterministic — he can explain exactly why a file went where.
3. **Bedrock classification.** Only if files are genuinely ambiguous and rules can't separate
   them. Sends content to Bedrock; same governance question as Type B. Avoid unless 1 and 2
   fail.

Build #1 first, add #2 only if he gets tired of picking.

### Processors — refactor the 4 scripts
Each script becomes a clean, importable function with **no hardcoded paths and no terminal
interaction**:

```python
def process(input_path: str, email_text: str = "", **params) -> str:
    """
    Run this tool on one input file.

    input_path : path to the uploaded file (temp file the app created)
    email_text : the free text the user pasted from the email (may be "")
    params     : any structured inputs the UI collected (dates, names, flags…)
    returns    : path to the finished .xlsx the app should hand back
    """
    ...
    return output_path
```

Same signature for all four → the app treats them uniformly. (If a Type-A script doesn't need
`email_text`, it just ignores it.)

---

## 6. Build plan (do in this order, check in with the user at each ✋)

1. **Interview (§2) + inspect scripts (§3).** ✋ Confirm the per-script Type A/B classification
   and the environment-policy answers before writing code.
2. **Confirm pip + localhost-server are allowed (§2 Q5).** ✋ If unsure, stop and have him ask
   IT. Do not proceed past this without it.
3. **Refactor script #1** into the `process(...)` signature. Test it from a tiny standalone
   Python call (not the web app yet) on a real sample file.
4. **Stand up a minimal Streamlit app** wrapping *only* script #1: one file uploader, one text
   box, a Run button, a download button. Get the full loop working for one tool.
5. **Refactor the remaining 3 scripts** the same way and register them in the app.
6. **Add the router.** Manual-pick first. Add rule-based only if he wants it.
7. **Add logging + error handling (§9).**
8. **Add the local launcher (§8).**
9. ✋ Walk through it together with real files. Then stop — resist scope creep into email
   reading or deployment (§10).

---

## 7. Starter skeleton (`app.py`)

A minimal, honest starting point. Adapt to the real scripts.

```python
import os
import tempfile
import datetime
import logging
import streamlit as st

# --- import the refactored tools (each exposes process(input_path, email_text, **params)) ---
# from tools.tool_pnl import process as run_pnl
# from tools.tool_recon import process as run_recon
# ... etc.

logging.basicConfig(
    filename="automation_hub.log",
    level=logging.INFO,
    format="%(asctime)s | %(message)s",
)

# Registry of tools. Add one entry per refactored script.
TOOLS = {
    "Tool 1 — <describe>": {"fn": None, "needs_email_text": True},
    "Tool 2 — <describe>": {"fn": None, "needs_email_text": False},
    "Tool 3 — <describe>": {"fn": None, "needs_email_text": True},
    "Tool 4 — <describe>": {"fn": None, "needs_email_text": True},
    # e.g. "Tool 1 — Daily P&L": {"fn": run_pnl, "needs_email_text": True},
}

st.set_page_config(page_title="Excel Automation Hub", layout="centered")
st.title("Excel Automation Hub")
st.caption("Local only. Files never leave this machine unless a tool explicitly calls Bedrock.")

# --- Router: manual pick (start here). Swap for rules later if wanted. ---
choice = st.selectbox("Which task is this?", list(TOOLS.keys()))
tool = TOOLS[choice]

uploaded = st.file_uploader("Drop the input file", type=["xlsx", "xls", "csv", "pdf"])

email_text = ""
if tool["needs_email_text"]:
    email_text = st.text_area("Paste the relevant words from the email", height=140)

if st.button("Run", type="primary", disabled=uploaded is None):
    if tool["fn"] is None:
        st.error("This tool isn't wired up yet.")
    else:
        try:
            # write the upload to a temp file so the script gets a real path
            suffix = os.path.splitext(uploaded.name)[1]
            with tempfile.NamedTemporaryFile(delete=False, suffix=suffix) as tmp:
                tmp.write(uploaded.getbuffer())
                in_path = tmp.name

            with st.spinner("Processing…"):
                out_path = tool["fn"](in_path, email_text)

            logging.info(f"OK | tool={choice} | in={uploaded.name} | out={os.path.basename(out_path)}")
            st.success("Done.")
            with open(out_path, "rb") as f:
                st.download_button(
                    "Download finished Excel",
                    data=f.read(),
                    file_name=os.path.basename(out_path),
                    mime="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
                )
        except Exception as e:
            logging.error(f"FAIL | tool={choice} | in={getattr(uploaded,'name','?')} | err={e}")
            st.error(f"Something went wrong: {e}")
            st.info("The file was not modified. Check the input and try again.")
```

> If a tool needs Bedrock (Type B), make the call **inside** that tool's `process()` using the
> company's normal Bedrock client/credentials — keep it isolated so the rest of the app stays
> purely local, and so it's obvious in code review which tools touch the network.

---

## 8. Running it locally

Bind to localhost only. Create a `.streamlit/config.toml`:

```toml
[server]
address = "127.0.0.1"
port = 8501
headless = true
```

Then a `run.bat` he can double-click (Windows):

```bat
@echo off
cd /d "%~dp0"
streamlit run app.py
```

It opens in his browser at `http://127.0.0.1:8501`. Nothing on the network can reach it.

---

## 9. Compliance & safety checklist (do not skip)

- [ ] Confirmed `pip install` is permitted, or used only pre-approved packages.
- [ ] Confirmed running a localhost web server is permitted (DLP/endpoint tools won't flag it).
- [ ] Bound to `127.0.0.1` only. Never `0.0.0.0`. No deployment, no public URL.
- [ ] **Logging:** every run records timestamp, which tool, input filename, output filename,
      success/failure — to a **local** log file. (Cheap, and valuable if anyone ever asks what
      the tool does. Don't log file *contents* or sensitive values — just metadata.)
- [ ] **No inbox access in v1.** Attachments are dragged in manually.
- [ ] **Egress is explicit:** the only way data leaves the machine is a Type-B/Bedrock tool,
      and the user has agreed to that per §2 Q6. If everything is Type A, the app is fully
      offline — say so plainly, it's the strongest compliance story.
- [ ] Temp files: clean them up; don't leave uploaded sensitive files scattered in temp dirs.
- [ ] This is a **personal productivity tool.** If it becomes load-bearing for the desk, that's
      the moment to loop in IT/Compliance for proper sanctioning — an unsanctioned tool that
      quietly becomes critical, or errs in a regulated workflow, is a real risk.

---

## 10. Out of scope for v1 (resist scope creep)

- Reading the mailbox / auto-pulling attachments.
- Deploying anywhere, multi-user access, or putting it on a shared server.
- Letting Claude orchestrate freely over desk systems.
- Anything touching MNPI, client PII, or trade data beyond what already passes through the
  existing 4 scripts.

Get the local drag-drop-and-route loop solid first. Everything else is a separate conversation
(and, past a point, a compliance conversation).
