#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/export-opencode-session-tree.sh ROOT_SESSION_ID OUTPUT_JSON

Export a root OpenCode session and every descendant session into one sanitized
JSON archive. Provider and model labels are normalized for public presentation.
EOF
}

[[ $# -eq 2 ]] || {
  usage >&2
  exit 2
}

root_id=$1
output=$2

for command_name in opencode sqlite3 jq shasum; do
  command -v "$command_name" >/dev/null 2>&1 || {
    printf '%s is required\n' "$command_name" >&2
    exit 1
  }
done

database=$(opencode db path)
[[ -f "$database" ]] || {
  printf 'OpenCode database does not exist: %s\n' "$database" >&2
  exit 1
}

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

sqlite3 -noheader "$database" "
WITH RECURSIVE tree(id, parent_id, depth, time_created) AS (
  SELECT id, parent_id, 0, time_created
  FROM session
  WHERE id = '$root_id'
  UNION ALL
  SELECT session.id, session.parent_id, tree.depth + 1, session.time_created
  FROM session
  JOIN tree ON session.parent_id = tree.id
)
SELECT id || '|' || coalesce(parent_id, '') || '|' || depth
FROM tree
ORDER BY depth, time_created;
" >"$tmp_dir/sessions.txt"

[[ -s "$tmp_dir/sessions.txt" ]] || {
  printf 'session was not found: %s\n' "$root_id" >&2
  exit 1
}

sanitize_filter=' 
  def public_model: {provider: "OpenAI", name: "GPT-5.6 Sol"};
  {
    id: .info.id,
    parent_id: (.info.parentID // null),
    title: .info.title,
    created_at: (.info.time.created / 1000 | todateiso8601),
    updated_at: (.info.time.updated / 1000 | todateiso8601),
    agent: .info.agent,
    model: public_model,
    opencode_version: .info.version,
    messages: [
      .messages[]
      | {
          id: .info.id,
          role: .info.role,
          created_at: (.info.time.created / 1000 | todateiso8601),
          agent: .info.agent,
          model: public_model,
          text: [.parts[]? | select(.type == "text" and (.synthetic != true)) | .text],
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
  | walk(
      if type == "string" then
        gsub($home; "$HOME")
        | gsub("9router/cx/gpt-5\\.6-sol-xhigh"; "OpenAI/GPT-5.6 Sol")
        | gsub("cx/gpt-5\\.6-sol-xhigh"; "GPT-5.6 Sol")
        | gsub("9router"; "OpenAI")
      else . end
    )
'

index=0
while IFS='|' read -r session_id parent_id depth; do
  raw="$tmp_dir/raw-$index.json"
  clean="$tmp_dir/clean-$index.json"
  opencode export "$session_id" >"$raw"
  jq --arg home "$HOME" --arg parent_id "$parent_id" --argjson depth "$depth" \
    "$sanitize_filter | .parent_id = (if \$parent_id == \"\" then null else \$parent_id end) | .depth = \$depth" \
    "$raw" >"$clean"
  jq -e --arg id "$session_id" '.id == $id and (.messages | type == "array")' "$clean" >/dev/null
  index=$((index + 1))
done <"$tmp_dir/sessions.txt"

jq -s --arg root_id "$root_id" '
  {
    schema_version: 2,
    exported_at: (now | todateiso8601),
    source: "opencode export with descendant sessions",
    root_session_id: $root_id,
    model: {provider: "OpenAI", name: "GPT-5.6 Sol"},
    session_count: length,
    sessions: .
  }
' "$tmp_dir"/clean-*.json >"$output"

shasum -a 256 "$output" >"$output.sha256"

printf 'Exported %d sessions to %s\n' "$index" "$output"
