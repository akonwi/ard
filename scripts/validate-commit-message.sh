#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/validate-commit-message.sh <commit-message-file>
  scripts/validate-commit-message.sh --message "feat(parser): add tuple literals"

Valid Conventional Commit types:
  build, chore, ci, docs, feat, fix, perf, refactor, revert, style, test

Examples:
  feat(formatter): support inferred closure params
  fix!: remove deprecated runtime API
  docs(readme): document formatter behavior
EOF
}

header_regex='^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([a-zA-Z0-9._/-]+\))?(!)?: .+$'

read_message() {
  if [[ $# -eq 1 && "$1" != "--message" && "$1" != "-h" && "$1" != "--help" ]]; then
    cat "$1"
    return
  fi

  if [[ $# -eq 2 && "$1" == "--message" ]]; then
    printf '%s\n' "$2"
    return
  fi

  usage >&2
  exit 2
}

message="$(read_message "$@")"
header="$(printf '%s\n' "$message" | head -n 1)"

if [[ "$header" == Merge* ]]; then
  exit 0
fi

if [[ "$header" == 'Revert "'* ]] && printf '%s\n' "$message" | grep -q '^This reverts commit '; then
  exit 0
fi

if [[ ! "$header" =~ $header_regex ]]; then
  cat >&2 <<EOF
Invalid commit message header:
  $header

Expected Conventional Commits format:
  <type>(<scope>)!: <description>

Examples:
  feat(formatter): support inferred closure params
  fix(vm): preserve generic return kinds
  docs: document release flow
  refactor(parser)!: simplify function parsing

Allowed types:
  build, chore, ci, docs, feat, fix, perf, refactor, revert, style, test
EOF
  exit 1
fi
