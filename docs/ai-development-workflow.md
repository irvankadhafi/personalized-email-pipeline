# How I developed this project with AI

This repository was built with an AI coding agent, starting from an empty Go module. I used the agent for research, planning, implementation, review, and verification. My role was to set the boundaries, challenge decisions, review the artifacts and diffs, and decide what counted as evidence.

This document is a public engineering account, not a transcript. It leaves out local paths, tool internals, and conversational noise. The chronological, sanitized [AI conversation exports](../transcripts/README.md) are committed separately as reviewable JSON.

## Why I used Compound Engineering

I usually reach for [Superpowers](https://github.com/obra/superpowers). Its design-first workflow, small implementation steps, TDD discipline, and verification rules are a good default for agent-assisted work. For an existing feature whose decision tree is still unclear, I may also use Matt Pocock's [Grill Me](https://github.com/mattpocock/skills) workflow to force the missing questions into the open.

This assignment was different. There was no existing application to interrogate, and the short prompt hid several consequential choices: what "send" means when real delivery is forbidden, how to prove that personalization happened, what one million completed records means, and how much production infrastructure belongs in a proof of concept.

I chose [Compound Engineering](https://every.to/guides/compound-engineering?source=post_button) because its workflow starts by clarifying constraints and producing a plan, which matched an assignment with several hidden design decisions. The loop then continues through implementation, review, and capturing useful context for the next pass. I used the [Compound Engineering plugin](https://github.com/EveryInc/compound-engineering-plugin) through OpenCode. A related [thread from Trevin](https://x.com/trevin/status/2079954613319749661?s=46&t=ez7zuM1JLOH_cVcKiyu4gw) is also part of the background reading that shaped my interest in this workflow.

## 1. Turn the task into requirements

The first substantial artifact was [the requirements document](brainstorms/2026-07-26-personalized-email-campaign-requirements.md). The brainstorming pass expanded a few lines of assignment text into testable behavior and explicit scope.

The important decision was to treat the default command as a real processing proof, not a fake sender. A recipient counts as complete only after the program has rendered the full personalized message and a local SHA-256 sink has accepted it. That gives the benchmark useful work while making accidental delivery impossible.

The same pass settled several details before they could become implementation accidents:

- missing names use one fixed neutral greeting;
- validation stays offline and provider-neutral;
- only the first valid normalized address is eligible;
- every terminal report must reconcile all examined and eligible records;
- reports must not leak recipient data, message bodies, credentials, or raw external errors;
- cancellation needs a defined admission cutoff and settlement period;
- fixture generation and campaign processing need separate timings;
- SMTP and distributed execution are optional demonstrations, never prerequisites for the main proof.

I reviewed the requirements and ran a separate document-review pass before planning. The follow-up commit `b81000c` records that review rather than silently folding the corrections into the first draft.

## 2. Plan the smallest authoritative path

The [local pipeline plan](plans/2026-07-27-002-feat-million-recipient-dry-run-plan.md) translated the requirements into packages, ownership rules, implementation units, tests, and verification commands.

The plan deliberately chose a single-process, standard-library path for the core evaluation. CSV input is streamed, validation and deduplication have one owner, workers are bounded, and one coordinator owns lifecycle accounting. The deduplication set is the only structure that grows with the number of unique recipients.

Redis, Asynq, and SMTP were considered and rejected from the first implementation. They added setup and failure modes without improving the required one-million-record proof. Keeping those ideas out of the initial code was a manual scope decision, not an omission discovered later.

## 3. Implement and verify the offline pipeline

The implementation followed the plan's units: domain contracts, constrained CSV input, deterministic fixture generation, rendering and digest acceptance, coordinated processing, CLI/reporting, then reproducibility and CI.

Tests focused on boundaries that throughput-oriented code can mishandle: malformed rows with recoverable boundaries, invalid records before valid duplicates, sink rejection, cancellation races, late worker results, fatal prefix-only accounting, and privacy canaries in process output.

After the first implementation, manual verification found that the cancellation report described the configured response bound rather than the observed response time. I corrected that in commit `488f93c` and kept the configured bound as a separate field. This is a useful example of why I did not treat a green test suite as the end of review.

## 4. Add optional modes without changing the default

Once the offline path was working, I returned to the optional requirements and wrote a second [implementation plan](plans/2026-07-27-003-feat-optional-delivery-distributed-plan.md). This plan kept backend and sink as independent choices:

| Backend | Sink | Purpose |
|---|---|---|
| local | dry-run | Authoritative service-free million-record proof |
| local | test-inbox | Bounded SMTP demonstration using generated recipients |
| asynq | dry-run | Distributed rendering with durable Redis accounting |
| asynq | test-inbox | Distributed execution with an at-most-once SMTP reservation |

The safety rules came before the adapters. Test delivery accepts only a generated fixture of at most ten records, requires an exact confirmation token, and sends all messages to one separately allowlisted destination. Generated `.test` addresses never become transport destinations.

For Asynq, I did not rely on queue uniqueness as a correctness guarantee. The Redis ledger owns lifecycle state, retries remain visible, duplicate transitions are inert, and loss of trustworthy Redis state produces an unknown-accounting failure instead of a local fallback. SMTP reservations are never reclaimed automatically; if a crash leaves acceptance uncertain, the result is `delivery_indeterminate`, not another send.

## 5. Review the behavior through its real surfaces

Review covered code, tests, and actual use. The final checks included:

- `go vet`, formatting, patch checks, the standard suite, and the race-enabled shuffled suite;
- default operation with Redis and SMTP configuration deliberately poisoned, proving lazy optional initialization;
- all four backend/sink combinations through the CLI;
- real standalone Redis for ledger and Asynq integration behavior;
- a local TLS SMTP catcher, which recorded four accepted messages across the two bounded delivery runs;
- privacy canaries across stdout, stderr, Redis state, task payloads, and optional failure paths;
- a fresh deterministic one-million-record fixture and processing run with balanced counts and the expected SHA-256 fixture hash.

The README records the reproducible commands and measured results. I kept generated CSV files, timing captures, credentials, and the original assignment attachment out of git.

## 6. Prepare reviewer-facing evidence

The final pass organized the evidence around how the project would be reviewed. Measured values came from completed runs, commands came from the tested CLI, and limitations stayed tied to behavior the repository can demonstrate.

The documentation has three parts. The README is the evaluator's runbook. This file explains how the work was shaped and checked. The [conversation export index](../transcripts/README.md) links the retained user/assistant text and delegated prompts, documents the filtering boundary, and provides integrity checks.

## Repository trail

The development history remains unsquashed on `main`. The main checkpoints are:

| Commit | Checkpoint |
|---|---|
| `99dac6a` | Define campaign requirements |
| `b81000c` | Apply requirements-review findings |
| `a4ede28` | Plan the offline million-recipient pipeline |
| `e86a922` | Implement the offline pipeline |
| `488f93c` | Correct observed cancellation timing |
| `a04b896` | Plan optional delivery and distributed execution |
| `58632e9` through `6f12211` | Add dependencies, contracts, SMTP, Redis, Asynq, and CLI modes in focused commits |
| `075cbf1` and `0fa820b` | Extend privacy coverage and isolate Redis CI |
| `0b91b54` through `7b2f3d4` | Bring requirements and evaluator documentation in line with the completed behavior |
| `19ba108` through `dc5500c` | Specify, implement, test, and document the bounded evaluator and typed message formats |
| `1193c2d` through `7f2fd06` | Add reproducible conversation exports and browser evidence |

That trail shows the repository sequence: requirements before implementation, a review-driven correction, optional work behind explicit guards, and documentation added after the behavior was complete.
