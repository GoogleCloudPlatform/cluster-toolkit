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

"""Unit tests for tools/cloud-build/reap_stale_pr_builds.py.

Run: python3 -m unittest tools/tests/test_reap_stale_pr_builds.py -v
"""

import io
import json
import os
import subprocess
import sys
import unittest
from unittest import mock
import urllib.error

sys.path.insert(
    0,
    os.path.abspath(os.path.join(os.path.dirname(__file__), "../cloud-build")),
)
import reap_stale_pr_builds as reaper  # noqa: E402

OLD = "a" * 40
NEW = "b" * 40


def _build(build_id, pr, sha, status="WORKING"):
    return {
        "id": build_id,
        "status": status,
        "substitutions": {"_PR_NUMBER": pr, "COMMIT_SHA": sha, "TRIGGER_NAME": "PR-test-gke"},
    }


def _http_response(payload):
    resp = mock.MagicMock()
    resp.read.return_value = json.dumps(payload).encode("utf-8")
    resp.__enter__.return_value = resp
    return resp


def _rate_limited():
    return urllib.error.HTTPError(
        "https://api.github.com", 403, "rate limit exceeded", {}, io.BytesIO(b"")
    )


def _fake_gcloud(builds, recorded):
    def run(cmd, **kwargs):
        recorded.append(list(cmd))
        if cmd[:3] == ["gcloud", "builds", "list"]:
            return subprocess.CompletedProcess(cmd, 0, stdout=json.dumps(builds), stderr="")
        if cmd[:3] == ["gcloud", "builds", "cancel"]:
            return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")
        raise AssertionError(f"unexpected command: {cmd}")

    return run


class IdentifyStaleBuildsTest(unittest.TestCase):
    """Pure classification logic."""

    def _never(self, pr):
        self.fail(f"unexpected closed-PR lookup for #{pr}")

    def test_open_pr_marks_older_commits_stale_and_keeps_head(self):
        builds = [
            _build("old-working", "6550", OLD),
            _build("old-queued", "6550", OLD, status="QUEUED"),
            _build("head", "6550", NEW),
        ]
        stale = reaper.identify_stale_builds(builds, {"6550": NEW}, self._never)
        self.assertEqual([b["id"] for b in stale], ["old-working", "old-queued"])

    def test_head_sha_comparison_is_case_insensitive(self):
        builds = [_build("head", "1", NEW.upper())]
        self.assertEqual(reaper.identify_stale_builds(builds, {"1": NEW}, self._never), [])

    def test_pr_missing_from_open_list_is_cancelled_only_when_confirmed_closed(self):
        builds = [_build("b1", "6551", NEW), _build("b2", "6551", OLD, status="QUEUED")]
        stale = reaper.identify_stale_builds(builds, {}, lambda pr: True)
        self.assertEqual([b["id"] for b in stale], ["b1", "b2"])
        self.assertTrue(all("closed" in b["stale_reason"] for b in stale))
        self.assertEqual(reaper.identify_stale_builds(builds, {}, lambda pr: False), [])

    def test_builds_without_numeric_pr_or_commit_are_ignored(self):
        builds = [_build("no-pr", "", OLD), _build("bad-pr", "../1", OLD), _build("no-sha", "7", "")]
        self.assertEqual(reaper.identify_stale_builds(builds, {"7": NEW}, self._never), [])


class GitHubTest(unittest.TestCase):
    """GitHub REST calls."""

    @mock.patch.dict(os.environ, {}, clear=True)
    @mock.patch("urllib.request.urlopen")
    def test_fetch_open_pr_heads_paginates_unauthenticated(self, mock_urlopen):
        page1 = [{"number": n, "head": {"sha": NEW}} for n in range(1, 101)]
        page2 = [{"number": 101, "head": {"sha": OLD.upper()}}]
        mock_urlopen.side_effect = [_http_response(page1), _http_response(page2)]

        heads = reaper.fetch_open_pr_heads("o/r")

        self.assertEqual(len(heads), 101)
        self.assertEqual(heads["101"], OLD)
        self.assertEqual(
            [c.args[0].full_url for c in mock_urlopen.call_args_list],
            [
                "https://api.github.com/repos/o/r/pulls?state=open&per_page=100&page=1",
                "https://api.github.com/repos/o/r/pulls?state=open&per_page=100&page=2",
            ],
        )
        self.assertIsNone(mock_urlopen.call_args.args[0].get_header("Authorization"))

    @mock.patch.dict(os.environ, {"GITHUB_TOKEN": " t0k "}, clear=True)
    @mock.patch("urllib.request.urlopen")
    def test_fetch_open_pr_heads_sends_stripped_token(self, mock_urlopen):
        mock_urlopen.return_value = _http_response([])
        self.assertEqual(reaper.fetch_open_pr_heads("o/r"), {})
        self.assertEqual(mock_urlopen.call_args.args[0].get_header("Authorization"), "Bearer t0k")

    @mock.patch("urllib.request.urlopen")
    def test_github_errors_fail_safe(self, mock_urlopen):
        for error in (_rate_limited(), TimeoutError("timed out")):
            mock_urlopen.side_effect = error
            self.assertIsNone(reaper.fetch_open_pr_heads("o/r"))
            self.assertFalse(reaper.is_pr_closed("o/r", "6550"))


class GcloudTest(unittest.TestCase):
    """gcloud subprocess calls."""

    @mock.patch("subprocess.run")
    def test_list_active_pr_builds_lifts_default_limit(self, mock_run):
        mock_run.return_value = subprocess.CompletedProcess([], 0, stdout="[]", stderr="")
        reaper.list_active_pr_builds("p")
        argv = mock_run.call_args.args[0]
        self.assertIn("--ongoing", argv)
        self.assertIn("--limit=unlimited", argv)  # `gcloud builds list` defaults to --limit=50.
        self.assertIn('--filter=substitutions._PR_NUMBER!=""', argv)

        reaper.list_active_pr_builds("p", target_pr=6550)
        self.assertIn('--filter=substitutions._PR_NUMBER="6550"', mock_run.call_args.args[0])

    @mock.patch("subprocess.run")
    def test_list_active_pr_builds_handles_timeout_and_invalid_json(self, mock_run):
        mock_run.side_effect = subprocess.TimeoutExpired(cmd=["gcloud"], timeout=60)
        self.assertEqual(reaper.list_active_pr_builds("p"), [])
        mock_run.side_effect = None
        mock_run.return_value = subprocess.CompletedProcess([], 0, stdout="{bad", stderr="")
        self.assertEqual(reaper.list_active_pr_builds("p"), [])

    @mock.patch("subprocess.run")
    def test_cancel_isolates_per_build_failure(self, mock_run):
        def run(cmd, **kwargs):
            rc = 1 if "already-finished" in cmd else 0
            return subprocess.CompletedProcess(cmd, rc, stdout="", stderr="FAILED_PRECONDITION")

        mock_run.side_effect = run
        builds = [
            {**_build("already-finished", "1", OLD), "stale_reason": "superseded"},
            {**_build("still-working", "1", OLD), "stale_reason": "superseded"},
        ]
        self.assertEqual(reaper.cancel_active_builds("p", builds), 1)
        self.assertEqual(mock_run.call_count, 2)


class ReapStalePRBuildsTest(unittest.TestCase):
    """End-to-end flow with gcloud and GitHub mocked."""

    @mock.patch("urllib.request.urlopen")
    @mock.patch("subprocess.run")
    def test_one_github_call_covers_all_prs(self, mock_run, mock_urlopen):
        recorded = []
        mock_run.side_effect = _fake_gcloud(
            [
                _build("pr1-old", "1", OLD),
                _build("pr1-head", "1", NEW),
                _build("pr2-old", "2", OLD),
                _build("pr3-head", "3", NEW),
            ],
            recorded,
        )
        mock_urlopen.return_value = _http_response(
            [{"number": n, "head": {"sha": NEW}} for n in (1, 2, 3)]
        )

        self.assertEqual(reaper.reap_stale_pr_builds("p", "o/r"), 2)
        self.assertEqual(mock_urlopen.call_count, 1)
        cancelled = [c[3] for c in recorded if c[:3] == ["gcloud", "builds", "cancel"]]
        self.assertEqual(cancelled, ["pr1-old", "pr2-old"])

    @mock.patch("urllib.request.urlopen")
    @mock.patch("subprocess.run")
    def test_no_github_call_without_active_builds(self, mock_run, mock_urlopen):
        mock_run.side_effect = _fake_gcloud([], [])
        self.assertEqual(reaper.reap_stale_pr_builds("p", "o/r"), 0)
        mock_urlopen.assert_not_called()

    @mock.patch("urllib.request.urlopen")
    @mock.patch("subprocess.run")
    def test_rate_limited_github_cancels_nothing(self, mock_run, mock_urlopen):
        recorded = []
        mock_run.side_effect = _fake_gcloud([_build("b", "1", OLD)], recorded)
        mock_urlopen.side_effect = _rate_limited()
        self.assertEqual(reaper.reap_stale_pr_builds("p", "o/r"), 0)
        self.assertEqual([c[:3] for c in recorded], [["gcloud", "builds", "list"]])

    @mock.patch("urllib.request.urlopen")
    @mock.patch("subprocess.run")
    def test_dry_run_performs_no_mutations(self, mock_run, mock_urlopen):
        recorded = []
        mock_run.side_effect = _fake_gcloud([_build("b", "1", OLD)], recorded)
        mock_urlopen.side_effect = [_http_response([]), _http_response({"state": "closed"})]
        self.assertEqual(reaper.reap_stale_pr_builds("p", "o/r", dry_run=True), 1)
        self.assertEqual([c[:3] for c in recorded], [["gcloud", "builds", "list"]])

    def test_non_numeric_pr_rejected_by_cli(self):
        with mock.patch("sys.stderr", new=io.StringIO()), self.assertRaises(SystemExit):
            reaper.parse_args(["--pr", '6550" OR status="SUCCESS'])


if __name__ == "__main__":
    unittest.main()
