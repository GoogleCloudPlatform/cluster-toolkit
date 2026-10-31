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

"""Unit tests for the stale PR Cloud Build reaper.

How to run these tests:
  Direct execution:
    python3 tools/tests/test_reap_stale_pr_builds.py

  Via unittest module:
    python3 -m unittest tools/tests/test_reap_stale_pr_builds.py -v
"""

import io
import json
import os
import subprocess
import sys
import tempfile
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

    def test_identify_stale_builds_open_pr_supersedes_older_commit_working_and_queued(
        self,
    ):
        """When GitHub PR is open, WORKING and QUEUED builds for older commits are marked stale."""
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
                "id": "build-new-working",
                "status": "WORKING",
                "createTime": "2026-10-31T06:00:00Z",
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
        self.assertEqual(
            [b["id"] for b in stale],
            ["build-old-working", "build-old-queued"],
        )

    def test_identify_stale_builds_closed_pr_marks_all_builds_stale(self):
        """When GitHub PR is closed or merged, all WORKING and QUEUED builds for that PR are stale."""
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
                "id": "build-closed-queued",
                "status": "QUEUED",
                "createTime": "2026-10-31T05:30:05Z",
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
            ["build-closed-working", "build-closed-queued"],
        )
        self.assertTrue(all("closed" in b["stale_reason"] for b in stale))

    def test_identify_stale_builds_github_api_failure_safely_skips_pr_without_cancelling(
        self,
    ):
        """When GitHub API is unreachable/rate-limited, safely skip the PR instead of guessing from active builds."""
        old_sha = "1111111111111111111111111111111111111111"
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
        ]

        stale = reaper.identify_stale_builds(
            builds,
            repo="GoogleCloudPlatform/cluster-toolkit",
            pr_fetcher=lambda repo, pr_num: None,
        )
        self.assertEqual(stale, [])

    @mock.patch("urllib.request.urlopen")
    def test_fetch_pr_info_uses_etag_cache_on_304_not_modified(self, mock_urlopen):
        """Verify fetch_pr_info caches ETag and returns cached PR info on HTTP 304 Not Modified."""
        with tempfile.TemporaryDirectory() as tmpdir:
            cache_path = os.path.join(tmpdir, "pr_etag_cache.json")
            sha = "dddd4444dddd4444dddd4444dddd4444dddd4444"

            # First call: 200 OK with ETag header
            resp_200 = mock.MagicMock()
            resp_200.read.return_value = json.dumps(
                {"state": "open", "head": {"sha": sha}}
            ).encode("utf-8")
            resp_200.headers = {"ETag": '"etag-abc-123"'}
            resp_200.__enter__.return_value = resp_200

            mock_urlopen.return_value = resp_200
            info_1 = reaper.fetch_pr_info(
                "GoogleCloudPlatform/cluster-toolkit",
                "6550",
                cache_path=cache_path,
            )
            self.assertEqual(info_1, {"state": "open", "head_sha": sha})

            # Second call: 304 Not Modified should send If-None-Match and return cached data
            seen_headers = {}

            def side_effect_304(req, timeout=5):
                seen_headers.update(dict(req.header_items()))
                raise urllib.error.HTTPError(
                    req.full_url,
                    304,
                    "Not Modified",
                    {},
                    io.BytesIO(b""),
                )

            mock_urlopen.side_effect = side_effect_304
            info_2 = reaper.fetch_pr_info(
                "GoogleCloudPlatform/cluster-toolkit",
                "6550",
                cache_path=cache_path,
            )
            self.assertEqual(info_2, {"state": "open", "head_sha": sha})
            self.assertEqual(
                seen_headers.get("If-none-match")
                or seen_headers.get("If-None-Match"),
                '"etag-abc-123"',
            )

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

    @mock.patch("reap_stale_pr_builds.fetch_pr_info")
    @mock.patch("subprocess.run")
    def test_reap_stale_pr_builds_cancels_active_builds(
        self, mock_subproc, mock_fetch_pr
    ):
        """Verify reap_stale_pr_builds lists WORKING/QUEUED builds and cancels superseded ones via gcloud."""
        old_sha = "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"
        new_sha = "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"
        mock_fetch_pr.return_value = {"state": "open", "head_sha": new_sha}

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
                    "id": "222222-current-working",
                    "status": "WORKING",
                    "createTime": "2026-10-31T06:00:00Z",
                    "substitutions": {
                        "_PR_NUMBER": "6550",
                        "COMMIT_SHA": new_sha,
                        "TRIGGER_NAME": "PR-test-gke",
                    },
                },
            ]
        )

        recorded_cmds = []

        def side_effect(cmd, **kwargs):
            recorded_cmds.append(list(cmd))
            if cmd[:3] == ["gcloud", "builds", "list"]:
                return subprocess.CompletedProcess(
                    cmd, 0, stdout=builds_json, stderr=""
                )
            if cmd[:3] == ["gcloud", "builds", "cancel"]:
                return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")
            raise AssertionError(f"Unexpected command: {cmd}")

        mock_subproc.side_effect = side_effect

        summary = reaper.reap_stale_pr_builds(
            project="hpc-toolkit-dev",
            repo="GoogleCloudPlatform/cluster-toolkit",
            dry_run=False,
        )

        self.assertEqual(summary["cancelled_active"], 1)
        self.assertEqual(len(recorded_cmds), 2)
        self.assertEqual(recorded_cmds[0][:3], ["gcloud", "builds", "list"])
        self.assertEqual(
            recorded_cmds[1],
            [
                "gcloud",
                "builds",
                "cancel",
                "111111-stale-working",
                "--project=hpc-toolkit-dev",
                "--quiet",
            ],
        )

    @mock.patch("reap_stale_pr_builds.fetch_pr_info")
    @mock.patch("subprocess.run")
    def test_dry_run_performs_no_mutations(self, mock_subproc, mock_fetch_pr):
        """Verify --dry-run only lists builds and never spawns gcloud builds cancel."""
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
            ]
        )

        mock_subproc.return_value = subprocess.CompletedProcess(
            ["gcloud", "builds", "list"], 0, stdout=builds_json, stderr=""
        )

        summary = reaper.reap_stale_pr_builds(
            project="hpc-toolkit-dev",
            repo="GoogleCloudPlatform/cluster-toolkit",
            dry_run=True,
        )

        self.assertEqual(summary["cancelled_active"], 1)
        self.assertEqual(mock_subproc.call_count, 1)
        self.assertEqual(
            mock_subproc.call_args[0][0][:3], ["gcloud", "builds", "list"]
        )

    @mock.patch("subprocess.run")
    def test_list_active_pr_builds_handles_timeout_gracefully(
        self, mock_subproc
    ):
        """Ensure gcloud builds list timeout returns an empty list without raising."""
        mock_subproc.side_effect = subprocess.TimeoutExpired(
            cmd=["gcloud", "builds", "list"],
            timeout=60,
        )
        builds = reaper.list_active_pr_builds("hpc-toolkit-dev")
        self.assertEqual(builds, [])


if __name__ == "__main__":
    unittest.main()
