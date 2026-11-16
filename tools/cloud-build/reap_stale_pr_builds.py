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

"""Cancels QUEUED/WORKING PR Cloud Builds that are superseded or whose PR closed.

GitHub is the source of truth: one paginated `GET /pulls?state=open` call per run
returns every open PR's head SHA. A PR that has active builds but is missing from
that list is confirmed closed with a single per-PR lookup before its builds are
cancelled. Any GitHub or gcloud failure skips the run instead of guessing.

Unauthenticated GitHub requests (304s included) count against a 60/hour per-IP
limit, so run this every few minutes, or set GITHUB_TOKEN to raise the limit.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from typing import Callable, Optional
import urllib.request

DEFAULT_PROJECT = "hpc-toolkit-dev"
DEFAULT_REPO = "GoogleCloudPlatform/cluster-toolkit"
SUBPROCESS_TIMEOUT_SECONDS = 60
GITHUB_TIMEOUT_SECONDS = 10
GITHUB_PAGE_SIZE = 100


def _github_get(path: str):
    """GET https://api.github.com/<path> and return the decoded JSON payload."""
    headers = {
        "Accept": "application/vnd.github+json",
        "User-Agent": "cluster-toolkit-pr-reaper",
    }
    token = (os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN") or "").strip()
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(f"https://api.github.com/{path}", headers=headers)
    with urllib.request.urlopen(req, timeout=GITHUB_TIMEOUT_SECONDS) as resp:
        return json.loads(resp.read().decode("utf-8"))


def fetch_open_pr_heads(repo: str) -> Optional[dict[str, str]]:
    """Return {pr_number: head_sha} for every open PR, or None if GitHub is unavailable."""
    heads: dict[str, str] = {}
    page = 1
    try:
        while True:
            prs = _github_get(
                f"repos/{repo}/pulls?state=open&per_page={GITHUB_PAGE_SIZE}&page={page}"
            )
            if not isinstance(prs, list):
                return None
            for pr in prs:
                sha = ((pr.get("head") or {}).get("sha") or "").strip().lower()
                if pr.get("number") and sha:
                    heads[str(pr["number"])] = sha
            if len(prs) < GITHUB_PAGE_SIZE:
                return heads
            page += 1
    except (OSError, ValueError) as exc:  # URLError, HTTPError and timeouts are OSErrors.
        print(f"WARNING: listing open PRs failed ({exc}); skipping this run.", file=sys.stderr)
        return None


def is_pr_closed(repo: str, pr_number: str) -> bool:
    """Return True only if GitHub confirms the PR is closed."""
    try:
        pr = _github_get(f"repos/{repo}/pulls/{pr_number}")
    except (OSError, ValueError) as exc:
        print(f"WARNING: looking up PR #{pr_number} failed ({exc}); skipping it.", file=sys.stderr)
        return False
    return isinstance(pr, dict) and pr.get("state") == "closed"


def list_active_pr_builds(project: str, target_pr: Optional[int] = None) -> list[dict]:
    """Return every QUEUED/WORKING Cloud Build that has a _PR_NUMBER substitution."""
    pr_filter = (
        f'substitutions._PR_NUMBER="{target_pr}"' if target_pr else 'substitutions._PR_NUMBER!=""'
    )
    cmd = [
        "gcloud",
        "builds",
        "list",
        f"--project={project}",
        "--ongoing",
        "--limit=unlimited",
        "--page-size=500",
        f"--filter={pr_filter}",
        "--format=json(id,status,substitutions)",
    ]
    try:
        res = subprocess.run(
            cmd, capture_output=True, text=True, check=False, timeout=SUBPROCESS_TIMEOUT_SECONDS
        )
    except (subprocess.TimeoutExpired, OSError) as exc:
        print(f"WARNING: gcloud builds list failed to execute: {exc}", file=sys.stderr)
        return []
    if res.returncode != 0:
        print(
            f"WARNING: gcloud builds list failed (exit {res.returncode}): {res.stderr.strip()}",
            file=sys.stderr,
        )
        return []
    try:
        data = json.loads(res.stdout or "[]")
    except ValueError as exc:
        print(f"WARNING: Failed to parse gcloud builds list JSON: {exc}", file=sys.stderr)
        return []
    return [b for b in data if isinstance(b, dict)] if isinstance(data, list) else []


def identify_stale_builds(
    builds: list[dict],
    open_pr_heads: dict[str, str],
    is_closed: Callable[[str], bool],
) -> list[dict]:
    """Return builds whose PR is closed or whose commit is no longer the PR head."""
    by_pr: dict[str, list[dict]] = {}
    for b in builds:
        pr_num = str((b.get("substitutions") or {}).get("_PR_NUMBER") or "").strip()
        if pr_num.isdigit():
            by_pr.setdefault(pr_num, []).append(b)

    stale: list[dict] = []
    for pr_num, pr_builds in by_pr.items():
        head_sha = open_pr_heads.get(pr_num)
        if head_sha is None:
            if is_closed(pr_num):
                stale.extend({**b, "stale_reason": f"PR #{pr_num} is closed"} for b in pr_builds)
            continue
        for b in pr_builds:
            build_sha = ((b.get("substitutions") or {}).get("COMMIT_SHA") or "").strip().lower()
            if build_sha and build_sha != head_sha:
                reason = f"superseded by PR #{pr_num} HEAD {head_sha[:7]}"
                stale.append({**b, "stale_reason": reason})
    return stale


def cancel_active_builds(project: str, builds: list[dict], dry_run: bool = False) -> int:
    """Cancel builds one at a time so one already-finished build cannot block the rest."""
    cancelled = 0
    for b in builds:
        subs = b.get("substitutions") or {}
        print(
            f"{'[DRY-RUN] Would cancel' if dry_run else 'Cancelling'} {b.get('status')} "
            f"Cloud Build {b['id']} (PR #{subs.get('_PR_NUMBER')}, "
            f"commit {(subs.get('COMMIT_SHA') or '')[:7]}, "
            f"trigger {subs.get('TRIGGER_NAME', 'unknown')}): {b['stale_reason']}"
        )
        if dry_run:
            cancelled += 1
            continue
        cmd = ["gcloud", "builds", "cancel", b["id"], f"--project={project}", "--quiet"]
        try:
            res = subprocess.run(
                cmd, capture_output=True, text=True, check=False, timeout=SUBPROCESS_TIMEOUT_SECONDS
            )
        except (subprocess.TimeoutExpired, OSError) as exc:
            print(f"WARNING: gcloud builds cancel {b['id']} failed to execute: {exc}", file=sys.stderr)
            continue
        if res.returncode == 0:
            cancelled += 1
        else:
            print(
                f"WARNING: gcloud builds cancel {b['id']} returned exit {res.returncode}: "
                f"{res.stderr.strip()}",
                file=sys.stderr,
            )
    return cancelled


def reap_stale_pr_builds(
    project: str = DEFAULT_PROJECT,
    repo: str = DEFAULT_REPO,
    target_pr: Optional[int] = None,
    dry_run: bool = False,
) -> int:
    """Cancel stale PR builds; return how many were (or, with dry_run, would be) cancelled."""
    builds = [b for b in list_active_pr_builds(project, target_pr) if b.get("id")]
    if not builds:
        return 0
    open_pr_heads = fetch_open_pr_heads(repo)
    if open_pr_heads is None:
        return 0
    stale = identify_stale_builds(builds, open_pr_heads, lambda pr: is_pr_closed(repo, pr))
    return cancel_active_builds(project, stale, dry_run=dry_run)


def parse_args(argv: Optional[list[str]] = None) -> argparse.Namespace:
    """Parse CLI arguments."""
    parser = argparse.ArgumentParser(
        description="Cancel superseded or closed-PR Cloud Builds (WORKING and QUEUED)."
    )
    parser.add_argument(
        "--project",
        default=os.environ.get("PROJECT_ID") or DEFAULT_PROJECT,
        help=f"GCP project ID (default: $PROJECT_ID or {DEFAULT_PROJECT})",
    )
    parser.add_argument(
        "--repo",
        default=DEFAULT_REPO,
        help=f"GitHub owner/repo (default: {DEFAULT_REPO})",
    )
    parser.add_argument(
        "--pr", type=int, default=None, help="Only consider builds of this PR number"
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Log stale builds that would be cancelled without cancelling them",
    )
    return parser.parse_args(argv)


def main(argv: Optional[list[str]] = None) -> int:
    """CLI entrypoint."""
    args = parse_args(argv)
    cancelled = reap_stale_pr_builds(args.project, args.repo, args.pr, args.dry_run)
    if cancelled or args.dry_run:
        print(f"Reap summary: {'would cancel' if args.dry_run else 'cancelled'} {cancelled} build(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
