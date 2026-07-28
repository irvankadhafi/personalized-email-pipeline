#!/usr/bin/env bash

set -Eeuo pipefail

usage() {
  cat <<'EOF'
Usage:
  export-opencode-session.sh [--force] [--output FILE] SESSION_ID

Export an OpenCode session as reviewable JSON, including visible conversation
text and prompts delegated to sub-agents. Tool output, reasoning, patches, and
file snapshots are omitted. Local home paths are replaced with $HOME.

Options:
  --force        Replace an existing output file.
  --output FILE  Output path. Default: transcripts/opencode-SESSION_ID.json
  -h, --help     Show this help.
EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

force=false
output=''
session_id=''

while (($#)); do
  case "$1" in
    --force)
      force=true
      shift
      ;;
    --output)
      (($# >= 2)) || die '--output requires a file path'
      output=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      (($# == 1)) || die 'provide exactly one SESSION_ID'
      session_id=$1
      shift
      ;;
    -*)
      die "unknown option: $1"
      ;;
    *)
      [[ -z $session_id ]] || die 'provide exactly one SESSION_ID'
      session_id=$1
      shift
      ;;
  esac
done

[[ -n $session_id ]] || {
  usage >&2
  exit 2
}
[[ $session_id == ses_* ]] || die 'SESSION_ID must start with ses_'

command -v opencode >/dev/null 2>&1 || die 'opencode is not installed or not in PATH'
command -v jq >/dev/null 2>&1 || die 'jq is required to process the export'

if [[ -z $output ]]; then
  output="transcripts/opencode-${session_id}.json"
fi

output_dir=$(dirname "$output")
mkdir -p "$output_dir"

if [[ -e $output && $force != true ]]; then
  die "output already exists: $output (use --force to replace it)"
fi

raw=$(mktemp "${output_dir}/.opencode-export-raw.XXXXXX")
clean=$(mktemp "${output_dir}/.opencode-export-clean.XXXXXX")
trap 'rm -f "$raw" "$clean"' EXIT

printf 'Exporting %s...\n' "$session_id" >&2
opencode export "$session_id" >"$raw"

jq -e --arg id "$session_id" \
  'type == "object" and .info.id == $id and (.messages | type == "array")' \
  "$raw" >/dev/null || die 'OpenCode returned invalid or unexpected session JSON'

jq --arg home "$HOME" '
  {
    schema_version: 1,
    exported_at: (now | todateiso8601),
    source: "opencode export",
    session: {
      id: .info.id,
      title: .info.title,
      created_at: (.info.time.created / 1000 | todateiso8601),
      updated_at: (.info.time.updated / 1000 | todateiso8601),
      agent: .info.agent,
      model: .info.model,
      opencode_version: .info.version
    },
    messages: [
      .messages[]
      | {
          id: .info.id,
          role: .info.role,
          created_at: (.info.time.created / 1000 | todateiso8601),
          agent: .info.agent,
          model: .info.model,
          text: [
            .parts[]?
            | select(.type == "text" and (.synthetic != true))
            | .text
          ],
          delegated_prompts: [
            .parts[]?
            | select(.type == "tool" and .tool == "task")
            | {
                session_id: (.state.metadata.sessionId // .state.metadata.taskId // null),
                agent: (.state.metadata.agent // .state.input.subagent_type // .state.input.category // null),
                description: (.state.metadata.description // .state.input.description // null),
                prompt: (.state.metadata.prompt // .state.input.prompt // null)
              }
          ]
        }
      | select((.text | length) > 0 or (.delegated_prompts | length) > 0)
    ]
  }
  | walk(if type == "string" then gsub($home; "$HOME") else . end)
' \
  "$raw" >"$clean"

jq -e --arg id "$session_id" \
  'type == "object" and .session.id == $id and (.messages | type == "array")' \
  "$clean" >/dev/null || die 'processed export is invalid'

mv -f "$clean" "$output"
trap 'rm -f "$raw"' EXIT

if command -v shasum >/dev/null 2>&1; then
  shasum -a 256 "$output" >"${output}.sha256"
elif command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$output" >"${output}.sha256"
else
  printf 'warning: no SHA-256 command found; checksum not written\n' >&2
fi

message_count=$(jq '.messages | length' "$output")
printf 'Exported %s messages to %s\n' "$message_count" "$output"
printf 'Review the file for personal data and secrets before sharing it.\n' >&2
