# Bounded evaluator prompts

This page contains the substantive prompts used to steer the root session. Standalone continuation messages and host-generated reminders are omitted here, but remain in the complete JSON archive.

## Compound Engineering: requirements brainstorm

`2026-07-28T08:08:05Z` · `/ce-brainstorm`

<details open>
<summary>Prompt</summary>

<pre>I need to add a small evaluator-facing benchmark page to this existing Go personalized-email pipeline.

The settled product direction is:
- the page runs only deterministic synthetic recipients;
- it uses the existing in-process campaign path and local digest sink;
- it exposes only count, fixture seed, worker count, and template format;
- it previews one application-owned promotional message;
- it supports plain-text and HTML templates;
- it cannot accept recipient data, arbitrary content, SMTP settings, Redis settings, or backend selection;
- it must never instantiate SMTP, Redis, Asynq, or the distributed ledger;
- the existing one-million-record CLI benchmark remains the authoritative large-run proof;
- the browser page is a bounded interactive demonstration.

Help me define the remaining WHAT before planning HOW.

Requirements for this brainstorm:
- Ask one question at a time.
- Challenge only decisions that affect evaluator-visible behavior, safety, or acceptance evidence.
- Resolve benchmark bounds, elapsed-time scope, preview behavior, non-HTMX fallback, validation responses, cancellation, concurrent requests, and the exact safety statement shown to the evaluator.
- Preserve existing plain-text output bytes and accounting semantics.
- Keep package layout, handlers, helper names, and exact APIs out of the requirements document.
- Keep SMTP, Redis, queue controls, arbitrary recipients, arbitrary subjects, and arbitrary HTML out of scope.
- Produce a concise requirements document under docs/brainstorms/ with acceptance examples, scope boundaries, assumptions, and non-goals.</pre>

</details>

## Compound Engineering: implementation plan

`2026-07-28T09:40:10Z` · `/ce-plan`

<details open>
<summary>Prompt</summary>

<pre>@docs/brainstorms/2026-07-28-evaluator-benchmark-page-requirements.md 
Build an implementation plan for the approved HTMX benchmark-page requirements. The existing repository is the authority for command structure, campaign APIs, report accounting, fixture generation, distributed payloads, SMTP delivery, and test style.

Keep the design small:
- Go net/http and server-rendered HTML;
- HTMX for the primary interaction;
- a pinned local HTMX asset served from go:embed;
- no JavaScript framework, Node toolchain, CDN, or external browser asset;
- minimal custom JavaScript;
- ordinary HTML form fallback.

Plan the work in dependency order:
1. a typed text/html template format at the shared rendering boundary;
2. propagation through local run configuration and CLI parsing;
3. backward-compatible propagation through distributed task payloads and workers;
4. per-delivery SMTP MIME selection without mutable global state;
5. the synthetic-only HTTP command and embedded assets;
6. evaluator-facing design states and responsive behavior;
7. automated, browser, CLI, and documentation evidence.

Preserve these contracts:
- omitted and explicit text formats produce the existing plain-text bytes;
- old distributed payloads without a template field decode as text;
- default text payloads omit the optional field when compatibility requires it;
- HTML enqueueing is disabled until every worker in the deployment understands the optional template field; deploy workers first and producers second;
- queue names, task IDs, Redis keys, ledger scripts, campaign snapshots, and accounting semantics do not change;
- the browser never reaches SMTP, Redis, Asynq, or the distributed ledger;
- invalid template input fails before optional SMTP or Redis configuration is read.

Give each implementation unit repo-relative files, dependencies, patterns to follow, behavioral test scenarios, and observable verification. Include request cancellation with a partial report, one-run server admission, loopback-only binding, bounded bodies and inputs, HTTP timeouts, no-store headers, HTMX non-2xx swapping, safe error output, HTML escaping, isolated preview rendering, keyboard use, reduced motion, and responsive checks at 375px, 768px, and 1280px.

Keep template format out of the shared serialized campaign report unless an existing contract requires it there. The HTTP result view model owns the displayed format. If discovery proves the shared report must change, the plan must name the wire-compatibility impact and tests explicitly.

Do not write application code. Ask only when a decision changes approved scope or cannot be resolved from the requirements and current source.</pre>

</details>

## Compound Engineering: implementation

`2026-07-28T10:19:34Z` · `/ce-work`

<details open>
<summary>Prompt</summary>

<pre>@docs/plans/2026-07-28-001-feat-evaluator-benchmark-page-plan.md 
Implement the approved HTMX benchmark-page plan end to end.

Execution requirements:
- Read the affected symbols, callers, and tests before editing.
- Follow the plan's implementation units and scope boundaries.
- Preserve the default plain-text bytes and all existing accounting behavior.
- Use typed template values internally and validate untrusted values at their boundaries.
- Reuse campaign and generated-fixture APIs directly. Do not invoke the CLI as a subprocess or parse its output.
- Keep browser execution synthetic-only and in process.
- Serve the pinned HTMX distribution and all other browser assets locally through go:embed.
- Return rendered HTML fragments for HTMX requests and usable complete HTML for normal submissions.
- Keep custom JavaScript to the single HTMX non-2xx swap listener unless browser evidence proves another small script is necessary.
- Use request-scoped source, sink, and benchmark state, with server-wide admission limited to one active benchmark.
- Reject non-loopback bind addresses.
- Use the fixed benchmark timing interval from the approved requirements.
- Return canceled runs as partial reports with trustworthy accounting.
- Apply bounded request bodies, bounded controls, context cancellation, safe methods, no-store headers, and sensible server timeouts.
- Keep errors free of recipient values, environment values, credentials, internal paths, parser internals, and panic details.
- Do not weaken tests or safety guards to make checks pass.

Use the approved `DESIGN.md` produced by `ce-frontend-design` for the page implementation. Use the Humanizer skill in a technical voice for interface copy, CLI help, validation messages, README changes, and the final report.

Run targeted tests during implementation. Finish with formatting, the full test suite, the race detector, vet, build, diff checks, manual CLI use, and real browser verification.

Stop only when the approved behavior works through the built CLI and browser surfaces, review findings are resolved, and the required evidence is recorded.</pre>

</details>

