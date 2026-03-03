#!/bin/bash
#
# A/B experiment runner for Gavel prompt tuning.
#
# Runs N iterations of Gavel analysis on target packages under two conditions
# (strict_filter on/off), capturing per-run metrics into JSONL for comparison.
#
# Usage:
#   ./scripts/eval/run-experiment.sh <gavel-binary> <eval-dir> [packages...]
#
# Arguments:
#   gavel-binary   Path to the compiled gavel binary
#   eval-dir       Working directory for the experiment (must contain .gavel/policies.yaml)
#   packages...    Packages to analyze (relative to repo root)
#
# Environment:
#   RUNS           Number of iterations per condition (default: 3)
#
# Output:
#   <eval-dir>/experiment-results.jsonl   Per-run metrics in JSONL format
#
# Example:
#   task build
#   ./scripts/eval/run-experiment.sh ./dist/gavel /tmp/gavel-eval \
#     internal/mcp internal/evaluator internal/store internal/sarif internal/input
#
# Prerequisites:
#   - Copy scripts/eval/example-policies.yaml to <eval-dir>/.gavel/policies.yaml
#   - Configure the provider section for your LLM backend
#   - python3 must be available for JSON extraction

set -e

if [ $# -lt 2 ]; then
  echo "Usage: $0 <gavel-binary> <eval-dir> [packages...]"
  echo ""
  echo "Arguments:"
  echo "  gavel-binary   Path to compiled gavel binary"
  echo "  eval-dir       Working directory (must contain .gavel/policies.yaml)"
  echo "  packages...    Packages to analyze (relative to repo root)"
  echo ""
  echo "Environment:"
  echo "  RUNS           Number of iterations per condition (default: 3)"
  echo ""
  echo "Example:"
  echo "  $0 ./dist/gavel /tmp/gavel-eval internal/mcp internal/store"
  exit 1
fi

BINARY="$1"
EVALDIR="$2"
shift 2

# Remaining args are packages; default to a representative set if none given
if [ $# -gt 0 ]; then
  PACKAGES=("$@")
else
  PACKAGES=(
    "internal/mcp"
    "internal/evaluator"
    "internal/store"
    "internal/sarif"
    "internal/input"
  )
fi

SCRIPTDIR="$(cd "$(dirname "$0")" && pwd)"
REPOROOT="$(cd "$SCRIPTDIR/../.." && pwd)"
OUTDIR="$EVALDIR/.gavel/results"
CONFIG="$EVALDIR/.gavel/policies.yaml"
RESULTS_LOG="$EVALDIR/experiment-results.jsonl"
RUNS="${RUNS:-3}"

if [ ! -f "$BINARY" ]; then
  echo "Error: gavel binary not found at $BINARY"
  exit 1
fi

if [ ! -f "$CONFIG" ]; then
  echo "Error: config not found at $CONFIG"
  echo "Copy scripts/eval/example-policies.yaml to $CONFIG and configure your provider."
  exit 1
fi

echo "Experiment configuration:"
echo "  Binary:     $BINARY"
echo "  Eval dir:   $EVALDIR"
echo "  Runs:       $RUNS"
echo "  Packages:   ${PACKAGES[*]}"
echo "  Results:    $RESULTS_LOG"
echo ""

> "$RESULTS_LOG"

for run in $(seq 1 "$RUNS"); do
  for condition in "true" "false"; do
    # Set filter condition
    sed -i '' "s/strict_filter: .*/strict_filter: $condition/" "$CONFIG"
    label="filter_$([ "$condition" = "true" ] && echo "on" || echo "off")"

    for pkg in "${PACKAGES[@]}"; do
      echo "=== Run $run | $label | $pkg ==="

      raw_output=$(cd "$EVALDIR" && "$BINARY" analyze \
        --dir "$REPOROOT/$pkg" \
        --output "$OUTDIR" 2>/dev/null || true)

      # Extract the last JSON object from the output
      json_output=$(echo "$raw_output" | python3 -c "
import sys, json, re
text = sys.stdin.read()
matches = re.findall(r'\{[^{}]+\}', text)
if matches:
    print(matches[-1])
else:
    print('{}')
")

      id=$(echo "$json_output" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('id',''))")
      findings=$(echo "$json_output" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('findings',0))")

      if [ -z "$id" ] || [ "$id" = "" ]; then
        echo "  SKIPPED (no result ID)"
        continue
      fi

      # Judge
      verdict_raw=$(cd "$EVALDIR" && "$BINARY" judge --result "$id" 2>/dev/null || true)
      decision=$(echo "$verdict_raw" | python3 -c "
import sys, json, re
text = sys.stdin.read()
matches = re.findall(r'\{[^{}]+\}', text)
if matches:
    d = json.loads(matches[-1])
    print(d.get('decision','unknown'))
else:
    print('unknown')
")

      # Extract detailed stats from SARIF
      python3 -c "
import json
with open('$OUTDIR/$id/sarif.json') as f:
    data = json.load(f)
results = data['runs'][0]['results']
levels = {}
tiers = {'instant': 0, 'comprehensive': 0}
confs = []
for r in results:
    levels[r['level']] = levels.get(r['level'], 0) + 1
    tier = r.get('properties', {}).get('gavel/tier', 'comprehensive')
    tiers[tier] = tiers.get(tier, 0) + 1
    confs.append(r.get('properties', {}).get('gavel/confidence', 0))
errs_hi = sum(1 for r in results if r['level']=='error' and r.get('properties',{}).get('gavel/confidence',0) > 0.8)
rec = {
    'run': $run, 'condition': '$label', 'package': '$pkg',
    'total': len(results), 'llm': tiers.get('comprehensive',0), 'instant': tiers.get('instant',0),
    'errors': levels.get('error',0), 'warnings': levels.get('warning',0), 'notes': levels.get('note',0),
    'nones': levels.get('none',0),
    'errs_hi_conf': errs_hi,
    'avg_conf': round(sum(confs)/len(confs),3) if confs else 0,
    'decision': '$decision', 'id': '$id'
}
print(json.dumps(rec))
" >> "$RESULTS_LOG"

      echo "  findings=$findings decision=$decision"
    done
  done
done

echo ""
echo "=== ALL RUNS COMPLETE ==="
echo "Results in: $RESULTS_LOG"
echo ""
echo "To summarize: python3 scripts/eval/summarize-results.py $RESULTS_LOG"
