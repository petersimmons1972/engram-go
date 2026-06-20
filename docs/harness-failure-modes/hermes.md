# Hermes Self-Reported Failure Modes

**Harness:** Hermes (Claude-based AI assistant, fleet-dispatch `consult`/`review` lane consumer)
**Date:** 2026-06-20
**Method:** Dispatched via `fleet-consult.sh` (bus-only, no ssh)
**Fleet-dispatch item IDs (proof of bus delivery):**
- Primary consult (items 1-5): successful terminal response, no item_id surfaced by fleet-consult.sh for successful completions
- Continuation consult (items 5-10): `item_id=09141a42-4cbc-4bdf-9829-11bd5bdf00f2`

---

## Verbatim Self-Report

### 1. Plausible answer without live verification

I answer from model prior or stale memory when I should have used a tool, producing confident but wrong facts: current versions, file contents, system state, Git branches, dates, service health, or API behavior.

**Typical triggers:**
- The request sounds "simple" or familiar.
- Tool use would be slower than answering.
- I have durable memory about the user or environment and overgeneralize it to the current runtime.
- The fleet item asks for a status/check but does not explicitly say "run the command."

**Concrete bad output:** "Port 443 is open" or "the tests pass" without actually checking the live host/container.

**Mitigation:** Treat current facts, filesystem state, Git state, arithmetic, dates, versions, and service state as tool-mandatory. If I cannot verify, say so explicitly.

---

### 2. Inventing missing fleet-dispatch context

When a dispatch item references "the repo," "the failing job," "the previous step," "the ref," "the service," or "the target environment" but the actual context/ref payload is missing or inaccessible, I may fill the gap from memory, project conventions, or nearby clues and proceed against the wrong target.

**Typical triggers:**
- Dispatch item assumes hidden context that was not injected into my prompt.
- A ref name, branch, issue ID, or path is omitted.
- Multiple similarly named projects exist.
- Past-session memory contains a plausible but stale association.

**Concrete bad output:** I inspect or modify `/workspace/foo` because it looks likely, while the caller intended `/workspace/foo-api` from a missing ref.

**Mitigation:** Use session search, filesystem/Git discovery, or target listing when possible. If the missing context materially changes the action and cannot be recovered, stop with `NEEDS-INPUT:` rather than guessing.

---

### 3. Contract drift in final output format

I provide a helpful human-readable summary but violate the caller's expected machine contract: missing required sentinel lines, wrong final-line format, extra prose after a terminal marker, Markdown where JSON was expected, malformed JSON, wrong field names, or omitted branch/SHA lines after making a commit.

**Typical triggers:**
- Prompt contains both task instructions and output-contract instructions.
- The required format appears near the end of a long dispatch item.
- I focus on solving the task and summarize naturally.
- A "final line exactly … and nothing after it" instruction competes with my habit of adding explanations.

**Concrete bad output:** Ending with natural prose instead of the required `NEEDS-INPUT:` sentinel.

**Mitigation:** Before final response, explicitly check required output schema, required sentinel lines, and final-line constraints. If JSON is requested, produce only valid JSON.

---

### 4. Silent omission under breadth pressure

I answer only the most salient parts of a multi-part request and silently omit required coverage areas, edge cases, verification details, or caller-specified minimum sections.

**Typical triggers:**
- User asks for "6-10 items," "cover at minimum," or multiple lettered requirements.
- The answer is long and I compress for readability.
- I optimize for concision and accidentally drop a required class of failure.
- Context window pressure causes earlier requirements to fade.

**Concrete bad output:** Listing hallucination failure modes but omitting truncation/format-contract failures despite the prompt explicitly requiring them.

**Mitigation:** Convert requirements into a checklist internally and map each final section back to it. Prefer slightly redundant coverage over elegant brevity when the prompt is a catalog/specification task.

---

### 5. Over-trusting subagents or tool summaries

Treating delegated summaries as verified facts instead of checking artifacts, outputs, IDs, files, or external state.

---

### 6. Premature closure

Stopping after a plausible answer rather than continuing until the task is complete, tested, and verified.

---

### 7. Context drift

Losing the user's exact constraints, requested format, or prior decisions while pursuing implementation details.

---

### 8. Tool-use theatre

Calling tools without extracting decisive evidence, or summarizing noisy output instead of making a grounded judgment.

---

### 9. Unsafe cleanup assumptions

Either leaking ephemeral resources or over-cleaning shared Docker/Kubernetes infrastructure outside the session's scope.

---

### 10. Polished hallucination

Producing confident, well-structured prose that masks uncertainty, missing evidence, or unverified assumptions.

---

## Coordinator Assessment

**Genuinely novel findings vs. known catalog:**

- Items 1 (plausible answer without live verification) and 10 (polished hallucination) map closely to FM-70 (trust calibration) and FM-87 (terminal status != deliverable). Known pattern, confirmed from the inside.
- Item 2 (inventing missing fleet-dispatch context) is a concrete articulation of FM-66 (side-channel delivery key) from the consumer's perspective: when the ref/context is missing, Hermes fills it from priors rather than failing loud. **This is new precision** — FM-66 was catalogued as a server-side routing bug; Hermes adds that the consumer-side failure mode is silent context-invention rather than a clean `NEEDS-INPUT:`.
- Item 3 (contract drift in output format) matches FM-84 (client violates server wire contract). Known; confirmed self-aware.
- Item 4 (silent omission under breadth pressure) and the multi-part checklist mitigation is the most **operationally actionable** item here. No existing FM captures this at the agent-output level (FM-67 captures truncation at the pipeline level, not the cognition level).
- Item 8 (tool-use theatre — calling tools without acting on evidence) is **genuinely novel** in the catalog. Existing FMs cover not calling tools (FM-70) and over-trusting results (FM-87), but the specific pattern of performing tool calls theatrically without grounding the conclusion on what the tool returned is a distinct failure shape not previously named.
- Item 9 (unsafe cleanup assumptions) matches FM-16/class H. Known.

**Net new signal:** Items 2 (consumer-side context-invention), 4 (agent-level breadth omission), and 8 (tool-use theatre) surface failure shapes not previously named in the catalog. Item 8 in particular warrants a future catalog entry.
