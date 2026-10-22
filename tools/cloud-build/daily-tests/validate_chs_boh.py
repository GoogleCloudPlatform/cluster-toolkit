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
from typing import Any, Dict, Iterable, List, Tuple

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
    if "range_values" in criteria:
        rng = criteria["range_values"]
        return f"[{rng.get('min_value')}, {rng.get('max_value')}]"
    if "set_values" in criteria:
        return str(criteria["set_values"].get("items", []))
    return "?"


def format_metric(metric: Dict[str, Any]) -> str:
    """Formats a validated metric as '<key>[ (<context>)] = <value><unit>'."""
    context = (metric.get("metadata") or {}).get("context") or ""
    label = f"{metric.get('key')} ({context})" if context else str(metric.get("key"))
    unit = f" {metric['unit']}" if metric.get("unit") else ""
    return f"{label} = {metric.get('value')}{unit}"


def format_criteria(metric: Dict[str, Any]) -> str:
    criteria = metric["validation_criteria"]
    comparator = COMPARATORS.get(criteria.get("comparator", ""), criteria.get("comparator", "?"))
    status = short_status(criteria.get("result_status", UNKNOWN))
    return (
        f"[{status}] {criteria.get('name', 'criteria')}: {format_metric(metric)}"
        f", expected {comparator} {format_expected(criteria)}"
    )


def validated_metrics(test_result: Dict[str, Any]) -> List[Dict[str, Any]]:
    return [m for m in test_result.get("metrics", []) if m.get("validation_criteria")]


def test_label(test_result: Dict[str, Any]) -> str:
    name = test_result.get("test_name", "unknown")
    level = test_result.get("test_level")
    nodes = ", ".join(
        r.get("node_name", "?") for r in test_result.get("target_resources", [])
    )
    label = f"{name} ({level})" if level else name
    return f"{label} on {nodes}" if nodes else label


def is_failure(test_result: Dict[str, Any]) -> bool:
    """True when the test or any of its metric criteria is non-success/non-unknown."""
    statuses = [test_result.get("result_status", UNKNOWN)]
    statuses += [
        m["validation_criteria"].get("result_status", UNKNOWN)
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
    has_failures = False
    for tr in sorted(
        report.get("test_results", []),
        key=lambda t: (t.get("test_name", ""), t.get("test_level", ""), test_label(t)),
    ):
        status = tr.get("result_status", UNKNOWN)
        counts[status] = counts.get(status, 0) + 1
        failed = is_failure(tr)
        has_failures = has_failures or failed
        lines.append(f"[{short_status(status)}] {test_label(tr)}")

        metrics = validated_metrics(tr)
        if failed:
            # Show every compared metric so the failing comparison is visible.
            lines += [f"    - {format_criteria(m)}" for m in metrics]
        elif metrics:
            # Compact view: which criteria were compared.
            names = sorted({m["validation_criteria"].get("name", "criteria") for m in metrics})
            lines.append(f"    - compared: {', '.join(names)}")

    lines.append("")
    summary = ", ".join(f"{counts[s]} {short_status(s)}" for s in sorted(counts))
    lines.append(f"Summary: {summary or 'no test results'}")
    if has_failures:
        lines.append(f"ERROR: CHS Bill of Health '{report_id}' contains non-success results.")
    else:
        lines.append(f"SUCCESS: CHS Bill of Health '{report_id}' has no failing results.")
    return lines, has_failures


def load_report(path: str) -> Dict[str, Any]:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
    if not isinstance(data, dict) or "test_results" not in data:
        raise ValueError(f"{path} is not a CHS Bill of Health (missing 'test_results')")
    return data


def main(argv: Iterable[str] = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("boh_json", help="Path to a downloaded bill_of_health JSON report")
    parser.add_argument("--bucket", default="chs-results", help="GCS bucket holding CHS results")
    args = parser.parse_args(argv)

    lines, has_failures = summarize(load_report(args.boh_json), args.bucket)
    print("\n".join(lines))
    return 1 if has_failures else 0


if __name__ == "__main__":
    sys.exit(main())
