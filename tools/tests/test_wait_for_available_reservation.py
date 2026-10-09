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

"""Unit tests for wait_for_available_reservation.sh.

How to run these tests:
  Direct execution:
    python3 tools/tests/test_wait_for_available_reservation.py

  Via unittest module:
    python3 -m unittest tools/tests/test_wait_for_available_reservation.py -v

  Via test runner discovery:
    python3 -m unittest discover tools/tests/
"""

import glob
import os
import re
import shlex
import shutil
import stat
import subprocess
import tempfile
import unittest
import yaml

WAIT_SCRIPT = os.path.abspath(
    os.path.join(
        os.path.dirname(__file__),
        "../cloud-build/wait_for_available_reservation.sh",
    )
)

BUILDS_DIR = os.path.abspath(
    os.path.join(
        os.path.dirname(__file__),
        "../cloud-build/daily-tests/builds",
    )
)

TESTS_DIR = os.path.abspath(
    os.path.join(
        os.path.dirname(__file__),
        "../cloud-build/daily-tests/tests",
    )
)

# Representative reservation configurations across single-project and shared-project setups
RESERVATION_TEST_CASES = [
    # 1. Explicit project argument ($3) with exact capacity match (Available == Required)
    {
        "arg": "g4-reservation-0",
        "expected_name": "g4-reservation-0",
        "zone": "us-south1-a",
        "project_arg": "hpc-toolkit-dev",
        "expected_project": "hpc-toolkit-dev",
        "required_count": "2",
        "total": 4,
        "in_use": 2,
    },
    # 2. Excess capacity (Available > Required)
    {
        "arg": "a4-reservation-0",
        "expected_name": "a4-reservation-0",
        "zone": "us-central1-b",
        "project_arg": "hpc-toolkit-dev",
        "expected_project": "hpc-toolkit-dev",
        "required_count": "4",
        "total": 8,
        "in_use": 2,
    },
    # 3. Same-project reservation (no project argument provided)
    {
        "arg": "nvidia-gb200-jlr31fm12d525",
        "expected_name": "nvidia-gb200-jlr31fm12d525",
        "zone": "us-central1-b",
        "project_arg": "",
        "expected_project": "",
        "required_count": "2",
        "total": 2,
        "in_use": 0,
    },
    # 4. Shared-project reservation via URI (no $3 project argument)
    {
        "arg": "projects/shared-owner-proj/zones/us-east4-a/reservations/a3-mega-res-1",
        "expected_name": "a3-mega-res-1",
        "zone": "us-east4-a",
        "project_arg": "",
        "expected_project": "shared-owner-proj",
        "required_count": "8",
        "total": 16,
        "in_use": 8,
    },
    # 5. Shared-project reservation via URI where $3 is consumer project (URI owner project wins)
    {
        "arg": "projects/shared-host-proj/zones/us-south1-a/reservations/shared-g4-res",
        "expected_name": "shared-g4-res",
        "zone": "us-south1-a",
        "project_arg": "consumer-build-proj",
        "expected_project": "shared-host-proj",
        "required_count": "2",
        "total": 4,
        "in_use": 1,
    },
]


class TestWaitForAvailableReservation(unittest.TestCase):

    def setUp(self):
        self.test_dir = tempfile.mkdtemp()
        self.bin_dir = os.path.join(self.test_dir, "bin")
        self.script_tmp_dir = os.path.join(self.test_dir, "script_tmp")
        os.makedirs(self.bin_dir)
        os.makedirs(self.script_tmp_dir)

        self.gcloud_args_log = os.path.join(self.test_dir, "gcloud_args.log")
        self.sleep_args_log = os.path.join(self.test_dir, "sleep_args.log")
        self.call_count_file = os.path.join(self.test_dir, "gcloud_calls.count")

        # Default mock sleep: logs sleep duration and returns 0 immediately
        self._write_executable(
            os.path.join(self.bin_dir, "sleep"),
            f'#!/bin/bash\necho "$*" >> "{self.sleep_args_log}"\nexit 0\n',
        )

        self.env = os.environ.copy()
        self.env["PATH"] = f"{self.bin_dir}:{self.env.get('PATH', '')}"
        self.env["TMPDIR"] = self.script_tmp_dir

    def tearDown(self):
        # Verify no temporary files were leaked by wait_for_available_reservation.sh
        if os.path.isdir(self.script_tmp_dir):
            leaked = os.listdir(self.script_tmp_dir)
        else:
            leaked = []
        shutil.rmtree(self.test_dir)
        self.assertEqual(
            leaked,
            [],
            f"Temporary file(s) leaked in TMPDIR after script exit: {leaked}",
        )

    def _reset_logs(self):
        for path in (self.gcloud_args_log, self.sleep_args_log, self.call_count_file):
            if os.path.exists(path):
                os.remove(path)

    def _write_executable(self, path, content):
        with open(path, "w", encoding="utf-8") as f:
            f.write(content)
        os.chmod(path, os.stat(path).st_mode | stat.S_IXUSR | stat.S_IXGRP)

    def _write_mock_gcloud(self, body):
        script = (
            "#!/bin/bash\n"
            f'echo "$*" >> "{self.gcloud_args_log}"\n'
            f"{body}\n"
        )
        self._write_executable(os.path.join(self.bin_dir, "gcloud"), script)

    def _run_script(self, args, script_path=WAIT_SCRIPT, timeout=10):
        return subprocess.run(
            ["bash", script_path] + args,
            env=self.env,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )

    def test_script_contains_required_defensive_constructs(self):
        """Verify defensive defaults for omitted counts and separated signal traps."""
        with open(WAIT_SCRIPT, "r", encoding="utf-8") as f:
            content = f.read()
        # Symmetric defaulting when GCE API omits zero-valued count fields
        self.assertIn("TOTAL_COUNT=${TOTAL_COUNT:-0}", content)
        self.assertIn("IN_USE_COUNT=${IN_USE_COUNT:-0}", content)
        # Separate EXIT cleanup trap from SIGTERM/SIGINT immediate termination trap
        self.assertIn("trap 'rm -f \"${RESERVATION_OUTPUT}\"' EXIT", content)
        self.assertIn("trap 'exit 1' SIGTERM SIGINT", content)
        self.assertNotIn(
            "trap 'rm -f \"${RESERVATION_OUTPUT}\"' EXIT SIGTERM SIGINT",
            content,
        )

    def test_missing_required_arguments_exits_1(self):
        """Missing RESERVATION or ZONE (including trailing-slash URI) prints usage and exits 1."""
        invalid_arg_sets = [
            [],
            ["any-reservation"],
            ["", "us-central1-a"],
            ["projects/my-proj/zones/us-central1-a/reservations/", "us-central1-a"],
        ]
        for args in invalid_arg_sets:
            with self.subTest(args=args):
                res = self._run_script(args)
                self.assertEqual(res.returncode, 1)
                self.assertIn("Usage:", res.stderr + res.stdout)

    def test_invalid_required_count_exits_1(self):
        """Non-positive or non-integer REQUIRED_COUNT should exit with code 1."""
        for bad_count in ["0", "-1", "abc", "1.5"]:
            with self.subTest(bad_count=bad_count):
                res = self._run_script(
                    ["any-reservation", "us-central1-a", "test-proj", bad_count]
                )
                self.assertEqual(res.returncode, 1)
                self.assertIn(
                    "Error: REQUIRED_COUNT must be a positive integer.",
                    res.stderr + res.stdout,
                )

    def test_mktemp_failure_exits_1(self):
        """If mktemp fails to create a temporary file, script exits 1 immediately."""
        self._write_executable(
            os.path.join(self.bin_dir, "mktemp"),
            "#!/bin/bash\nexit 1\n",
        )
        res = self._run_script(["any-reservation", "us-central1-a", "proj", "1"])
        self.assertEqual(res.returncode, 1)
        self.assertIn("Failed to create temp file", res.stderr)

    def test_default_required_count_is_1_when_omitted(self):
        """When REQUIRED_COUNT ($4) is omitted, it defaults to 1."""
        self._write_mock_gcloud('printf "READY\\t1\\t0\\n"')

        res = self._run_script(["any-reservation", "us-central1-a"])
        self.assertEqual(res.returncode, 0)
        self.assertIn(
            "Reservation 'any-reservation' (Project: '<default>') capacity: Total=1, InUse=0, Available=1",
            res.stdout,
        )
        self.assertIn("--- SUCCESS: Reservation slot is available. ---", res.stdout)

    def test_sufficient_capacity_across_various_reservations_exit_0(self):
        """Test capacity check across multiple reservation names, projects, and URIs."""
        for case in RESERVATION_TEST_CASES:
            with self.subTest(reservation=case["arg"]):
                self._reset_logs()
                self._write_mock_gcloud(
                    f'printf "READY\\t{case["total"]}\\t{case["in_use"]}\\n"'
                )

                args = [
                    case["arg"],
                    case["zone"],
                    case["project_arg"],
                    case["required_count"],
                ]
                res = self._run_script(args)
                self.assertEqual(res.returncode, 0)

                available = case["total"] - case["in_use"]
                expected_proj = case["expected_project"] or "<default>"
                self.assertIn(
                    f"Reservation '{case['expected_name']}' (Project: '{expected_proj}') capacity: "
                    f"Total={case['total']}, InUse={case['in_use']}, Available={available}",
                    res.stdout,
                )
                self.assertIn(
                    "--- SUCCESS: Reservation slot is available. ---", res.stdout
                )

                with open(self.gcloud_args_log, "r", encoding="utf-8") as f:
                    gcloud_cmd = f.read().strip()
                self.assertIn(
                    f"compute reservations describe {case['expected_name']}",
                    gcloud_cmd,
                )
                self.assertIn(f"--zone={case['zone']}", gcloud_cmd)
                self.assertIn(
                    "--format=value(status, specificReservation.count, specificReservation.inUseCount)",
                    gcloud_cmd,
                )
                if case["expected_project"]:
                    self.assertIn(
                        f"--project={case['expected_project']}", gcloud_cmd
                    )
                else:
                    self.assertNotIn("--project=", gcloud_cmd)

    def test_omitted_in_use_count_defaults_to_zero(self):
        """When GCE omits specificReservation.inUseCount (0 in use), it defaults to 0."""
        for res_name in ["g4-reservation-0", "a4-reservation-0", "custom-res"]:
            with self.subTest(reservation=res_name):
                self._reset_logs()
                self._write_mock_gcloud('printf "READY\\t2\\t\\n"')

                res = self._run_script([res_name, "us-south1-a", "", "2"])
                self.assertEqual(res.returncode, 0)
                self.assertIn(
                    f"Reservation '{res_name}' (Project: '<default>') capacity: Total=2, InUse=0, Available=2",
                    res.stdout,
                )

    def test_empty_reservation_zero_or_omitted_counts_exits_3(self):
        """When count/inUseCount are omitted or 0, exits 3 with 0 capacity message."""
        for gcloud_out in ['printf "READY\\t\\t\\n"', 'printf "READY\\t0\\t0\\n"']:
            with self.subTest(gcloud_out=gcloud_out):
                self._reset_logs()
                self._write_mock_gcloud(gcloud_out)

                res = self._run_script(["any-reservation", "us-south1-a", "proj", "1"])
                self.assertEqual(res.returncode, 3)
                self.assertIn(
                    "Reservation 'any-reservation' (Project: 'proj') capacity: Total=0, InUse=0, Available=0",
                    res.stdout,
                )
                self.assertIn(
                    "Reservation 'any-reservation' has 0 total capacity.",
                    res.stdout,
                )
                self.assertIn(
                    "--- FATAL ERROR: Reservation configuration or state error. Exiting. ---",
                    res.stderr,
                )
                self.assertNotIn("Failed to parse numeric capacity", res.stdout)

    def test_required_count_exceeds_total_count_exits_3(self):
        """When REQUIRED_COUNT > TOTAL_COUNT, exits immediately with code 3."""
        self._write_mock_gcloud('printf "READY\\t2\\t0\\n"')

        res = self._run_script(["any-reservation", "us-south1-a", "proj", "4"])
        self.assertEqual(res.returncode, 3)
        self.assertIn(
            "Reservation 'any-reservation' total capacity (2) is less than required count (4).",
            res.stdout,
        )
        self.assertIn(
            "--- FATAL ERROR: Reservation configuration or state error. Exiting. ---",
            res.stderr,
        )
        self.assertFalse(os.path.exists(self.sleep_args_log))

    def test_non_ready_reservation_status_exits_3(self):
        """When reservation status is not READY (or empty -> UNKNOWN), exits 3."""
        cases = [
            ('printf "CREATING\\t4\\t0\\n"', "CREATING"),
            ('printf "DELETING\\t4\\t0\\n"', "DELETING"),
            ('printf "\\n"', "UNKNOWN"),
        ]
        for gcloud_out, expected_status in cases:
            with self.subTest(expected_status=expected_status):
                self._reset_logs()
                self._write_mock_gcloud(gcloud_out)

                res = self._run_script(["any-reservation", "us-south1-a", "proj", "2"])
                self.assertEqual(res.returncode, 3)
                self.assertIn(
                    f"Reservation 'any-reservation' is not in READY state (current status: {expected_status}).",
                    res.stdout,
                )
                self.assertIn(
                    "--- FATAL ERROR: Reservation configuration or state error. Exiting. ---",
                    res.stderr,
                )

    def test_non_numeric_capacity_output_exits_3(self):
        """Malformed non-numeric capacity from gcloud should exit with code 3."""
        for gcloud_out in [
            'printf "READY\\tcorrupted\\t0\\n"',
            'printf "READY\\t4\\tcorrupted\\n"',
        ]:
            with self.subTest(gcloud_out=gcloud_out):
                self._reset_logs()
                self._write_mock_gcloud(gcloud_out)

                res = self._run_script(["any-reservation", "us-south1-a", "proj", "1"])
                self.assertEqual(res.returncode, 3)
                self.assertIn("Failed to parse numeric capacity", res.stdout)
                self.assertIn(
                    "--- FATAL ERROR: Reservation configuration or state error. Exiting. ---",
                    res.stderr,
                )

    def test_insufficient_capacity_retries_with_sleep_300_then_succeeds(self):
        """Subshell exit code 2 (insufficient capacity) sleeps 300s and retries until available."""
        self._write_mock_gcloud(
            f'COUNT=$(cat "{self.call_count_file}" 2>/dev/null || echo 0)\n'
            "COUNT=$((COUNT + 1))\n"
            f'echo "$COUNT" > "{self.call_count_file}"\n'
            'if [ "$COUNT" -eq 1 ]; then\n'
            '  printf "READY\\t2\\t1\\n"\n'
            "else\n"
            '  printf "READY\\t2\\t0\\n"\n'
            "fi"
        )

        res = self._run_script(["any-reservation", "us-south1-a", "proj", "2"])
        self.assertEqual(res.returncode, 0)
        self.assertIn(
            "Insufficient capacity in reservation 'any-reservation' (1 available, 2 required).",
            res.stdout,
        )
        self.assertIn(
            "--- Insufficient capacity. RETRYING in 5 minutes... ---", res.stderr
        )
        self.assertIn("--- SUCCESS: Reservation slot is available. ---", res.stdout)

        with open(self.sleep_args_log, "r", encoding="utf-8") as f:
            sleeps = f.read().splitlines()
        self.assertEqual(sleeps, ["300"])

    def test_fatal_gcloud_permission_or_not_found_error_exits_1_without_retry(self):
        """Fatal gcloud errors (400/401/403/404, not found, authentication, permission denied, forbidden) exit 1."""
        fatal_errors = [
            "ERROR: (gcloud.compute.reservations.describe) HTTPError 400: Bad Request",
            "ERROR: (gcloud.compute.reservations.describe) HTTPError 401: Unauthorized",
            "ERROR: (gcloud.compute.reservations.describe) HTTPError 403: Permission denied",
            "ERROR: (gcloud.compute.reservations.describe) HTTPError 404: The resource was not found",
            "ERROR: Resource not_found in zone",
            "ERROR: Resource not-found in zone",
            "ERROR: PERMISSION_DENIED: Caller does not have permission",
            "ERROR: permission-denied on resource",
            "ERROR: Caller is forbidden from accessing resource",
            "ERROR: UNAUTHENTICATED: Request had invalid authentication credentials",
            "ERROR: Token refresh failed: invalid_grant",
        ]
        for err_msg in fatal_errors:
            with self.subTest(err_msg=err_msg):
                self._reset_logs()
                self._write_mock_gcloud(f'echo "{err_msg}" >&2\nexit 1')
                res = self._run_script(["any-reservation", "us-south1-a", "proj", "1"])
                self.assertEqual(res.returncode, 1)
                self.assertIn(
                    "Failed to query reservation details from gcloud.",
                    res.stdout,
                )
                self.assertIn(
                    "--- FATAL ERROR: Reservation query failed due to authentication, permission, or non-existent resource. Exiting. ---",
                    res.stderr,
                )
                self.assertFalse(os.path.exists(self.sleep_args_log))

    def test_transient_gcloud_error_retries_with_sleep_60_then_succeeds(self):
        """Transient gcloud errors sleep 60s and retry until gcloud succeeds."""
        self._write_mock_gcloud(
            f'COUNT=$(cat "{self.call_count_file}" 2>/dev/null || echo 0)\n'
            "COUNT=$((COUNT + 1))\n"
            f'echo "$COUNT" > "{self.call_count_file}"\n'
            'if [ "$COUNT" -eq 1 ]; then\n'
            '  echo "ERROR: 503 Service Unavailable" >&2\n'
            "  exit 1\n"
            "else\n"
            '  printf "READY\\t2\\t0\\n"\n'
            "fi"
        )

        res = self._run_script(["any-reservation", "us-south1-a", "proj", "2"])
        self.assertEqual(res.returncode, 0)
        self.assertIn(
            "Failed to query reservation details from gcloud.",
            res.stdout,
        )
        self.assertIn(
            "--- WARNING: Transient gcloud error encountered. Retrying in 1 minute... ---",
            res.stderr,
        )
        self.assertIn("--- SUCCESS: Reservation slot is available. ---", res.stdout)

        with open(self.sleep_args_log, "r", encoding="utf-8") as f:
            sleeps = f.read().splitlines()
        self.assertEqual(sleeps, ["60"])

    def test_multi_step_transition_transient_then_insufficient_then_available(self):
        """Verify state transitions across 3 polls: transient error (60s) -> busy (300s) -> ready (0)."""
        self._write_mock_gcloud(
            f'COUNT=$(cat "{self.call_count_file}" 2>/dev/null || echo 0)\n'
            "COUNT=$((COUNT + 1))\n"
            f'echo "$COUNT" > "{self.call_count_file}"\n'
            'if [ "$COUNT" -eq 1 ]; then\n'
            '  echo "ERROR: 502 Bad Gateway" >&2\n'
            "  exit 1\n"
            'elif [ "$COUNT" -eq 2 ]; then\n'
            '  printf "READY\\t4\\t3\\n"\n'
            "else\n"
            '  printf "READY\\t4\\t2\\n"\n'
            "fi"
        )

        res = self._run_script(["any-reservation", "us-south1-a", "proj", "2"])
        self.assertEqual(res.returncode, 0)
        with open(self.sleep_args_log, "r", encoding="utf-8") as f:
            sleeps = f.read().splitlines()
        self.assertEqual(sleeps, ["60", "300"])

    def test_max_wait_seconds_timeout_exits_1(self):
        """When elapsed time exceeds MAX_WAIT_SECONDS, script exits 1 with timeout error."""
        with open(WAIT_SCRIPT, "r", encoding="utf-8") as f:
            script_content = f.read()

        # Simulate elapsed time > MAX_WAIT_SECONDS by setting MAX_WAIT_SECONDS=-1
        timeout_script = os.path.join(self.test_dir, "wait_timeout.sh")
        self._write_executable(
            timeout_script,
            script_content.replace("MAX_WAIT_SECONDS=28800", "MAX_WAIT_SECONDS=-1"),
        )

        res = self._run_script(
            ["any-reservation", "us-south1-a", "proj", "1"],
            script_path=timeout_script,
        )
        self.assertEqual(res.returncode, 1)
        self.assertIn(
            "--- FATAL ERROR: Timed out waiting for available reservation after 8 hours. Exiting. ---",
            res.stderr,
        )

    def test_signals_terminate_immediately_and_clean_up_temp_file(self):
        """Receiving SIGTERM or SIGINT during sleep exits 1 and removes temp file."""
        for sig in ["TERM", "INT"]:
            with self.subTest(signal=sig):
                self._reset_logs()
                self._write_mock_gcloud('printf "READY\\t2\\t2\\n"')
                self._write_executable(
                    os.path.join(self.bin_dir, "sleep"),
                    f'#!/bin/bash\nkill -{sig} "$PPID"\nexit 0\n',
                )

                res = self._run_script(["any-reservation", "us-south1-a", "proj", "2"])
                self.assertEqual(res.returncode, 1)
                self.assertEqual(os.listdir(self.script_tmp_dir), [])
                with open(self.gcloud_args_log, "r", encoding="utf-8") as f:
                    calls = f.read().splitlines()
                # Must terminate on the first poll's sleep without looping to a second poll
                self.assertEqual(len(calls), 1)

    def test_all_build_yaml_invocations_dynamically(self):
        """Dynamically discover and validate all wait_for_available_reservation.sh calls in builds/*.yaml."""
        if not os.path.isdir(BUILDS_DIR):
            self.skipTest(f"Builds directory not found: {BUILDS_DIR}")

        pattern = re.compile(
            r"wait_for_available_reservation\.sh\s+([^\n#]+)"
        )
        test_yaml_pattern = re.compile(
            r"--extra-vars=['\"]@tools/cloud-build/daily-tests/tests/([^'\"]+\.yml)['\"]"
        )

        for yaml_path in sorted(glob.glob(os.path.join(BUILDS_DIR, "*.yaml"))):
            with open(yaml_path, "r", encoding="utf-8") as f:
                content = f.read()

            for match in pattern.finditer(content):
                orig_args = shlex.split(match.group(1).strip())
                raw_args = [
                    "placeholder-val" if arg.startswith("$") else arg
                    for arg in orig_args
                ]
                build_file = os.path.basename(yaml_path)
                with self.subTest(build_file=build_file, args=raw_args):
                    self._reset_logs()
                    req_count = (
                        int(raw_args[3])
                        if len(raw_args) >= 4 and raw_args[3].isdigit()
                        else 1
                    )
                    self._write_mock_gcloud(
                        f'printf "READY\\t{req_count}\\t0\\n"'
                    )

                    res = self._run_script(raw_args)
                    self.assertEqual(
                        res.returncode,
                        0,
                        f"Invocation in {build_file} with args {raw_args} failed:\n"
                        f"{res.stdout}\n{res.stderr}",
                    )

                    # Cross-check against corresponding tests/<name>.yml if present
                    test_match = test_yaml_pattern.search(content)
                    if test_match and os.path.isdir(TESTS_DIR):
                        test_yml_path = os.path.join(TESTS_DIR, test_match.group(1))
                        if os.path.isfile(test_yml_path):
                            with open(test_yml_path, "r", encoding="utf-8") as tf:
                                test_cfg = yaml.safe_load(tf) or {}
                            cli_vars = test_cfg.get("cli_deployment_vars", {})
                            expected_res = cli_vars.get("reservation")
                            expected_zone = test_cfg.get("zone")
                            expected_count = test_cfg.get("static_node_count")
                            if (
                                expected_res
                                and "{{" not in str(expected_res)
                                and not orig_args[0].startswith("$")
                            ):
                                self.assertEqual(orig_args[0], str(expected_res))
                            if (
                                expected_zone
                                and "{{" not in str(expected_zone)
                                and not orig_args[1].startswith("$")
                            ):
                                self.assertEqual(orig_args[1], str(expected_zone))
                            if (
                                expected_count
                                and "{{" not in str(expected_count)
                                and len(orig_args) >= 4
                                and orig_args[3].isdigit()
                            ):
                                self.assertEqual(
                                    int(orig_args[3]), int(expected_count)
                                )


    def test_blueprint_file_injects_reservation_project_on_success(self):
        """When the parameter BLUEPRINT_FILE is passed, injects project under specific_reservations on exit 0."""
        self._write_mock_gcloud('printf "READY\\t4\\t2\\n"')
        bp_path = os.path.join(self.test_dir, "blueprint.yaml")
        with open(bp_path, "w", encoding="utf-8") as f:
            f.write(
                "  - id: g4-pool\n"
                "    source: modules/compute/gke-node-pool\n"
                "    settings:\n"
                "      reservation_affinity:\n"
                "        consume_reservation_type: SPECIFIC_RESERVATION\n"
                "        specific_reservations:\n"
                "        - name: $(vars.reservation)\n"
                "    outputs: [instructions]\n"
            )

        res = self._run_script(
            ["g4-reservation-0", "us-south1-a", "hpc-toolkit-dev", "2", bp_path]
        )
        self.assertEqual(res.returncode, 0)

        with open(bp_path, "r", encoding="utf-8") as f:
            updated_content = f.read()
        self.assertIn('project: "hpc-toolkit-dev"', updated_content)

        updated_bp = yaml.safe_load(updated_content)
        spec_res = updated_bp[0]["settings"]["reservation_affinity"][
            "specific_reservations"
        ]
        self.assertEqual(
            spec_res,
            [{"name": "$(vars.reservation)", "project": "hpc-toolkit-dev"}],
        )

if __name__ == "__main__":
    unittest.main()
