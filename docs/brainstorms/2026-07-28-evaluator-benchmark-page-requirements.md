---
date: 2026-07-28
topic: evaluator-benchmark-page
---

# Evaluator Benchmark Page Requirements

## Summary

Add a bounded local browser demonstration of the existing personalized-email campaign path. Evaluators can vary deterministic synthetic workload inputs and compare application-owned plain-text and HTML rendering while the CLI remains the authoritative one-million-record benchmark.

---

## Problem Frame

The CLI already supplies the reproducible correctness and performance proof, but it requires an evaluator to work from terminal commands and inspect machine-oriented output. A small browser surface can make the same safe in-process work easier to explore without turning the assessment into an email-delivery, infrastructure, or campaign-authoring product.

The browser must not create a second interpretation of completion, accounting, privacy, or plain-text rendering. Its evidence is an interactive demonstration on the evaluator's current machine, not a replacement for the documented CLI benchmark procedure and environment.

---

## Actor

- A1. Evaluator: Selects bounded synthetic benchmark inputs, inspects one application-owned message preview, runs or cancels the demonstration, and interprets its safety and accounting evidence.

---

## Key Flows

- F1. Inspect and run a demonstration
  - **Trigger:** A1 opens the page or submits valid benchmark controls.
  - **Actor:** A1
  - **Steps:** The page shows the fixed safety statement and deterministic preview; A1 selects count, seed, worker count, and format; the selected format is rendered and consumed for every eligible synthetic recipient; the page returns the existing campaign report plus separately labeled page metadata.
  - **Outcome:** A1 can inspect trustworthy accounting and local performance evidence without supplying recipient data or initializing optional services.
  - **Covered by:** R1-R14, R17-R20, R23-R27
- F2. Cancel an active demonstration
  - **Trigger:** A1 cancels an enhanced-page run, or disconnects from a non-enhanced submission.
  - **Actor:** A1
  - **Steps:** New admission stops under the existing cancellation contract, started work receives the existing bounded settlement behavior, and remaining work is reconciled using existing accounting semantics.
  - **Outcome:** A privacy-safe interrupted report is produced when the client remains available to receive it; disconnected work does not continue unseen.
  - **Covered by:** R15, R21-R24

---

## Requirements

**Controls and bounds**

- R1. The page must accept only four benchmark controls: synthetic recipient count, deterministic fixture seed, worker count, and template format.
- R2. Count must accept integers from 1 through 1,000,000 inclusive. The browser ceiling is a demonstration capability, not authoritative large-run benchmark evidence.
- R3. Fixture seed must accept the full unsigned 64-bit range and must be included in visible result evidence.
- R4. Worker count must accept integers from 1 through the machine's reported logical CPU count inclusive. Results must show the effective worker count.
- R5. Template format must offer exactly plain text and HTML.
- R6. First load must default to count 100,000, seed 7, all reported logical CPUs, and plain text. Documentation must also include count 4 with seed 7 as the quick acceptance run that exercises named and fallback personalization.

**Rendering and preview**

- R7. Every eligible recipient must be fully rendered and accepted by the local digest sink in the selected format before counting as completed. Format selection affects measured campaign work, not preview alone.
- R8. Existing plain-text message bytes must remain byte-for-byte unchanged.
- R9. HTML may change presentation only. Its subject, named or fallback greeting, promotion wording, and personalization semantics must match the plain-text message.
- R10. The page must preview one fixed application-owned message for `Customer 000001`. Preview content must not depend on count, seed, worker count, or a completed run.
- R11. Changing template format must update the preview. Plain text must show the exact text message; HTML must show both a constrained rendered view and escaped source generated from the same application-owned message.
- R12. HTML preview content must contain no evaluator-supplied markup, scripts, forms, remote assets, or active external links.

**Evidence and compatibility**

- R13. The page must show campaign-processing elapsed time with the existing timing scope and a separately labeled total server-request duration. The two values must not be combined into one benchmark claim.
- R14. Total server-request duration must begin after successful validation and end when the response representation is ready. It must explicitly exclude browser rendering and network-transfer time.
- R15. Existing outcomes, accounting categories, reconciliation identities, privacy-safe samples, and cancellation settlement semantics must remain unchanged for both formats.
- R16. An explicit plain-text response must preserve the existing compact report JSON bytes plus its trailing newline exactly. Selected format and total server-request duration may be exposed only as separate response metadata, not by changing that body.
- R17. The page must label its measurements as machine-specific interactive evidence and direct evaluators to the CLI procedure for authoritative one-million-record evidence.

**Interaction and failure behavior**

- R18. A non-HTMX valid submission must return the complete page with submitted controls, safety statement, preview, and completed report. The page must remain usable without JavaScript.
- R19. Invalid input must start no campaign work, preserve every submitted value, identify only the affected field or fields, and expose no raw parser, runtime, or internal error.
- R20. Only one demonstration may run at a time. A concurrent submission must be rejected immediately with a visible busy response; it must not run concurrently, replace the active run, or enter a queue.
- R21. During an enhanced-page run, A1 must have a visible cancellation action. A cancellation request with no applicable active run must return a visible conflict response and must not affect another request.
- R22. Without HTMX or JavaScript, navigating away or disconnecting is the cancellation mechanism. The page must state this limitation, and disconnected work must stop rather than continue unseen.
- R23. Completed success, partial-success, failure, and interrupted reports must remain evaluator-readable in full-page, enhanced-fragment, and explicit plain-text representations. Validation failures, cancellation conflicts, and busy responses must remain safe and readable in every applicable representation.
- R24. Response status evidence must distinguish completed reports, invalid input, cancellation conflicts, and concurrent-run refusal as success, bad request, conflict, and too many requests respectively.

**Safety boundary**

- R25. The following statement must appear verbatim and remain visible beside the benchmark controls: “Synthetic demonstration only. Uses deterministic .test recipients and an in-memory digest sink. No email is sent, no recipient data is accepted, and SMTP, Redis, Asynq, and the distributed ledger are not initialized. Use the CLI benchmark for authoritative one-million-record evidence.”
- R26. The browser workflow must never accept recipient data, arbitrary message content, arbitrary subjects, arbitrary HTML, SMTP settings, Redis settings, backend selection, or queue controls.
- R27. Starting, previewing, running, cancelling, validating, or reporting through the page must not instantiate SMTP, Redis, Asynq, or the distributed ledger and must not contact any network service apart from its local HTTP listener.

---

## Acceptance Examples

- AE1. **Covers R1-R6, R10-R11.** Given a first page load, the controls show count 100,000, seed 7, all reported logical CPUs, and plain text; the fixed `Customer 000001` preview shows the existing plain-text message exactly.
- AE2. **Covers R2-R9, R13-R17.** Given count 4, seed 7, a valid worker count, and plain text, when A1 runs the demonstration, four synthetic recipients are examined, rendered, accepted, and reconciled under existing accounting semantics; named and fallback evidence is present; campaign time and total server-request time are separate; the text message and explicit plain-text report bytes remain unchanged.
- AE3. **Covers R5, R7-R12, R15.** Given HTML format, when A1 runs the same deterministic fixture, every eligible recipient is rendered and digested as HTML while counts retain their existing meanings; the preview shows both constrained rendering and escaped source with the same subject, greeting branch, and promotion as plain text and no active or remote content.
- AE4. **Covers R18, R23.** Given HTMX and JavaScript are unavailable, when A1 submits valid controls, the response is a complete usable page containing preserved controls, safety statement, selected-format preview, and completed report.
- AE5. **Covers R19, R23-R24.** Given count above 1,000,000, a negative seed, workers above the reported logical CPU count, or an unsupported format, when A1 submits, no work starts; submitted values remain visible; only affected fields receive safe correction messages; and the response is identified as a bad request.
- AE6. **Covers R20, R23-R24.** Given one active run, when another valid run is submitted, the second request receives an immediate visible busy response identified as too many requests, while the active run is neither cancelled nor contended by another benchmark or hidden queue.
- AE7. **Covers R15, R21, R23-R24.** Given an active enhanced run, when A1 selects Cancel, new admissions stop and the final interrupted report reconciles completed, failed, and unprocessed work under existing semantics. Given no applicable active run, cancellation returns a visible conflict response and changes no campaign state.
- AE8. **Covers R18, R22.** Given a non-enhanced synchronous run, when A1 disconnects or navigates away, associated campaign work is cancelled and does not continue unseen; the page had disclosed that disconnect is the non-enhanced cancellation mechanism.
- AE9. **Covers R16.** Given explicit plain-text content negotiation, when an otherwise identical plain-text run completes, the response body is exactly the existing compact report JSON plus newline; selected format and total server-request duration are available only as separate response metadata.
- AE10. **Covers R25-R27.** Given poisoned or unreachable SMTP and Redis configuration, when A1 loads, previews, validates, runs, or cancels through the page, the fixed safety statement remains visible, no optional service is initialized or contacted, and no control accepts recipients, arbitrary content, transport, backend, or queue settings.

---

## Success Criteria

- An evaluator can safely explore deterministic campaign rendering, concurrency, accounting, and cancellation from a browser without supplying recipient data or external infrastructure.
- Plain-text campaigns and explicit plain-text reports remain byte-compatible with existing behavior.
- HTML demonstrates the same application-owned promotion and personalization semantics without introducing arbitrary or active content.
- Every displayed timing value has an honest, distinct scope, and the page cannot be mistaken for replacement evidence for the authoritative CLI benchmark.
- Planning can choose implementation details without inventing control bounds, preview behavior, fallbacks, validation, cancellation, concurrency, status semantics, safety wording, or acceptance evidence.

---

## Scope Boundaries

- No recipient input, file upload, arbitrary message content, arbitrary subject, arbitrary HTML, or campaign authoring.
- No SMTP settings, test-inbox delivery, Redis settings, Asynq execution, distributed-ledger use, backend selection, queue controls, or optional-service fallback.
- No external browser assets or network calls beyond the local HTTP listener.
- No parallel benchmark runs, queued submissions, cancel-and-replace behavior, detached runs, persisted history, status polling, or dashboard.
- No change to existing plain-text message bytes, compact report JSON body bytes, outcomes, accounting categories, reconciliation identities, privacy policy, or cancellation settlement semantics.
- No claim that browser measurements replace the documented CLI benchmark or constitute a hardware-independent SLA.

---

## Key Decisions

- Benchmark the selected format: format comparison represents real per-recipient render-and-digest work rather than a preview-only switch.
- Keep preview fixed: `Customer 000001` makes text bytes, HTML source, and screenshots deterministic without coupling preview identity to benchmark inputs.
- Separate timing domains: campaign processing remains comparable to existing evidence, while total server-request duration describes page-side server work without pretending to measure browser transfer or rendering.
- Reject concurrency: one active run prevents resource contention from making interactive timings misleading and avoids introducing queue semantics.
- Preserve exact plain-text bodies: new format and request metadata remain outside established message and report payloads.
- Keep the full safety boundary visible: evaluator safety must be explicit at the point of action, including named optional services that remain uninitialized.

---

## Dependencies / Assumptions

- The evaluator runs the page locally on a machine that reports at least one logical CPU.
- Deterministic `.test` fixtures, local digest acceptance, privacy-safe reporting, accounting reconciliation, and cancellation settlement continue to behave as established by the existing campaign workflow.
- The application owns both template representations and the fixed promotional content.
- The browser ceiling of one million is acceptable as an interactive capability because the page explicitly distinguishes its evidence from the authoritative CLI procedure.
