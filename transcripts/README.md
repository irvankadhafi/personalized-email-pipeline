# AI development record

The submission includes two views of the AI-assisted work:

1. Complete JSON archives preserve the root OpenCode sessions and every child-agent session from the first message to the last.
2. Curated Markdown pages show the substantive prompts used to steer requirements, planning, implementation, debugging, review, and verification. Standalone `continue` messages are omitted only from this readable view.

Provider and model metadata are normalized to `OpenAI` and `GPT-5.6 Sol` throughout the public artifacts.

## Start with the prompts

| Workstream | Readable prompts | What it covers |
|---|---|---|
| Pipeline assessment | [Pipeline assessment prompts](pipeline-prompts.md) | Requirements brainstorm, requirements review, implementation plan, local implementation, optional-mode decisions, debugging, code review, and final verification |
| Bounded evaluator | [Bounded evaluator prompts](evaluator-prompts.md) | Browser requirements, typed text/HTML plan, evaluator implementation, timing correction, and completion steering |

The workflow mostly used Compound Engineering:

- `ce-brainstorm` to settle behavior and assumptions before architecture;
- `ce-doc-review` to challenge the requirements;
- `ce-plan` to turn approved requirements into dependency-ordered work;
- `ce-work` for implementation and verification;
- `ce-code-review` for a report-first final review.

Ponytail was applied during planning to keep the required path small and avoid speculative abstractions. Debugging was used for concrete failures and mismatches, including cancellation timing and web timing behavior. Those modes supported the Compound Engineering loop rather than replacing it.

## Complete conversation archives

| Root session | Sessions included | Archive |
|---|---:|---|
| Pipeline `ses_06112ff84ffeyK3UCMb8kxSrp7` | 63 total: 1 root and 62 child sessions | [Complete pipeline conversation JSON](opencode-tree-ses_06112ff84ffeyK3UCMb8kxSrp7.json) · [SHA-256](opencode-tree-ses_06112ff84ffeyK3UCMb8kxSrp7.json.sha256) |
| Evaluator `ses_05868c9d2ffeU0Sa42ZNDbNeSj` | 103 total: 1 root and 102 descendant sessions | [Complete evaluator conversation JSON](opencode-tree-ses_05868c9d2ffeU0Sa42ZNDbNeSj.json) · [SHA-256](opencode-tree-ses_05868c9d2ffeU0Sa42ZNDbNeSj.json.sha256) |

Each archive contains session metadata, parent-child linkage, user and assistant text, and prompts delegated through the task tool. The evaluator archive includes nested descendants, not only direct children.

The complete archives retain ordinary continuation messages because they are part of the original conversation. The curated Markdown removes them so a reviewer can focus on decisions and steering prompts.

## Sanitization boundary

The export keeps conversational text and delegated prompts. It removes private reasoning, tool inputs and outputs, patches, file snapshots, and synthetic host reminders. Those records are execution internals rather than prompts, and the raw root export alone exceeded GitHub's normal 100 MB file limit.

Local home-directory prefixes are replaced with `$HOME`. The assignment attachment, personal addresses, credentials, generated recipient data, and benchmark working files are not included.

## Reproduce the artifacts

Export a root session and all descendants:

```sh
scripts/export-opencode-session-tree.sh \
  ses_06112ff84ffeyK3UCMb8kxSrp7 \
  transcripts/opencode-tree-ses_06112ff84ffeyK3UCMb8kxSrp7.json

scripts/export-opencode-session-tree.sh \
  ses_05868c9d2ffeU0Sa42ZNDbNeSj \
  transcripts/opencode-tree-ses_05868c9d2ffeU0Sa42ZNDbNeSj.json
```

Generate the readable prompt trail from the compact root exports:

```sh
scripts/render-opencode-prompts.sh \
  transcripts/opencode-ses_06112ff84ffeyK3UCMb8kxSrp7.json \
  transcripts/pipeline-prompts.md \
  "Pipeline assessment prompts"

scripts/render-opencode-prompts.sh \
  transcripts/opencode-ses_05868c9d2ffeU0Sa42ZNDbNeSj.json \
  transcripts/evaluator-prompts.md \
  "Bounded evaluator prompts"
```

Verify the complete archives:

```sh
shasum -a 256 -c transcripts/opencode-tree-*.sha256
```

The checksums prove that the reviewed files have not changed; they do not authenticate the original OpenCode database.
