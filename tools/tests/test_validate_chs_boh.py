#!/usr/bin/env python3
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Unit tests for the CHS Bill of Health validator used by daily tests."""

import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest

_SCRIPT = (
    Path(__file__).resolve().parents[1]
    / "cloud-build"
    / "daily-tests"
    / "validate_chs_boh.py"
)
_SPEC = importlib.util.spec_from_file_location("validate_chs_boh", _SCRIPT)
validate_chs_boh = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(validate_chs_boh)


def _metric(key, value, status, name="bandwidth", expected=304.0, **extra):
    metric = {
        "key": key,
        "value": value,
        "validation_criteria": {
            "comparator": "COMPARATOR_GTE",
            "expected_value": expected,
            "name": name,
            "result_status": status,
        },
    }
    metric.update(extra)
    return metric


def _test(name, level, status, metrics=None, nodes=("node-0",)):
    result = {
        "test_name": name,
        "test_level": level,
        "target_resources": [{"node_name": n, "instance_id": "1"} for n in nodes],
        "metrics": metrics or [],
    }
    # proto3 JSON omits RESULT_STATUS_UNKNOWN (enum value 0).
    if status != "RESULT_STATUS_UNKNOWN":
        result["result_status"] = status
    return result


def _report(*test_results):
    return {
        "report_id": "20261014-103955_cluster-standard-abcd",
        "chs_version": "0.3.1",
        "test_results": list(test_results),
    }


class SummarizeTest(unittest.TestCase):

    def test_all_success(self):
        report = _report(
            _test(
                "nccl-pairwise",
                "all-gather",
                "RESULT_STATUS_SUCCESS",
                [_metric("Peak Detected Bandwidth", 181.0, "RESULT_STATUS_SUCCESS")],
                nodes=("node-0", "node-1"),
            ),
            _test("dcgm", "3", "RESULT_STATUS_SUCCESS"),
        )
        lines, has_failures = validate_chs_boh.summarize(report, "chs-results")
        text = "\n".join(lines)
        self.assertFalse(has_failures)
        self.assertIn("[SUCCESS] nccl-pairwise (all-gather) on node-0, node-1", text)
        self.assertIn("compared: bandwidth", text)
        self.assertIn("Summary: 2 SUCCESS", text)
        self.assertIn("gs://chs-results/bills_of_health/20261014-103955_cluster-standard-abcd.json", text)

    def test_failed_metric_is_reported_with_comparison(self):
        report = _report(
            _test(
                "nccl-pairwise",
                "all-reduce",
                "RESULT_STATUS_FAILED",
                [
                    {"key": "Avg. Bus Bandwidth", "value": 224.7, "unit": "GB/s"},
                    _metric(
                        "Peak Detected Bandwidth",
                        224.774,
                        "RESULT_STATUS_FAILED",
                        unit="GB/s",
                    ),
                ],
                nodes=("node-0", "node-1"),
            )
        )
        lines, has_failures = validate_chs_boh.summarize(report, "chs-results")
        text = "\n".join(lines)
        self.assertTrue(has_failures)
        self.assertIn("[FAILED] nccl-pairwise (all-reduce) on node-0, node-1", text)
        self.assertIn(
            "[FAILED] bandwidth: Peak Detected Bandwidth = 224.774 GB/s, expected >= 304.0",
            text,
        )
        # Metrics without validation criteria are not listed as comparisons.
        self.assertNotIn("Avg. Bus Bandwidth", text)
        # Failures are recapped at the end of the report.
        self.assertIn(
            "Failed tests (1):\n  - [FAILED] nccl-pairwise (all-reduce) on node-0, node-1",
            text,
        )
        self.assertIn("ERROR:", text)

    def test_unknown_status_is_not_a_failure(self):
        report = _report(_test("tpu-info", "local", "RESULT_STATUS_UNKNOWN"))
        lines, has_failures = validate_chs_boh.summarize(report, "chs-results")
        text = "\n".join(lines)
        self.assertFalse(has_failures)
        self.assertIn("[UNKNOWN] tpu-info (local) on node-0", text)
        self.assertIn("Summary: 1 UNKNOWN", text)

    def test_timeout_and_cancelled_are_failures(self):
        for status in ("RESULT_STATUS_TIMEOUT", "RESULT_STATUS_CANCELLED", "RESULT_STATUS_DEADLINE"):
            with self.subTest(status=status):
                _, has_failures = validate_chs_boh.summarize(
                    _report(_test("gemm", "check", status)), "chs-results"
                )
                self.assertTrue(has_failures)

    def test_failed_metric_under_successful_test_is_a_failure(self):
        report = _report(
            _test(
                "stream",
                "standard",
                "RESULT_STATUS_SUCCESS",
                [_metric("Achieved Bandwidth", 1.0, "RESULT_STATUS_FAILED", name="efficiency", expected=0.85)],
            )
        )
        _, has_failures = validate_chs_boh.summarize(report, "chs-results")
        self.assertTrue(has_failures)

    def test_range_and_set_expectations(self):
        rng = {"comparator": "COMPARATOR_RANGE", "range_values": {"min_value": 1, "max_value": 2}}
        st = {"comparator": "COMPARATOR_IN", "set_values": {"items": ["a", "b"]}}
        self.assertEqual(validate_chs_boh.format_expected(rng), "[1, 2]")
        self.assertEqual(validate_chs_boh.format_expected(st), "['a', 'b']")

    def test_defensive_null_and_missing_values(self):
        # Null range_values or set_values should not raise AttributeError
        self.assertEqual(validate_chs_boh.format_expected({"range_values": None}), "?")
        self.assertEqual(validate_chs_boh.format_expected({"set_values": None}), "?")

        # Metric without unit or context
        metric = {"key": "gpu_count", "value": 8}
        self.assertEqual(validate_chs_boh.format_metric(metric), "gpu_count = 8")

        # Test result with null target_resources or null metrics
        tr = {"test_name": "dummy", "target_resources": None, "metrics": None}
        self.assertEqual(validate_chs_boh.test_label(tr), "dummy")
        self.assertEqual(validate_chs_boh.validated_metrics(tr), [])

    def test_empty_report_no_test_results(self):
        # When 0 tests run, proto3 omits test_results
        report = {"report_id": "empty-run", "chs_version": "0.3.1"}
        lines, has_failures = validate_chs_boh.summarize(report, "chs-results")
        text = "\n".join(lines)
        self.assertFalse(has_failures)
        self.assertIn("Summary: no test results", text)


class MainTest(unittest.TestCase):

    def _write(self, data):
        fd, path = tempfile.mkstemp(suffix=".json")
        with os.fdopen(fd, "w") as f:
            json.dump(data, f)
        self.addCleanup(os.remove, path)
        return path

    def test_exit_code_reflects_failures(self):
        ok = self._write(_report(_test("dcgm", "3", "RESULT_STATUS_SUCCESS")))
        bad = self._write(_report(_test("dcgm", "3", "RESULT_STATUS_FAILED")))
        self.assertEqual(validate_chs_boh.main([ok]), 0)
        self.assertEqual(validate_chs_boh.main([bad]), 1)

    def test_empty_report_exits_zero(self):
        empty = self._write({"report_id": "empty-run", "chs_version": "0.3.1"})
        self.assertEqual(validate_chs_boh.main([empty]), 0)

    def test_invalid_json_raises(self):
        fd, path = tempfile.mkstemp(suffix=".json")
        with os.fdopen(fd, "w") as f:
            f.write("not json")
        self.addCleanup(os.remove, path)
        with self.assertRaises(json.JSONDecodeError):
            validate_chs_boh.main([path])


if __name__ == "__main__":
    unittest.main()
