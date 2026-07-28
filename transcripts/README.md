# AI conversation exports

These files preserve the AI-assisted development conversation in chronological order. They are committed as JSON because the source is hierarchical: each message has a role, timestamp, model, and zero or more text blocks or delegated sub-agent prompts.

## Session order

1. [`opencode-ses_06112ff84ffeyK3UCMb8kxSrp7.json`](opencode-ses_06112ff84ffeyK3UCMb8kxSrp7.json) covers requirements discovery, planning, the offline pipeline, optional SMTP and distributed modes, review, and verification. It contains 904 retained messages, 911 visible text blocks, and 16 delegated prompts.
2. [`opencode-ses_05868c9d2ffeU0Sa42ZNDbNeSj.json`](opencode-ses_05868c9d2ffeU0Sa42ZNDbNeSj.json) covers the bounded local evaluator, its review, browser behavior, and supporting documentation. It contains 368 retained messages, 364 visible text blocks, and 75 delegated prompts.

The identifiers are the original OpenCode root-session IDs. A delegated prompt records the child session ID when OpenCode supplied one, so the review trail remains attributable without embedding every sub-agent tool event.

## Export scope

Each export retains:

- root-session metadata;
- user and assistant message text;
- timestamps, roles, agents, and model identifiers;
- prompts delegated to sub-agents, including their descriptions and child-session IDs.

The exporter deliberately omits private reasoning, tool inputs and outputs, patches, file snapshots, and synthetic system reminders. Those records are execution internals rather than conversation, and raw inclusion made one source export larger than GitHub's 100 MB file limit. Local home-directory prefixes are replaced with `$HOME`. The original assignment attachment, personal addresses, credentials, generated recipient data, and benchmark working files are not included.

The transformation is reproducible with [`scripts/export-opencode-session.sh`](../scripts/export-opencode-session.sh). It requires `opencode`, `jq`, and `shasum`:

```sh
scripts/export-opencode-session.sh --force \
  --output transcripts/opencode-ses_06112ff84ffeyK3UCMb8kxSrp7.json \
  ses_06112ff84ffeyK3UCMb8kxSrp7

scripts/export-opencode-session.sh --force \
  --output transcripts/opencode-ses_05868c9d2ffeU0Sa42ZNDbNeSj.json \
  ses_05868c9d2ffeU0Sa42ZNDbNeSj
```

## Integrity check

Each JSON file has a neighboring SHA-256 manifest. Verify both committed artifacts from the repository root:

```sh
shasum -a 256 -c transcripts/*.sha256
```

Both lines should end in `OK`. The checksums prove that the reviewed files have not changed; they do not authenticate the original OpenCode database.
