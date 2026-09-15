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

"""Unit tests for Cluster Toolkit Cloud Build zone retry and exclusion logic.

How to run these tests:
  Direct execution:
    python3 tools/tests/test_cloud_build_zone_retry.py

  Via unittest module:
    python3 -m unittest tools/tests/test_cloud_build_zone_retry.py -v

  Via test runner discovery:
    python3 -m unittest discover tools/tests/

  Via pre-commit (automatically run on commit):
    pre-commit run pytest-check --all-files
"""

import os
import shutil
import subprocess
import tempfile
import unittest
import yaml

UPDATE_SCRIPT = os.path.abspath(
    os.path.join(os.path.dirname(__file__), "../cloud-build/update_job_exclude_zones.py")
)

SAMPLE_JOB_YAML = """\
apiVersion: batch/v1
kind: Job
metadata:
  name: ml-a3-ultragpu-onspot-slurm-123456
  namespace: default
  labels:
    kueue.x-k8s.io/queue-name: local-queue-test-locks
spec:
  suspend: true
  template:
    spec:
      containers:
      - name: runner
        image: us-central1-docker.pkg.dev/proj/repo/test-runner:123
        env:
        - name: CHECK_LUSTRE
          value: "true"
        - name: REQUIRED_LUSTRE_GB
          value: "36000"
        - name: BUILD_ID
          value: "123456"
"""


class TestCloudBuildZoneRetry(unittest.TestCase):

    def setUp(self):
        self.test_dir = tempfile.mkdtemp()

    def tearDown(self):
        shutil.rmtree(self.test_dir)

    def test_extract_failed_zone_from_lustre_exhaustion_log(self):
        """Test extracting failed zone from Terraform/Managed Lustre resource exhausted logs."""
        log_content = (
            "module.homefs.google_lustre_instance.lustre_instance: Creating...\n"
            "Error: Error waiting for creating Instance: Error code 8, message: "
            "resource exhausted: not enough resources available to fulfill the request in us-west1-b\n"
            "with module.homefs.google_lustre_instance.lustre_instance,\n"
        )
        log_file = os.path.join(self.test_dir, "job_logs.txt")
        with open(log_file, "w", encoding="utf-8") as f:
            f.write(log_content)

        cmd = (
            f"sed -n 's/.*resource exhausted: not enough resources available to fulfill the request in "
            f"\\([a-z0-9-]*\\).*/\\1/p' {log_file} | tail -n 1"
        )
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, check=True)
        self.assertEqual(result.stdout.strip(), "us-west1-b")

    def test_extract_failed_zone_from_spot_stockout_log(self):
        """Test extracting zone from 'Deploying in ZONE:' only when capacity error is present."""
        log_content = (
            "Deploying in ZONE: us-south1-b, MODEL: SPOT\n"
            "Starting terraform deployment...\n"
            "google_compute_region_instance_group_manager: ZONE_RESOURCE_POOL_EXHAUSTED\n"
        )
        log_file = os.path.join(self.test_dir, "job_logs.txt")
        with open(log_file, "w", encoding="utf-8") as f:
            f.write(log_content)

        script = f"""
        FAILED_ZONE=$(sed -n 's/.*resource exhausted: not enough resources available to fulfill the request in \\([a-z0-9-]*\\).*/\\1/p' {log_file} | tail -n 1 || true)
        if [ -z "$FAILED_ZONE" ]; then
            if grep -qE "ZONE_RESOURCE_POOL_EXHAUSTED|does not have enough resources available" {log_file}; then
                FAILED_ZONE=$(sed -n 's/.*Deploying in ZONE: \\([a-z0-9-]*\\).*/\\1/p' {log_file} | tail -n 1 || true)
            fi
        fi
        echo "$FAILED_ZONE"
        """
        result = subprocess.run(["bash", "-c", script], capture_output=True, text=True, check=True)
        self.assertEqual(result.stdout.strip(), "us-south1-b")

    def test_no_zone_extraction_on_non_capacity_retriable_error(self):
        """Verify that non-capacity transient errors do not extract or blacklist healthy zones."""
        log_content = (
            "Deploying in ZONE: us-central1-a, MODEL: SPOT\n"
            "Error: googleapi: Error 429: Rate Limit Exceeded, RATE_LIMIT_EXCEEDED\n"
        )
        log_file = os.path.join(self.test_dir, "job_logs.txt")
        with open(log_file, "w", encoding="utf-8") as f:
            f.write(log_content)

        script = f"""
        FAILED_ZONE=$(sed -n 's/.*resource exhausted: not enough resources available to fulfill the request in \\([a-z0-9-]*\\).*/\\1/p' {log_file} | tail -n 1 || true)
        if [ -z "$FAILED_ZONE" ]; then
            if grep -qE "ZONE_RESOURCE_POOL_EXHAUSTED|does not have enough resources available" {log_file}; then
                FAILED_ZONE=$(sed -n 's/.*Deploying in ZONE: \\([a-z0-9-]*\\).*/\\1/p' {log_file} | tail -n 1 || true)
            fi
        fi
        echo "$FAILED_ZONE"
        """
        result = subprocess.run(["bash", "-c", script], capture_output=True, text=True, check=True)
        self.assertEqual(result.stdout.strip(), "")

    def test_update_job_exclude_zones_basic_injection_and_update(self):
        """Test update_job_exclude_zones.py injecting and updating EXCLUDE_ZONES."""
        job_file = os.path.join(self.test_dir, "job.yaml")
        with open(job_file, "w", encoding="utf-8") as f:
            f.write(SAMPLE_JOB_YAML)

        # 1. First injection
        subprocess.run(
            ["python3", UPDATE_SCRIPT, "--inject", "--file", job_file, "--zones", "us-west1-b"],
            check=True,
        )

        with open(job_file, "r", encoding="utf-8") as f:
            parsed = yaml.safe_load(f)

        runner = next(c for c in parsed["spec"]["template"]["spec"]["containers"] if c["name"] == "runner")
        entry = next((e for e in runner["env"] if e["name"] == "EXCLUDE_ZONES"), None)
        self.assertIsNotNone(entry)
        self.assertEqual(entry["value"], "us-west1-b")

        # 2. Extraction
        res = subprocess.run(
            ["python3", UPDATE_SCRIPT, "--extract", "--file", job_file],
            capture_output=True,
            text=True,
            check=True,
        )
        self.assertEqual(res.stdout.strip(), "us-west1-b")

        # 3. Update with accumulated zones
        subprocess.run(
            ["python3", UPDATE_SCRIPT, "--inject", "--file", job_file, "--zones", "us-west1-b us-south1-b"],
            check=True,
        )

        with open(job_file, "r", encoding="utf-8") as f:
            parsed_retry = yaml.safe_load(f)

        runner_retry = next(c for c in parsed_retry["spec"]["template"]["spec"]["containers"] if c["name"] == "runner")
        entry_retry = next((e for e in runner_retry["env"] if e["name"] == "EXCLUDE_ZONES"), None)
        self.assertIsNotNone(entry_retry)
        self.assertEqual(entry_retry["value"], "us-west1-b us-south1-b")

    def test_update_job_exclude_zones_preserves_metadata_and_sidecars(self):
        """Test that metadata labels (env: staging) and sidecar containers are not corrupted."""
        complex_job = """\
apiVersion: batch/v1
kind: Job
metadata:
  name: test-job
  labels:
    env: staging
    app: test
spec:
  template:
    spec:
      containers:
      - name: sidecar
        image: gcr.io/sidecar:latest
        env:
        - name: SIDECAR_CONFIG
          value: "default"
      - name: runner
        image: gcr.io/runner:latest
        env:
        - name: CHECK_LUSTRE
          value: "true"
      volumes:
      - name: scratch
        emptyDir: {}
"""
        job_file = os.path.join(self.test_dir, "job.yaml")
        with open(job_file, "w", encoding="utf-8") as f:
            f.write(complex_job)

        subprocess.run(
            ["python3", UPDATE_SCRIPT, "--inject", "--file", job_file, "--zones", "us-west1-a us-west1-b"],
            check=True,
        )

        with open(job_file, "r", encoding="utf-8") as f:
            parsed = yaml.safe_load(f)

        # Verify metadata label was NOT modified or confused with container env
        self.assertEqual(parsed["metadata"]["labels"]["env"], "staging")

        # Verify sidecar container env was NOT injected
        sidecar = next(c for c in parsed["spec"]["template"]["spec"]["containers"] if c["name"] == "sidecar")
        self.assertFalse(any(e["name"] == "EXCLUDE_ZONES" for e in sidecar["env"]))

        # Verify runner container received EXCLUDE_ZONES
        runner = next(c for c in parsed["spec"]["template"]["spec"]["containers"] if c["name"] == "runner")
        entry = next((e for e in runner["env"] if e["name"] == "EXCLUDE_ZONES"), None)
        self.assertIsNotNone(entry)
        self.assertEqual(entry["value"], "us-west1-a us-west1-b")

    def test_update_job_exclude_zones_inverted_key_order(self):
        """Test extraction and update when value: precedes name: in YAML."""
        inverted_job = """\
apiVersion: batch/v1
kind: Job
spec:
  template:
    spec:
      containers:
      - name: runner
        image: gcr.io/runner:latest
        env:
        - value: "us-central1-a us-central1-b"
          name: EXCLUDE_ZONES
        - name: CHECK_LUSTRE
          value: "true"
"""
        job_file = os.path.join(self.test_dir, "job.yaml")
        with open(job_file, "w", encoding="utf-8") as f:
            f.write(inverted_job)

        # Extract pre-existing
        res = subprocess.run(
            ["python3", UPDATE_SCRIPT, "--extract", "--file", job_file],
            capture_output=True,
            text=True,
            check=True,
        )
        self.assertEqual(res.stdout.strip(), "us-central1-a us-central1-b")

        # Update
        subprocess.run(
            ["python3", UPDATE_SCRIPT, "--inject", "--file", job_file, "--zones", "us-central1-a us-central1-b us-west1-b"],
            check=True,
        )

        with open(job_file, "r", encoding="utf-8") as f:
            parsed = yaml.safe_load(f)

        runner = next(c for c in parsed["spec"]["template"]["spec"]["containers"] if c["name"] == "runner")
        entry = next((e for e in runner["env"] if e["name"] == "EXCLUDE_ZONES"), None)
        self.assertIsNotNone(entry)
        self.assertEqual(entry["value"], "us-central1-a us-central1-b us-west1-b")

    def test_update_job_exclude_zones_missing_env_fallback(self):
        """Test injection when runner container has no env block at all."""
        no_env_job = """\
apiVersion: batch/v1
kind: Job
spec:
  template:
    spec:
      containers:
      - name: runner
        image: gcr.io/runner:latest
        command: ["bash"]
"""
        job_file = os.path.join(self.test_dir, "job.yaml")
        with open(job_file, "w", encoding="utf-8") as f:
            f.write(no_env_job)

        subprocess.run(
            ["python3", UPDATE_SCRIPT, "--inject", "--file", job_file, "--zones", "europe-west2-c"],
            check=True,
        )

        with open(job_file, "r", encoding="utf-8") as f:
            parsed = yaml.safe_load(f)

        runner = next(c for c in parsed["spec"]["template"]["spec"]["containers"] if c["name"] == "runner")
        self.assertEqual(len(runner["env"]), 1)
        self.assertEqual(runner["env"][0]["name"], "EXCLUDE_ZONES")
        self.assertEqual(runner["env"][0]["value"], "europe-west2-c")

    def test_zone_exclusion_matching_token_safety(self):
        """Test that token-padded pattern matching correctly isolates zone names."""
        script = """
        check_zone() {
            local ZONE=$1
            local EXCLUDE_ZONES=$2
            if [[ " ${EXCLUDE_ZONES//,/ } " == *" ${ZONE} "* ]]; then
                echo "EXCLUDED"
            else
                echo "ALLOWED"
            fi
        }

        # Exact match in space-delimited list
        check_zone "us-west1-b" "us-west1-a us-west1-b us-west1-c"
        # Exact match in comma-delimited list
        check_zone "us-west1-b" "us-west1-a,us-west1-b,us-west1-c"
        # Partial prefix should NOT match (us-west1-b must not match us-west1-ba)
        check_zone "us-west1-ba" "us-west1-b"
        # Partial suffix should NOT match (us-west1-b must not match us-west1)
        check_zone "us-west1" "us-west1-b"
        # Single element match
        check_zone "europe-west2-c" "europe-west2-c"
        """
        result = subprocess.run(["bash", "-c", script], capture_output=True, text=True, check=True)
        outputs = result.stdout.strip().splitlines()
        self.assertEqual(outputs[0], "EXCLUDED")
        self.assertEqual(outputs[1], "EXCLUDED")
        self.assertEqual(outputs[2], "ALLOWED")
        self.assertEqual(outputs[3], "ALLOWED")
        self.assertEqual(outputs[4], "EXCLUDED")

    def test_find_available_zone_lustre_defaults_and_override(self):
        """Test default Lustre exclusions, bypass with 'none', and non-Lustre immunity."""
        script = """
        filter_zones() {
            local CHECK_LUSTRE=$1
            local LUSTRE_EXCLUDE_ZONES=$2
            local EXCLUDE_ZONES=$3
            local ZONES=("us-west1-a" "us-west1-b" "us-west1-c" "us-south1-b" "europe-west2-c" "us-central1-a")
            local KEPT=()

            for ZONE in "${ZONES[@]}"; do
                if [[ -n "${EXCLUDE_ZONES:-}" ]]; then
                    if [[ " ${EXCLUDE_ZONES//,/ } " == *" ${ZONE} "* ]]; then
                        continue
                    fi
                fi

                if [[ "${CHECK_LUSTRE:-false}" == "true" ]]; then
                    LUSTRE_EXCLUDE="${LUSTRE_EXCLUDE_ZONES:-us-west1-a us-west1-b us-west1-c us-south1-b europe-west2-c}"
                    if [[ "${LUSTRE_EXCLUDE}" != "none" ]]; then
                        if [[ " ${LUSTRE_EXCLUDE//,/ } " == *" ${ZONE} "* ]]; then
                            continue
                        fi
                    fi
                fi

                KEPT+=("${ZONE}")
            done
            echo "${KEPT[*]}"
        }

        # Case 1: CHECK_LUSTRE=true, default exclusions active -> only us-central1-a survives
        filter_zones "true" "" ""

        # Case 2: CHECK_LUSTRE=false (e.g. JBVM or GKE non-Lustre test) -> all survive
        filter_zones "false" "" ""

        # Case 3: CHECK_LUSTRE=true, but LUSTRE_EXCLUDE_ZONES="none" -> all survive
        filter_zones "true" "none" ""

        # Case 4: Custom EXCLUDE_ZONES="us-central1-a" with CHECK_LUSTRE=false -> us-central1-a filtered out
        filter_zones "false" "" "us-central1-a"
        """
        result = subprocess.run(["bash", "-c", script], capture_output=True, text=True, check=True)
        outputs = result.stdout.strip().splitlines()

        self.assertEqual(outputs[0], "us-central1-a")
        self.assertEqual(outputs[1], "us-west1-a us-west1-b us-west1-c us-south1-b europe-west2-c us-central1-a")
        self.assertEqual(outputs[2], "us-west1-a us-west1-b us-west1-c us-south1-b europe-west2-c us-central1-a")
        self.assertEqual(outputs[3], "us-west1-a us-west1-b us-west1-c us-south1-b europe-west2-c")


if __name__ == "__main__":
    unittest.main()
