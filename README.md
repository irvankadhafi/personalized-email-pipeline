# Personalized Email Pipeline

An offline-by-default Go CLI that generates deterministic recipient fixtures, processes personalized campaign messages locally, and optionally demonstrates guarded SMTP delivery or Asynq/Redis execution.

## Build and discover

Requires Go 1.26.5.

```sh
go build -trimpath -o bin/email-pipeline ./cmd/email-pipeline
bin/email-pipeline help
```

The default workflow remains service-free:

```sh
bin/email-pipeline generate --output assessment-1000000.csv --count 1000000 --seed 7
bin/email-pipeline run --input assessment-1000000.csv
```

`generate` reports its own elapsed time and expected named/fallback counts. `run` measures only campaign processing: CSV parsing, validation, exact deduplication, full personalization, and acceptance by the local digest sink.

## Optional execution modes

`run` independently selects an execution backend and message sink. Omitted selectors are `--backend=local --sink=dry-run`.

| Backend | Sink | Input | External service |
|---|---|---|---|
| `local` | `dry-run` | Supplied CSV | None |
| `local` | `test-inbox` | Generated fixture, 1-10 records | SMTP |
| `asynq` | `dry-run` | Generated fixture | Standalone Redis |
| `asynq` | `test-inbox` | Generated fixture, 1-10 records | Standalone Redis and SMTP |

Test-inbox runs require the exact `--confirm-test-inbox=SEND_SYNTHETIC_TEST` flag. They deliver every rendered synthetic message to one independently configured allowlisted destination, never to generated fixture addresses.

```sh
export EMAIL_PIPELINE_SMTP_HOST=smtp.example.test
export EMAIL_PIPELINE_SMTP_PORT=587
export EMAIL_PIPELINE_SMTP_USERNAME=test-user
export EMAIL_PIPELINE_SMTP_PASSWORD='use-an-isolated-secret'
export EMAIL_PIPELINE_SMTP_FROM=sender@example.test
export EMAIL_PIPELINE_TEST_DESTINATION=inbox@example.test
export EMAIL_PIPELINE_TEST_ALLOWLIST=inbox@example.test
# Optional for an isolated SMTP server signed by a private test CA:
# export EMAIL_PIPELINE_SMTP_CA_FILE=/path/to/test-ca.pem

bin/email-pipeline run --backend=local --sink=test-inbox \
  --count=2 --seed=7 --confirm-test-inbox=SEND_SYNTHETIC_TEST
```

Distributed runs require standalone Redis and a separately started sink-specific worker:

```sh
export EMAIL_PIPELINE_REDIS_ADDR=127.0.0.1:6379
export EMAIL_PIPELINE_REDIS_DB=0

# terminal 1
bin/email-pipeline worker --sink=dry-run --concurrency=4

# terminal 2
bin/email-pipeline run --backend=asynq --sink=dry-run \
  --count=10000 --seed=7 --task-size=100
```

For distributed test-inbox delivery, configure both Redis and SMTP, start `worker --sink=test-inbox`, and run:

```sh
bin/email-pipeline run --backend=asynq --sink=test-inbox \
  --count=2 --seed=7 --task-size=1 \
  --confirm-test-inbox=SEND_SYNTHETIC_TEST
```

Asynq task execution is at least once. Redis owns an atomic, non-reclaimable SMTP reservation, so redelivery cannot submit the same intended synthetic message twice. A reservation whose final SMTP state cannot be proven is reported as `delivery_indeterminate`; the tool does not claim exactly-once delivery. Redis ambiguity produces `accounting_scope:"unknown"` and never falls back to local execution.

## Input contract

- UTF-8 CSV with the exact header `email,name`.
- One record per physical line; `email` is required and `name` is optional.
- Quoted commas are supported; embedded newlines are not.
- A UTF-8 BOM is accepted only before the header.
- Each physical record is limited to 1 MiB.
- Recoverable malformed rows are counted by static reason and processing continues.
- Missing/wrong headers, open failures, and unreadable boundaries are fatal.

Email validity is deliberately conservative and offline. Surrounding Unicode whitespace is removed, comparison identities are lowercase, and provider-specific dot or plus-tag rewriting is never performed. Invalid rows do not reserve identities.

## Processing and privacy

Every first valid identity is rendered with the fixed promotion and either a usable supplied name or `Hello there,`. Dry-run completion is counted only after the full message is accepted by the SHA-256 digest sink. Test-inbox completion requires confirmed SMTP acceptance after `DATA`.

Reports never include supplied addresses, names, personalized bodies, raw parser errors, panic values, environment values, or credentials. Diagnostic examples use synthetic source-ordinal labels. Rendering samples retain the two lowest ordinals per represented category; invalid/failed reasons retain the three lowest ordinals and state how many details were omitted.

## Outcomes and exit codes

| Outcome | Exit | Meaning |
|---|---:|---|
| `success` | 0 | At least one eligible recipient, no invalid rows, and all eligible work completed. Duplicates alone do not downgrade success. |
| `failure` | 1 | Invalid configuration/input trust, empty or zero-eligible input, or untrustworthy accounting. |
| `partial_success` | 2 | Trustworthy accounting with invalid rows or eligible work failures/unprocessed work. |
| `interrupted` | 130 | First SIGINT/SIGTERM stopped admission and remaining trustworthy input was reconciled. |

The report enforces:

```text
examined = invalid + duplicate + eligible
eligible = completed + failed + unprocessed
started = completed + failed
```

`run` handles the first SIGINT/SIGTERM gracefully. Admission closes within the reported 100 ms response bound; started work receives the configured settlement duration (default 5 seconds). Reading of a trustworthy regular file continues in accounting-only mode, so total command completion can exceed the settlement duration. A second signal uses normal platform behavior.

## Reproduce the one-million run

Generation and processing are intentionally separate:

```sh
/usr/bin/time -l bin/email-pipeline generate \
  --output assessment-1000000.csv --count 1000000 --seed 7 \
  > generate-report.json 2> generate-time.txt

/usr/bin/time -l bin/email-pipeline run \
  --input assessment-1000000.csv --workers 11 \
  > benchmark-report.json 2> benchmark-time.txt
```

On Linux, use `/usr/bin/time -v` and read `Maximum resident set size (kbytes)`. On macOS, `/usr/bin/time -l` reports `maximum resident set size` in bytes. The JSON field `peak_heap_inuse_bytes` is sampled Go `HeapInuse` at 10 ms intervals and is not RSS.

### Reference measurement (2026-07-27)

- Go: `go1.26.5 darwin/arm64`
- Machine: Apple M3 Pro, 11 logical CPUs, 19,327,352,832 bytes physical memory
- Fixture: algorithm `v1`, seed `7`, 1,000,000 records, 500,000 named and 500,000 fallback
- Fixture size/SHA-256: 55,500,012 bytes / `34c90f44002f2b0f7df2a5c937a03b4c7c5db6e3058ff401185f36ab62bb965c`
- Generation: 0.304050 seconds; 11,665,408-byte maximum RSS
- Processing: 3.123577 seconds with 11 workers and queue capacity 22
- Counts: 1,000,000 examined/eligible/started/completed; 0 invalid/duplicate/failed/unprocessed
- Input and primary completion throughput: 320,146 records/renderings per second
- Sampled peak Go `HeapInuse`: 233,775,104 bytes
- OS maximum RSS: 249,872,384 bytes

These figures are environment-specific evidence, not a hardware-independent SLA. Exact deduplication makes the normalized identity set the only input-sized structure.

## Verification

```sh
test -z "$(gofmt -l .)"
go vet ./...
go test -count=1 ./...
go test -race -shuffle=on -count=1 ./...
```

CI keeps the default job service-free, proves poisoned optional endpoints do not affect local execution, and runs a 10,000-record end-to-end smoke test. A separate Redis job exercises atomic ledger and real Asynq integration tests. SMTP protocol tests use an in-process TLS wire server and require no credentials. `Dockerfile.bench` is an optional pinned Linux environment; Docker is not needed for normal use.
