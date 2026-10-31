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

"""Reaps stale Cloud Build PR runs and GKE Kueue test jobs when newer commits are pushed.

When a new commit is pushed to a Pull Request (or when a PR is closed/merged),
previously triggered Cloud Build integration tests (`PR-test-*`) from older
commits on the same PR become obsolete. If left running or queued, their
corresponding Kubernetes Jobs on `test-kueue-cluster` continue holding scarce
`test-locks/*` quota and GCP hardware resources for hours.

This script:
1. Queries non-terminal (`WORKING`, `QUEUED`, `PENDING`) PR builds in Cloud Build
   using `--format=json(...)`.
2. Identifies builds whose PR is closed/merged or whose commit SHA has been
   superseded by a newer commit on the same PR (via GitHub REST API with
   automatic fallback to Cloud Build `createTime` ordering).
3. Cancels stale `QUEUED` and `WORKING` Cloud Builds first (so `submit_and_monitor_kueue_job.sh`
   cannot mistake K8s Job deletion for a retriable failure).
4. Deletes the corresponding Kubernetes Jobs on `test-kueue-cluster` using an
   isolated temporary `KUBECONFIG` so queued jobs exit Kueue immediately and
   running pods receive `SIGTERM` to run `cleanup_pod` (`terraform destroy`).
5. Rejects unapproved `PENDING` Cloud Build triggers from superseded commits or
   closed PRs concurrently via the Cloud Build REST API.
"""

from __future__ import annotations

import argparse
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timezone
import json
import os
import subprocess
import sys
import tempfile
from typing import Callable, Optional
import urllib.error
import urllib.request

DEFAULT_PROJECT = "hpc-toolkit-dev"
DEFAULT_REPO = "GoogleCloudPlatform/cluster-toolkit"
DEFAULT_CLUSTER_NAME = "test-kueue-cluster"
DEFAULT_CLUSTER_REGION = "us-central1"
DEFAULT_NAMESPACE = "default"
CANCEL_BATCH_SIZE = 50
REJECT_MAX_WORKERS = 16
SUBPROCESS_TIMEOUT_SECONDS = 60


def parse_rfc3339(ts_str: str) -> datetime:
    """Parse an RFC3339 timestamp string into a timezone-aware datetime."""
    if not ts_str:
        return datetime.min.replace(tzinfo=timezone.utc)
    normalized = ts_str.strip()
    if normalized.endswith("Z"):
        normalized = normalized[:-1] + "+00:00"
    try:
        if "." in normalized:
            before_dot, after_dot = normalized.split(".", 1)
            non_digit_idx = 0
            for char in after_dot:
                if not char.isdigit():
                    break
                non_digit_idx += 1
            frac = after_dot[:non_digit_idx][:6]
            tz = after_dot[non_digit_idx:]
            normalized = f"{before_dot}.{frac}{tz}"
        dt = datetime.fromisoformat(normalized)
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        return dt
    except ValueError:
        return datetime.min.replace(tzinfo=timezone.utc)


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


def fetch_pr_info(repo: str, pr_number: str) -> Optional[dict]:
    """Query GitHub REST API for a PR's state ('open'/'closed') and head.sha."""
    url = f"https://api.github.com/repos/{repo}/pulls/{pr_number}"
    headers = {
        "Accept": "application/vnd.github+json",
        "User-Agent": "cluster-toolkit-pr-reaper",
    }
    token = os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")
    if token:
        headers["Authorization"] = f"Bearer {token.strip()}"

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
            return {"state": state, "head_sha": head_sha}
    except (
        urllib.error.URLError,
        urllib.error.HTTPError,
        TimeoutError,
        ValueError,
        OSError,
    ) as exc:
        print(
            f"WARNING: GitHub API lookup failed for PR #{pr_number} ({exc}); "
            "falling back to Cloud Build timestamp ordering.",
            file=sys.stderr,
        )
        return None


def list_ongoing_and_pending_pr_builds(project: str) -> list[dict]:
    """Fetch all WORKING, QUEUED, and PENDING PR builds in the project as JSON."""
    build_filter = (
        '(status="WORKING" OR status="QUEUED" OR status="PENDING") '
        'AND substitutions._PR_NUMBER!=""'
    )
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
        return data if isinstance(data, list) else []
    except ValueError as exc:
        print(
            f"WARNING: Failed to parse gcloud builds list JSON: {exc}",
            file=sys.stderr,
        )
        return []


def _latest_commit_from_builds(
    pr_builds: list[dict],
    current_build_id: str = "",
    current_commit_sha: str = "",
) -> str:
    """Determine the newest commit SHA for a PR purely from Cloud Build timestamps."""
    commit_latest_time: dict[str, datetime] = {}
    for b in pr_builds:
        subs = b.get("substitutions") or {}
        sha = extract_commit_sha(subs)
        if b.get("id") == current_build_id and current_commit_sha:
            sha = current_commit_sha
        if not sha:
            continue
        created = parse_rfc3339(b.get("createTime", ""))
        if (
            sha not in commit_latest_time
            or created > commit_latest_time[sha]
        ):
            commit_latest_time[sha] = created

    if not commit_latest_time:
        return current_commit_sha or ""

    return max(commit_latest_time.items(), key=lambda kv: kv[1])[0]


def identify_stale_builds(
    builds: list[dict],
    repo: str = DEFAULT_REPO,
    target_pr: str = "",
    current_build_id: str = "",
    current_commit_sha: str = "",
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

        if pr_info and pr_info.get("state") == "closed":
            for b in pr_builds:
                if current_build_id and b.get("id") == current_build_id:
                    continue
                b_copy = dict(b)
                b_copy["stale_reason"] = f"PR #{pr_num} is closed"
                stale_builds.append(b_copy)
            continue

        latest_sha = ""
        if pr_info and pr_info.get("state") == "open":
            latest_sha = (pr_info.get("head_sha") or "").strip()

        if not latest_sha:
            latest_sha = _latest_commit_from_builds(
                pr_builds,
                current_build_id=current_build_id,
                current_commit_sha=current_commit_sha,
            )

        if not latest_sha:
            continue

        for b in pr_builds:
            if current_build_id and b.get("id") == current_build_id:
                continue
            subs = b.get("substitutions") or {}
            build_sha = extract_commit_sha(subs)
            if not build_sha or not commits_match(build_sha, latest_sha):
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
    """Cancel QUEUED and WORKING Cloud Builds in batches."""
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
    for i in range(0, len(build_ids), CANCEL_BATCH_SIZE):
        chunk = build_ids[i : i + CANCEL_BATCH_SIZE]
        cmd = [
            "gcloud",
            "builds",
            "cancel",
            *chunk,
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
                f"WARNING: gcloud builds cancel failed to execute: {exc}",
                file=sys.stderr,
            )
            continue
        if res.returncode == 0:
            cancelled += len(chunk)
        else:
            print(
                f"WARNING: gcloud builds cancel returned exit {res.returncode}: "
                f"{res.stderr.strip()}",
                file=sys.stderr,
            )
    return cancelled


def find_matching_kueue_jobs(
    k8s_jobs_payload: dict,
    stale_build_ids: set[str],
) -> list[str]:
    """Find active/suspended K8s Job names matching stale_build_ids by label or env."""
    if not stale_build_ids:
        return []

    matched_names: list[str] = []
    for item in k8s_jobs_payload.get("items") or []:
        metadata = item.get("metadata") or {}
        job_name = metadata.get("name") or ""
        if not job_name:
            continue

        # Skip jobs that have already reached terminal Complete or Failed state
        conditions = (item.get("status") or {}).get("conditions") or []
        is_terminal = any(
            cond.get("type") in ("Complete", "Failed")
            and cond.get("status") == "True"
            for cond in conditions
        )
        if is_terminal:
            continue

        labels = metadata.get("labels") or {}
        label_build_id = (labels.get("build-id") or "").strip()
        if label_build_id and label_build_id in stale_build_ids:
            matched_names.append(job_name)
            continue

        # Fallback: inspect container env BUILD_ID (for builds that omit label build-id)
        containers = (
            ((item.get("spec") or {}).get("template") or {}).get("spec") or {}
        ).get("containers") or []
        found_in_env = any(
            (env_entry.get("value") or "").strip() in stale_build_ids
            for container in containers
            for env_entry in container.get("env") or []
            if env_entry.get("name") == "BUILD_ID"
        )
        if found_in_env:
            matched_names.append(job_name)

    return matched_names


def delete_kueue_jobs_for_builds(
    project: str,
    cluster_name: str,
    cluster_region: str,
    namespace: str,
    stale_build_ids: set[str],
    dry_run: bool = False,
) -> int:
    """Delete Kueue K8s Jobs for stale builds using an isolated temporary KUBECONFIG."""
    if not stale_build_ids:
        return 0

    with tempfile.NamedTemporaryFile(
        prefix="ctk-reaper-kubeconfig-", delete=False
    ) as tmp_kc:
        kubeconfig_path = tmp_kc.name

    env = dict(os.environ)
    env["KUBECONFIG"] = kubeconfig_path

    try:
        cred_cmd = [
            "gcloud",
            "container",
            "clusters",
            "get-credentials",
            cluster_name,
            f"--region={cluster_region}",
            f"--project={project}",
        ]
        try:
            cred_res = subprocess.run(
                cred_cmd,
                env=env,
                capture_output=True,
                text=True,
                check=False,
                timeout=SUBPROCESS_TIMEOUT_SECONDS,
            )
        except (subprocess.TimeoutExpired, OSError) as exc:
            print(
                f"WARNING: Failed to get credentials for {cluster_name} in {project}: {exc}",
                file=sys.stderr,
            )
            return 0
        if cred_res.returncode != 0:
            print(
                f"WARNING: Failed to get credentials for {cluster_name} in {project}: "
                f"{cred_res.stderr.strip()}",
                file=sys.stderr,
            )
            return 0

        get_cmd = [
            "kubectl",
            "get",
            "jobs",
            "-n",
            namespace,
            "--request-timeout=15s",
            "-o",
            "json",
        ]
        try:
            get_res = subprocess.run(
                get_cmd,
                env=env,
                capture_output=True,
                text=True,
                check=False,
                timeout=SUBPROCESS_TIMEOUT_SECONDS,
            )
        except (subprocess.TimeoutExpired, OSError) as exc:
            print(
                f"WARNING: kubectl get jobs failed to execute: {exc}",
                file=sys.stderr,
            )
            return 0
        if get_res.returncode != 0:
            print(
                f"WARNING: kubectl get jobs failed: {get_res.stderr.strip()}",
                file=sys.stderr,
            )
            return 0

        try:
            jobs_payload = json.loads(get_res.stdout or "{}")
        except ValueError as exc:
            print(
                f"WARNING: Failed to parse kubectl jobs JSON: {exc}",
                file=sys.stderr,
            )
            return 0

        matched_jobs = find_matching_kueue_jobs(jobs_payload, stale_build_ids)
        if not matched_jobs:
            print(
                "No active or suspended Kubernetes Jobs matched the cancelled PR builds."
            )
            return 0

        if dry_run:
            print(
                f"[DRY-RUN] Would delete {len(matched_jobs)} stale Kubernetes Job(s) from {cluster_name}: "
                f"{', '.join(matched_jobs)}"
            )
            return len(matched_jobs)

        print(
            f"Deleting {len(matched_jobs)} stale Kubernetes Job(s) from {cluster_name}: "
            f"{', '.join(matched_jobs)}"
        )
        deleted = 0
        for i in range(0, len(matched_jobs), CANCEL_BATCH_SIZE):
            chunk = matched_jobs[i : i + CANCEL_BATCH_SIZE]
            del_cmd = [
                "kubectl",
                "delete",
                "job",
                *chunk,
                "-n",
                namespace,
                "--wait=false",
                "--ignore-not-found=true",
                "--request-timeout=15s",
            ]
            try:
                del_res = subprocess.run(
                    del_cmd,
                    env=env,
                    capture_output=True,
                    text=True,
                    check=False,
                    timeout=SUBPROCESS_TIMEOUT_SECONDS,
                )
            except (subprocess.TimeoutExpired, OSError) as exc:
                print(
                    f"WARNING: kubectl delete job failed to execute: {exc}",
                    file=sys.stderr,
                )
                continue
            if del_res.returncode == 0:
                deleted += len(chunk)
            else:
                print(
                    f"WARNING: kubectl delete job returned exit {del_res.returncode}: "
                    f"{del_res.stderr.strip()}",
                    file=sys.stderr,
                )
        return deleted
    finally:
        if os.path.exists(kubeconfig_path):
            try:
                os.remove(kubeconfig_path)
            except OSError:
                pass


def _reject_single_pending_build(
    project: str,
    build_id: str,
    reason: str,
    token: str,
) -> bool:
    """Reject a single PENDING Cloud Build via the Cloud Build v1 REST API."""
    url = f"https://cloudbuild.googleapis.com/v1/projects/{project}/builds/{build_id}:approve"
    body = json.dumps(
        {
            "approvalResult": {
                "decision": "REJECTED",
                "comment": f"Automatically cancelled stale PR build ({reason})",
            }
        }
    ).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=body,
        headers={
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=10):
            return True
    except urllib.error.HTTPError as exc:
        print(
            f"WARNING: Could not reject PENDING build {build_id} "
            f"(HTTP {exc.code}: {exc.reason}). Continuing.",
            file=sys.stderr,
        )
        return False
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        print(
            f"WARNING: Network error rejecting PENDING build {build_id} ({exc}). Continuing.",
            file=sys.stderr,
        )
        return False


def reject_pending_builds(
    project: str,
    pending_builds: list[dict],
    dry_run: bool = False,
) -> int:
    """Reject stale PENDING (unapproved) Cloud Builds concurrently."""
    if not pending_builds:
        return 0

    if dry_run:
        print(
            f"[DRY-RUN] Would reject {len(pending_builds)} stale PENDING Cloud Build(s)."
        )
        return len(pending_builds)

    try:
        token_res = subprocess.run(
            ["gcloud", "auth", "print-access-token"],
            capture_output=True,
            text=True,
            check=False,
            timeout=SUBPROCESS_TIMEOUT_SECONDS,
        )
    except (subprocess.TimeoutExpired, OSError) as exc:
        print(
            f"WARNING: Unable to obtain gcloud access token to reject PENDING builds: {exc}",
            file=sys.stderr,
        )
        return 0
    token = token_res.stdout.strip()
    if token_res.returncode != 0 or not token:
        print(
            "WARNING: Unable to obtain gcloud access token to reject PENDING builds.",
            file=sys.stderr,
        )
        return 0

    print(f"Rejecting {len(pending_builds)} stale PENDING Cloud Build(s)...")
    rejected = 0
    with ThreadPoolExecutor(max_workers=REJECT_MAX_WORKERS) as pool:
        futures = [
            pool.submit(
                _reject_single_pending_build,
                project,
                b["id"],
                b.get("stale_reason", "superseded"),
                token,
            )
            for b in pending_builds
            if b.get("id")
        ]
        for fut in as_completed(futures):
            if fut.result():
                rejected += 1

    print(f"Rejected {rejected}/{len(pending_builds)} stale PENDING build(s).")
    return rejected


def reap_stale_pr_builds(
    project: str = DEFAULT_PROJECT,
    repo: str = DEFAULT_REPO,
    target_pr: str = "",
    current_build_id: str = "",
    current_commit_sha: str = "",
    cluster_name: str = DEFAULT_CLUSTER_NAME,
    cluster_region: str = DEFAULT_CLUSTER_REGION,
    namespace: str = DEFAULT_NAMESPACE,
    skip_pending: bool = False,
    dry_run: bool = False,
) -> dict[str, int]:
    """Discover and reap stale PR Cloud Builds and their GKE Kueue Jobs."""
    builds = list_ongoing_and_pending_pr_builds(project)
    if not builds:
        print("No active or pending PR Cloud Builds found.")
        return {
            "cancelled_active": 0,
            "deleted_kueue_jobs": 0,
            "rejected_pending": 0,
        }

    stale_builds = identify_stale_builds(
        builds,
        repo=repo,
        target_pr=target_pr,
        current_build_id=current_build_id,
        current_commit_sha=current_commit_sha,
    )
    if not stale_builds:
        print("No stale PR Cloud Builds detected.")
        return {
            "cancelled_active": 0,
            "deleted_kueue_jobs": 0,
            "rejected_pending": 0,
        }

    stale_active = [
        b for b in stale_builds if b.get("status") in ("WORKING", "QUEUED")
    ]
    stale_pending = [
        b for b in stale_builds if b.get("status") == "PENDING"
    ]

    # 1. Cancel QUEUED and WORKING Cloud Builds FIRST so step 3 in Cloud Build
    # cannot mistake K8s Job deletion for a retriable failure.
    cancelled_active = cancel_active_builds(
        project=project,
        active_builds=stale_active,
        dry_run=dry_run,
    )

    # 2. Delete corresponding GKE Kueue Jobs so suspended jobs leave the queue
    # immediately and running pods receive SIGTERM to run terraform destroy.
    stale_active_ids = {b["id"] for b in stale_active if b.get("id")}
    deleted_kueue_jobs = delete_kueue_jobs_for_builds(
        project=project,
        cluster_name=cluster_name,
        cluster_region=cluster_region,
        namespace=namespace,
        stale_build_ids=stale_active_ids,
        dry_run=dry_run,
    )

    # 3. Reject stale PENDING (unapproved) builds from older commits / closed PRs.
    rejected_pending = 0
    if not skip_pending and stale_pending:
        rejected_pending = reject_pending_builds(
            project=project,
            pending_builds=stale_pending,
            dry_run=dry_run,
        )

    return {
        "cancelled_active": cancelled_active,
        "deleted_kueue_jobs": deleted_kueue_jobs,
        "rejected_pending": rejected_pending,
    }


def parse_args(argv: Optional[list[str]] = None) -> argparse.Namespace:
    """Parse CLI arguments."""
    parser = argparse.ArgumentParser(
        description=(
            "Cancel superseded or closed-PR Cloud Builds (WORKING, QUEUED, PENDING) "
            "and delete their GKE Kueue Jobs."
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
        "--current-commit-sha",
        default=os.environ.get("COMMIT_SHA") or "",
        help="Commit SHA of the current push event",
    )
    parser.add_argument(
        "--cluster-name",
        default=DEFAULT_CLUSTER_NAME,
        help=f"GKE Kueue cluster name (default: {DEFAULT_CLUSTER_NAME})",
    )
    parser.add_argument(
        "--cluster-region",
        default=DEFAULT_CLUSTER_REGION,
        help=f"GKE Kueue cluster region (default: {DEFAULT_CLUSTER_REGION})",
    )
    parser.add_argument(
        "--namespace",
        default=DEFAULT_NAMESPACE,
        help=f"Kubernetes namespace for Kueue jobs (default: {DEFAULT_NAMESPACE})",
    )
    parser.add_argument(
        "--skip-pending",
        action="store_true",
        help="Do not reject unapproved PENDING builds from older commits",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Log stale builds that would be reaped without cancelling or deleting",
    )
    return parser.parse_args(argv)


def main(argv: Optional[list[str]] = None) -> int:
    """CLI entrypoint."""
    args = parse_args(argv)
    summary = reap_stale_pr_builds(
        project=args.project,
        repo=args.repo,
        target_pr=str(args.pr).strip(),
        current_build_id=args.current_build_id,
        current_commit_sha=args.current_commit_sha,
        cluster_name=args.cluster_name,
        cluster_region=args.cluster_region,
        namespace=args.namespace,
        skip_pending=args.skip_pending,
        dry_run=args.dry_run,
    )
    print(
        "Reaper summary: "
        f"cancelled_active={summary['cancelled_active']}, "
        f"deleted_kueue_jobs={summary['deleted_kueue_jobs']}, "
        f"rejected_pending={summary['rejected_pending']}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
