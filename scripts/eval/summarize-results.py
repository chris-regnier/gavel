#!/usr/bin/env python3
"""
Summarize A/B experiment results from run-experiment.sh.

Reads JSONL output and produces aggregate metrics averaged across runs,
grouped by condition (filter_on vs filter_off), with per-package breakdown
and per-run variance.

Usage:
    python3 scripts/eval/summarize-results.py <experiment-results.jsonl>

Example:
    python3 scripts/eval/summarize-results.py /tmp/gavel-eval/experiment-results.jsonl
"""

import json
import math
import sys
from collections import defaultdict


def load_results(path):
    records = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))
    return records


def mean(values):
    return sum(values) / len(values) if values else 0


def stddev(values):
    if len(values) < 2:
        return 0
    m = mean(values)
    return math.sqrt(sum((x - m) ** 2 for x in values) / (len(values) - 1))


def aggregate_by_condition(records):
    """Group records by (condition, run), sum per-run totals, then average across runs."""
    # First: sum metrics per (condition, run) across all packages
    run_totals = defaultdict(lambda: defaultdict(lambda: {
        "total": 0, "llm": 0, "instant": 0,
        "errors": 0, "warnings": 0, "notes": 0,
        "errs_hi_conf": 0, "confs_sum": 0, "confs_count": 0,
    }))

    for r in records:
        cond = r["condition"]
        run = r["run"]
        t = run_totals[cond][run]
        t["total"] += r["total"]
        t["llm"] += r["llm"]
        t["instant"] += r["instant"]
        t["errors"] += r["errors"]
        t["warnings"] += r["warnings"]
        t["notes"] += r["notes"]
        t["errs_hi_conf"] += r["errs_hi_conf"]
        t["confs_sum"] += r["avg_conf"] * r["total"]
        t["confs_count"] += r["total"]

    # Then: average across runs for each condition
    result = {}
    for cond in sorted(run_totals.keys()):
        runs = run_totals[cond]
        n = len(runs)
        metrics = {}
        for key in ["total", "llm", "instant", "errors", "warnings", "notes", "errs_hi_conf"]:
            values = [runs[run][key] for run in runs]
            metrics[key] = {"mean": mean(values), "std": stddev(values)}

        # Weighted average confidence across runs
        conf_values = []
        for run in runs:
            if runs[run]["confs_count"] > 0:
                conf_values.append(runs[run]["confs_sum"] / runs[run]["confs_count"])
        metrics["avg_conf"] = {"mean": mean(conf_values), "std": stddev(conf_values)}
        metrics["n_runs"] = n
        result[cond] = metrics

    return result


def per_package_breakdown(records):
    """Average metrics per (condition, package) across runs."""
    grouped = defaultdict(list)
    for r in records:
        grouped[(r["condition"], r["package"])].append(r)

    result = {}
    for (cond, pkg), recs in sorted(grouped.items()):
        result[(cond, pkg)] = {
            "total": mean([r["total"] for r in recs]),
            "llm": mean([r["llm"] for r in recs]),
            "errors": mean([r["errors"] for r in recs]),
            "errs_hi_conf": mean([r["errs_hi_conf"] for r in recs]),
            "avg_conf": mean([r["avg_conf"] for r in recs]),
            "n": len(recs),
        }
    return result


def print_aggregate(agg):
    conditions = sorted(agg.keys())
    if len(conditions) != 2:
        print(f"Warning: expected 2 conditions, got {len(conditions)}: {conditions}")

    on = agg.get("filter_on", {})
    off = agg.get("filter_off", {})
    n = on.get("n_runs", 0)

    print(f"AGGREGATE METRICS (averaged across {n} runs)")
    print("=" * 60)

    rows = [
        ("Total findings", "total"),
        ("LLM findings", "llm"),
        ("Instant findings", "instant"),
        ("Error-level", "errors"),
        ("Warning-level", "warnings"),
        ("Note-level", "notes"),
        ("Errors w/ conf>0.8", "errs_hi_conf"),
    ]

    for label, key in rows:
        on_val = on.get(key, {}).get("mean", 0)
        off_val = off.get(key, {}).get("mean", 0)
        delta = on_val - off_val
        if off_val > 0:
            pct = (delta / off_val) * 100
            pct_str = f"({pct:+.1f}%)"
        else:
            pct_str = ""
        print(f"  {label:<22s} ON={on_val:<7.1f} OFF={off_val:<7.1f} "
              f"Δ={delta:<+7.1f} {pct_str}")

    # Confidence (3 decimal places)
    on_conf = on.get("avg_conf", {}).get("mean", 0)
    off_conf = off.get("avg_conf", {}).get("mean", 0)
    delta_conf = on_conf - off_conf
    print(f"  {'Avg confidence':<22s} ON={on_conf:<7.3f} OFF={off_conf:<7.3f} "
          f"Δ={delta_conf:<+7.3f}")

    print()

    # Variance table
    print(f"PER-RUN VARIANCE (std dev across {n} runs)")
    print("-" * 60)
    for label, key in rows:
        on_std = on.get(key, {}).get("std", 0)
        off_std = off.get(key, {}).get("std", 0)
        print(f"  {label:<22s} ON=±{on_std:<6.1f} OFF=±{off_std:<6.1f}")
    on_conf_std = on.get("avg_conf", {}).get("std", 0)
    off_conf_std = off.get("avg_conf", {}).get("std", 0)
    print(f"  {'Avg confidence':<22s} ON=±{on_conf_std:<6.3f} OFF=±{off_conf_std:<6.3f}")


def print_per_package(pkg_data):
    print()
    print("PER-PACKAGE BREAKDOWN (averaged across runs)")
    print("=" * 80)

    packages = sorted(set(pkg for (_, pkg) in pkg_data.keys()))
    conditions = sorted(set(cond for (cond, _) in pkg_data.keys()))

    header = f"  {'Package':<25s}"
    for cond in conditions:
        short = "ON" if "on" in cond else "OFF"
        header += f" | {short:>5s} tot {short:>5s} llm {short:>5s} err {short:>5s} hi"
    print(header)
    print("  " + "-" * 78)

    for pkg in packages:
        line = f"  {pkg:<25s}"
        for cond in conditions:
            d = pkg_data.get((cond, pkg), {})
            line += (f" | {d.get('total',0):>5.1f}     "
                     f"{d.get('llm',0):>5.1f}     "
                     f"{d.get('errors',0):>5.1f}     "
                     f"{d.get('errs_hi_conf',0):>5.1f}")
        print(line)


def main():
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <experiment-results.jsonl>")
        sys.exit(1)

    path = sys.argv[1]
    records = load_results(path)

    if not records:
        print(f"No records found in {path}")
        sys.exit(1)

    print(f"Loaded {len(records)} records from {path}")
    print()

    agg = aggregate_by_condition(records)
    print_aggregate(agg)
    print()

    pkg_data = per_package_breakdown(records)
    print_per_package(pkg_data)


if __name__ == "__main__":
    main()
