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

import argparse
import json
import os
import sys


def validate_boh_report(filepath: str, project_id: str, bucket_name: str) -> None:
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
    gcs_link = f"https://pantheon.corp.google.com/storage/browser/_details/{bucket_name}/bills_of_health/{report_id}.json?project={project_id}"

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
            vc_status = vc.get("result_status")
            if vc_status and vc_status != "RESULT_STATUS_SUCCESS":
                has_metric_failure = True
                break

        # Explicitly require RESULT_STATUS_SUCCESS (or None if no error present)
        is_success = (r_status == "RESULT_STATUS_SUCCESS" or r_status is None) and not has_metric_failure

        raw_status = r_status if r_status else ("RESULT_STATUS_SUCCESS" if is_success else "RESULT_STATUS_FAILED")
        entry = f"  - [{raw_status}] {lbl}"

        if is_success:
            passed.append(entry)
        else:
            failed.append(entry)

    print("Passed Tests:")
    for p in sorted(set(passed)):
        print(p)

    if failed:
        print("\nNon-Success Tests:")
        for f in sorted(set(failed)):
            print(f)
        print(f"\nERROR: CHS Bill of Health {report_id} failed with {len(failed)} non-success test(s).")
        sys.exit(1)

    print(f"\nSUCCESS: All tests in CHS Bill of Health {report_id} passed.")


def main():
    parser = argparse.ArgumentParser(description="Validate CHS Bill of Health JSON report.")
    parser.add_argument("filepath", help="Path to Bill of Health JSON report file")
    parser.add_argument(
        "--project",
        default=os.environ.get("PROJECT_ID", "hpc-toolkit-dev"),
        help="GCP Project ID for storage link",
    )
    parser.add_argument(
        "--bucket",
        default=os.environ.get("CHS_BUCKET", "chs-results"),
        help="GCS Bucket Name for storage link",
    )
    args = parser.parse_args()

    validate_boh_report(args.filepath, args.project, args.bucket)


if __name__ == "__main__":
    main()
