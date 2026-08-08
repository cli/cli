#!/bin/bash

# Regression suite for the spam detection criteria.
#
# Parses the corpus, runs each case through `copilot -p` with a lightweight
# model matching the engine the issue-triage workflow uses, and grades the
# verdict against the expected one.
#
# The system prompt is assembled from two parts:
#
#   1. eval-instructions.md    - the PASS/FAIL output contract, eval-only
#   2. shared/spam-criteria.md - the criteria, shared with issue-triage.md
#
# The criteria file is deliberately role-neutral, because the workflow acts on
# it by applying a label while the eval acts on it by emitting a verdict. Only
# part 2 is under test; part 1 just makes the corpus gradeable.
#
# Usage:
#   ./.github/workflows/scripts/spam-detection/eval.sh
#   ./.github/workflows/scripts/spam-detection/eval.sh -c criteria.md -o run.json
#   ./.github/workflows/scripts/spam-detection/eval.sh -d before.json,after.json
#
# To A/B a criteria change, capture both arms and diff them by disagreement set
# with -d. Aggregate pass rate alone is not reliable: re-running an unchanged
# prompt moves it by ~0.7 points, more than a real but small change would.
#
#   ./.github/workflows/scripts/spam-detection/eval.sh -c before.md -o before.json
#   ./.github/workflows/scripts/spam-detection/eval.sh -c after.md  -o after.json
#   ./.github/workflows/scripts/spam-detection/eval.sh -d before.json,after.json

set -euo pipefail

SPAM_DIR="$(dirname "$(realpath "$0")")"
REPO_ROOT="$(git -C "$SPAM_DIR" rev-parse --show-toplevel)"

criteria="${REPO_ROOT}/.github/workflows/shared/spam-criteria.md"
instructions="${SPAM_DIR}/eval-instructions.md"
corpus="${SPAM_DIR}/eval-prompts.yml"
out=""
compare=""
model="gpt-5-mini"
effort="low"
concurrency=8
limit=0
filter=""
validate_only=0

usage() {
    cat >&2 <<'EOF'
usage: eval.sh [options]
  -c FILE   criteria file under test   (default shared/spam-criteria.md)
  -i FILE   eval instructions          (default eval-instructions.md)
  -p FILE   corpus                     (default eval-prompts.yml)
  -o FILE   write per-case JSON results here
  -d A,B    compare two result files by disagreement set, then exit
  -m NAME   model                      (default gpt-5-mini)
  -e NAME   reasoning effort           (default low)
  -j N      concurrent invocations     (default 8)
  -n N      run only the first N cases
  -f STR    run only cases whose name contains STR
  -V        parse and validate the corpus without calling the model
EOF
    exit 2
}

while getopts ":c:i:p:o:d:m:e:j:n:f:Vh" opt; do
    case "$opt" in
        c) criteria="$OPTARG" ;;
        i) instructions="$OPTARG" ;;
        p) corpus="$OPTARG" ;;
        o) out="$OPTARG" ;;
        d) compare="$OPTARG" ;;
        m) model="$OPTARG" ;;
        e) effort="$OPTARG" ;;
        j) concurrency="$OPTARG" ;;
        n) limit="$OPTARG" ;;
        f) filter="$OPTARG" ;;
        V) validate_only=1 ;;
        *) usage ;;
    esac
done

for tool in copilot jq python3; do
    command -v "$tool" >/dev/null || { echo "error: $tool is required" >&2; exit 1; }
done

# The corpus is YAML, which python3 cannot read without PyYAML. Check up front
# rather than letting the parser die with a traceback partway through.
python3 -c 'import yaml' 2>/dev/null || {
    echo "error: python3 is missing the PyYAML module (try: python3 -m pip install pyyaml)" >&2
    exit 1
}

# ---------------------------------------------------------------------------
# Compare mode. Diffs two arms by disagreement set rather than headline pass
# rate: with LLM-judged cases a one or two point difference is noise, so the
# useful question is which specific cases moved and in which direction.
# ---------------------------------------------------------------------------
if [[ -n "$compare" ]]; then
    a="${compare%%,*}"
    b="${compare##*,}"
    [[ "$a" != "$b" ]] || usage
    jq -rn --slurpfile a "$a" --slurpfile b "$b" '
        ($a[0].results | INDEX(.name)) as $A |
        ($b[0].results | INDEX(.name)) as $B |
        [ $A | keys[] | select($B[.] != null) | . as $k |
          { name: $k, from: $A[$k].actual, to: $B[$k].actual,
            change: (if   $A[$k].correct and ($B[$k].correct | not) then "broke"
                     elif ($A[$k].correct | not) and $B[$k].correct then "fixed"
                     elif ($A[$k].correct | not) then "still wrong"
                     else "same" end) } ]
        | map(select(.change != "same")) as $moved
        | ([$A | keys[]] - [$B | keys[]]) as $onlyA
        | "a: \($a[0].results | map(select(.correct)) | length)/\($a[0].results | length)  \($a[0].systemPath // "?")",
          "b: \($b[0].results | map(select(.correct)) | length)/\($b[0].results | length)  \($b[0].systemPath // "?")",
          "",
          "disagreement set: \($moved | length) cases",
          ($moved | sort_by(.change, .name)[] | "  [\(.change)] \(.name): \(.from) -> \(.to)"),
          (if ($onlyA | length) > 0 then "\nonly in a: \($onlyA | length) cases" else empty end)
    '
    exit 0
fi

for f in "$criteria" "$instructions" "$corpus"; do
    [[ -f "$f" ]] || { echo "error: no such file: $f" >&2; exit 1; }
done

# `copilot` loads plugins, skills and custom instructions from $HOME. Left
# unset, a developer's local setup leaks into the prompt and the measurement is
# not reproducible; a single local skill can inflate a call from 15.1k to 36.6k
# tokens. Every invocation therefore runs under a throwaway HOME.
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

# Concatenate the eval-only output contract with the criteria under test,
# stripping the criteria file's YAML frontmatter exactly as the gh-aw runtime
# import does, so the eval grades the same text the agent sees. That includes
# dropping the blank lines the strip leaves behind, otherwise the separator
# between the two parts depends on how the criteria file happens to be spaced.
# awk rather than sed because the GNU and BSD dialects disagree on range
# deletion.
system="${workdir}/system.md"
{
    cat "$instructions"
    printf '\n\n'
    awk '
        NR == 1 && $0 == "---" { in_fm = 1; next }
        in_fm && $0 == "---"   { in_fm = 0; next }
        in_fm                  { next }
        !started && $0 == ""   { next }
        { started = 1; print }
    ' "$criteria"
} > "$system"

python3 - "$corpus" > "${workdir}/cases.json" <<'PY'
import json, sys, yaml

with open(sys.argv[1]) as fh:
    doc = yaml.safe_load(fh)

cases = doc.get("testData") or []
for i, case in enumerate(cases):
    missing = [k for k in ("name", "expected", "input") if not case.get(k)]
    if missing:
        sys.exit(f"corpus case {i} is missing: {', '.join(missing)}")
    if case["expected"] not in ("PASS", "FAIL"):
        sys.exit(f"corpus case {i} ({case['name']}) has expected={case['expected']!r}")

json.dump(cases, sys.stdout)
PY

jq --arg f "$filter" --argjson n "$limit" '
    map(select($f == "" or (.name | contains($f))))
    | if $n > 0 then .[:$n] else . end
' "${workdir}/cases.json" > "${workdir}/selected.json"

total=$(jq length "${workdir}/selected.json")
[[ "$total" -gt 0 ]] || { echo "error: no cases selected" >&2; exit 1; }

if [[ "$validate_only" == 1 ]]; then
    jq -r 'group_by(.expected)[] | "\(.[0].expected)  \(length)"' "${workdir}/selected.json"
    echo "total $total"
    exit 0
fi

run_case() {
    local i="$1" name expected input raw actual err errfile rc
    name=$(jq -r ".[$i].name" "${workdir}/selected.json")
    expected=$(jq -r ".[$i].expected" "${workdir}/selected.json")
    input=$(jq -r ".[$i].input" "${workdir}/selected.json")

    # On success stderr is just a stats footer, so it is noise. On failure it
    # carries the only useful diagnostic (bad model name, auth, rate limit),
    # so it is captured and kept rather than discarded, otherwise an
    # unauthenticated run looks identical to a corpus the model simply got
    # wrong.
    errfile="${workdir}/err.$i"
    rc=0
    raw=$(HOME="$workdir" copilot -p "$(cat "$system")

${input}" \
        --model "$model" --effort "$effort" --allow-all-tools --no-color \
        --log-level none --disable-builtin-mcps --no-custom-instructions 2>"$errfile") || rc=$?

    err=""
    if [[ "$rc" -ne 0 ]]; then
        err="exit ${rc}: $(tr -d '\r' < "$errfile" | grep -v '^[[:space:]]*$' | head -3 | tr '\n' ' ')"
        raw=""
    fi
    rm -f "$errfile"

    # Take the last verdict token, so a model that reasons aloud before
    # answering is graded on its conclusion rather than its first mention.
    # Splitting on non-letters isolates whole words without the \b escape,
    # which is a GNU extension rather than POSIX, and it strips any surrounding
    # markdown or punctuation the model added.
    actual=$(printf '%s' "$raw" | tr '[:lower:]' '[:upper:]' | tr -cs '[:alpha:]' '\n' \
        | grep -xE 'PASS|FAIL' | tail -1) || actual=""

    jq -nc --arg n "$name" --arg e "$expected" --arg a "$actual" --arg r "$raw" --arg x "$err" \
        '{name: $n, expected: $e, actual: $a, correct: ($a != "" and $a == $e), raw: $r}
         + (if $x == "" then {} else {error: $x} end)'
}
export -f run_case
export workdir system model effort

started=$(date +%s)
echo "running $total cases on $model (effort $effort, concurrency $concurrency)" >&2
seq 0 $((total - 1)) | xargs -P "$concurrency" -I{} bash -c 'run_case {}' \
    > "${workdir}/results.jsonl"
duration=$(( $(date +%s) - started ))

jq -s --arg m "$model" --arg e "$effort" --arg p "$criteria" \
    --argjson d "$duration" --arg s "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '{model: $m, effort: $e, systemPath: $p, startedAt: $s, durationSec: $d, results: .}' \
    "${workdir}/results.jsonl" > "${workdir}/run.json"

[[ -z "$out" ]] || cp "${workdir}/run.json" "$out"

# A false positive is a legitimate issue judged spam. It is the costlier error
# of the two here, since it closes real reports, so the two are never merged
# into a single accuracy figure.
#
# Errored cases are counted apart from unparseable ones: an unparseable case
# means the model answered something unexpected, an errored case means it never
# answered at all, and only the first is a statement about the criteria.
jq -r '
    .results as $r
    | ($r | map(select(.correct))                                   | length) as $correct
    | ($r | map(select(.error == null and .actual == ""))           | length) as $unparsed
    | ($r | map(select(.error != null))                             | length) as $errored
    | ($r | map(select((.correct | not) and .actual == "FAIL" and .expected == "PASS")) | length) as $fp
    | ($r | map(select((.correct | not) and .actual == "PASS" and .expected == "FAIL")) | length) as $fn
    | "",
      "cases            \($r | length)",
      "correct          \($correct) (\(($correct * 1000 / ($r | length) | round) / 10)%)",
      "false positives  \($fp)  (legitimate issue judged spam)",
      "false negatives  \($fn)  (spam issue judged legitimate)",
      (if $unparsed > 0 then "unparseable      \($unparsed)" else empty end),
      (if $errored  > 0 then "errored          \($errored)  (no verdict returned)" else empty end),
      "duration         \(.durationSec)s",
      (if $errored > 0
       then "", "first error:", "  \($r | map(select(.error != null))[0].error)"
       else empty end),
      (if ($r | map(select(.correct | not)) | length) > 0
       then "", "incorrect cases:",
            ($r | map(select(.correct | not)) | sort_by(.name)[]
             | "  [want \(.expected) got \(if .error != null then "error" elif .actual == "" then "unparseable" else .actual end)] \(.name)")
       else empty end)
' "${workdir}/run.json"

# Exit non-zero when any case failed to produce a verdict, so a run degraded by
# a bad flag, expired auth or rate limiting is not mistaken for a measurement.
errored=$(jq '[.results[] | select(.error != null)] | length' "${workdir}/run.json")
[[ "$errored" -eq 0 ]] || exit 1
