# Pipeline assessment prompts

This page contains the substantive prompts used to steer the root session. Standalone continuation messages and host-generated reminders are omitted here, but remain in the complete JSON archive.

## Compound Engineering: requirements brainstorm

`2026-07-26T14:58:16Z` · `/ce-brainstorm`

<details open>
<summary>Prompt</summary>

<pre>I need to complete a software engineering assessment from scratch.

Assignment brief:
- Process 1,000,000 customer email addresses.
- Send the same promotional message, personalized with each recipient's name.
- Finish as fast as reasonably possible.
- Deliver either a script or a small page.
- Do not email real recipients; use a test inbox or dry run.
- Submit the final code and the complete AI conversation.

Context:
- I intend to use Go and expose the solution as a CLI.
- The default execution must be safe and perform no real email delivery.
- I am considering an optional distributed execution mode using Asynq and Redis.
- The job description emphasizes production-grade engineering, live data, performance, Redis, Docker, SQL, message queues, CI/CD, and sound judgment, but the assessment itself does not mandate a stack.

Please help me define WHAT this solution must be before planning HOW to implement it.

Requirements for this brainstorm:
- Ask one question at a time.
- Challenge hidden assumptions and identify evaluator-visible acceptance criteria.
- Separate mandatory assessment behavior from optional production-grade demonstrations.
- Resolve input semantics, duplicate handling, invalid records, dry-run completion semantics, failure reporting, cancellation, reproducibility, and measurable performance evidence.
- Explicitly decide whether Asynq/Redis belongs in the required path, an optional mode, or only future architecture.
- Keep implementation details, package layout, and exact APIs out of the requirements document.
- Produce a durable requirements document under docs/brainstorms/ with clear requirements, actors, flows, acceptance examples, scope boundaries, assumptions, and non-goals.</pre>

</details>

## Compound Engineering: document review

`2026-07-27T06:57:06Z` · `/ce-doc-review`

<details open>
<summary>Prompt</summary>

<pre>@docs/brainstorms/2026-07-26-personalized-email-campaign-requirements.md 
Review these requirements before implementation planning.

Check for contradictions, unclear behavior, and decisions a planner would otherwise have to guess. Make sure the acceptance examples cover the important paths and can be verified by an evaluator.

Pay close attention to safe default delivery, recipient privacy, duplicates, invalid input, cancellation, partial failure, and performance claims. Check that Asynq and Redis remain optional and do not expand the core acceptance scope.

You may fix wording, formatting, and internal references when the meaning stays the same. For anything that changes behavior or scope, show me the finding and ask before editing.

Do not start planning or write application code.</pre>

</details>

## Compound Engineering: implementation plan

`2026-07-27T07:43:04Z` · `/ce-plan`

<details open>
<summary>Prompt</summary>

<pre>@docs/brainstorms/2026-07-26-personalized-email-campaign-requirements.md 
Build the implementation plan from this reviewed requirements document. Treat its requirements, acceptance examples, boundaries, and deferred technical questions as the source of truth. The product decisions are settled.

Use the `ponytail` skill at `full` intensity while planning. I prefer the smallest design that fully satisfies the requirements: standard library before dependencies, one concrete path before abstractions, and no scaffolding for hypothetical future needs. Do not simplify away input validation, privacy, failure handling, cancellation, tests, or measurement.

I want the required Go CLI planned first. It must be safe by default, measurable, and runnable without Redis or another external service. Keep the dependency list small and use the standard library where it fits.

Asynq and Redis are optional. Decide whether their demonstrator mode earns its cost after the local path is complete. If it stays in the plan, isolate it from core execution and cover at-least-once delivery, idempotency, retries, small task payloads, Redis failure, and evaluator setup. Do not force a distributed design just because the role mentions queues.

Give me an ordered plan with repo-relative files, tests tied to the acceptance examples, and clear verification for each unit. Cover streaming input, deterministic fixtures, bounded concurrency, cancellation, personalization, accounting, privacy-safe diagnostics, and performance measurement. Include the one-million-record run and explain how its results will be reproduced.

Consider Docker and CI only where they make the submission easier to run or verify. Leave out a database, web UI, cloud deployment, and other infrastructure unless the requirements justify them.

Research technical questions before settling them. Ask me only when a choice changes scope or an important trade-off cannot be resolved from the requirements and current documentation. Write the plan, not the application code.</pre>

</details>

## Compound Engineering: implementation

`2026-07-27T08:21:16Z` · `/ce-work`

<details open>
<summary>Prompt</summary>

<pre>@docs/plans/2026-07-27-002-feat-million-recipient-dry-run-plan.md 
Implement the approved personalized email campaign assessment plan end to end.

Execution requirements:
- Follow the origin requirements and approved plan exactly.
- Treat the in-process, standard-library Go CLI as the complete required scope.
- Do not implement Asynq, Redis, network delivery, or placeholder extension architecture.
- Keep execution streaming, bounded, race-free, privacy-safe, and offline by default.
- Run formatting, vet, tests, race detection, manual CLI QA, and the documented one-million-recipient benchmark.
- Record measured evidence honestly in the README.
- Stop only when the acceptance criteria work through the actual CLI surface.</pre>

</details>

## Compound Engineering: code review

`2026-07-27T13:33:00Z` · `/ce-code-review`

<details open>
<summary>Prompt</summary>

<pre>Please review the current feature branch against the approved requirements and plan:

- docs/brainstorms/2026-07-26-personalized-email-campaign-requirements.md
- docs/plans/2026-07-27-002-feat-million-recipient-dry-run-plan.md

This is a report-only pass. Do not modify the branch yet.

Focus on issues that could affect the assessment result:

- incorrect accounting or outcome classification;
- cancellation races, goroutine leaks, and channel ownership;
- malformed or fatal CSV handling;
- recipient-data leakage through reports, errors, panics, or test artifacts;
- behavior that could perform network delivery;
- benchmark claims that are not supported by the implementation;
- weak tests around the documented acceptance examples;
- unnecessary complexity that makes the solution harder to explain.

The approved scope is the offline standard-library Go CLI. Asynq, Redis, network delivery, and additional infrastructure are intentionally out of scope.

Review the committed diff from origin/main to HEAD. Order findings by severity, cite the relevant files, and explain the concrete failure scenario. If there are no blocking findings, say so and list the remaining risks or verification gaps.</pre>

</details>

## Compound Engineering: requirements brainstorm

`2026-07-27T14:28:06Z` · `/ce-brainstorm`

<details open>
<summary>Prompt</summary>

<pre>I want to extend the completed CLI with two optional capabilities while keeping the current local dry run unchanged as the default:

- a guarded test-inbox sink;
- an Asynq/Redis execution backend.

Treat delivery and execution as separate choices. My current direction is:

--sink=dry-run|test-inbox
--backend=local|asynq

The test-inbox mode must require explicit confirmation, an independently configured allowlist, synthetic recipient data, and a small hard send limit.

For Asynq, define the smallest credible distributed mode. Cover task granularity, idempotency, retries, duplicate execution, Redis failure, graceful shutdown, measured concurrency, and queue priority. Prefer weighted queues unless strict priority has a clear reason. Do not choose concurrency or batch sizes without benchmark evidence.

The existing local path, report semantics, privacy rules, and million-record benchmark must remain valid without Redis or network access.

Update the existing requirements rather than creating a second product. Stop after the revised scope and acceptance criteria are clear; do not write code yet.</pre>

</details>

## Compound Engineering: document review

`2026-07-27T14:52:39Z` · `/ce-doc-review`

<details open>
<summary>Prompt</summary>

<pre>@docs/brainstorms/2026-07-26-personalized-email-campaign-requirements.md 
Review the revised requirements specifically for R16-R18 and AE17-AE21.

Check whether the guarded test-inbox and Asynq/Redis requirements are:
- internally consistent;
- implementable without changing the default local dry-run behavior;
- explicit about refusal-first delivery guards;
- explicit about at-least-once execution and application-owned idempotency;
- complete for retries, duplicate execution, Redis failure, graceful shutdown, campaign cancellation, queue priority, and benchmark-selected operational values;
- appropriately scoped for the smallest credible optional demonstration rather than a production campaign platform.

Identify contradictions, missing acceptance criteria, ambiguous outcome semantics, and requirements that would force the implementation planner to invent product behavior.

Do not modify code. Report findings first and recommend exact requirements changes where needed.</pre>

</details>

## Compound Engineering: implementation

`2026-07-28T01:24:05Z` · `/ce-work`

<details open>
<summary>Prompt</summary>

<pre>mode:return-to-caller @docs/plans/2026-07-27-003-feat-optional-delivery-distributed-plan.md 
Resume the existing optional-mode implementation from the current working tree. Do not restart, discard, revert, or duplicate the uncommitted U1-U3 work.

Current state:

- U1 shared fixture and optional evidence contracts are present.
- U2 guarded SMTP package is present.
- U3 Redis ledger is present.
- The distributed task codec has been started.
- `go test ./...` currently passes 90 tests across 6 packages.
- U4 producer/worker, U5 CLI integration, and U6 benchmark/docs/CI remain incomplete.

Before continuing U4, resolve this authoritative scope conflict:

- R18 in `docs/brainstorms/2026-07-26-personalized-email-campaign-requirements.md` requires all four independent combinations:
  - local + dry-run
  - local + test-inbox
  - asynq + dry-run
  - asynq + test-inbox
- The current plan incorrectly refuses `asynq + test-inbox` and describes only three supported combinations.

Treat the requirements as authoritative. Update the plan so `asynq + test-inbox` is supported rather than refused.

For the combined mode, preserve these requirements:

- campaign execution remains at-least-once;
- externally visible SMTP delivery must be committed at most once per intended synthetic message;
- retries, duplicate execution, worker crash, uniqueness expiry, and redelivery must not send the same synthetic message twice;
- the campaign ledger must own an atomic delivery reservation and terminal delivery result;
- an indeterminate SMTP result must not be retried automatically;
- the hard limit of 10 messages applies across the entire invocation, including retries and duplicate attempts;
- confirmation, independent allowlist, synthetic substitution, destination exclusion, and all refusal-first guards remain mandatory;
- delayed execution after campaign closure or cancellation must be inert;
- no silent fallback to local execution or dry-run delivery is allowed.

Then continue from the existing worktree:

1. Verify and finish U1-U3 without rewriting already-correct code.
2. Complete U4 Asynq producer and worker.
3. Complete U5 CLI selectors, worker command, all four combinations, reports, and privacy boundaries.
4. Complete U6 real Redis integration, benchmark matrix, measured defaults, README, and CI.
5. Preserve the existing local dry-run behavior and million-record benchmark.
6. Run diagnostics, tests, race tests, vet, privacy checks, real Redis integration, CLI manual QA, and the documented local million-record benchmark.
7. Do not perform a real SMTP send unless a deliberately configured sandbox inbox and explicit confirmation are available; use the local SMTP wire server for automated proof.
8. Do not commit or push until the implementation and review gates are complete.

Stop only when the implementation plan and authoritative R16-R18 / AE17-AE21 acceptance criteria are satisfied, or when a genuinely user-owned product decision remains unresolved.</pre>

</details>

