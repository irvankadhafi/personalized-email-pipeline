---
date: 2026-07-26
topic: personalized-email-campaign
updated: 2026-07-27
---

# Personalized Email Campaign Requirements

## Summary

Define a safe Go CLI that proves one million personalized campaign messages can be processed correctly, reproducibly, and measurably without external infrastructure. The unchanged local dry run remains authoritative, while independently selected guarded test-inbox delivery and Asynq/Redis execution provide optional, narrower demonstrations without becoming prerequisites for core evaluation.

---

## Problem Frame

The assessment asks for the same promotional message to be personalized and processed for 1,000,000 customer email addresses as quickly as reasonably possible. It must not send email to real recipients, and the evaluator needs enough evidence to distinguish actual personalization work from a validation-only or no-op throughput claim.

The submission must balance two concerns. It should be easy and safe for an evaluator to run locally, while also demonstrating sound engineering judgment around malformed data, duplicate recipients, partial failure, cancellation, performance, and production-oriented extensions. Overbuilding a production campaign platform would increase setup and review cost without strengthening the core proof.

---

## Actors

- A1. Evaluator: Runs the required workflow, inspects its output, and reproduces correctness and performance evidence.
- A2. Operator: Supplies or generates recipient data, starts or cancels a campaign run, and interprets its terminal outcome.
- A3. Test delivery environment: An optional sandbox or designated test inbox that can receive deliberately enabled test messages without contacting campaign recipients.

---

## Key Flows

- F1. Required local dry run
  - **Trigger:** A1 or A2 starts a campaign using supplied recipient data or a generated fixture.
  - **Actors:** A1, A2
  - **Steps:** The solution accepts records, validates and normalizes them, excludes invalid and duplicate records, personalizes the message for each eligible recipient, consumes each rendering through a no-network sink, and emits a terminal report.
  - **Outcome:** Every examined record is accounted for, no network delivery occurs, and the report provides correctness and performance evidence.
  - **Covered by:** R1-R15
- F2. Graceful cancellation
  - **Trigger:** A2 requests cancellation while a run is active.
  - **Actors:** A2
  - **Steps:** The solution records the operator interrupt, stops admitting new recipient work within a documented response interval, allows already-started work to settle until a documented deadline, classifies unsettled started work as failed due to interruption and never-started work as unprocessed, and emits a terminal report.
  - **Outcome:** The run ends as interrupted with trustworthy accounting rather than appearing successful or losing records silently.
  - **Covered by:** R10-R12
- F3. Reproducible million-record demonstration
  - **Trigger:** A1 generates and processes a deterministic fixture using declared generation parameters.
  - **Actors:** A1
  - **Steps:** The solution recreates the same ordered fixture and expected summary, processes it through the required dry-run path, separates fixture-generation measurements from campaign-processing measurements, and reports the stated execution environment.
  - **Outcome:** A1 can independently repeat both the correctness result and the measured performance claim.
  - **Covered by:** R2, R12-R15
- F4. Guarded test-inbox demonstration
  - **Trigger:** A1 or A2 deliberately selects the test-inbox sink, supplies its external prerequisites, and explicitly confirms the guarded send.
  - **Actors:** A1, A2, A3
  - **Steps:** Before campaign work begins, the solution verifies that the run contains at most 10 synthetic recipients, validates an independently configured allowlisted destination, confirms the guarded send, and rejects any supplied-recipient personal data. It then renders and submits each eligible synthetic message to the test inbox and reports delivery acceptance separately from dry-run acceptance.
  - **Outcome:** Every completed item in the bounded run was accepted by the designated test inbox without contacting or disclosing campaign recipients, and any missing guard causes failure before campaign work or network delivery.
  - **Covered by:** R16, R18
- F5. Optional distributed execution
  - **Trigger:** A1 or A2 deliberately selects the Asynq backend and supplies a reachable Redis service.
  - **Actors:** A1, A2
  - **Steps:** The operator starts Redis and one or more separate worker processes, then the campaign command submits benchmark-selected bounded work units from a deterministic synthetic fixture, waits for distributed terminal completion, tolerates retry and duplicate execution through application-owned idempotency, handles Redis and worker lifecycle failures explicitly, and emits the existing reconciled terminal report with distributed measurements.
  - **Outcome:** Distributed execution can be evaluated without changing local-mode behavior, claiming exactly-once execution, or requiring a separate campaign-status product surface.
  - **Covered by:** R17-R18

---

## Requirements

**Required input and recipient semantics**

- R1. The required workflow must accept an evaluator-supplied recipient file in a documented format whose records contain a required email address and an optional recipient name.
- R2. The solution must also generate synthetic recipient fixtures of a requested size. The same declared seed, record count, and generation options must reproduce the exact ordered records and expected summary. The documented assessment fixture must include both usable-name and missing-name recipients so both personalization branches are reproducibly exercised.
- R3. A missing or blank name must not invalidate an otherwise valid record. Personalization must use one documented, deterministic, neutral fallback salutation and must not infer a name from the email address.
- R4. Email identity must use conservative normalization: surrounding whitespace is ignored and comparison is case-insensitive. Provider-specific dot removal, plus-tag removal, alias resolution, or equivalent provider-aware rewriting must not occur.
- R5. A record that cannot provide an email address meeting one documented, conservative syntax-validity rule must be classified as invalid, skipped, and included in reason-grouped reporting without stopping valid records from being processed. Validity must not depend on network-based mailbox, domain, or deliverability verification. A malformed record is recoverable only when its boundary is known and processing can safely resume at the next record; an unreadable source or corruption that makes record boundaries untrustworthy is a run-fatal input error rather than an invalid record.
- R6. Within one run, only the first valid record for a normalized email identity is eligible. Later valid occurrences must be classified as duplicates and skipped. An invalid occurrence must not claim the identity or suppress a later valid occurrence.

**Safe campaign processing**

- R7. The same promotional message must be personalized for every eligible recipient, using either the recipient's usable name or the fixed fallback salutation.
- R8. The default mode must perform no network delivery and must require no Redis service, database, cloud infrastructure, email provider, or test inbox.
- R9. An eligible recipient may count as completed in the required dry run only after the full personalized message has been rendered and accepted by the no-network dry-run sink. Validation alone must not count as completion, and persisting every rendered message must not be required.
- R10. An active run must treat receipt of the operator interrupt as the cancellation request point. It must stop admitting new recipient work within a documented maximum response interval and allow already-started work to settle only until a documented deadline measured from that request. Started work that does not settle by the deadline must be classified as failed due to interruption; eligible work that never started must be classified as unprocessed. Both bounds must be visible in run configuration or reporting, while their exact default values are deferred to planning.

**Outcomes, accounting, and evidence**

- R11. Every terminal run must have exactly one clear outcome: success, partial success, failure, or interrupted. Success requires at least one eligible recipient, every eligible message completed, and no invalid records; duplicate skips alone do not downgrade success. Partial success requires at least one eligible recipient and trustworthy terminal accounting, but includes invalid records or eligible-message failures; a run in which all eligible messages fail may therefore be partial success if its accounting remains trustworthy. Empty input and non-empty input with zero eligible recipients are failures. Failure also applies when the run cannot meaningfully start, proceed, or produce trustworthy terminal accounting. Interrupted applies to operator cancellation.
- R12. The terminal report must distinguish total examined, invalid, duplicate, eligible, completed, failed, and unprocessed counts. Each examined record must belong to exactly one of invalid, duplicate, or eligible, and each eligible record must belong to exactly one of completed, failed, or unprocessed, enforcing `examined = invalid + duplicate + eligible` and `eligible = completed + failed + unprocessed`. It must group invalid and failed records by reason. A fatal input error before any accepted record must report failure with no campaign work; one after an accepted prefix must report failure, preserve trustworthy prefix counts, label accounting as prefix-only, and must not claim full-input reconciliation.
- R13. Privacy protections must apply to every user-visible or persisted output surface, including terminal reports, rendered and failure samples, progress output, parser and runtime errors, crash diagnostics, optional-mode diagnostics, and benchmark artifacts. By default, none may contain a recoverable supplied-recipient email address, cleartext supplied-recipient name, or raw personalized body from supplied data. Record-level diagnostics must remain bounded, use deterministic masked placeholders or synthetic exemplars, include aggregate reason counts and the number of omitted details, and preserve enough distinction to demonstrate named and fallback behavior. Exact masking format and sample bounds are deferred to planning.
- R14. The required dry run must include a small, bounded, deterministic, privacy-safe sample of renderings for each personalization category represented in that run, without storing every rendered message. The documented deterministic assessment fixture must demonstrate both named and fallback personalization; arbitrary supplied input is not required to contain both categories.
- R15. A completed performance demonstration must report campaign-processing elapsed time, stage counts, peak memory, input throughput as examined records per second, and completion throughput as completed personalized renderings accepted by the dry-run sink per second. Both rates use the same campaign-processing elapsed-time denominator, completion throughput is the primary campaign-performance claim, and comparisons must disclose invalid, duplicate, failed, and unprocessed proportions. Synthetic fixture-generation time must be reported separately. If one invocation includes both generation and processing, its total end-to-end elapsed time must also be reported separately. The repository must document a repeatable one-million-record procedure and the execution environment; no hardware-independent completion-time SLA is required.

**Optional delivery and execution**

- R16. Test-inbox delivery may be selected with either backend, but only after refusal-first validation of an additional deliberate operator confirmation, an independently configured allowlisted destination, and a deterministically generated synthetic fixture containing no more than 10 records. The destination must not be inferred from fixture contents, provider configuration, or any supplied campaign address, and the run must reject evaluator-supplied recipient files rather than attempt to prove that they are synthetic. Failure of any preflight guard produces `failure` with zero campaign work, zero delivery attempts, a privacy-safe static refusal reason, and no fallback or truncation. For a permitted run, each eligible recipient counts as completed only after the full synthetic personalized message receives confirmed test-inbox acceptance; rejection or definitively failed delivery counts as failed. Ambiguous transport acceptance counts as failed with a distinct indeterminate-delivery reason and must not be retried automatically. The report must distinguish confirmed test delivery from dry-run sink acceptance, and the mode must not claim exactly-once delivery.
- R17. Asynq/Redis execution may be used with either sink, only with a deterministic fixture generated by the solution, and only after the operator separately starts Redis and at least one worker process. Evaluator-supplied recipient files must be refused before campaign work so task payloads and Redis state contain only synthetic fixture values, non-sensitive campaign identifiers, and bounded execution metadata. The campaign command must submit work and wait for distributed terminal completion, but must not silently start an in-process worker; detached submission and a separate status-retrieval flow are outside scope. Distributed correctness must assume at-least-once task execution: application-owned idempotency must ensure that enqueue duplication, retry, worker crash, uniqueness-lock expiry, and redelivery cannot double-count a recipient or repeat its committed effect. For test-inbox work, the campaign ledger must atomically reserve each intended synthetic delivery before SMTP activity and own its terminal delivery result; a reservation is never reclaimed for automatic retry, and an unresolved reservation is settled as indeterminate so no intended message is submitted more than once. Duplicate attempts and retry exhaustion must remain observable, and retries must distinguish transient infrastructure failures from permanent input or configuration failures.

  The waiting CLI must have a documented distributed completion deadline independent of worker shutdown timeout. Expiry without an operator interrupt produces `failure`; an operator interrupt produces `interrupted` only when accounting remains trustworthy, while loss of trustworthy Redis state takes precedence as `failure`. Redis failure before any acknowledged enqueue produces failure with zero campaign work. Redis failure after a trustworthy acknowledged prefix produces failure with prefix-only accounting. If enqueue acknowledgment or terminal task state becomes indeterminate, the report must stop at the last trustworthy prefix, disclose the omitted or indeterminate remainder separately, and must not classify unknown work as completed, failed, or unprocessed. Retry exhaustion with trustworthy state classifies the affected eligible work as failed and otherwise follows the existing outcome rules.

  Worker shutdown must stop fetching new tasks, give active handlers a bounded settlement interval, and leave unfinished acknowledged tasks available to another worker. Campaign cancellation must classify eligible work not yet granted an attempt as unprocessed and allow started work to settle only until the campaign deadline; unsettled started work is failed due to interruption. Queued, scheduled, or retryable records may remain physically present, but every later handler attempt must observe durable closed-campaign state before rendering or sink work and terminate without changing terminal counts. Task granularity, worker concurrency, retry limits, settlement values, and batching must remain benchmark-selected. The smallest demonstration uses one queue unless at least two measured workload classes have a documented fairness or latency objective; when multiple queues are justified, weighted selection is the default, and strict priority requires evidence that its starvation trade-off is acceptable.
- R18. Sink and backend remain independent concepts exposed as `--sink=dry-run|test-inbox` and `--backend=local|asynq`; all four combinations are supported. Omitting both selectors is equivalent to `--backend=local --sink=dry-run`, which remains the unchanged authoritative million-record benchmark and must require no Redis, network access, optional credentials, or optional runtime initialization. Every combination must preserve applicable recipient, personalization, safety, privacy, accounting, and outcome semantics. The Asynq test-inbox combination retains at-least-once campaign execution while using the durable reservation in R17 to make external SMTP submission at most once per intended synthetic message across retries, duplicate execution, worker crash, uniqueness expiry, and redelivery. The hard limit of 10 intended deliveries applies to the entire invocation, not each attempt. Delayed work after campaign closure is inert, indeterminate delivery is terminal without automatic retry, and no backend or sink may silently fall back. When distributed state remains trustworthy, R12's reconciliation identities apply in full or to the explicitly labeled trustworthy prefix. When R17 requires an indeterminate remainder, the report must instead label distributed accounting untrustworthy, report known-enqueued, known-terminal, and unknown-task totals separately, and make no R12 reconciliation claim over unknown work. Distributed evidence must add enqueue, queue-wait, processing, retry, duplicate-attempt, effective-concurrency, and end-to-end measurements without replacing the local benchmark. Candidate operational values must be compared by completed useful work per second subject to zero accounting violations, bounded queue growth and memory, successful retry and duplicate recovery, and no starvation during the benchmark window; tested candidates and the selected trade-off must be disclosed without claiming production-optimal values. Optional credentials and Redis connection details must be supplied per environment, documented by purpose and least privilege, and excluded from task payloads, logs, reports, benchmark artifacts, and committed files.

---

## Acceptance Examples

- AE1. **Covers R1, R3, R7-R9, R11-R15.** Given a supplied file containing only valid, unique recipients, when the evaluator runs the default mode, every recipient is rendered and consumed without network activity; the run reports success, balanced reconciliation identities, privacy-safe content samples for the categories present, and measured campaign-processing evidence.
- AE2. **Covers R2, R3, R7, R13, R14.** Given the documented deterministic assessment fixture includes one recipient with a usable name and one with a missing or blank name, when both are processed, privacy-safe deterministic content samples distinguish the named greeting from the documented neutral fallback without exposing supplied-recipient PII.
- AE3. **Covers R4, R6, R11, R12.** Given two valid records whose email addresses differ only by surrounding whitespace or letter case, when the run completes, the first valid occurrence is processed, the second is counted as a duplicate, and the duplicate alone does not downgrade success.
- AE4. **Covers R4, R6.** Given two otherwise valid addresses that differ by a plus-tag or provider-specific dot placement, when the run processes them, it does not merge them using provider-specific assumptions.
- AE5. **Covers R5, R6, R11-R13.** Given a recoverably malformed record and an unrelated valid record, when the run completes, the malformed row is reported as invalid, the valid row remains eligible, and the run reports partial success. The malformed row does not claim any deduplication identity.
- AE6. **Covers R5, R11-R13.** Given a large input containing many malformed rows across several reasons, when processing reaches a terminal state, valid unique recipients still complete, the outcome is partial success, reason totals remain complete, record-level samples remain within the configured bound, and the report states how many details were omitted.
- AE7. **Covers R9, R11, R12.** Given a dry-run sink that cannot accept some eligible renderings, when the run reaches a trustworthy terminal state, accepted renderings count as completed, rejected renderings count as failed, and the outcome is partial success rather than success.
- AE8. **Covers R10-R12.** Given a workload that remains active beyond the documented cancellation response interval and settlement deadline, when the process receives the operator interrupt, new admissions stop within the response bound, started work settles only until the deadline, unsettled started work is failed due to interruption, never-started eligible work is unprocessed, both bounds are visible, reconciliation identities balance, and the terminal outcome is interrupted.
- AE9. **Covers R2, R15.** Given identical fixture seed, count, and generation options on repeated runs, when fixtures are generated, their ordered records and expected summaries are identical. Generation time is separate from campaign-processing time, and an invocation performing both also reports its end-to-end time.
- AE10. **Covers R2, R8, R9, R12-R15.** Given the documented one-million-record fixture procedure on a stated environment, when the evaluator runs the required path without external services, the terminal report balances all reconciliation identities and provides processing elapsed time, examined-record input throughput, completed-rendering throughput, stage counts, peak memory, disclosed invalid/duplicate/failed/unprocessed proportions, and bounded privacy-safe content evidence.
- AE11. **Covers R8, R13, R16, R18.** Given no explicit test-inbox selection or no additional confirmation, when any workflow runs, no message is sent over the network. Given test-inbox delivery is selected without an independently allowlisted destination, with a destination present in campaign input, beyond the hard send limit, or with supplied-recipient personal fields in the payload, the solution refuses all delivery rather than weakening a guard or silently falling back.
- AE12. **Covers R17-R18.** Given Redis is unavailable, when the evaluator runs the default local dry run and million-record benchmark, core correctness and performance evaluation remain unchanged and available. Distributed execution is attempted only after the evaluator explicitly selects the Asynq backend, and its failure never silently changes the selected backend.
- AE13. **Covers R11, R12.** Given empty input or non-empty input whose records produce zero eligible recipients, when processing terminates, the outcome is failure and no campaign progress is claimed. Given at least one eligible recipient and every eligible message fails with trustworthy balanced accounting, the outcome is partial success with zero completions.
- AE14. **Covers R12.** Given a report whose individual counts are non-negative but violate either required reconciliation identity, when the evaluator checks the terminal evidence, the report is not considered trustworthy or compliant.
- AE15. **Covers R5, R11, R12.** Given corruption before any accepted record makes record boundaries untrustworthy, when processing terminates, the outcome is failure with no campaign work. Given the same fatal condition follows a valid accepted prefix, the outcome is failure, prefix counts are preserved and labeled prefix-only, and the report does not claim full-input reconciliation.
- AE16. **Covers R13, R14, R18.** Given supplied rows and runtime failures contain identifiable names, email addresses, and message content, when any output surface covered by R13 is produced, including optional-mode diagnostics, no supplied-recipient field is recoverable and optional credentials are absent.
- AE17. **Covers R16, R18.** Given either backend with test-inbox mode, an independently allowlisted destination, explicit confirmation, and a deterministic synthetic fixture of at most 10 records, when the run completes, each completed item has confirmed test-inbox acceptance and delivery outcomes are distinguished from dry-run acceptance. Given a supplied file, absent confirmation, disallowed destination, or fixture above 10 records, the invocation fails before campaign work or network activity with zero delivery attempts and no fallback or truncation. Given ambiguous transport acceptance, the item is failed with the indeterminate-delivery reason and is not retried automatically.
- AE18. **Covers R17-R18.** Given Redis and a separate worker process are running and the campaign uses a deterministic generated fixture, when an Asynq task is retried or executed more than once after enqueue duplication, transient failure, uniqueness-lock expiry, or worker crash, the effect and terminal lifecycle count are committed idempotently once, while duplicate attempts, retries, and retry exhaustion remain observable. For test-inbox tasks, one durable reservation permits at most one SMTP submission for each intended synthetic message; duplicate or redelivered handlers are inert, and a crash that leaves acceptance unresolved settles the reservation as indeterminate without another send. Retry exhaustion classifies the affected eligible item as failed without double counting it. Given an evaluator-supplied recipient file or no separately running worker, the distributed invocation is refused or reaches its documented failure deadline without silently starting an in-process worker.
- AE19. **Covers R17-R18.** Given Redis is unavailable before any acknowledged enqueue, when the distributed run terminates, it reports failure with zero campaign work. Given failure after a trustworthy acknowledged prefix, it reports failure with prefix-only accounting. Given enqueue acknowledgment or terminal task state is indeterminate, it reports failure, preserves only the last trustworthy prefix, discloses the indeterminate remainder separately, does not place unknown work in terminal lifecycle buckets, and does not switch to local execution.
- AE20. **Covers R17-R18.** Given a distributed worker begins graceful shutdown, when its settlement interval expires, it has stopped fetching tasks and unfinished acknowledged tasks remain available to another worker without double counting. Given campaign cancellation, unstarted eligible work is unprocessed, unsettled started work is failed due to interruption, and delayed retries or redeliveries observe closed-campaign state before rendering or sink work. Given reachable Redis but no worker progress until the distributed completion deadline, the run terminates as failure; an operator interrupt produces interrupted only while accounting remains trustworthy.
- AE21. **Covers R17-R18.** Given benchmark candidates for task granularity, concurrency, retry limits, settlement values, and batching, when operational values are selected, the report discloses every candidate and chooses the best completed-useful-work throughput that also maintains reconciliation, bounded queue growth and memory, duplicate/retry recovery, and freedom from observed starvation. One queue is used unless benchmarks define at least two workload classes with a fairness or latency objective; if multiple queues are justified, weighted selection is evaluated first and strict priority is absent unless its starvation trade-off is explicitly accepted.

---

## Success Criteria

- An evaluator can run the required workflow locally with no external service and observe that one million records are validated, deduplicated, personalized, consumed by a no-network sink, and fully reconciled.
- Default execution cannot email real recipients, while any optional delivery demonstration is visibly deliberate and restricted to a test destination.
- Correctness remains explainable under duplicates, malformed data, missing names, per-message failures, and cancellation rather than being reduced to a single throughput number.
- Every trustworthy terminal report satisfies the required reconciliation identities, or explicitly limits its claim to a trustworthy prefix after fatal input corruption.
- Default outputs and optional integrations do not disclose supplied-recipient PII or optional-mode credentials.
- The same synthetic-generation parameters reproduce the same ordered fixture and expected results.
- Performance claims are based on the complete required processing path, include resource evidence and timing boundaries, and can be repeated using the documented procedure and environment.
- Optional test delivery cannot expose or contact campaign recipients, cannot exceed its invocation-wide hard send limit, and remains at-most-once at the SMTP boundary under distributed retry or redelivery.
- Optional distributed execution remains honest about at-least-once behavior, preserves idempotent accounting under retries and duplicate attempts, and fails visibly when Redis or terminal evidence is unavailable.
- The local dry run remains independently buildable, runnable, testable, and benchmarkable with no Redis service, network access, or optional credentials.
- A planner can choose implementation details without inventing product behavior, outcome semantics, safety boundaries, or the distinction between mandatory and optional capabilities.

---

## Scope Boundaries

- No web UI.
- No delivery to arbitrary real recipients; optional network delivery is limited to independently allowlisted test inboxes using synthetic payloads and a hard send cap.
- No provider-specific email alias rewriting.
- No live mailbox, domain, or deliverability verification.
- No requirement to persist every rendered message.
- No Redis, Asynq, database, cloud service, or email provider in the required local path; Redis and Asynq exist only behind explicit backend selection.
- No Kubernetes or cloud deployment.
- No custom message broker or full observability stack.
- No production database design.
- No exactly-once delivery claim.
- No exactly-once task-execution claim, detached campaign submission, status polling API, dashboard, scheduler, or general distributed campaign platform.
- No silent fallback between sinks or backends and no automatic weakening of delivery guards.
- No concurrency, task granularity, batch size, queue weight, retry count, or strict-priority choice without benchmark or operational evidence.
- No universal completion-time SLA independent of hardware.
- No complete production email-marketing platform, including campaign authoring, audience management, scheduling, analytics, unsubscribe management, or deliverability operations.

---

## Key Decisions

- One product with independent concepts: The local dry-run CLI remains the authoritative default; guarded delivery and distributed execution remain separately selected and may be composed through a durable at-most-once delivery reservation without claiming exactly-once task execution or confirmed delivery after an unresolved crash.
- Process actual personalized output: Completion requires rendering and sink consumption so throughput cannot be claimed from validation or iteration alone.
- Continue through record-level data problems: Invalid records and duplicates are accounted for without preventing valid unique recipients from completing.
- Prefer conservative email identity: Case-insensitive comparison and whitespace trimming avoid obvious duplicates without making provider-specific identity assumptions.
- Bound diagnostic detail: Complete aggregate accounting is retained while record-level examples remain limited and redacted for usability and privacy.
- Evidence instead of an arbitrary SLA: The evaluator receives reproducible measurements and environment context rather than a speed promise that ignores hardware differences.
- Separate timing domains: Fixture-generation, campaign-processing, and combined end-to-end timings are reported distinctly so the main throughput claim has a stable meaning.
- Separate throughput claims: Input examination rate remains visible, but completed personalized renderings per second is the primary campaign-performance measure.
- Guard optional delivery by destination, payload, and volume: A test inbox alone is insufficient unless it is independently allowlisted, receives no supplied-recipient PII, and is protected by a small send bound and explicit confirmation.
- Wait for distributed completion: The Asynq backend preserves the single-command terminal-report experience rather than adding detached submission and campaign-status retrieval.
- Own idempotency above the queue: At-least-once execution and duplicate attempts are expected; queue uniqueness may reduce duplicates but cannot establish correctness.
- Prefer weighted priority: Weighted queues are the default because they express preference without deliberate starvation; strict priority requires separate evidence.
- Start with one queue: Multiple priority queues exist only when measured workload classes establish a fairness or latency objective.
- Benchmark operational values: Distributed task granularity, concurrency, batching, queue weights, retry limits, and settlement values are selected from measurements, not requirements-stage guesses.

---

## Dependencies / Assumptions

- The evaluator has a local environment capable of running the submitted Go CLI and the documented benchmark procedure.
- Supplied recipient data can be represented by one documented, stream-compatible file format containing email and optional name values; selection and exact syntax of that format are deferred to planning.
- “Peak memory” refers to the campaign process measurement available in the evaluator's environment; the documentation must state how it was obtained and any platform limitations.
- The promotional message content may be fixed for the assessment, but named and fallback personalization must remain observable.
- Optional infrastructure and credentials, if those modes are included, are supplied separately and are not assumptions of the required path. Credentials remain external to the repository and evaluator-visible artifacts.
- Asynq provides at-least-once execution, retry, worker-crash recovery, weighted and strict queue options, and time-bounded uniqueness locks; application behavior remains responsible for idempotent effects, durable campaign reconciliation, guard enforcement, and honest outcomes when Redis state is unavailable.
- A distributed benchmark environment must provide Redis and enough independent worker capacity to measure useful concurrency; its results supplement rather than replace the dependency-free local benchmark.

---

## Outstanding Questions

### Deferred to Planning

- [Affects R1][Technical] Which evaluator-friendly input file format best preserves streaming behavior and clear validation semantics?
- [Affects R10][Technical] What default cancellation response interval and settlement deadline satisfy the documented, evaluator-visible bounds?
- [Affects R13, R14][Technical] What default sample bounds and deterministic masking format satisfy the privacy invariant while preserving useful evidence?
- [Affects R15][Needs research] Which portable measurement method should report peak process memory and clearly document platform differences?
- [Affects R16][Technical] Which independently configured transport and credential mechanism provides the smallest safe test-inbox demonstration while preserving refusal-first guards?
- [Affects R17][Needs research] Which candidate task granularities, concurrency levels, retry limits, settlement bounds, and weighted queue configurations should enter the benchmark matrix before operational values are selected?
- [Affects R17-R18][Technical] What minimum durable campaign evidence implements the specified trustworthy-prefix, closed-campaign, retry, duplicate-attempt, Redis-interruption, and graceful-shutdown semantics?
- [Affects R18][Technical] How should optional dependencies and configuration be packaged so the default local binary and benchmark remain usable without Redis, network access, or optional credentials?
