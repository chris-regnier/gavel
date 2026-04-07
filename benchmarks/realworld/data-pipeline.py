# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements. See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to you under the Apache License, Version 2.0.
# Source: github.com/apache/airflow (Apache 2.0 License)
# This is a representative snippet for benchmarking purposes.

from __future__ import annotations

import csv
import hashlib
import io
import json
import logging
from dataclasses import dataclass, field
from datetime import date, datetime
from pathlib import Path
from typing import Any, Dict, Generator, Iterable, List, Optional, Tuple

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Data models
# ---------------------------------------------------------------------------

@dataclass
class RawRecord:
    source_id: str
    payload: Dict[str, Any]
    ingested_at: datetime = field(default_factory=datetime.utcnow)


@dataclass
class TransformedRecord:
    record_id: str
    event_date: date
    category: str
    amount: float
    currency: str
    metadata: Dict[str, Any]
    checksum: str


@dataclass
class PipelineStats:
    total_read: int = 0
    total_transformed: int = 0
    total_skipped: int = 0
    total_errors: int = 0
    start_time: datetime = field(default_factory=datetime.utcnow)
    end_time: Optional[datetime] = None

    @property
    def duration_seconds(self) -> float:
        end = self.end_time or datetime.utcnow()
        return (end - self.start_time).total_seconds()

    @property
    def success_rate(self) -> float:
        if self.total_read == 0:
            return 0.0
        return self.total_transformed / self.total_read


# ---------------------------------------------------------------------------
# Extraction
# ---------------------------------------------------------------------------

def extract_from_csv(path: Path, encoding: str = "utf-8") -> Generator[RawRecord, None, None]:
    """Yield RawRecord objects from a CSV file."""
    logger.info("extracting from %s", path)
    with path.open(encoding=encoding, newline="") as fh:
        reader = csv.DictReader(fh)
        for row in reader:
            yield RawRecord(source_id=str(path), payload=dict(row))


def extract_from_jsonl(path: Path) -> Generator[RawRecord, None, None]:
    """Yield RawRecord objects from a newline-delimited JSON file."""
    logger.info("extracting from %s", path)
    with path.open() as fh:
        for lineno, line in enumerate(fh, start=1):
            line = line.strip()
            if not line:
                continue
            try:
                data = json.loads(line)
            except json.JSONDecodeError as exc:
                logger.warning("skipping malformed JSON on line %d: %s", lineno, exc)
                continue
            yield RawRecord(source_id=f"{path}:{lineno}", payload=data)


# ---------------------------------------------------------------------------
# Transformation
# ---------------------------------------------------------------------------

REQUIRED_FIELDS = {"event_date", "category", "amount", "currency"}


def _parse_amount(value: Any) -> float:
    """Coerce a value to float, stripping currency symbols."""
    if isinstance(value, (int, float)):
        return float(value)
    cleaned = str(value).replace(",", "").strip().lstrip("$€£¥")
    return float(cleaned)


def _compute_checksum(record: Dict[str, Any]) -> str:
    serialized = json.dumps(record, sort_keys=True, default=str).encode()
    return hashlib.sha256(serialized).hexdigest()[:16]


def transform_record(raw: RawRecord) -> Optional[TransformedRecord]:
    """Transform a RawRecord into a TransformedRecord, or None if invalid."""
    missing = REQUIRED_FIELDS - set(raw.payload.keys())
    if missing:
        logger.debug("skipping %s: missing fields %s", raw.source_id, missing)
        return None

    try:
        event_date = date.fromisoformat(raw.payload["event_date"])
        amount = _parse_amount(raw.payload["amount"])
        currency = str(raw.payload["currency"]).upper().strip()
        category = str(raw.payload["category"]).strip().lower()
    except (ValueError, TypeError) as exc:
        logger.warning("transform error for %s: %s", raw.source_id, exc)
        return None

    if amount < 0:
        logger.debug("skipping %s: negative amount %.2f", raw.source_id, amount)
        return None

    metadata = {k: v for k, v in raw.payload.items() if k not in REQUIRED_FIELDS}
    checksum = _compute_checksum({
        "event_date": str(event_date),
        "category": category,
        "amount": amount,
        "currency": currency,
    })

    return TransformedRecord(
        record_id=f"{category}-{checksum}",
        event_date=event_date,
        category=category,
        amount=amount,
        currency=currency,
        metadata=metadata,
        checksum=checksum,
    )


# ---------------------------------------------------------------------------
# Loading
# ---------------------------------------------------------------------------

def load_to_jsonl(records: Iterable[TransformedRecord], out: io.TextIOBase) -> int:
    """Write records as newline-delimited JSON. Returns the count written."""
    count = 0
    for rec in records:
        row = {
            "record_id": rec.record_id,
            "event_date": rec.event_date.isoformat(),
            "category": rec.category,
            "amount": rec.amount,
            "currency": rec.currency,
            "metadata": rec.metadata,
            "checksum": rec.checksum,
        }
        out.write(json.dumps(row) + "\n")
        count += 1
    return count


# ---------------------------------------------------------------------------
# Pipeline orchestration
# ---------------------------------------------------------------------------

def run_pipeline(
    input_path: Path,
    output_path: Path,
    *,
    format: str = "csv",
) -> PipelineStats:
    """
    Execute the extract-transform-load pipeline.

    Parameters
    ----------
    input_path:  path to the input file (CSV or JSONL)
    output_path: path where the transformed JSONL will be written
    format:      "csv" or "jsonl"
    """
    stats = PipelineStats()

    # Extract
    if format == "csv":
        raw_stream = extract_from_csv(input_path)
    elif format == "jsonl":
        raw_stream = extract_from_jsonl(input_path)
    else:
        raise ValueError(f"unsupported format: {format!r}")

    output_path.parent.mkdir(parents=True, exist_ok=True)

    with output_path.open("w") as out_fh:
        for raw in raw_stream:
            stats.total_read += 1

            try:
                transformed = transform_record(raw)
            except Exception as exc:
                logger.error("unexpected error transforming %s: %s", raw.source_id, exc)
                stats.total_errors += 1
                continue

            if transformed is None:
                stats.total_skipped += 1
                continue

            row = {
                "record_id": transformed.record_id,
                "event_date": transformed.event_date.isoformat(),
                "category": transformed.category,
                "amount": transformed.amount,
                "currency": transformed.currency,
                "metadata": transformed.metadata,
                "checksum": transformed.checksum,
            }
            out_fh.write(json.dumps(row) + "\n")
            stats.total_transformed += 1

    stats.end_time = datetime.utcnow()
    logger.info(
        "pipeline complete: read=%d transformed=%d skipped=%d errors=%d duration=%.2fs",
        stats.total_read,
        stats.total_transformed,
        stats.total_skipped,
        stats.total_errors,
        stats.duration_seconds,
    )
    return stats


# ---------------------------------------------------------------------------
# CLI entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    import argparse
    import sys

    parser = argparse.ArgumentParser(description="Run the data transformation pipeline")
    parser.add_argument("input", type=Path, help="Input file path")
    parser.add_argument("output", type=Path, help="Output JSONL path")
    parser.add_argument("--format", choices=["csv", "jsonl"], default="csv")
    parser.add_argument("--log-level", default="INFO")
    args = parser.parse_args()

    logging.basicConfig(level=args.log_level.upper(), stream=sys.stderr)

    stats = run_pipeline(args.input, args.output, format=args.format)
    print(json.dumps({
        "total_read": stats.total_read,
        "total_transformed": stats.total_transformed,
        "total_skipped": stats.total_skipped,
        "total_errors": stats.total_errors,
        "duration_seconds": stats.duration_seconds,
        "success_rate": stats.success_rate,
    }, indent=2))
