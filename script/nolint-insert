#!/bin/bash
# Usage: script/nolint-insert
#        script/nolint-insert 'nolint:staticcheck // <explanation>'
set -e

insert-line() {
  local n=$'\n'
  sed -i.bak "${2}i\\${n}${3}${n}" "$1"
  rm "$1.bak"
}

reverse() {
  awk '{a[i++]=$0} END {for (j=i-1; j>=0;) print a[j--] }'
}

comment="${1}"

golangci-lint run --out-format json | jq -r '.Issues[] | [.Pos.Filename, .Pos.Line, .FromLinter, .Text] | @tsv' | reverse | while IFS=$'\t' read -r filename line linter text; do
  directive="nolint:${linter} // $text"
  [ -z "$comment" ] || directive="$comment"
  insert-line "$filename" "$line" "//${directive}"
done

go fmt ./...
