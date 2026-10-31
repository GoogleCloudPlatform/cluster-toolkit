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

"""Reaps stale Cloud Build PR runs when newer commits are pushed or PRs close.

When a new commit is pushed to a Pull Request (or when a PR is closed/merged),
previously triggered Cloud Build integration tests (`PR-test-*`) from older
commits on the same PR become obsolete. If left running or queued, their
corresponding Kubernetes Jobs on `test-kueue-cluster` continue holding scarce
`test-locks/*` quota and GCP hardware resources for hours.

This script:
1. Queries active (`WORKING` and `QUEUED`) PR builds in Cloud Build using
   `--format=json(id,status,createTime,substitutions)`.
2. Queries the GitHub REST API for each active PR (using `ETag` / `If-None-Match`
   conditional requests so `304 Not Modified` responses do not consume GitHub's
   unauthenticated rate limit). If a GitHub API lookup fails, that PR is safely
   skipped until the next run.
3. Cancels stale `QUEUED` and `WORKING` Cloud Builds via `gcloud builds cancel`.
   Once a Cloud Build transitions to `CANCELLED`, the in-cluster `kueue-reaper`
   pod on `test-kueue-cluster` automatically deletes the corresponding Kubernetes
   `Job` so queued jobs exit Kueue immediately and running pods receive `SIGTERM`
   to execute `cleanup_pod` (`terraform destroy`).
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from typing import Callable, Optional
import urllib.error
import urllib.request

DEFAULT_PROJECT = "hpc-toolkit-dev"
DEFAULT_REPO = "GoogleCloudPlatform/cluster-toolkit"
DEFAULT_ETAG_CACHE_PATH = "/tmp/ctk_pr_etag_cache.json"
SUBPROCESS_TIMEOUT_SECONDS = 60


def extract_commit_sha(substitutions: dict) -> str:
    """Extract the commit SHA from Cloud Build substitutions."""
    return (
        substitutions.get("COMMIT_SHA")
        or substitutions.get("REVISION_ID")
        or substitutions.get("SHORT_SHA")
        or ""
    ).strip()


def commits_match(sha_a: str, sha_b: str) -> bool:
    """Return True if two full or abbreviated commit SHAs refer to the same commit."""
    a = (sha_a or "").strip().lower()
    b = (sha_b or "").strip().lower()
    if not a or not b:
        return False
    if a == b:
        return True
    min_len = min(len(a), len(b))
    if min_len >= 7 and (a.startswith(b) or b.startswith(a)):
        return True
    return False


_ETAG_CACHES: dict[str, dict] = {}


def _load_etag_cache(cache_path: str) -> dict:
    """Load the on-disk ETag cache dictionary if present."""
    if not cache_path or not os.path.exists(cache_path):
        _ETAG_CACHES.pop(cache_path, None)
        return {}
    if cache_path in _ETAG_CACHES:
        return _ETAG_CACHES[cache_path]
    try:
        with open(cache_path, "r", encoding="utf-8") as fh:
            data = json.load(fh)
            if isinstance(data, dict):
                _ETAG_CACHES[cache_path] = data
                return data
            return {}
    except (OSError, ValueError):
        return {}


def _save_etag_cache(cache_path: str, cache: dict) -> None:
    """Persist the ETag cache dictionary to disk atomically."""
    if not cache_path:
        return
    _ETAG_CACHES[cache_path] = cache
    tmp_path = f"{cache_path}.tmp.{os.getpid()}"
    try:
        with open(tmp_path, "w", encoding="utf-8") as fh:
            json.dump(cache, fh)
        os.replace(tmp_path, cache_path)
    except OSError:
        try:
            os.remove(tmp_path)
        except OSError:
            pass


def fetch_pr_info(
    repo: str,
    pr_number: str,
    cache_path: str = DEFAULT_ETAG_CACHE_PATH,
) -> Optional[dict]:
    """Query GitHub REST API for a PR's state ('open'/'closed') and head.sha."""
    url = f"https://api.github.com/repos/{repo}/pulls/{pr_number}"
    cache_key = f"{repo}#{pr_number}"
    cache = _load_etag_cache(cache_path)
    cached_entry = cache.get(cache_key) if isinstance(cache.get(cache_key), dict) else {}

    headers = {
        "Accept": "application/vnd.github+json",
        "User-Agent": "cluster-toolkit-pr-reaper",
    }
    token = (
        os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN") or ""
    ).strip()
    if token:
        headers["Authorization"] = f"Bearer {token}"
    if cached_entry.get("etag"):
        headers["If-None-Match"] = str(cached_entry["etag"])

    req = urllib.request.Request(url, headers=headers, method="GET")
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            payload = json.loads(resp.read().decode("utf-8"))
            if not isinstance(payload, dict):
                return None
            state = (payload.get("state") or "").lower()
            head_sha = ((payload.get("head") or {}).get("sha") or "").strip()
            if not state:
                return None
            etag = resp.headers.get("ETag") if resp.headers else None
            if etag:
                cache[cache_key] = {
                    "etag": etag,
                    "state": state,
                    "head_sha": head_sha,
                }
                _save_etag_cache(cache_path, cache)
            return {"state": state, "head_sha": head_sha}
    except urllib.error.HTTPError as exc:
        if exc.code == 304 and cached_entry.get("state"):
            return {
                "state": str(cached_entry["state"]),
                "head_sha": str(cached_entry.get("head_sha") or ""),
            }
        print(
            f"WARNING: GitHub API lookup failed for PR #{pr_number} ({exc}); "
            f"skipping PR #{pr_number} on this run.",
            file=sys.stderr,
        )
        return None
    except (
        urllib.error.URLError,
        TimeoutError,
        ValueError,
        OSError,
    ) as exc:
        print(
            f"WARNING: GitHub API lookup failed for PR #{pr_number} ({exc}); "
            f"skipping PR #{pr_number} on this run.",
            file=sys.stderr,
        )
        return None


def list_active_pr_builds(project: str, target_pr: str = "") -> list[dict]:
    """Fetch all WORKING and QUEUED PR builds in the project as JSON."""
    pr_filter = (
        f'substitutions._PR_NUMBER="{target_pr.strip()}"'
        if target_pr and target_pr.strip()
        else 'substitutions._PR_NUMBER!=""'
    )
    build_filter = f'(status="WORKING" OR status="QUEUED") AND {pr_filter}'
    cmd = [
        "gcloud",
        "builds",
        "list",
        f"--project={project}",
        f"--filter={build_filter}",
        "--page-size=500",
        "--format=json(id,status,createTime,substitutions)",
    ]
    try:
        res = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            check=False,
            timeout=SUBPROCESS_TIMEOUT_SECONDS,
        )
    except (subprocess.TimeoutExpired, OSError) as exc:
        print(
            f"WARNING: gcloud builds list failed to execute: {exc}",
            file=sys.stderr,
        )
        return []
    if res.returncode != 0:
        print(
            f"WARNING: gcloud builds list failed (exit {res.returncode}): {res.stderr.strip()}",
            file=sys.stderr,
        )
        return []
    out = res.stdout.strip()
    if not out:
        return []
    try:
        data = json.loads(out)
        return [b for b in data if isinstance(b, dict)] if isinstance(data, list) else []
    except ValueError as exc:
        print(
            f"WARNING: Failed to parse gcloud builds list JSON: {exc}",
            file=sys.stderr,
        )
        return []


def identify_stale_builds(
    builds: list[dict],
    repo: str = DEFAULT_REPO,
    target_pr: str = "",
    current_build_id: str = "",
    pr_fetcher: Optional[Callable[[str, str], Optional[dict]]] = None,
) -> list[dict]:
    """Return the subset of builds that belong to closed PRs or superseded commits."""
    if pr_fetcher is None:
        pr_fetcher = fetch_pr_info

    by_pr: dict[str, list[dict]] = {}
    for b in builds:
        subs = b.get("substitutions") or {}
        pr_num = str(subs.get("_PR_NUMBER") or "").strip()
        if not pr_num:
            continue
        if target_pr and pr_num != str(target_pr).strip():
            continue
        by_pr.setdefault(pr_num, []).append(b)

    stale_builds: list[dict] = []
    for pr_num, pr_builds in by_pr.items():
        pr_info = pr_fetcher(repo, pr_num)
        if not pr_info:
            continue

        if pr_info.get("state") == "closed":
            for b in pr_builds:
                if current_build_id and b.get("id") == current_build_id:
                    continue
                b_copy = dict(b)
                b_copy["stale_reason"] = f"PR #{pr_num} is closed"
                stale_builds.append(b_copy)
            continue

        if pr_info.get("state") == "open":
            latest_sha = (pr_info.get("head_sha") or "").strip()
            if not latest_sha:
                continue

            for b in pr_builds:
                if current_build_id and b.get("id") == current_build_id:
                    continue
                subs = b.get("substitutions") or {}
                build_sha = extract_commit_sha(subs)
                if build_sha and not commits_match(build_sha, latest_sha):
                    b_copy = dict(b)
                    b_copy["stale_reason"] = (
                        f"superseded by PR #{pr_num} HEAD {latest_sha[:7]}"
                    )
                    stale_builds.append(b_copy)

    return stale_builds


def cancel_active_builds(
    project: str,
    active_builds: list[dict],
    dry_run: bool = False,
) -> int:
    """Cancel QUEUED and WORKING Cloud Builds individually for failure isolation."""
    if not active_builds:
        return 0

    build_ids = [b["id"] for b in active_builds if b.get("id")]
    if not build_ids:
        return 0

    for b in active_builds:
        subs = b.get("substitutions") or {}
        trig = subs.get("TRIGGER_NAME", "unknown-trigger")
        pr_num = subs.get("_PR_NUMBER", "?")
        sha = extract_commit_sha(subs)[:7]
        prefix = "[DRY-RUN] Would cancel" if dry_run else "Cancelling"
        print(
            f"{prefix} {b.get('status')} Cloud Build {b.get('id')} "
            f"(PR #{pr_num}, commit {sha}, trigger {trig}): {b.get('stale_reason')}"
        )

    if dry_run:
        return len(build_ids)

    cancelled = 0
    for build_id in build_ids:
        cmd = [
            "gcloud",
            "builds",
            "cancel",
            build_id,
            f"--project={project}",
            "--quiet",
        ]
        try:
            res = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                check=False,
                timeout=SUBPROCESS_TIMEOUT_SECONDS,
            )
        except (subprocess.TimeoutExpired, OSError) as exc:
            print(
                f"WARNING: gcloud builds cancel {build_id} failed to execute: {exc}",
                file=sys.stderr,
            )
            continue
        if res.returncode == 0:
            cancelled += 1
        else:
            print(
                f"WARNING: gcloud builds cancel {build_id} returned exit {res.returncode}: "
                f"{res.stderr.strip()}",
                file=sys.stderr,
            )
    return cancelled


def reap_stale_pr_builds(
    project: str = DEFAULT_PROJECT,
    repo: str = DEFAULT_REPO,
    target_pr: str = "",
    current_build_id: str = "",
    dry_run: bool = False,
) -> dict[str, int]:
    """Discover and cancel stale QUEUED/WORKING PR Cloud Builds."""
    builds = list_active_pr_builds(project, target_pr=target_pr)
    if not builds:
        print("No active PR Cloud Builds found.")
        return {"cancelled_active": 0}

    stale_builds = identify_stale_builds(
        builds,
        repo=repo,
        target_pr=target_pr,
        current_build_id=current_build_id,
    )
    if not stale_builds:
        print("No stale PR Cloud Builds detected.")
        return {"cancelled_active": 0}

    cancelled_active = cancel_active_builds(
        project=project,
        active_builds=stale_builds,
        dry_run=dry_run,
    )
    return {"cancelled_active": cancelled_active}


def parse_args(argv: Optional[list[str]] = None) -> argparse.Namespace:
    """Parse CLI arguments."""
    parser = argparse.ArgumentParser(
        description=(
            "Cancel superseded or closed-PR Cloud Builds (WORKING and QUEUED)."
        )
    )
    parser.add_argument(
        "--project",
        default=os.environ.get("PROJECT_ID") or DEFAULT_PROJECT,
        help=f"GCP project ID (default: {DEFAULT_PROJECT})",
    )
    parser.add_argument(
        "--repo",
        default=os.environ.get("GITHUB_UPSTREAM_REPO") or DEFAULT_REPO,
        help=f"GitHub owner/repo (default: {DEFAULT_REPO})",
    )
    parser.add_argument(
        "--pr",
        default="",
        help="Optional PR number to scope reaping to a single PR",
    )
    parser.add_argument(
        "--current-build-id",
        default=os.environ.get("BUILD_ID") or "",
        help="Current Cloud Build ID to exclude from cancellation",
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
    summary = reap_stale_pr_builds(
        project=args.project,
        repo=args.repo,
        target_pr=args.pr,
        current_build_id=args.current_build_id,
        dry_run=args.dry_run,
    )
    print(
        f"Reap summary: cancelled_active={summary['cancelled_active']}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
