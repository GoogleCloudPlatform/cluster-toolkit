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

import json
import sys


def validate_boh_report(filepath: str) -> None:
    try:
        with open(filepath) as f:
            data = json.load(f)
    except Exception as e:
        print(f"Skipping non-JSON or invalid file {filepath}: {e}")
        return

    if not isinstance(data, dict) or "test_results" not in data:
        print(f"Skipping non-BOH JSON file: {filepath}")
        return

    report_id = data.get("report_id", "unknown")
    gcs_link = f"https://pantheon.corp.google.com/storage/browser/_details/chs-results/bills_of_health/{report_id}.json?project=hpc-toolkit-dev"

    print(f"\n=== CHS Bill of Health: {report_id} ===")
    print(f"BOH Storage Link: {gcs_link}\n")

    passed = []
    failed = []

    for tr in data.get("test_results", []):
        tname = tr.get("test_name", "unknown")
        tlevel = tr.get("test_level", "")
        lbl = f"{tname} ({tlevel})" if tlevel else tname

        r_status = tr.get("result_status")

        has_metric_failure = False
        for m in tr.get("metrics", []):
            vc = m.get("validation_criteria", {})
            if vc.get("result_status") and vc.get("result_status") != "RESULT_STATUS_SUCCESS":
                has_metric_failure = True
                break

        is_success = (r_status == "RESULT_STATUS_SUCCESS" or r_status is None) and not has_metric_failure

        if is_success:
            passed.append(lbl)
        else:
            failed.append(lbl)

    print("Passed Tests:")
    for p in sorted(set(passed)):
        print(f"  - [PASS] {p}")

    if failed:
        print("\nFailed Tests:")
        for f in sorted(set(failed)):
            print(f"  - [FAIL] {f}")
        print(f"\nERROR: CHS Bill of Health {report_id} failed with {len(failed)} failed test(s).")
        sys.exit(1)

    print(f"\nSUCCESS: All tests in CHS Bill of Health {report_id} passed.")


def main():
    if len(sys.argv) < 2:
        print("Usage: validate_chs_boh.py <filepath>")
        sys.exit(1)
    validate_boh_report(sys.argv[1])


if __name__ == "__main__":
    main()
