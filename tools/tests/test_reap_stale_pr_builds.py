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

"""Unit tests for the push-triggered stale PR Cloud Build and Kueue job reaper.

How to run these tests:
  Direct execution:
    python3 tools/tests/test_reap_stale_pr_builds.py

  Via unittest module:
    python3 -m unittest tools/tests/test_reap_stale_pr_builds.py -v
"""

import io
import json
import os
import sys
import unittest
from unittest import mock
import urllib.error

sys.path.insert(
    0,
    os.path.abspath(os.path.join(os.path.dirname(__file__), "../cloud-build")),
)
import reap_stale_pr_builds as reaper  # noqa: E402


class TestReapStalePRBuilds(unittest.TestCase):
    """Tests for tools/cloud-build/reap_stale_pr_builds.py."""

    def test_parse_rfc3339_handles_missing_and_present_fractional_seconds(self):
        """Ensure RFC3339 timestamps without fractional seconds sort before microsecond timestamps."""
        ts_whole = reaper.parse_rfc3339("2026-10-31T06:00:00Z")
        ts_frac = reaper.parse_rfc3339("2026-10-31T06:00:00.500000Z")
        self.assertLess(ts_whole, ts_frac)

    def test_identify_stale_builds_open_pr_supersedes_older_commit_working_queued_pending(
        self,
    ):
        """When GitHub PR is open, builds for older commits are marked stale across WORKING, QUEUED, PENDING."""
        old_sha = "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"
        new_sha = "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"
        builds = [
            {
                "id": "build-old-working",
                "status": "WORKING",
                "createTime": "2026-10-31T05:00:00Z",
                "substitutions": {
                    "_PR_NUMBER": "6550",
                    "COMMIT_SHA": old_sha,
                    "TRIGGER_NAME": "PR-test-gke",
                },
            },
            {
                "id": "build-old-queued",
                "status": "QUEUED",
                "createTime": "2026-10-31T05:00:01Z",
                "substitutions": {
                    "_PR_NUMBER": "6550",
                    "COMMIT_SHA": old_sha,
                    "TRIGGER_NAME": "PR-test-ml-gke",
                },
            },
            {
                "id": "build-old-pending",
                "status": "PENDING",
                "createTime": "2026-10-31T05:00:02Z",
                "substitutions": {
                    "_PR_NUMBER": "6550",
                    "COMMIT_SHA": old_sha,
                    "TRIGGER_NAME": "PR-test-slurm",
                },
            },
            {
                "id": "build-new-working",
                "status": "WORKING",
                "createTime": "2026-10-31T06:00:00Z",
                "substitutions": {
                    "_PR_NUMBER": "6550",
                    "COMMIT_SHA": new_sha,
                    "TRIGGER_NAME": "PR-validation",
                },
            },
            {
                "id": "build-new-pending",
                "status": "PENDING",
                "createTime": "2026-10-31T06:00:01Z",
                "substitutions": {
                    "_PR_NUMBER": "6550",
                    "COMMIT_SHA": new_sha,
                    "TRIGGER_NAME": "PR-test-gke",
                },
            },
        ]

        def fake_fetch_pr(repo, pr_num):
            self.assertEqual(pr_num, "6550")
            return {"state": "open", "head_sha": new_sha}

        stale = reaper.identify_stale_builds(
            builds,
            repo="GoogleCloudPlatform/cluster-toolkit",
            pr_fetcher=fake_fetch_pr,
        )
        stale_ids = [b["id"] for b in stale]
        self.assertEqual(
            stale_ids,
            ["build-old-working", "build-old-queued", "build-old-pending"],
        )

    def test_identify_stale_builds_closed_pr_marks_all_builds_stale(self):
        """When GitHub PR is closed or merged, all WORKING, QUEUED, and PENDING builds for that PR are stale."""
        sha = "cccc3333cccc3333cccc3333cccc3333cccc3333"
        builds = [
            {
                "id": "build-closed-working",
                "status": "WORKING",
                "createTime": "2026-10-31T05:30:00Z",
                "substitutions": {
                    "_PR_NUMBER": "6551",
                    "COMMIT_SHA": sha,
                    "TRIGGER_NAME": "PR-test-gke-tpu-7x",
                },
            },
            {
                "id": "build-closed-pending",
                "status": "PENDING",
                "createTime": "2026-10-31T05:30:00Z",
                "substitutions": {
                    "_PR_NUMBER": "6551",
                    "COMMIT_SHA": sha,
                    "TRIGGER_NAME": "PR-test-gke",
                },
            },
        ]

        stale = reaper.identify_stale_builds(
            builds,
            repo="GoogleCloudPlatform/cluster-toolkit",
            pr_fetcher=lambda repo, pr_num: {"state": "closed", "head_sha": sha},
        )
        self.assertEqual(
            [b["id"] for b in stale],
            ["build-closed-working", "build-closed-pending"],
        )
        self.assertTrue(all("closed" in b["stale_reason"] for b in stale))

    def test_identify_stale_builds_github_api_failure_falls_back_to_cloud_build_timestamps(
        self,
    ):
        """When GitHub API is unreachable/rate-limited, fallback uses newest createTime per COMMIT_SHA."""
        old_sha = "1111111111111111111111111111111111111111"
        new_sha = "2222222222222222222222222222222222222222"
        builds = [
            {
                "id": "b-old",
                "status": "WORKING",
                "createTime": "2026-10-31T06:00:00Z",
                "substitutions": {
                    "_PR_NUMBER": "6552",
                    "COMMIT_SHA": old_sha,
                    "TRIGGER_NAME": "PR-test-gke",
                },
            },
            {
                "id": "b-new",
                "status": "WORKING",
                "createTime": "2026-10-31T06:00:00.250000Z",
                "substitutions": {
                    "_PR_NUMBER": "6552",
                    "COMMIT_SHA": new_sha,
                    "TRIGGER_NAME": "PR-validation",
                },
            },
        ]

        stale = reaper.identify_stale_builds(
            builds,
            repo="GoogleCloudPlatform/cluster-toolkit",
            pr_fetcher=lambda repo, pr_num: None,
        )
        self.assertEqual([b["id"] for b in stale], ["b-old"])

    def test_identify_stale_builds_never_cancels_current_build_id(self):
        """Ensure current_build_id is never marked stale even if PR is closed."""
        sha = "dddd4444dddd4444dddd4444dddd4444dddd4444"
        builds = [
            {
                "id": "self-build-id",
                "status": "WORKING",
                "createTime": "2026-10-31T06:00:00Z",
                "substitutions": {
                    "_PR_NUMBER": "6553",
                    "COMMIT_SHA": sha,
                    "TRIGGER_NAME": "PR-validation",
                },
            },
            {
                "id": "other-build-id",
                "status": "WORKING",
                "createTime": "2026-10-31T05:00:00Z",
                "substitutions": {
                    "_PR_NUMBER": "6553",
                    "COMMIT_SHA": "eeee5555eeee5555eeee5555eeee5555eeee5555",
                    "TRIGGER_NAME": "PR-test-gke",
                },
            },
        ]

        stale = reaper.identify_stale_builds(
            builds,
            repo="GoogleCloudPlatform/cluster-toolkit",
            current_build_id="self-build-id",
            current_commit_sha=sha,
            pr_fetcher=lambda repo, pr_num: {"state": "closed", "head_sha": sha},
        )
        self.assertEqual([b["id"] for b in stale], ["other-build-id"])

    def test_identify_stale_builds_pr_filter_scopes_to_single_pr(self):
        """Ensure target_pr filters evaluation strictly to the requested PR number."""
        builds = [
            {
                "id": "b-pr100",
                "status": "WORKING",
                "createTime": "2026-10-31T05:00:00Z",
                "substitutions": {
                    "_PR_NUMBER": "100",
                    "COMMIT_SHA": "old100",
                },
            },
            {
                "id": "b-pr200",
                "status": "WORKING",
                "createTime": "2026-10-31T05:00:00Z",
                "substitutions": {
                    "_PR_NUMBER": "200",
                    "COMMIT_SHA": "old200",
                },
            },
        ]

        stale = reaper.identify_stale_builds(
            builds,
            repo="GoogleCloudPlatform/cluster-toolkit",
            target_pr="100",
            pr_fetcher=lambda repo, pr_num: {"state": "closed", "head_sha": "new"},
        )
        self.assertEqual([b["id"] for b in stale], ["b-pr100"])

    def test_match_kueue_jobs_by_label_and_container_env_skips_completed_jobs(self):
        """Match K8s Jobs by metadata.labels['build-id'] and container env BUILD_ID while skipping finished jobs."""
        k8s_jobs_payload = {
            "items": [
                {
                    "metadata": {
                        "name": "ml-gke-111111",
                        "labels": {"build-id": "111111-full-uuid"},
                    },
                    "status": {"conditions": []},
                    "spec": {"template": {"spec": {"containers": []}}},
                },
                {
                    "metadata": {
                        "name": "gke-222222",
                        "labels": {
                            "kueue.x-k8s.io/queue-name": "local-queue-test-locks"
                        },
                    },
                    "status": {"conditions": []},
                    "spec": {
                        "template": {
                            "spec": {
                                "containers": [
                                    {
                                        "name": "runner",
                                        "env": [
                                            {
                                                "name": "BUILD_ID",
                                                "value": "222222-full-uuid",
                                            }
                                        ],
                                    }
                                ]
                            }
                        }
                    },
                },
                {
                    "metadata": {
                        "name": "gke-completed",
                        "labels": {"build-id": "111111-full-uuid"},
                    },
                    "status": {
                        "conditions": [{"type": "Complete", "status": "True"}]
                    },
                    "spec": {"template": {"spec": {"containers": []}}},
                },
                {
                    "metadata": {
                        "name": "gke-unrelated",
                        "labels": {"build-id": "999999-full-uuid"},
                    },
                    "status": {"conditions": []},
                    "spec": {"template": {"spec": {"containers": []}}},
                },
            ]
        }

        matched = reaper.find_matching_kueue_jobs(
            k8s_jobs_payload,
            {"111111-full-uuid", "222222-full-uuid"},
        )
        self.assertEqual(matched, ["ml-gke-111111", "gke-222222"])

    @mock.patch("reap_stale_pr_builds.reject_pending_builds")
    @mock.patch("reap_stale_pr_builds.fetch_pr_info")
    @mock.patch("subprocess.run")
    def test_reap_executes_cloud_build_cancel_before_kubectl_delete_and_uses_isolated_kubeconfig(
        self, mock_subproc, mock_fetch_pr, mock_reject_pending
    ):
        """Verify Cloud Build cancel precedes K8s Job deletion and uses an isolated temporary KUBECONFIG."""
        old_sha = "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"
        new_sha = "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"
        mock_fetch_pr.return_value = {"state": "open", "head_sha": new_sha}
        mock_reject_pending.return_value = 1

        builds_json = json.dumps(
            [
                {
                    "id": "111111-stale-working",
                    "status": "WORKING",
                    "createTime": "2026-10-31T05:00:00Z",
                    "substitutions": {
                        "_PR_NUMBER": "6550",
                        "COMMIT_SHA": old_sha,
                        "TRIGGER_NAME": "PR-test-gke",
                    },
                },
                {
                    "id": "222222-stale-pending",
                    "status": "PENDING",
                    "createTime": "2026-10-31T05:00:01Z",
                    "substitutions": {
                        "_PR_NUMBER": "6550",
                        "COMMIT_SHA": old_sha,
                        "TRIGGER_NAME": "PR-test-slurm",
                    },
                },
            ]
        )
        jobs_json = json.dumps(
            {
                "items": [
                    {
                        "metadata": {
                            "name": "gke-111111",
                            "labels": {"build-id": "111111-stale-working"},
                        },
                        "status": {},
                        "spec": {},
                    }
                ]
            }
        )

        recorded_calls = []

        def side_effect(cmd, **kwargs):
            env = kwargs.get("env") or {}
            kubeconfig = env.get("KUBECONFIG")
            recorded_calls.append((list(cmd), kubeconfig))
            if cmd[:3] == ["gcloud", "builds", "list"]:
                return subprocess.CompletedProcess(cmd, 0, stdout=builds_json, stderr="")
            if cmd[:3] == ["gcloud", "builds", "cancel"]:
                return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")
            if cmd[:4] == ["gcloud", "container", "clusters", "get-credentials"]:
                self.assertIsNotNone(kubeconfig)
                self.assertTrue(os.path.exists(kubeconfig))
                return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")
            if cmd[:3] == ["kubectl", "get", "jobs"]:
                self.assertIsNotNone(kubeconfig)
                return subprocess.CompletedProcess(cmd, 0, stdout=jobs_json, stderr="")
            if cmd[:3] == ["kubectl", "delete", "job"]:
                self.assertIsNotNone(kubeconfig)
                return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")
            raise AssertionError(f"Unexpected command: {cmd}")

        import subprocess

        mock_subproc.side_effect = side_effect

        summary = reaper.reap_stale_pr_builds(
            project="hpc-toolkit-dev",
            repo="GoogleCloudPlatform/cluster-toolkit",
            current_build_id="current-build-999",
            current_commit_sha=new_sha,
            cluster_name="test-kueue-cluster",
            cluster_region="us-central1",
            namespace="default",
            skip_pending=False,
            dry_run=False,
        )

        self.assertEqual(summary["cancelled_active"], 1)
        self.assertEqual(summary["deleted_kueue_jobs"], 1)
        self.assertEqual(summary["rejected_pending"], 1)

        cmd_prefixes = [c[0][:3] for c in recorded_calls]
        self.assertEqual(
            cmd_prefixes,
            [
                ["gcloud", "builds", "list"],
                ["gcloud", "builds", "cancel"],
                ["gcloud", "container", "clusters"],
                ["kubectl", "get", "jobs"],
                ["kubectl", "delete", "job"],
            ],
        )
        # Verify temporary KUBECONFIG was isolated and cleaned up after execution
        kubeconfig_paths = {c[1] for c in recorded_calls[2:]}
        self.assertEqual(len(kubeconfig_paths), 1)
        isolated_kc = next(iter(kubeconfig_paths))
        self.assertIsNotNone(isolated_kc)
        self.assertFalse(os.path.exists(isolated_kc))

    @mock.patch("urllib.request.urlopen")
    @mock.patch("subprocess.run")
    def test_reject_pending_builds_403_and_404_are_non_fatal(
        self, mock_subproc, mock_urlopen
    ):
        """Ensure HTTP 403 Forbidden and 404 Not Found during PENDING build rejection are caught and non-fatal."""
        import subprocess

        mock_subproc.return_value = subprocess.CompletedProcess(
            ["gcloud", "auth", "print-access-token"],
            0,
            stdout="fake-token\n",
            stderr="",
        )

        def urlopen_side_effect(req, timeout=10):
            if "build-403" in req.full_url:
                raise urllib.error.HTTPError(
                    req.full_url,
                    403,
                    "Forbidden",
                    {},
                    io.BytesIO(b"Permission denied"),
                )
            if "build-404" in req.full_url:
                raise urllib.error.HTTPError(
                    req.full_url,
                    404,
                    "Not Found",
                    {},
                    io.BytesIO(b"Build not found"),
                )
            resp = mock.MagicMock()
            resp.status = 200
            resp.__enter__.return_value = resp
            return resp

        mock_urlopen.side_effect = urlopen_side_effect

        pending_builds = [
            {"id": "build-403", "stale_reason": "superseded"},
            {"id": "build-404", "stale_reason": "superseded"},
            {"id": "build-200", "stale_reason": "superseded"},
        ]

        rejected_count = reaper.reject_pending_builds(
            project="hpc-toolkit-dev",
            pending_builds=pending_builds,
            dry_run=False,
        )
        self.assertEqual(rejected_count, 1)

    @mock.patch("reap_stale_pr_builds.fetch_pr_info")
    @mock.patch("urllib.request.urlopen")
    @mock.patch("subprocess.run")
    def test_dry_run_performs_no_mutations(
        self, mock_subproc, mock_urlopen, mock_fetch_pr
    ):
        """Verify --dry-run only lists builds/jobs and never cancels, rejects, or deletes."""
        import subprocess

        mock_fetch_pr.return_value = {"state": "closed", "head_sha": "sha"}
        builds_json = json.dumps(
            [
                {
                    "id": "stale-working-1",
                    "status": "WORKING",
                    "createTime": "2026-10-31T05:00:00Z",
                    "substitutions": {
                        "_PR_NUMBER": "6550",
                        "COMMIT_SHA": "sha",
                        "TRIGGER_NAME": "PR-test-gke",
                    },
                },
                {
                    "id": "stale-pending-1",
                    "status": "PENDING",
                    "createTime": "2026-10-31T05:00:00Z",
                    "substitutions": {
                        "_PR_NUMBER": "6550",
                        "COMMIT_SHA": "sha",
                        "TRIGGER_NAME": "PR-test-slurm",
                    },
                },
            ]
        )

        mock_subproc.return_value = subprocess.CompletedProcess(
            ["gcloud", "builds", "list"], 0, stdout=builds_json, stderr=""
        )

        reaper.reap_stale_pr_builds(
            project="hpc-toolkit-dev",
            repo="GoogleCloudPlatform/cluster-toolkit",
            dry_run=True,
        )

        # Only the single read-only `gcloud builds list` call should have occurred
        self.assertEqual(mock_subproc.call_count, 1)
        self.assertEqual(
            mock_subproc.call_args[0][0][:3], ["gcloud", "builds", "list"]
        )
        mock_urlopen.assert_not_called()


if __name__ == "__main__":
    unittest.main()
