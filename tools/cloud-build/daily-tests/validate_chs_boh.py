#!/usr/bin/env python3
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Summarizes a CHS (Cluster Health Scanner) Bill of Health JSON report.

The Bill of Health is produced by CHS (see src/chs/protos/bill_of_health.proto
in the CHS repository) and uploaded to
gs://<bucket>/bills_of_health/<report_id>.json. This script prints, for every
test result, the CHS RESULT_STATUS_* value, the target nodes, and the metrics
that were compared against validation criteria. It exits non-zero when any
test (or metric validation criterion) reports a non-success status.

RESULT_STATUS_UNKNOWN is emitted by CHS when a test has no thresholds
configured for the platform; it is reported but not treated as a failure.
"""

import argparse
import json
import sys
from typing import Any, Dict, Iterable, List, Optional, Tuple

SUCCESS = "RESULT_STATUS_SUCCESS"
# proto3 omits default enum values from JSON, so a missing status is UNKNOWN.
UNKNOWN = "RESULT_STATUS_UNKNOWN"

COMPARATORS = {
    "COMPARATOR_EQ": "==",
    "COMPARATOR_NE": "!=",
    "COMPARATOR_GT": ">",
    "COMPARATOR_GTE": ">=",
    "COMPARATOR_LT": "<",
    "COMPARATOR_LTE": "<=",
    "COMPARATOR_RANGE": "in range",
    "COMPARATOR_IN": "in",
    "COMPARATOR_NOT_IN": "not in",
    "COMPARATOR_CONTAINS": "contains",
}


def short_status(status: str) -> str:
    return status.replace("RESULT_STATUS_", "")


def format_expected(criteria: Dict[str, Any]) -> str:
    """Renders the expected side of a ValidationCriteria oneof."""
    if "expected_value" in criteria:
        return str(criteria["expected_value"])
    rng = criteria.get("range_values")
    if isinstance(rng, dict):
        return f"[{rng.get('min_value')}, {rng.get('max_value')}]"
    set_vals = criteria.get("set_values")
    if isinstance(set_vals, dict):
        return str(set_vals.get("items") or [])
    return "?"


def format_metric(metric: Dict[str, Any]) -> str:
    """Formats a validated metric as '<key>[ (<context>)] = <value><unit>'."""
    metadata = metric.get("metadata") or {}
    context = metadata.get("context") or ""
    label = f"{metric.get('key')} ({context})" if context else str(metric.get("key"))
    if "relative" in metadata:
        rel = metadata["relative"]
        rel_str = f"{rel:.3f}" if isinstance(rel, (int, float)) else str(rel)
        unit = f" {metric.get('unit')}" if metric.get("unit") else ""
        raw = f"{metric.get('value')}{unit}" if metric.get("value") is not None else ""
        return f"{label} = {rel_str} (ratio, raw: {raw})" if raw else f"{label} = {rel_str} (ratio)"
    unit = f" {metric.get('unit')}" if metric.get("unit") else ""
    return f"{label} = {metric.get('value')}{unit}"


def format_criteria(metric: Dict[str, Any]) -> str:
    criteria = metric.get("validation_criteria") or {}
    comparator = COMPARATORS.get(criteria.get("comparator", ""), criteria.get("comparator", "?"))
    status = short_status(criteria.get("result_status", UNKNOWN))
    return (
        f"[{status}] {criteria.get('name', 'criteria')}: {format_metric(metric)}"
        f", expected {comparator} {format_expected(criteria)}"
    )


def validated_metrics(test_result: Dict[str, Any]) -> List[Dict[str, Any]]:
    metrics = test_result.get("metrics") or []
    return [m for m in metrics if isinstance(m, dict) and m.get("validation_criteria")]


def test_label(test_result: Dict[str, Any]) -> str:
    name = test_result.get("test_name", "unknown")
    level = test_result.get("test_level")
    resources = test_result.get("target_resources") or []
    nodes = ", ".join(
        r.get("node_name", "?") for r in resources if isinstance(r, dict)
    )
    label = f"{name} ({level})" if level else name
    return f"{label} on {nodes}" if nodes else label


def is_failure(test_result: Dict[str, Any]) -> bool:
    """True when the test or any of its metric criteria is non-success/non-unknown."""
    statuses = [test_result.get("result_status", UNKNOWN)]
    statuses += [
        m.get("validation_criteria", {}).get("result_status", UNKNOWN)
        for m in validated_metrics(test_result)
    ]
    return any(s not in (SUCCESS, UNKNOWN) for s in statuses)


def summarize(report: Dict[str, Any], bucket: str) -> Tuple[List[str], bool]:
    """Builds the report lines. Returns (lines, has_failures)."""
    report_id = report.get("report_id", "unknown")
    lines = [
        f"=== CHS Bill of Health: {report_id} (chs {report.get('chs_version', '?')}) ===",
        f"JSON: gs://{bucket}/bills_of_health/{report_id}.json",
        f"HTML: https://storage.cloud.google.com/{bucket}/bills_of_health/{report_id}.html",
        "",
    ]

    counts: Dict[str, int] = {}
    failed_labels: List[str] = []
    test_results = [t for t in (report.get("test_results") or []) if isinstance(t, dict)]

    if not test_results:
        lines.append("")
        lines.append("Summary: no test results")
        lines.append(f"ERROR: CHS Bill of Health '{report_id}' contains no test results.")
        return lines, True

    for tr in sorted(
        test_results,
        key=lambda t: (t.get("test_name") or "", t.get("test_level") or "", test_label(t)),
    ):
        status = tr.get("result_status", UNKNOWN)
        counts[status] = counts.get(status, 0) + 1
        failed = is_failure(tr)
        lines.append(f"[{short_status(status)}] {test_label(tr)}")

        metrics = validated_metrics(tr)
        if failed:
            failed_labels.append(f"[{short_status(status)}] {test_label(tr)}")
            # Show every compared metric so the failing comparison is visible.
            lines += [f"    - {format_criteria(m)}" for m in metrics]
        elif metrics:
            # Compact view: which criteria were compared.
            names = sorted({m["validation_criteria"].get("name", "criteria") for m in metrics})
            lines.append(f"    - compared: {', '.join(names)}")

    has_failures = bool(failed_labels)
    lines.append("")
    summary = ", ".join(f"{counts[s]} {short_status(s)}" for s in sorted(counts))
    lines.append(f"Summary: {summary or 'no test results'}")
    if has_failures:
        # Recap failures at the end so they are easy to find in long build logs.
        lines.append(f"Failed tests ({len(failed_labels)}):")
        lines += [f"  - {label}" for label in failed_labels]
        lines.append(f"ERROR: CHS Bill of Health '{report_id}' contains non-success results.")
    else:
        lines.append(f"SUCCESS: CHS Bill of Health '{report_id}' has no failing results.")
    return lines, has_failures


def load_report(path: str) -> Dict[str, Any]:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
    if not isinstance(data, dict):
        raise ValueError(f"{path} is not a valid JSON object")
    return data


def main(argv: Optional[Iterable[str]] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("boh_json", help="Path to a downloaded bill_of_health JSON report")
    parser.add_argument("--bucket", default="chs-results", help="GCS bucket holding CHS results")
    args = parser.parse_args(argv)

    lines, has_failures = summarize(load_report(args.boh_json), args.bucket)
    print("\n".join(lines))
    return 1 if has_failures else 0


if __name__ == "__main__":
    sys.exit(main())
