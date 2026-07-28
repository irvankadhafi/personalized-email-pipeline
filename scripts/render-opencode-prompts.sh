#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/render-opencode-prompts.sh INPUT_JSON OUTPUT_MARKDOWN TITLE

Render the substantive user prompts from one compact root-session export.
Standalone continuation messages and system reminders are excluded. Full root
and child conversations remain available in the checksummed tree JSON archive.
EOF
}

[[ $# -eq 3 ]] || {
  usage >&2
  exit 2
}

input=$1
output=$2
title=$3

[[ -f "$input" ]] || {
  printf 'input does not exist: %s\n' "$input" >&2
  exit 1
}

jq -r --arg title "$title" '
  def html:
    gsub("&"; "&amp;")
    | gsub("<"; "&lt;")
    | gsub(">"; "&gt;");

  def body: .text | join("\n");

  def is_noise:
    (body | ascii_downcase | gsub("^[[:space:]]+|[[:space:]]+$"; "")) as $text
    | ($text | test("^<system-reminder>"))
      or ($text | test("^(please[[:space:]]+)?continue([[:space:]].*)?$"))
      or ($text | test("^continue remains$"));

  def command_name:
    (body | try capture("# /(?<name>[^ ]+) Command").name catch null);

  def has_user_arguments:
    body | contains("**User Arguments**:");

  def user_arguments:
    body
    | if contains("**User Arguments**:") then
        split("**User Arguments**:")[1]
        | split("**Scope**:")[0]
        | gsub("^[[:space:]]+|[[:space:]]+$"; "")
      else . end;

  def phase:
    command_name as $command
    | if $command == "ce-brainstorm" then "Compound Engineering: requirements brainstorm"
      elif $command == "ce-doc-review" then "Compound Engineering: document review"
      elif $command == "ce-plan" then "Compound Engineering: implementation plan"
      elif $command == "ce-work" then "Compound Engineering: implementation"
      elif $command == "ce-code-review" then "Compound Engineering: code review"
      elif $command == "ce-commit" then "Compound Engineering: commit checkpoint"
      elif $command then "Command: /" + $command
      elif (body | test("(?i)debug|timing|cancellation|finding|fix web")) then "Debugging and corrective work"
      elif (body | test("(?i)review|audit|hands-on check")) then "Review and verification"
      elif (body | test("(?i)plan|requirements")) then "Planning decision"
      else "Steering prompt" end;

  def is_primary_prompt:
    command_name as $command
    | (($command == "ce-brainstorm"
        or $command == "ce-doc-review"
        or $command == "ce-plan"
        or $command == "ce-work"
        or $command == "ce-code-review")
       and has_user_arguments)
      or ((command_name == null)
          and (body | test("(?i)^the smallest credible submission|^the synthesis matches|^please address finding|^do one final hands-on check|^first, create an implementation plan|^oh no need, please create your design|fix web timing origin")));

  "# " + $title + "\n\n"
  + "This page contains the substantive prompts used to steer the root session. "
  + "Standalone continuation messages and host-generated reminders are omitted here, but remain in the complete JSON archive.\n\n"
  + ([.messages[]
      | select(.role == "user")
      | select(is_noise | not)
      | select(is_primary_prompt)
      | "## " + phase + "\n\n"
        + "`" + .created_at + "`"
        + (if command_name then " · `/" + command_name + "`" else "" end)
        + "\n\n<details open>\n<summary>Prompt</summary>\n\n<pre>"
        + (user_arguments | html)
        + "</pre>\n\n</details>\n"] | join("\n"))
' "$input" >"$output"

printf 'Rendered curated prompts to %s\n' "$output"
